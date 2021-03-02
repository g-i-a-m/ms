package main

import (
	"container/list"
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
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			go sess.ReleaseSubscriberPeer(sessionid, srcsessionid, peerid)
		}
		sess.subsmux.RUnlock()
	} else if command == "offer" {
		sess.subsmux.RLock()
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			go peer.HandleMessage(j)
		} else {
			fmt.Println("not publish yet:")
		}
		sess.subsmux.RUnlock()
	} else if command == "answer" {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			peer := sess.subscribers[sessionid+srcsessionid+peerid]
			go peer.HandleMessage(j)
		} else {
			fmt.Println("not subscribe yet:")
		}
		sess.subsmux.RUnlock()
	} else if command == "candidate" {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			go peer.HandleMessage(j)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock()

		sess.subsmux.RLock()
		if _, ok := sess.subscribers[sessionid+srcsessionid+peerid]; ok {
			peer := sess.subscribers[sessionid+srcsessionid+peerid]
			go peer.HandleMessage(j)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()

		fmt.Println("candidate not process, because not found peer")
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
			go sess.parent.OnPublisherReady(sid, pid)
			return
		}
	}

	if role == 2 {
		if _, ok := sess.publishers[pid]; ok {
			peer := sess.publishers[pid]
			go peer.SendFir()
		}
	}
}

// 1>. clean all publish peer if exist
// 2>. clean all subscribe peer if exist
// 3>. clean watch other people' publish peer if exist (by return sid, in last callstack)
func (sess *session) DoLeave() string {
	sess.pubsmux.Lock()
	for _, value := range sess.publishers {
		go value.Destroy()
	}
	sess.publishers = make(map[string]*peer)
	sess.pubsmux.Unlock()

	sess.subsmux.Lock()
	for _, value := range sess.subscribers {
		go value.Destroy()
	}
	sess.subscribers = make(map[string]*peer)
	sess.subsmux.Unlock()

	return sess.sessionid
}

func (sess *session) OnIceDisconnected(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[pid]; ok {
			go sess.parent.OnPublisherPeerDisconnected(sid, pid)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock()
	}

	if role == 2 {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[pid]; ok {
			go sess.parent.OnSubscriberPeerDisconnected(sid, ssid, pid)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()
	}
}

func (sess *session) OnIceConnectionFailed(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[pid]; ok {
			go sess.parent.OnPublisherPeerFailed(sid, pid)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock()
	}

	if role == 2 {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[pid]; ok {
			go sess.parent.OnSubscriberPeerFailed(sid, ssid, pid)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()
	}
}

func (sess *session) OnIceConnectionClosed(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[pid]; ok {
			go sess.parent.OnPublisherPeerFailed(sid, pid)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock()
	}

	if role == 2 {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[pid]; ok {
			go sess.parent.OnSubscriberPeerFailed(sid, ssid, pid)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()
	}
}

func (sess *session) ReleasePublisher(sid, pid string) {
	sess.pubsmux.Lock()
	if _, ok := sess.publishers[pid]; ok {
		peer := sess.publishers[pid]
		go peer.Destroy()
		delete(sess.publishers, pid)
	}
	sess.pubsmux.Unlock()

	sess.subsmux.Lock()
	for _, value := range sess.subscribers {
		if value.peerid == pid {
			go value.Destroy()
			key := value.sessionid + value.srcSessionid + value.peerid
			delete(sess.subscribers, key)
		}
	}
	sess.subsmux.Unlock()
}

func (sess *session) ReleaseSubscriberPeer(sid, ssid, pid string) {
	sess.subsmux.Lock()
	if _, ok := sess.subscribers[sid+ssid+pid]; ok {
		peer := sess.subscribers[sid+ssid+pid]
		go peer.Destroy()
		delete(sess.subscribers, sid+ssid+pid)
	}
	sess.subsmux.Unlock()
}

func (sess *session) HandleSubscriberLeaved(sid string) {
	var lst list.List
	sess.pubsmux.RLock()
	for _, value := range sess.publishers {
		lst.PushBack(value.peerid)
	}
	sess.pubsmux.RUnlock()

	sess.subsmux.Lock()
	for it := lst.Front(); it != nil; it = it.Next() {
		if _, ok := sess.subscribers[sid+sess.sessionid+it.Value.(string)]; ok {
			peer := sess.subscribers[sid+sess.sessionid+it.Value.(string)]
			go peer.Destroy()
			delete(sess.subscribers, sid+sess.sessionid+it.Value.(string))
		}
	}
	sess.subsmux.Unlock()
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

func (sess *session) OnReceivedAppData(fromSid, fromPid string, buff []byte, len int) {
	sess.parent.OnReceivedAppData(fromSid, fromPid, buff, len)
}

func (sess *session) BroadcastAppData(fromSid, fromPid string, buff []byte, len int) {
	sess.pubsmux.RLock()
	for _, value := range sess.publishers {
		if value.sessionid != fromSid || value.peerid != fromPid {
			value.deliverAppData(buff, len)
		}
	}
	sess.pubsmux.RUnlock()

	sess.subsmux.RLock()
	for _, value := range sess.subscribers {
		if value.sessionid != fromSid || value.peerid != fromPid {
			value.deliverAppData(buff, len)
		}
	}
	sess.subsmux.RUnlock()
}

func (sess *session) CheckKeepAlive() {
	if (time.Now().Unix() - sess.alivetime) > 60 {
		go sess.parent.HandleKeepaliveTimeout(sess.sessionid)
	}
}
