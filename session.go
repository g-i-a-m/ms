package main

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
)

type session struct {
	sessionid            string
	publisher            map[string]peer
	subscriber           map[string]peer
	onSendMessageHandler func(topic, msg string)
}

// CreateSession is create a session object
func CreateSession(id string) *session {
	return &session{
		sessionid:  id,
		publisher:  make(map[string]peer),
		subscriber: make(map[string]peer),
	}
}

func (sess *session) SetSendMessageHandler(f func(topic, msg string)) {
	sess.onSendMessageHandler = f
}

func (sess *session) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	sessionid := GetValue(j, "sessionid")
	peerid := GetValue(j, "peerid")
	// srcpeerid := GetValue(j, "srcpeerid")
	if command == "push" {
		if _, ok := sess.publisher[peerid]; ok {
			delete(sess.publisher, peerid)
		}
		peer := CreatePeer(peerid, 1)
		peer.Init(nil)
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.publisher[peerid] = *peer

		topic := GetValue(j, "topic")
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "push",
			"sessionid": sessionid,
			"peerid":    peerid,
			"code":      0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		sess.onSendMessageHandler(topic, string(msg))
	} else if command == "stoppush" {
		if _, ok := sess.publisher[peerid]; ok {
			delete(sess.publisher, peerid)

			topic := GetValue(j, "topic")
			msg, err := json.Marshal(map[string]interface{}{
				"type":      "stoppush",
				"sessionid": sessionid,
				"peerid":    peerid,
				"code":      0,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			sess.onSendMessageHandler(topic, string(msg))
		}
	} else if command == "sub" {
		var sdp *webrtc.SessionDescription
		if _, ok := sess.publisher[peerid]; ok {
			sdp = sess.publisher[peerid].peerConnection.CurrentLocalDescription()
		}
		if _, ok := sess.subscriber[peerid]; ok {
			delete(sess.subscriber, peerid)
		}
		peer := CreatePeer(peerid, 2)
		peer.Init(sdp)
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.subscriber[peerid] = *peer
		peer.HandleMessage(j)
	} else if command == "stopsub" {
		if _, ok := sess.subscriber[peerid]; ok {
			delete(sess.subscriber, peerid)
		}
	} else if command == "offer" {
		if _, ok := sess.publisher[peerid]; ok {
			peer := sess.publisher[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish yet:")
		}
	} else if command == "answer" {
		if _, ok := sess.subscriber[peerid]; ok {
			peer := sess.subscriber[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not subscribe yet:")
		}
	} else if command == "candidate" {
		if _, ok := sess.publisher[peerid]; ok {
			peer := sess.publisher[peerid]
			peer.HandleMessage(j)
		} else if _, ok := sess.subscriber[peerid]; ok {
			peer := sess.subscriber[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish/subscribe yet")
		}
	}
}
