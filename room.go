// +build !js

package main

import (
	"encoding/json"
	"fmt"
)

type room struct {
	roomid               string
	sessions             map[string]session
	onSendMessageHandler func(topic, msg string)
}

// CreateRoom is create a room object
func CreateRoom(id string) *room {
	return &room{
		roomid:   id,
		sessions: make(map[string]session),
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
		sess := CreateSession(sessionid)
		r.sessions[sessionid] = *sess

		topic := GetValue(j, "topic")
		msg, err := json.Marshal(map[string]interface{}{
			"type":      "login",
			"sessionid": sessionid,
			"code":      0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(topic, string(msg))
	} else if command == "logout" {
		if _, ok := r.sessions[sessionid]; ok {
			delete(r.sessions, sessionid)
		}
	} else if command == "push" ||
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
	}
}
