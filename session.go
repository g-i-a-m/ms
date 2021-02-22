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
	publishers           map[string]*peer
	subscribers          map[string]*peer
	onSendMessageHandler func(topic, msg string)
	parent               *room
}

// CreateSession is create a session object
func CreateSession(r *room, id string) *session {
	return &session{
		sessionid:   id,
		publishers:  make(map[string]*peer),
		subscribers: make(map[string]*peer),
		parent:      r,
	}
}

func (sess *session) SetSendMessageHandler(f func(topic, msg string)) {
	sess.onSendMessageHandler = f
}

func (sess *session) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	sessionid := GetValue(j, "sessionid")
	srcsessionid := GetValue(j, "srcsessionid")
	peerid := GetValue(j, "peerid")
	// srcpeerid := GetValue(j, "srcpeerid")
	if command == "heartbeat" {

	} else if command == "publish" {
		if _, ok := sess.publishers[peerid]; ok {
			delete(sess.publishers, peerid)
		}
		peer, err := CreatePublishPeer(sess, sessionid, peerid)
		if err != nil {
			fmt.Println("publish to create peer failed", err)
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.publishers[peerid] = peer

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
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			delete(sess.subscribers, sessionid+srcsessionid+peerid)
		}
		sess.submux.Unlock()
		peer, err := CreateSubscribePeer(sess, sessionid, srcsessionid, peerid, sdp)
		if err != nil {
			fmt.Println("sub to create peer failed")
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.submux.Lock()
		sess.subscribers[sessionid+srcsessionid+peerid] = peer
		sess.submux.Unlock()
		peer.HandleMessage(j)
	} else if command == "stopsub" {
		sess.submux.Lock()
		defer sess.submux.Unlock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			delete(sess.subscribers, sessionid+srcsessionid+peerid)
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
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			peer := sess.subscribers[sessionid+srcsessionid+peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not subscribe yet:")
		}
	} else if command == "candidate" {
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
		} else if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			peer := sess.subscribers[sessionid+srcsessionid+peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish/subscribe yet")
		}
	} else {
		fmt.Println("session unsupport msg type:", command)
	}
}

func (sess *session) hasPublisher() (ret bool, sid, pid string) {
	if _, ok := sess.publishers["camera"]; ok {
		peer := sess.publishers["camera"]
		if ret := peer.IsReady(); ret {
			return true, sess.sessionid, "camera"
		}
	}
	return false, "", ""
}

func (sess *session) OnIceReady(role int, sid, ssid, pid string) {
	if role == 1 {
		if _, ok := sess.publishers[pid]; ok {
			sess.parent.OnPublisherReady(sid, pid)
			return
		}
	}

	if role == 2 {
		// do nothing
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
