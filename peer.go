// +build !js

package main

import (
	"container/list"
	"encoding/json"
	"fmt"

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
	candidateCaches      list.List
	isReady              bool
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
		candidateCaches: *list.New(),
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
		return nil, err // fmt.Errorf("Create publish peer failed")
	}

	p.peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Got the remote track\n")
	})

	p.peerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open.\n", channel.Label(), channel.ID())
		})

		channel.OnMessage(func(msg webrtc.DataChannelMessage) {
			p.parent.OnReceivedVideoData(msg.Data)
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
func CreateSubscribePeer(s *session, sid, ssid, pid string, sdp *webrtc.SessionDescription) (*peer, error) {
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
		publishersdp:    sdp,
		parent:          s,
		setedRemoteDesc: false,
		candidateCaches: *list.New(),
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

	sdp, err := p.publishersdp.Unmarshal()
	if err != nil {
		panic(err)
	}
	for _, media := range sdp.MediaDescriptions {
		if media.MediaName.Media == "video" {
			// Create the video track
			p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion-v")
			if err != nil {
				panic(err)
			}
			p.videoSender, err = p.peerConnection.AddTrack(p.videoTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "audio" {
			// Create the audio track
			p.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion-a")
			if err != nil {
				panic(err)
			}
			p.audioSender, err = p.peerConnection.AddTrack(p.audioTrack)
			if err != nil {
				panic(err)
			}
		} else if media.MediaName.Media == "application" {
			// Create the data channel for whiteboard
			if p.dataChannel, err = p.peerConnection.CreateDataChannel("whiteboard", nil); err != nil {
				panic(err)
			}
		}
	}

	// Create an offer to send to the other process
	offer, err := p.peerConnection.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err := p.peerConnection.SetLocalDescription(offer); err != nil {
		panic(err)
	}

	// Response offer to subscriber
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

// HandleRemoteOffer is
func (p *peer) HandleRemoteOffer(j jsonparser) {
	sessionid := GetValue(j, "sessionid")
	peerid := GetValue(j, "peerid")
	sdp := GetValue(j, "sdp")
	// Generate the offer sdp
	mapSdp := map[string]interface{}{
		"type": "offer",
		"sdp":  sdp,
	}
	offer := webrtc.SessionDescription{}
	strJSON, err := json.Marshal(mapSdp)
	err = json.Unmarshal(strJSON, &offer)
	if err != nil {
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

	// Response answer to publisher
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

// HandleRemoteAnswer is
func (p *peer) HandleRemoteAnswer(j jsonparser) {
	strsdp := GetValue(j, "sdp")

	// Generate the answer sdp
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

// HandleRemoteCandidate is
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

func (p *peer) deliverVideoData(buff []byte) {
	if p.videoSender != nil {

	}
}

func (p *peer) deliverAudioData(buff []byte) {
	if p.audioSender != nil {

	}
}

func (p *peer) deliverAppData(buff []byte) {
	if p.dataChannel != nil {
		sendErr := p.dataChannel.SendText(string(buff))
		if sendErr != nil {
			panic(sendErr)
		}
	}
}
