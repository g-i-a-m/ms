package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

type session struct {
	pubsmux              sync.RWMutex
	subsmux              sync.RWMutex
	sessionid            string
	publishers           map[string]*peer
	subscribers          map[string]*peer
	onSendMessageHandler func(topic, msg string)
	parent               *room
	alivetime            int64
}

// CreateSession is create a session object
func CreateSession(r *room, id string) *session {
	return &session{
		sessionid:   id,
		publishers:  make(map[string]*peer),
		subscribers: make(map[string]*peer),
		parent:      r,
		alivetime:   time.Now().Unix(),
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

	if command == "heartbeat" {
		sess.alivetime = time.Now().Unix()
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
		sess.subsmux.Lock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			delete(sess.subscribers, sessionid+srcsessionid+peerid)
		}
		sess.subsmux.Unlock()
		peer, err := CreateSubscribePeer(sess, sessionid, srcsessionid, peerid, sdp)
		if err != nil {
			fmt.Println("sub to create peer failed")
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.subsmux.Lock()
		sess.subscribers[sessionid+srcsessionid+peerid] = peer
		sess.subsmux.Unlock()
		peer.HandleMessage(j)
	} else if command == "stopsub" {
		sess.subsmux.Lock()
		defer sess.subsmux.Unlock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			delete(sess.subscribers, sessionid+srcsessionid+peerid)
		}
	} else if command == "offer" {
		sess.subsmux.RLock()
		defer sess.subsmux.RUnlock()
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Println("not publish yet:")
		}
	} else if command == "answer" {
		sess.subsmux.RLock()
		defer sess.subsmux.RUnlock()
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
	sess.pubsmux.RLock()
	defer sess.pubsmux.RUnlock()
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
		sess.pubsmux.RLock()
		defer sess.pubsmux.RUnlock()
		if _, ok := sess.publishers[pid]; ok {
			sess.parent.OnPublisherReady(sid, pid)
			return
		}
	}

	if role == 2 {
		// do nothing
	}
}

func (sess *session) OnReceivedAudioData(buff []byte, len int) {
	sess.subsmux.RLock()
	defer sess.subsmux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverAudioData(buff, len)
	}
}

func (sess *session) OnReceivedVideoData(buff []byte, len int) {
	sess.subsmux.RLock()
	defer sess.subsmux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverVideoData(buff, len)
	}
}

func (sess *session) OnReceivedAppData(buff []byte, len int) {
	sess.subsmux.RLock()
	defer sess.subsmux.RUnlock()
	for _, value := range sess.subscribers {
		value.deliverAppData(buff, len)
	}
}

func (sess *session) IsStillAlive() bool {
	if (time.Now().Unix() - sess.alivetime) > 60 {
		return false
	}
	return true
}
