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
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type peer struct {
	sessionid            string
	srcSessionid         string
	peerid               string
	role                 int
	peerConnection       *webrtc.PeerConnection
	videoTrack           *webrtc.TrackLocalStaticRTP
	videoSender          *webrtc.RTPSender
	audioTrack           *webrtc.TrackLocalStaticRTP
	audioSender          *webrtc.RTPSender
	dataChannel          *webrtc.DataChannel
	onSendMessageHandler func(topic, msg string)
	publishersdp         *webrtc.SessionDescription
	parent               *session
	setedRemoteDesc      bool
	candidateCaches      *list.List
	isReady              bool
	audiossrc            *list.List
	videossrc            *list.List
	appssrc              *list.List
}

// CreatePublishPeer to create a object for publish peer
func CreatePublishPeer(s *session, sid, pid string) (*peer, error) {
	p := peer{
		sessionid:       sid,
		srcSessionid:    "",
		peerid:          pid,
		role:            1,
		peerConnection:  nil,
		videoTrack:      nil,
		videoSender:     nil,
		audioTrack:      nil,
		audioSender:     nil,
		dataChannel:     nil,
		publishersdp:    nil,
		parent:          s,
		setedRemoteDesc: false,
		candidateCaches: list.New(),
		isReady:         false,
		audiossrc:       list.New(),
		videossrc:       list.New(),
		appssrc:         list.New(),
	}
	var err error
	p.peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{"turn:node.offcncloud.com:9900"},
				Username:   "ctf",
				Credential: "ctf123",
			},
		},
		ICETransportPolicy: webrtc.NewICETransportPolicy("all"),
	})
	if err != nil {
		return nil, err // fmt.Errorf("Create publish peer failed")
	}

	p.peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		ssrc := remoteTrack.SSRC()
		if p.isAudioSsrc(int(ssrc)) {
			fmt.Printf("Got audio remote track, id:%s, streamid:%s, ssrc:%x\n", []byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())
			go func() {
				ticker := time.NewTicker(1)
				for range ticker.C {
					if rtcpSendErr := p.peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}}); rtcpSendErr != nil {
						fmt.Println(rtcpSendErr)
					}
				}
			}()
			go func() {
				rtpBuf := make([]byte, 1500)
				for {
					i, _, readErr := remoteTrack.Read(rtpBuf)
					if readErr != nil {
						panic(readErr)
					}
					p.parent.OnReceivedAudioData(rtpBuf, i)
				}
			}()
			return
		}
		if p.isVideoSsrc(int(ssrc)) {
			fmt.Printf("Got video remote track, id:%s, streamid:%s, ssrc:%x\n", []byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())
			go func() {
				ticker := time.NewTicker(1)
				for range ticker.C {
					if rtcpSendErr := p.peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}}); rtcpSendErr != nil {
						fmt.Println(rtcpSendErr)
					}
				}
			}()
			go func() {
				rtpBuf := make([]byte, 1500)
				for {
					i, _, readErr := remoteTrack.Read(rtpBuf)
					if readErr != nil {
						panic(readErr)
					}
					p.parent.OnReceivedVideoData(rtpBuf, i)
				}
			}()
			return
		}
		fmt.Printf("Got unknow remote track, id:%s, streamid:%s, ssrc:%x\n", []byte(remoteTrack.ID()), []byte(remoteTrack.StreamID()), remoteTrack.SSRC())
	})

	p.peerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
		p.dataChannel = channel
		/* p.dataChannel.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' opened\n", channel.Label(), channel.ID())
			channel.Detach()
		}) */

		p.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			p.parent.OnReceivedAppData(msg.Data, 0)
		})
	})

	p.peerConnection.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		fmt.Println("ICE Gatherer state change:", state.String())
	})

	p.peerConnection.OnICECandidate(func(candi *webrtc.ICECandidate) {
		if candi == nil {
			return // must be publish ICE Gatherer completed
		}
		c := candi.ToJSON()
		jsonBytes, err := json.Marshal(c)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Got a local candidate: %s\n", jsonBytes)
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "candidate",
			"sessionid": p.sessionid,
			"peerid":    p.peerid,
			"candidate": fmt.Sprintf("%s", jsonBytes),
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		p.onSendMessageHandler(p.sessionid, string(msg))
	})

	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed: %s\n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			p.isReady = true
			p.parent.OnIceReady(p.role, p.sessionid, p.srcSessionid, p.peerid)
		}
	})

	return &p, nil
}

// CreateSubscribePeer to create a object for subscribe peer
func CreateSubscribePeer(s *session, sid, ssid, pid string, publisherSdp *webrtc.SessionDescription) (*peer, error) {
	p := peer{
		sessionid:       sid,
		srcSessionid:    ssid,
		peerid:          pid,
		role:            2,
		peerConnection:  nil,
		videoTrack:      nil,
		videoSender:     nil,
		audioTrack:      nil,
		audioSender:     nil,
		dataChannel:     nil,
		publishersdp:    publisherSdp,
		parent:          s,
		setedRemoteDesc: false,
		candidateCaches: list.New(),
		isReady:         false,
	}
	var err error
	p.peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{"turn:node.offcncloud.com:9900"},
				Username:   "ctf",
				Credential: "ctf123",
			},
		},
		ICETransportPolicy: webrtc.NewICETransportPolicy("all"),
	})
	if err != nil {
		return nil, err // fmt.Errorf("Create subscribe peer failed")
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
		fmt.Printf("Got a local candidate: %s\n", candi.String())
		c := candi.ToJSON()
		jsonBytes, err := json.Marshal(c)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Got a local candidate: %s\n", jsonBytes)
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "candidate",
			"sessionid": p.sessionid,
			"peerid":    p.peerid,
			"candidate": fmt.Sprintf("%s", jsonBytes),
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		p.onSendMessageHandler(p.sessionid, string(msg))
	})

	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed: %s\n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			p.isReady = true
			p.parent.OnIceReady(p.role, p.sessionid, p.srcSessionid, p.peerid)
		}
	})
	return &p, nil
}

func (p *peer) SetSendMessageHandler(f func(topic, msg string)) {
	p.onSendMessageHandler = f
}

func (p *peer) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	if command == "sub" {
		p.HandleSubscribe(j)
	} else if command == "offer" {
		p.HandleRemoteOffer(j)
	} else if command == "answer" {
		p.HandleRemoteAnswer(j)
	} else if command == "candidate" {
		p.HandleRemoteCandidate(j)
	} else {
		fmt.Println("peer unsupport msg type:", command)
	}
}

func (p *peer) HandleSubscribe(j jsonparser) {
	sessionid := GetValue(j, "sessionid")
	srcsessionid := GetValue(j, "srcsessionid")
	peerid := GetValue(j, "peerid")

	psdp, err := p.publishersdp.Unmarshal()
	if err != nil {
		panic(err)
	}
	for _, media := range psdp.MediaDescriptions {
		if media.MediaName.Media == "video" {
			p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion-v")
			if err != nil {
				panic(err)
			}
			p.videoSender, err = p.peerConnection.AddTrack(p.videoTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "audio" {
			p.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion-a")
			if err != nil {
				panic(err)
			}
			p.audioSender, err = p.peerConnection.AddTrack(p.audioTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "application" {
			if p.dataChannel, err = p.peerConnection.CreateDataChannel("whiteboard", nil); err != nil {
				panic(err)
			}
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
		"type":         "offer",
		"sessionid":    sessionid,
		"srcsessionid": srcsessionid,
		"peerid":       peerid,
		"sdp":          offer.SDP,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	p.onSendMessageHandler(sessionid, string(msg))
}

// HandleRemoteOffer
func (p *peer) HandleRemoteOffer(j jsonparser) {
	sessionid := GetValue(j, "sessionid")
	peerid := GetValue(j, "peerid")
	strSdp := GetValue(j, "sdp")

	mapSdp := map[string]interface{}{
		"type": "offer",
		"sdp":  strSdp,
	}
	offer := webrtc.SessionDescription{}
	strJSON, err := json.Marshal(mapSdp)
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
		"type":      "answer",
		"sessionid": sessionid,
		"peerid":    peerid,
		"sdp":       answer.SDP,
		"code":      0,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	p.onSendMessageHandler(sessionid, string(msg))
}

// HandleRemoteAnswer
func (p *peer) HandleRemoteAnswer(j jsonparser) {
	strsdp := GetValue(j, "sdp")

	mapSdp := map[string]interface{}{
		"type": "answer",
		"sdp":  strsdp,
	}
	answer := webrtc.SessionDescription{}

	strJSON, err := json.Marshal(mapSdp)
	if err = json.Unmarshal(strJSON, &answer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err := p.peerConnection.SetRemoteDescription(answer); err != nil {
		panic(err)
	}
	p.SetCachedCandidates()
}

// HandleRemoteCandidate
func (p *peer) HandleRemoteCandidate(j jsonparser) {
	strcandidate := GetValue(j, "candidate")
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

func (p *peer) IsReady() bool {
	return p.isReady
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
		if err != nil {
			panic(err)
		}
	}
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

func (p *peer) setSsrcFromSDP(remoteSdp *webrtc.SessionDescription) error {
	offersdp, err := remoteSdp.Unmarshal()
	if err != nil {
		return err
	}
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
