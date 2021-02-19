// +build !js

package main

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/examples/internal/signal"
)

type peer struct {
	peerid               string
	role                 int
	peerConnection       *webrtc.PeerConnection
	videoTrack           *webrtc.TrackLocalStaticRTP
	videoSender          *webrtc.RTPSender
	audioTrack           *webrtc.TrackLocalStaticRTP
	audioSender          *webrtc.RTPSender
	whiteboardChannel    *webrtc.DataChannel
	onSendMessageHandler func(topic, msg string)
	publishersdp         *webrtc.SessionDescription
	parent               *session
}

// CreatePeer is create a peer object
func CreatePeer(id string, role int) *peer {
	return &peer{
		peerid:         id,
		role:           role,
		peerConnection: nil,
		videoTrack:     nil,
		videoSender:    nil,
		audioTrack:     nil,
		audioSender:    nil,
		publishersdp:   nil,
		parent:         nil,
	}
}

func (p *peer) Init(sdp *webrtc.SessionDescription) bool {
	var err error
	p.peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"turn:node.offcncloud.com:9900"},
			},
		},
	})
	if err != nil {
		return false
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
	}

	// Read incoming RTCP packets
	// Before these packets are retuned they are processed by interceptors. For things
	// like NACK this needs to be called.
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := p.videoSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	p.peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &offer)

	// Set the remote SessionDescription
	if err := p.peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	// Create answer
	answer, err := p.peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(p.peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	if err = p.peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(*p.peerConnection.LocalDescription()))

	// Read RTP packets forever and send them to the WebRTC Client
	inboundRTPPacket := make([]byte, 1600) // UDP MTU
	for {
		//n, _, err := listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}

		if _, err = p.videoTrack.Write(inboundRTPPacket[:3]); err != nil {
			panic(err)
		}
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
			if p.whiteboardChannel, err = p.peerConnection.CreateDataChannel("whiteboard", nil); err != nil {
				panic(err)
			}
		}
	}

	if err := p.peerConnection.SetLocalDescription(*p.publishersdp); err != nil {
		panic(err)
	}

	// Response offer to subscriber
	topic := GetValue(j, "topic")
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
	p.onSendMessageHandler(topic, string(msg))
}

// HandleRemoteOffer is
func (p *peer) HandleRemoteOffer(j jsonparser) {
	sessionid := GetValue(j, "sessionid")
	srcsessionid := GetValue(j, "srcsessionid")
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

	// Create the video track
	p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion-v")
	if err != nil {
		panic(err)
	}
	p.videoSender, err = p.peerConnection.AddTrack(p.videoTrack)
	if err != nil {
		panic(err)
	}

	// Create the audio track
	p.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion-a")
	if err != nil {
		panic(err)
	}
	p.audioSender, err = p.peerConnection.AddTrack(p.audioTrack)
	if err != nil {
		panic(err)
	}

	// Create the data channel for whiteboard
	if p.whiteboardChannel, err = p.peerConnection.CreateDataChannel("whiteboard", nil); err != nil {
		panic(err)
	}

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
	})

	// Set the handler for data channel
	p.peerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
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
	topic := GetValue(j, "topic")
	msg, err := json.Marshal(map[string]interface{}{
		"type":         "answer",
		"sessionid":    sessionid,
		"srcsessionid": srcsessionid,
		"peerid":       peerid,
		"sdp":          answer.SDP,
		"code":         0,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	p.onSendMessageHandler(topic, string(msg))
}

// HandleRemoteAnswer is
func (p *peer) HandleRemoteAnswer(j jsonparser) {
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

	// Set the handler for local ICE candidate
	p.peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		fmt.Printf("Got a local candidate: %s\n", candidate.String())
	})

	p.peerConnection.OnTrack(func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {

	})
	p.peerConnection.OnDataChannel(func(*webrtc.DataChannel) {

	})
}

// HandleRemoteCandidate is
func (p *peer) HandleRemoteCandidate(j jsonparser) {
	strcandidate := GetValue(j, "candidate")
	if err := p.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(strcandidate)}); err != nil {
		panic(err)
	}
}

func (p *peer) deliverVideoData(buff []byte) {

}

func (p *peer) deliverAudioData(buff []byte) {

}

func (p *peer) deliverAppData(buff []byte) {

}
