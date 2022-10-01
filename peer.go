// +build !js

package main

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type fCandiMsg struct {
	Cmd          string                  `json:"cmd"`
	Clusterid    string                  `json:"clusterid"`
	Podid        string                  `json:"podid"`
	Roomid       string                  `json:"roomid"`
	Userid       string                  `json:"userid"`
	RemoteUserid string                  `json:"remoteuserid"`
	Peerid       string                  `json:"peerid"`
	ISPType      int                     `json:"isptype"`
	Candidate    webrtc.ICECandidateInit `json:"candidate"`
}

type candiMsg struct {
	Cmd          string                  `json:"cmd"`
	Userid       string                  `json:"userid"`
	RemoteUserid string                  `json:"remoteuserid"`
	Peerid       string                  `json:"peerid"`
	ISPType      int                     `json:"isptype"`
	Candidate    webrtc.ICECandidateInit `json:"candidate"`
}

type peer struct {
	conf                 *Config
	userid               string
	remoteuserid         string
	peerid               string
	role                 int
	isFake               bool
	peerConnection       *webrtc.PeerConnection
	videoTrack           *webrtc.TrackLocalStaticRTP
	videoSender          *webrtc.RTPSender
	audioTrack           *webrtc.TrackLocalStaticRTP
	audioSender          *webrtc.RTPSender
	dataChannel          *webrtc.DataChannel
	filter               *KeyframeFilter
	onSendMessageHandler func(topic, msg string)
	publishersdp         *webrtc.SessionDescription
	parent               *session
	setedRemoteDesc      bool
	candidateCaches      *list.List
	isReady              bool
	shouldNotify         bool
	videoCodec           webrtc.RTPCodecParameters
	audioCodec           webrtc.RTPCodecParameters
	audiossrc            *list.List
	videossrc            *list.List
	appssrc              *list.List
	remoteSdpReceived    chan int
	exitCh               chan int
	waitGroup            *sync.WaitGroup
}

func initMediaCodec(m *webrtc.MediaEngine) error {
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus,
				ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
				RTCPFeedback: nil},
			PayloadType: 111,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{{Type: webrtc.TypeRTCPFBTransportCC,
		Parameter: ""}, {Type: "ccm", Parameter: "fir"}, {Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"}}
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264,
				ClockRate: 90000, Channels: 0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 102,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}
	return nil
}

// CreatePublishPeer to create a object for publish peer
func CreatePublishPeer(s *session, sid, pid, podid string, c *Config, isFake bool) (*peer, error) {
	p := peer{
		conf:              c,
		userid:            sid,
		remoteuserid:      "",
		peerid:            pid,
		role:              1,
		isFake:            isFake,
		peerConnection:    nil,
		videoTrack:        nil,
		videoSender:       nil,
		audioTrack:        nil,
		audioSender:       nil,
		dataChannel:       nil,
		filter:            CreateKeyframeFilter(!isFake),
		publishersdp:      nil,
		parent:            s,
		setedRemoteDesc:   false,
		candidateCaches:   list.New(),
		isReady:           false,
		shouldNotify:      false,
		audiossrc:         list.New(),
		videossrc:         list.New(),
		appssrc:           list.New(),
		remoteSdpReceived: make(chan int),
		exitCh:            make(chan int),
		waitGroup:         &sync.WaitGroup{},
	}
	var err error
	m := &webrtc.MediaEngine{}
	if err = initMediaCodec(m); err != nil {
		return nil, err
	}

	if err = m.RegisterDefaultHeaderExtension(); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err = webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))
	if p.peerConnection, err = api.NewPeerConnection(webrtc.Configuration{}); err != nil {
		fmt.Println("Create publish peer failed")
		return nil, err
	}

	p.peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		ssrc := remoteTrack.SSRC()
		if p.isAudioSsrc(int(ssrc)) {
			p.audioCodec = remoteTrack.Codec()
			p.parent.UpdateAudioCodec(p.audioCodec)
			fmt.Println("publish audio codec: ", p.audioCodec)
			fmt.Printf("Got audio remote track, id:%s, streamid:%s, ssrc:%x\n",
				[]byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())
			p.waitGroup.Add(1)
			go func() {
				defer p.waitGroup.Done()
				rtpBuf := make([]byte, 1500)
				for {
					i, _, readErr := remoteTrack.Read(rtpBuf)

					if readErr != nil && readErr == io.EOF {
						return
					}

					p.parent.OnReceivedAudioData(rtpBuf, i)
				}
			}()
			return
		}
		if p.isVideoSsrc(int(ssrc)) {
			p.videoCodec = remoteTrack.Codec()
			p.parent.UpdateVideoCodec(p.videoCodec)
			fmt.Println("publish video codec: ", p.videoCodec)
			fmt.Printf("Got video remote track, id:%s, streamid:%s, ssrc:%x\n",
				[]byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())

			p.waitGroup.Add(1)
			go func() {
				defer p.waitGroup.Done()
				rtpBuf := make([]byte, 1500)
				for {
					i, _, readErr := remoteTrack.Read(rtpBuf)
					if readErr != nil && readErr == io.EOF {
						return
					}
					p.parent.OnReceivedVideoData(rtpBuf, i)
				}
			}()
			return
		}
		fmt.Printf("Got unknow remote track, id:%s, streamid:%s, ssrc:%x\n",
			[]byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())
	})

	p.peerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
		p.dataChannel = channel
		/* p.dataChannel.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' opened\n", channel.Label(), channel.ID())
			channel.Detach()
		}) */

		p.dataChannel.OnError(func(err error) {
			fmt.Printf("pub data channel got an error:%e\n", err)
		})

		p.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			if !p.isFake {
				// send to record service
			}
			p.parent.OnReceivedAppData(p.userid, p.peerid, msg.Data, len(msg.Data))
			fmt.Printf("%s:%s ### recv channel data len:%d\n", p.userid, p.peerid, len(msg.Data))
		})
	})

	p.peerConnection.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		fmt.Println("ICE Gatherer state change:", state.String())
	})

	p.peerConnection.OnICECandidate(func(candi *webrtc.ICECandidate) {
		if candi == nil {
			return // must be publish ICE Gatherer completed
		}
		conf, err := GetConfig()
		if conf == nil || err != nil {
			return
		}

		if conf.Localaddr != candi.Address {
			return // filter candidate
		}

		go p.CheckIceInfo(candi.TCPType, candi.Address, candi.Port)

		if conf.Proxyaddr != "" {
			candi.Address = conf.Proxyaddr
			candi.Port = uint16(conf.Proxyport)
		}

		c := candi.ToJSON()
		if isFake {
			m := fCandiMsg{
				Cmd:          CmdFakeCandidate,
				Clusterid:    p.conf.Clusterid,
				Podid:        p.conf.Uuid,
				Roomid:       p.parent.parent.roomid,
				Userid:       p.userid,
				RemoteUserid: p.userid,
				Peerid:       p.peerid,
				ISPType:      0,
				Candidate:    c,
			}

			msg, err := json.Marshal(m)
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			PublishMessage2CrossClusterPod(podid, string(msg))
		} else {
			msg, err := json.Marshal(candiMsg{
				Cmd:       CmdCandidate,
				Userid:    p.userid,
				Peerid:    p.peerid,
				ISPType:   0,
				Candidate: c,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			p.onSendMessageHandler(p.userid, string(msg))
		}
	})

	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("%s:%s Connection State has changed: %s\n", p.userid, p.peerid, connectionState.String())
		if p.peerConnection == nil {
			return
		}
		if connectionState == webrtc.ICEConnectionStateConnected {
			p.isReady = true
			p.shouldNotify = true
			p.filter.Startup(func() {
				p.sendPli()
			})
			p.parent.OnIceReady(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateDisconnected {
			p.isReady = false
			p.filter.Shutdown()
			p.parent.OnIceDisconnected(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateFailed {
			p.isReady = false
			p.filter.Shutdown()
			p.parent.OnIceFailed(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateClosed {
			p.isReady = false
			p.filter.Shutdown()
			p.parent.OnIceClosed(p.role, p.userid, p.remoteuserid, p.peerid)
		}
	})

	return &p, nil
}

// CreateSubscribePeer to create a object for subscribe peer
func CreateSubscribePeer(s *session, sid, ssid, pid, podid string, c *Config, publisherSdp *webrtc.SessionDescription,
	acodec, vcodec webrtc.RTPCodecParameters, isFake bool) (*peer, error) {
	p := peer{
		conf:              c,
		userid:            sid,
		remoteuserid:      ssid,
		peerid:            pid,
		role:              2,
		isFake:            isFake,
		peerConnection:    nil,
		videoTrack:        nil,
		videoSender:       nil,
		audioTrack:        nil,
		audioSender:       nil,
		dataChannel:       nil,
		filter:            nil,
		publishersdp:      publisherSdp,
		parent:            s,
		setedRemoteDesc:   false,
		candidateCaches:   list.New(),
		isReady:           false,
		shouldNotify:      false,
		videoCodec:        vcodec,
		audioCodec:        acodec,
		remoteSdpReceived: make(chan int),
		exitCh:            make(chan int),
		waitGroup:         &sync.WaitGroup{},
	}
	var err error
	m := &webrtc.MediaEngine{}
	initMediaCodec(m)

	if err = m.RegisterDefaultHeaderExtension(); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err = webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))
	if p.peerConnection, err = api.NewPeerConnection(webrtc.Configuration{}); err != nil {
		fmt.Println("Create subscribe peer failed")
		return nil, err
	}

	p.peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Got the remote track\n")
	})

	p.peerConnection.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		fmt.Println("ICE Gatherer state change:", state.String())
	})

	p.peerConnection.OnICECandidate(func(candi *webrtc.ICECandidate) {
		if candi == nil {
			return // must be subscribe ICE Gatherer completed
		}

		conf, err := GetConfig()
		if conf == nil || err != nil {
			return
		}

		if conf.Localaddr != candi.Address {
			return // filter candidate
		}

		go p.CheckIceInfo(candi.TCPType, candi.Address, candi.Port)

		if conf.Proxyaddr != "" {
			candi.Address = conf.Proxyaddr
			candi.Port = uint16(conf.Proxyport)
		}

		c := candi.ToJSON()
		if isFake {
			m := fCandiMsg{
				Cmd:          CmdFakeCandidate,
				Clusterid:    p.conf.Clusterid,
				Podid:        p.conf.Uuid,
				Roomid:       p.parent.parent.roomid,
				Userid:       p.remoteuserid,
				RemoteUserid: p.remoteuserid,
				Peerid:       p.peerid,
				ISPType:      0,
				Candidate:    c,
			}

			msg, err := json.Marshal(m)
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			PublishMessage2CrossClusterPod(podid, string(msg))
		} else {
			msg, err := json.Marshal(candiMsg{
				Cmd:          CmdCandidate,
				Userid:       p.userid,
				RemoteUserid: p.remoteuserid,
				Peerid:       p.peerid,
				ISPType:      0,
				Candidate:    c,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			p.onSendMessageHandler(p.userid, string(msg))
		}
	})

	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("%s <- %s:%s Connection State has changed: %s\n", p.userid, p.remoteuserid, p.peerid, connectionState.String())
		if p.peerConnection == nil {
			return
		}
		if connectionState == webrtc.ICEConnectionStateConnected {
			p.isReady = true
			p.shouldNotify = true
			p.parent.OnIceReady(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateDisconnected {
			p.isReady = false
			// p.parent.OnIceDisconnected(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateFailed {
			p.isReady = false
			p.parent.OnIceFailed(p.role, p.userid, p.remoteuserid, p.peerid)
		} else if connectionState == webrtc.ICEConnectionStateClosed {
			p.isReady = false
			p.parent.OnIceClosed(p.role, p.userid, p.remoteuserid, p.peerid)
		}
	})
	return &p, nil
}

func (p *peer) SetSendMessageHandler(f func(topic, msg string)) {
	p.onSendMessageHandler = f
}

func (p *peer) Destroy() {
	//p.exitCh <- 1
	//p.waitGroup.Wait()
	if p.filter != nil {
		p.filter.Shutdown()
	}

	close(p.exitCh)
	if p.peerConnection != nil {
		p.peerConnection.Close()
		p.peerConnection = nil
	}
}

func (p *peer) HandleMessage(j jsonparser) {
	command := GetValue(j, LabelCmd)
	if command == CmdSub {
		p.HandleSubscribe(j)
	} else if command == CmdOffer {
		p.HandleRemoteOffer(j)
	} else if command == CmdAnswer {
		p.HandleRemoteAnswer(j)
	} else if command == CmdCandidate {
		p.HandleRemoteCandidate(j)
	} else if command == CmdFakeSubscribe {
		p.HandleFakeSubscribe(j)
	} else if command == CmdFakeOffer {
		p.HandleFakeOffer(j)
	} else if command == CmdFakeAnswer {
		p.HandleFakeAnswer(j)
	} else if command == CmdFakeCandidate {
		p.HandleRemoteCandidate(j)
	} else {
		fmt.Println("peer unsupport msg type:", command)
	}
}

func (p *peer) HandleSubscribe(j jsonparser) {
	userID := GetValue(j, LabelSessID)
	remoteUserID := GetValue(j, LabelSrcSessID)
	peerid := GetValue(j, LabelPeerID)

	psdp, err := p.publishersdp.Unmarshal()
	if err != nil {
		panic(err)
	}
	for _, media := range psdp.MediaDescriptions {
		if media.MediaName.Media == "video" {
			p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion-v")
			if err != nil {
				panic(err)
			}
			p.videoSender, err = p.peerConnection.AddTrack(p.videoTrack)
			if err != nil {
				panic(err)
			}
			go func() {
				for {
					var pkts []rtcp.Packet
					if pkts, _, err = p.videoSender.ReadRTCP(); err != nil {
						return
					}
					for _, pkt := range pkts {
						_, ok := pkt.(*rtcp.PictureLossIndication)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp PLI\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
						_, ok = pkt.(*rtcp.SliceLossIndication)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp SLI\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
						_, ok = pkt.(*rtcp.FullIntraRequest)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp FIR\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
					}
				}
			}()
		} else if media.MediaName.Media == "audio" {
			p.audioTrack, err = webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion-a")
			if err != nil {
				panic(err)
			}
			p.audioSender, err = p.peerConnection.AddTrack(p.audioTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "application" {
			var order = true
			//var lefttime uint16 = 5000
			var retransmit uint16 = 5
			options := &webrtc.DataChannelInit{
				Ordered: &order,
				//MaxPacketLifeTime: &lefttime,
				MaxRetransmits: &retransmit,
			}
			if p.dataChannel, err = p.peerConnection.CreateDataChannel("whiteboard", options); err != nil {
				panic(err)
			}
			p.dataChannel.OnError(func(err error) {
				fmt.Printf("sub data channel got an error:%e\n", err)
			})
			p.dataChannel.OnClose(func() {
				fmt.Println("sub data channel has closed")
			})
			p.dataChannel.OnOpen(func() {
				fmt.Println("sub data channel has opened")
			})
			p.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				// must send to record service

				p.parent.OnReceivedAppData(p.userid, p.peerid, msg.Data, len(msg.Data))
				fmt.Printf("%s <- %s:%s ### recv channel data len:%d\n", p.userid, p.remoteuserid, p.peerid, len(msg.Data))
			})
		}
	}

	// Create an offer
	offer, err := p.peerConnection.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err := p.peerConnection.SetLocalDescription(offer); err != nil {
		panic(err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdOffer,
		LabelSessID:    userID,
		LabelSrcSessID: remoteUserID,
		LabelPeerID:    peerid,
		LabelSDP:       offer.SDP,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	p.onSendMessageHandler(userID, string(msg))
}

// HandleRemoteOffer
func (p *peer) HandleRemoteOffer(j jsonparser) {
	userID := GetValue(j, LabelSessID)
	peerid := GetValue(j, LabelPeerID)
	strSdp := GetValue(j, LabelSDP)

	mapSdp := map[string]interface{}{
		"type": "offer",
		"sdp":  strSdp,
	}
	offer := webrtc.SessionDescription{}
	strJSON, err := json.Marshal(mapSdp)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(strJSON, &offer)
	if err != nil {
		panic(err)
	}

	if err = p.setSsrcFromSDP(&offer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err = p.peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	go func() { p.remoteSdpReceived <- 0 }()

	// Create answer
	answer, err := p.peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	if err = p.peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	p.SetCachedCandidates()

	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    CmdAnswer,
		LabelSessID: userID,
		LabelPeerID: peerid,
		LabelSDP:    answer.SDP,
		LabelCode:   0,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	p.onSendMessageHandler(userID, string(msg))
}

// HandleRemoteAnswer
func (p *peer) HandleRemoteAnswer(j jsonparser) {
	strsdp := GetValue(j, LabelSDP)

	mapSdp := map[string]interface{}{
		"type": "answer",
		"sdp":  strsdp,
	}
	answer := webrtc.SessionDescription{}

	strJSON, err := json.Marshal(mapSdp)
	if err != nil {
		fmt.Println("")
		panic(err)
	}
	if err = json.Unmarshal(strJSON, &answer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err := p.peerConnection.SetRemoteDescription(answer); err != nil {
		panic(err)
	}
	go func() { p.remoteSdpReceived <- 0 }()
	p.SetCachedCandidates()
}

// HandleRemoteCandidate
func (p *peer) HandleRemoteCandidate(j jsonparser) {
	strcandidate := GetValue(j, CmdCandidate)
	if !p.setedRemoteDesc {
		p.candidateCaches.PushBack(strcandidate)
		return
	}

	if err := p.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: strcandidate}); err != nil {
		panic(err)
	}
}

// SetCachedCandidates consume all remote cached candidates
func (p *peer) SetCachedCandidates() {
	p.setedRemoteDesc = true

	for candi := p.candidateCaches.Front(); candi != nil; candi = candi.Next() {
		if err := p.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: candi.Value.(string)}); err != nil {
			panic(err)
		}
	}
}

func (p *peer) HandleFakeSubscribe(j jsonparser) {
	podid := GetValue(j, LabelPodId)
	userID := GetValue(j, LabelSessID)
	// remoteUserID := GetValue(j, LabelSrcSessID)
	peerid := GetValue(j, LabelPeerID)

	psdp, err := p.publishersdp.Unmarshal()
	if err != nil {
		panic(err)
	}
	for _, media := range psdp.MediaDescriptions {
		if media.MediaName.Media == "video" {
			p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion-v")
			if err != nil {
				panic(err)
			}
			p.videoSender, err = p.peerConnection.AddTrack(p.videoTrack)
			if err != nil {
				panic(err)
			}
			go func() {
				for {
					var pkts []rtcp.Packet
					if pkts, _, err = p.videoSender.ReadRTCP(); err != nil {
						return
					}
					for _, pkt := range pkts {
						_, ok := pkt.(*rtcp.PictureLossIndication)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp PLI\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
						_, ok = pkt.(*rtcp.SliceLossIndication)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp SLI\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
						_, ok = pkt.(*rtcp.FullIntraRequest)
						if ok {
							fmt.Printf("recv from %s<-%s:%s rtcp FIR\n", p.userid, p.remoteuserid, p.peerid)
							p.parent.RequestPli(p.peerid)
						}
					}
				}
			}()
		} else if media.MediaName.Media == "audio" {
			p.audioTrack, err = webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion-a")
			if err != nil {
				panic(err)
			}
			p.audioSender, err = p.peerConnection.AddTrack(p.audioTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "application" {
			var order = true
			//var lefttime uint16 = 5000
			var retransmit uint16 = 5
			options := &webrtc.DataChannelInit{
				Ordered: &order,
				//MaxPacketLifeTime: &lefttime,
				MaxRetransmits: &retransmit,
			}
			if p.dataChannel, err = p.peerConnection.CreateDataChannel("whiteboard", options); err != nil {
				panic(err)
			}
			p.dataChannel.OnError(func(err error) {
				fmt.Printf("sub data channel got an error:%e\n", err)
			})
			p.dataChannel.OnClose(func() {
				fmt.Println("sub data channel has closed")
			})
			p.dataChannel.OnOpen(func() {
				fmt.Println("sub data channel has opened")
			})
			p.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				// Don't sending to record server, because it has already been sended on original pod
				p.parent.OnReceivedAppData(p.userid, p.peerid, msg.Data, len(msg.Data))
				fmt.Printf("%s <- %s:%s ### recv channel data len:%d\n", p.userid, p.remoteuserid, p.peerid, len(msg.Data))
			})
		}
	}

	// Create an offer
	offer, err := p.peerConnection.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err := p.peerConnection.SetLocalDescription(offer); err != nil {
		panic(err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdFakeOffer,
		LabelClusterId: p.conf.Clusterid,
		LabelPodId:     p.conf.Uuid,
		LabelRoomId:    p.parent.parent.roomid,
		LabelSessID:    userID,
		// LabelSrcSessID: remoteUserID,
		LabelPeerID: peerid,
		LabelSDP:    offer.SDP,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	PublishMessage2CrossClusterPod(podid, string(msg))
}

// HandleFakeOffer
func (p *peer) HandleFakeOffer(j jsonparser) {
	podid := GetValue(j, LabelPodId)
	userID := GetValue(j, LabelSessID)
	peerid := GetValue(j, LabelPeerID)
	strSdp := GetValue(j, LabelSDP)

	mapSdp := map[string]interface{}{
		"type": "offer",
		"sdp":  strSdp,
	}
	offer := webrtc.SessionDescription{}
	strJSON, err := json.Marshal(mapSdp)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(strJSON, &offer)
	if err != nil {
		panic(err)
	}

	if err = p.setSsrcFromSDP(&offer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err = p.peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	go func() { p.remoteSdpReceived <- 0 }()

	// Create answer
	answer, err := p.peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	if err = p.peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	p.SetCachedCandidates()

	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdFakeAnswer,
		LabelClusterId: p.conf.Clusterid,
		LabelPodId:     p.conf.Uuid,
		LabelRoomId:    p.parent.parent.roomid,
		LabelSessID:    userID,
		LabelSrcSessID: userID,
		LabelPeerID:    peerid,
		LabelSDP:       answer.SDP,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	PublishMessage2CrossClusterPod(podid, string(msg))
}

// HandleFakeAnswer
func (p *peer) HandleFakeAnswer(j jsonparser) {
	strsdp := GetValue(j, LabelSDP)

	mapSdp := map[string]interface{}{
		"type": "answer",
		"sdp":  strsdp,
	}
	answer := webrtc.SessionDescription{}

	strJSON, err := json.Marshal(mapSdp)
	if err != nil {
		fmt.Println("")
		panic(err)
	}
	if err = json.Unmarshal(strJSON, &answer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err := p.peerConnection.SetRemoteDescription(answer); err != nil {
		panic(err)
	}
	go func() { p.remoteSdpReceived <- 0 }()
	p.SetCachedCandidates()
}

func (p *peer) IsReady() bool {
	return p.isReady
}

func (p *peer) ShouldNotify() bool {
	if p.shouldNotify {
		p.shouldNotify = false
		return true
	}
	return p.shouldNotify
}

/* func (p *peer) sendFir() {
	if p.role != 1 {
		fmt.Println("illegal rtcp fir request")
		return
	}

	if rtcpSendErr := p.peerConnection.WriteRTCP(
		[]rtcp.Packet{&rtcp.FullIntraRequest{MediaSSRC: uint32(p.getVideoSsrc())}}); rtcpSendErr != nil {
		fmt.Println(rtcpSendErr)
	}
} */

func (p *peer) RequestKeyframe() {
	if p.filter != nil && p.isReady {
		p.filter.TryRequestKeyframe()
	}
}

func (p *peer) sendPli() {
	if p.role != 1 {
		fmt.Println("illegal rtcp pli request")
		return
	}

	if rtcpSendErr := p.peerConnection.WriteRTCP(
		[]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(p.getVideoSsrc())}}); rtcpSendErr != nil {
		fmt.Println(rtcpSendErr)
	}
}

func (p *peer) deliverAudioData(buff []byte, len int) {
	if p.audioSender != nil {
		if _, err := p.audioTrack.Write(buff[:len]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			panic(err)
		}
	}
}

func (p *peer) deliverVideoData(buff []byte, len int) {
	if p.videoTrack != nil {
		if _, err := p.videoTrack.Write(buff[:len]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			panic(err)
		}
	}
}

func (p *peer) deliverAppData(buff []byte, len int) {
	if p.dataChannel != nil {
		err := p.dataChannel.SendText(string(buff))
		fmt.Printf("%s %s:%s ### send channel data len:%d\n", p.userid, p.remoteuserid, p.peerid, len)
		if err != nil {
			fmt.Printf("role:%d %s-%s ### send channel data failed:%e\n", p.role, p.userid, p.peerid, err)
		}
	}
}

func (p *peer) SetAudioCodec(codec webrtc.RTPCodecParameters) {
	p.audioCodec = codec
}

func (p *peer) SetVideoCodec(codec webrtc.RTPCodecParameters) {
	p.videoCodec = codec
}

func (p *peer) GetAudioCodec() webrtc.RTPCodecParameters {
	return p.audioCodec
}

func (p *peer) GetVideoCodec() webrtc.RTPCodecParameters {
	return p.videoCodec
}

func (p *peer) isAudioSsrc(s int) bool {
	for e := p.audiossrc.Front(); e != nil; e = e.Next() {
		if e.Value.(int) == s {
			return true
		}
	}
	return false
}

func (p *peer) isVideoSsrc(s int) bool {
	for e := p.videossrc.Front(); e != nil; e = e.Next() {
		if e.Value.(int) == s {
			return true
		}
	}
	return false
}

func (p *peer) getVideoSsrc() (ssrc int) {
	for e := p.videossrc.Front(); e != nil; e = e.Next() {
		return e.Value.(int)
	}
	return
}

func (p *peer) setSsrcFromSDP(remoteSdp *webrtc.SessionDescription) error {
	offersdp, err := remoteSdp.Unmarshal()
	if err != nil {
		return err
	}
	p.audiossrc.Init()
	p.videossrc.Init()
	p.appssrc.Init()
	for _, value := range offersdp.MediaDescriptions {
		if value.MediaName.Media == "audio" {
			for _, a := range value.Attributes {
				if a.Key == "ssrc" {
					array := strings.FieldsFunc(a.Value, func(s rune) bool {
						if s == ':' || s == ' ' {
							return true
						}
						return false
					})
					fmt.Printf("audio ssrc attributes: %s\n", a.Value)
					fmt.Println(array)
					if array[1] == "cname" {
						ssrc, err := strconv.Atoi(array[0])
						if err != nil {
							panic(err)
						}
						p.audiossrc.PushBack(ssrc)
					}
				}
			}
		} else if value.MediaName.Media == "video" {
			for _, a := range value.Attributes {
				if a.Key == "ssrc" {
					array := strings.FieldsFunc(a.Value, func(s rune) bool {
						if s == ':' || s == ' ' {
							return true
						}
						return false
					})
					fmt.Printf("video ssrc attributes: %s\n", a.Value)
					fmt.Println(array)
					if array[1] == "cname" {
						ssrc, err := strconv.Atoi(array[0])
						if err != nil {
							panic(err)
						}
						p.videossrc.PushBack(ssrc)
					}
				}
			}
		} else if value.MediaName.Media == "Application" {
			for _, a := range value.Attributes {
				if a.Key == "ssrc" {
					array := strings.FieldsFunc(a.Value, func(s rune) bool {
						if s == ':' || s == ' ' {
							return true
						}
						return false
					})
					fmt.Printf("application ssrc attributes: %s\n", a.Value)
					fmt.Println(array)
					if array[1] == "cname" {
						ssrc, err := strconv.Atoi(array[0])
						if err != nil {
							panic(err)
						}
						p.appssrc.PushBack(ssrc)
					}
				}
			}
		}
	}
	return nil
}

func (p *peer) CheckIceInfo(typ, ip string, port uint16) {
	<-p.remoteSdpReceived

	lDesc := p.peerConnection.LocalDescription()
	rDesc := p.peerConnection.RemoteDescription()
	if lDesc == nil || rDesc == nil {
		return
	}
	lSdp, err1 := lDesc.Unmarshal()
	rSdp, err2 := rDesc.Unmarshal()
	if err1 != nil || err2 != nil {
		panic("")
	}

	var lUfrag, rUfrag string
	for _, m := range lSdp.MediaDescriptions {
		if m.MediaName.Media == "video" {
			ufrag, haveUfrag := m.Attribute("ice-ufrag")
			if haveUfrag {
				lUfrag = ufrag
				break
			}
		}
	}

	for _, m := range rSdp.MediaDescriptions {
		if m.MediaName.Media == "video" {
			ufrag, haveUfrag := m.Attribute("ice-ufrag")
			if haveUfrag {
				rUfrag = ufrag
				break
			}
		}
	}

	msg, err := json.Marshal(map[string]interface{}{
		"action":     "iceinfo",
		"local_ice":  lUfrag,
		"remote_ice": rUfrag,
		"protocol":   "udp",
		"local_ip":   ip,
		"local_port": port,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	BroadcastMessage2Proxy(string(msg))
}
