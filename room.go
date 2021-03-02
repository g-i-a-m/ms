// +build !js

package main

import (
	"encoding/json"
	"fmt"
	"sync"
)

type room struct {
	roomid               string
	sessions             map[string]*session
	sessionsLock         sync.RWMutex
	onSendMessageHandler func(topic, msg string)
}

// CreateRoom is create a room object
func CreateRoom(id string) *room {
	return &room{
		roomid:   id,
		sessions: make(map[string]*session),
	}
}

func (r *room) SetSendMessageHandler(f func(topic, msg string)) {
	r.onSendMessageHandler = f
}

func (r *room) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	sessionid := GetValue(j, "sessionid")

	if command == "login" {
		if _, ok := r.sessions[sessionid]; ok {
			delete(r.sessions, sessionid)
		}
		sess := CreateSession(r, sessionid)
		sess.SetSendMessageHandler(r.onSendMessageHandler)
		r.sessionsLock.Lock()
		r.sessions[sessionid] = sess
		r.sessionsLock.Unlock()

		msg, err := json.Marshal(map[string]interface{}{
			"type":      "login",
			"sessionid": sessionid,
			"code":      0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(sessionid, string(msg))

		for _, value := range r.sessions {
			if ret, sid, pid := value.hasPublisher(); ret {
				msg, err := json.Marshal(map[string]interface{}{
					"type":      "pub",
					"roomid":    r.roomid,
					"sessionid": sid,
					"peerid":    pid,
				})
				if err != nil {
					fmt.Println("generate json error:", err)
				}
				r.onSendMessageHandler(sessionid, string(msg))
			}
		}
	} else if command == "heartbeat" {
		if _, ok := r.sessions[sessionid]; ok {
			sess := r.sessions[sessionid]
			sess.HandleMessage(j)
		} else {
			fmt.Println("not login yet:")
		}
	} else if command == "logout" {
		if _, ok := r.sessions[sessionid]; ok {
			delete(r.sessions, sessionid)
		}
	} else if command == "publish" ||
		command == "offer" ||
		command == "stoppush" {
		if _, ok := r.sessions[sessionid]; ok {
			sess := r.sessions[sessionid]
			sess.HandleMessage(j)
		} else {
			fmt.Println("not login yet:")
		}
	} else if command == "sub" ||
		command == "stopsub" ||
		command == "answer" {
		srcsessionid := GetValue(j, "srcsessionid")
		if _, ok := r.sessions[srcsessionid]; ok {
			sess := r.sessions[srcsessionid]
			sess.HandleMessage(j)
		} else {
			fmt.Println("not found source session: ", srcsessionid)
		}
	} else if command == "candidate" {
		srcsessionid := GetValue(j, "srcsessionid")
		if srcsessionid != "" {
			sessionid = srcsessionid
		}

		if _, ok := r.sessions[sessionid]; ok {
			sess := r.sessions[sessionid]
			sess.HandleMessage(j)
		} else {
			fmt.Println("not found session:", sessionid)
		}
	} else {
		fmt.Println("room unsupport msg type:", command)
	}
}

func (r *room) OnPublisherReady(sid, pid string) {
	for _, value := range r.sessions {
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "pub",
			"roomid":    r.roomid,
			"sessionid": sid,
			"peerid":    pid,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(value.sessionid, string(msg))
	}
}

func (r *room) OnPublisherPeerDisconnected(sid, pid string) {
	if _, ok := r.sessions[sid]; ok {
		sess := r.sessions[sid]
		sess.ReleasePublisher(sid, pid)
	}

	for _, value := range r.sessions {
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "unpub",
			"roomid":    r.roomid,
			"sessionid": sid,
			"peerid":    pid,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(value.sessionid, string(msg))
	}
}

func (r *room) OnPublisherPeerFailed(sid, pid string) {

}

func (r *room) OnSubscriberPeerDisconnected(sid, ssid, pid string) {
	if _, ok := r.sessions[ssid]; ok {
		sess := r.sessions[ssid]
		sess.ReleaseSubscriberPeer(sid, ssid, pid)
	}
}

func (r *room) OnSubscriberPeerFailed(sid, ssid, pid string) {

}

func (r *room) OnCheckKeepalive() {
	r.sessionsLock.RLock()
	for _, sess := range r.sessions {
		go sess.CheckKeepAlive()
	}
	r.sessionsLock.RUnlock()
}

func (r *room) HandleKeepaliveTimeout(sid string) {
	r.sessionsLock.Lock()
	if _, ok := r.sessions[sid]; ok {
		sess := r.sessions[sid]
		go sess.DoLeave()
		delete(r.sessions, sess.sessionid)
		fmt.Printf("client %s heartbeat timeout, kickout now\n", sid)
	}
	r.sessionsLock.Unlock()

	r.sessionsLock.RLock()
	for _, sess := range r.sessions {
		go sess.HandleSubscriberLeaved(sid)
	}
	r.sessionsLock.RUnlock()
}

func (r *room) OnReceivedAppData(fromSid, fromPid string, buff []byte, len int) {
	r.sessionsLock.RLock()
	for _, s := range r.sessions {
		s.BroadcastAppData(fromSid, fromPid, buff, len)
	}
	r.sessionsLock.RUnlock()
}
