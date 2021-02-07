// +build !js

package main

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/examples/internal/signal"
	"github.com/pion/webrtc/v3/examples/media-server/mqttclient"
)

type peer struct {
	peerid         string
	role           int
	peerConnection *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticRTP
	rtpsender      *webrtc.RTPSender
}

// CreatePeer is create a peer object
func CreatePeer(id string, role int) *peer {
	return &peer{
		peerid:         id,
		role:           role,
		peerConnection: nil,
		videoTrack:     nil,
		rtpsender:      nil,
	}
}

func (p *peer) Init() bool {
	var err error
	p.peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return false
	}
	return true
}

func (p *peer) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	if command == "offer" {
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
			if _, _, rtcpErr := p.rtpsender.Read(rtcpBuf); rtcpErr != nil {
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

		if _, err = p.videoTrack.Write(inboundRTPPacket[:n]); err != nil {
			panic(err)
		}
	}
}

// HandleRemoteOffer is
func (p *peer) HandleRemoteOffer(j jsonparser) {
	strsdp := GetValue(j, "sdp")
	// Create a video track
	var err error
	p.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	if err != nil {
		panic(err)
	}
	p.rtpsender, err = p.peerConnection.AddTrack(p.videoTrack)
	if err != nil {
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

	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	err = json.Unmarshal([]byte(strsdp), offer)
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
	jsonBytes, err := json.Marshal(answer)
	if err != nil {
		panic(err)
	}
	mqttclient.GetInstance().ReplyMessage(string(jsonBytes))
}

// HandleRemoteAnswer is
func (p *peer) HandleRemoteAnswer(j jsonparser) {

}

// HandleRemoteCandidate is
func (p *peer) HandleRemoteCandidate(j jsonparser) {
	// Set the remote ICE candidate
	// strcandidate := GetValue(j, "candidate")
	candi := webrtc.ICECandidateInit{}
	p.peerConnection.AddICECandidate(candi)
}
