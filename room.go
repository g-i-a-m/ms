// +build !js

package main

import (
	"encoding/json"
	"fmt"
)

type room struct {
	roomid               string
	sessions             map[string]*session
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
		r.sessions[sessionid] = sess

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
