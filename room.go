// +build !js

package main

import "fmt"

type room struct {
	roomid   string
	sessions map[string]session
}

// CreateRoom is create a room object
func CreateRoom(id string) *room {
	return &room{
		roomid:   id,
		sessions: make(map[string]session),
	}
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
	} else if command == "push" ||
		command == "stoppush" ||
		command == "sub" ||
		command == "stopsub" ||
		command == "offer" ||
		command == "answer" ||
		command == "candidate" {
		if _, ok := r.sessions[sessionid]; ok {
			r := r.sessions[sessionid]
			r.HandleMessage(j)
		} else {
			fmt.Println("not login yet:")
		}
	}
}
