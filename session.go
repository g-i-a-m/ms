package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v3"
)

type session struct {
	pubmux               sync.RWMutex
	submux               sync.RWMutex
	sessionid            string
	publishers           map[string]peer
	subscribers          map[string]peer
	onSendMessageHandler func(topic, msg string)
}

// CreateSession is create a session object
func CreateSession(id string) *session {
	return &session{
		sessionid:   id,
		publishers:  make(map[string]peer),
		subscribers: make(map[string]peer),
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
	if command == "publish" {
		if _, ok := sess.publishers[peerid]; ok {
			delete(sess.publishers, peerid)
		}
		peer := CreatePeer(peerid, 1)
		peer.Init(nil)
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.publishers[peerid] = *peer

		msg, err := json.Marshal(map[string]interface{}{
			"type":      "publish",
			"sessionid": sessionid,
			"peerid":    peerid,
			"code":      0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		sess.onSendMessageHandler(sessionid, string(msg))
	} else if command == "stoppush" {
		if _, ok := sess.publishers[peerid]; ok {
			delete(sess.publishers, peerid)

			msg, err := json.Marshal(map[string]interface{}{
				"type":      "stoppush",
				"sessionid": sessionid,
				"peerid":    peerid,
				"code":      0,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			sess.onSendMessageHandler(sessionid, string(msg))
		}
	} else if command == "sub" {
		var sdp *webrtc.SessionDescription
		if _, ok := sess.publishers[peerid]; ok {
			sdp = sess.publishers[peerid].peerConnection.CurrentLocalDescription()
		}
		sess.submux.Lock()
		if _, ok := sess.subscribers[peerid]; ok {
			delete(sess.subscribers, peerid)
		}
		sess.submux.Unlock()
		peer := CreatePeer(peerid, 2)
		peer.Init(sdp)
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.submux.Lock()
		sess.subscribers[peerid] = *peer
		sess.submux.Unlock()
		peer.HandleMessage(j)
	} else if command == "stopsub" {
		sess.submux.Lock()
		defer sess.submux.Unlock()
		if _, ok := sess.subscribers[peerid]; ok {
			delete(sess.subscribers, peerid)
		}
	} else if command == "offer" {
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish yet:")
		}
	} else if command == "answer" {
		sess.submux.RLock()
		defer sess.submux.RUnlock()
		if _, ok := sess.subscribers[peerid]; ok {
			peer := sess.subscribers[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not subscribe yet:")
		}
	} else if command == "candidate" {
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
		} else if _, ok := sess.subscribers[peerid]; ok {
			peer := sess.subscribers[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish/subscribe yet")
		}
	} else {
		fmt.Println("session unsupport msg type:", command)
	}
}

func (sess *session) OnReceivedVideoData(buff []byte) {
	sess.submux.RLock()
	defer sess.submux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverVideoData(buff)
	}
}

func (sess *session) OnReceivedAudioData(buff []byte) {
	sess.submux.RLock()
	defer sess.submux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverAudioData(buff)
	}
}

func (sess *session) OnReceivedAppData(buff []byte) {
	sess.submux.RLock()
	defer sess.submux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverAppData(buff)
	}
}
