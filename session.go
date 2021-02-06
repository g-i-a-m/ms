package main

import "fmt"

type session struct {
	sessionid  string
	publisher  map[string]peer
	subscriber map[string]peer
}

// CreateSession is create a session object
func CreateSession(id string) *session {
	return &session{
		sessionid:  id,
		publisher:  make(map[string]peer),
		subscriber: make(map[string]peer),
	}
}

func (sess *session) HandleMessage(j jsonparser) {
	command := GetValue(j, "type")
	peerid := GetValue(j, "peerid")
	if command == "push" {
		if _, ok := sess.publisher[peerid]; ok {
			delete(sess.publisher, peerid)
		}
		peer := CreatePeer(peerid, 1)
		peer.Init()
		sess.publisher[peerid] = *peer
	} else if command == "stoppush" {
		if _, ok := sess.publisher[peerid]; ok {
			delete(sess.publisher, peerid)
		}
	} else if command == "sub" {
		if _, ok := sess.subscriber[peerid]; ok {
			delete(sess.subscriber, peerid)
		}
		peer := CreatePeer(peerid, 2)
		peer.Init()
		sess.subscriber[peerid] = *peer
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
			fmt.Println("not publish/subscribe yet:")
		}
	}
}
