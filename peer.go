// +build !js

package main

import (
	"container/list"
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
)

type peer struct {
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
}

// CreatePeer is create a peer object
func CreatePeer(id string, role int) *peer {
	return &peer{
		peerid:          id,
		role:            role,
		peerConnection:  nil,
		videoTrack:      nil,
		videoSender:     nil,
		audioTrack:      nil,
		audioSender:     nil,
		dataChannel:     nil,
		publishersdp:    nil,
		parent:          nil,
		setedRemoteDesc: false,
		candidateCaches: *list.New(),
	}
}

func (p *peer) Init(sdp *webrtc.SessionDescription) bool {
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
		panic(err)
		// return false
	}

	if p.role == 2 {
		p.publishersdp = sdp
	}
	return true
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
		fmt.Print(media, "\t")
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

	if err := p.peerConnection.SetLocalDescription(*p.publishersdp); err != nil {
		panic(err)
	}

	// Set the handler for local ICE candidate
	p.peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		fmt.Printf("Got a local candidate: %s\n", candidate.String())
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "candidate",
			"sessionid": sessionid,
			"peerid":    peerid,
			"candidate": candidate.String(),
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		p.onSendMessageHandler(sessionid, string(msg))
	})

	// Response offer to subscriber
	msg, err := json.Marshal(map[string]interface{}{
		"type":         "offer",
		"sessionid":    sessionid,
		"srcsessionid": srcsessionid,
		"peerid":       peerid,
		"sdp":          *p.publishersdp,
		"code":         0,
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
	p.SetCachedCandidates()

	// Set the handler for ICE connection state
	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed: %s\n", connectionState.String())
	})

	// Set the handler for remote Track
	p.peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Got the remote track\n")
	})

	// Set the handler for local ICE candidate
	p.peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		fmt.Printf("Got a local candidate: %s\n", candidate.String())
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "candidate",
			"sessionid": sessionid,
			"peerid":    peerid,
			"candidate": candidate.String(),
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		p.onSendMessageHandler(sessionid, string(msg))
	})

	// Set the handler for data channel
	p.peerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open.\n", channel.Label(), channel.ID())
		})

		channel.OnMessage(func(msg webrtc.DataChannelMessage) {
			p.parent.OnReceivedVideoData(msg.Data)
		})
	})

	// Create answer
	answer, err := p.peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

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
	sessionid := GetValue(j, "sessionid")
	peerid := GetValue(j, "peerid")
	strsdp := GetValue(j, "sdp")

	// Generate the answer sdp
	answer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(strsdp), &answer); err != nil {
		panic(err)
	}

	// Set the remote SessionDescription
	if err := p.peerConnection.SetRemoteDescription(answer); err != nil {
		panic(err)
	}
	p.SetCachedCandidates()

	// Set the handler for local ICE candidate
	p.peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		fmt.Printf("Got a local candidate: %s\n", candidate.String())
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "candidate",
			"sessionid": sessionid,
			"peerid":    peerid,
			"candidate": candidate.String(),
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		p.onSendMessageHandler(sessionid, string(msg))
	})
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
