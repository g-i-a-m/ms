//go:build !js
// +build !js

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type room struct {
	conf                 *Config
	roomid               string
	parent               *roommgr
	sessions             map[string]*session
	sessionsLock         sync.RWMutex
	onSendMessageHandler func(topic, msg string)
}

// CreateRoom is create a room object
func CreateRoom(id string, rm *roommgr) *room {
	return &room{
		conf:     rm.conf,
		roomid:   id,
		parent:   rm,
		sessions: make(map[string]*session),
	}
}

func (r *room) SetSendMessageHandler(f func(topic, msg string)) {
	r.onSendMessageHandler = f
}

func (r *room) DestroyRoom() {
	r.sessionsLock.Lock()
	defer r.sessionsLock.Unlock()
	for _, value := range r.sessions {
		value.DoLeave()
	}
	r.sessions = make(map[string]*session)
}

func (r *room) HandleMessage(j jsonparser) {
	command := GetValue(j, LabelCmd)
	userID := GetValue(j, LabelSessID)

	if command == CmdLogin {
		userName := GetValue(j, LabelUserName)
		if _, ok := r.sessions[userID]; ok {
			r.sessionsLock.Lock()
			r.sessions[userID].DoLeave()
			delete(r.sessions, userID)
			r.sessionsLock.Unlock()
		}
		sess := CreateSession(r, userID, userName, r.conf, r.onSendMessageHandler)
		r.sessionsLock.Lock()
		r.sessions[userID] = sess
		r.sessionsLock.Unlock()

		msg, err := json.Marshal(map[string]interface{}{
			LabelCmd:    CmdLogin,
			LabelSessID: userID,
			LabelCode:   0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(userID, string(msg))

		for _, value := range r.sessions {
			if ret, sid, pid := value.hasPublisher(); ret {
				msg, err := json.Marshal(map[string]interface{}{
					LabelCmd:      CmdPub,
					LabelRoomId:   r.roomid,
					LabelSessID:   sid,
					LabelUserName: value.username,
					LabelPeerID:   pid,
				})
				if err != nil {
					fmt.Println("generate json error:", err)
				}
				r.onSendMessageHandler(userID, string(msg))
			}
		}
	} else if command == CmdHeartbeat {
		if _, ok := r.sessions[userID]; ok {
			sess := r.sessions[userID]
			sess.HandleMessage(j)
		} else {
			r.notifyNotLogin(command, userID)
		}
	} else if command == CmdLogout {
		r.KickoutFromRoom(userID)
	} else if command == CmdPush ||
		command == CmdOffer ||
		command == CmdStopPush {
		if _, ok := r.sessions[userID]; ok {
			sess := r.sessions[userID]
			sess.HandleMessage(j)
		} else {
			r.notifyNotLogin(command, userID)
		}
	} else if command == CmdSub ||
		command == CmdStopSub ||
		command == CmdAnswer {
		remoteUserID := GetValue(j, LabelSrcSessID)
		if _, ok := r.sessions[remoteUserID]; ok {
			sess := r.sessions[remoteUserID]
			sess.HandleMessage(j)
		} else {
			r.notifyNotFoundPublishSession(command, userID, remoteUserID)
		}
	} else if command == CmdCandidate {
		remoteUserID := GetValue(j, LabelSrcSessID)
		if remoteUserID != "" {
			userID = remoteUserID
		}

		if _, ok := r.sessions[userID]; ok {
			sess := r.sessions[userID]
			sess.HandleMessage(j)
		} else {
			fmt.Printf("cmd candidate not found user:%s\n", userID)
		}
	} else if command == CmdRoomCreated {
		userid := GetValue(j, LabelSessID)
		for _, sess := range r.sessions {
			if ok, _, _ := sess.HasOriginPublisher(); ok {
				if userid != sess.userid {
					sess.HandleMessage(j)
				}
			}
		}
	} else if command == CmdFakePublish {
		clusterid := GetValue(j, LabelClusterId)
		podid := GetValue(j, LabelPodId)
		userName := GetValue(j, LabelUserName)
		if _, ok := r.sessions[userID]; ok {
			r.sessionsLock.Lock()
			r.sessions[userID].DoLeave()
			delete(r.sessions, userID)
			r.sessionsLock.Unlock()
		}

		if !r.hasOriginSession() {
			// release room
			go r.parent.ReleaseRoom(r.roomid)
			return
		}

		sess := CreateSession(r, userID, userName, r.conf, r.onSendMessageHandler)
		sess.SetFakeSession(clusterid, podid)
		r.sessionsLock.Lock()
		r.sessions[userID] = sess
		r.sessionsLock.Unlock()

		sess.HandleMessage(j)
	} else if command == CmdFakeUnpublish {
		//cross-cluster cancel publish
		podid := GetValue(j, LabelPodId)
		r.sessionsLock.Lock()
		if value, ok := r.sessions[userID]; ok {
			if value.podid == podid {
				value.DoLeave()
				delete(r.sessions, userID)
			} else {
				fmt.Printf("cmd %s not process, fakesession's podid:%s\n", command, value.podid)
			}
		}
		r.sessionsLock.Unlock()

	} else if command == CmdFakeSubscribe {
		remoteUserID := GetValue(j, LabelSrcSessID)
		if _, ok := r.sessions[remoteUserID]; ok {
			sess := r.sessions[remoteUserID]
			sess.HandleMessage(j)
		} else {
			fmt.Printf("cmd %s not found source user: %s\n", command, remoteUserID)
		}
	} else if command == CmdFakeOffer ||
		command == CmdFakeAnswer ||
		command == CmdFakeCandidate {
		if _, ok := r.sessions[userID]; ok {
			sess := r.sessions[userID]
			sess.HandleMessage(j)
		} else {
			fmt.Printf("receive %s, not login yet\n", command)
		}
	} else {
		fmt.Printf("in room, unsupport msg type:%s\n", command)
	}
}

func (r *room) OnPublisherReady(sid, name, pid string) {
	for _, value := range r.sessions {
		if !value.origin {
			continue
		}
		msg, err := json.Marshal(map[string]interface{}{
			LabelCmd:      CmdPub,
			LabelRoomId:   r.roomid,
			LabelSessID:   sid,
			LabelUserName: name,
			LabelPeerID:   pid,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		r.onSendMessageHandler(value.userid, string(msg))
	}
}

func (r *room) OnPublisherPeerDisconnected(sid, pid string) {
}

func (r *room) OnPublisherPeerFailed(sid, pid string) {
	if _, ok := r.sessions[sid]; ok {
		sess := r.sessions[sid]
		sess.ReleasePublisher(sid, pid)
	}
}

func (r *room) OnPublisherPeerClosed(sid, pid string) {
	if _, ok := r.sessions[sid]; ok {
		sess := r.sessions[sid]
		sess.ReleasePublisher(sid, pid)
	}
}

func (r *room) OnSubscriberPeerDisconnected(sid, ssid, pid string) {

}

func (r *room) OnSubscriberPeerFailed(sid, ssid, pid string) {
	if _, ok := r.sessions[ssid]; ok {
		sess := r.sessions[ssid]
		sess.ReleaseSubscriberPeer(sid, ssid, pid)
	}
}

func (r *room) OnSubscriberPeerClosed(sid, ssid, pid string) {
	if _, ok := r.sessions[ssid]; ok {
		sess := r.sessions[ssid]
		sess.ReleaseSubscriberPeer(sid, ssid, pid)
	}
}

func (r *room) OnCheckKeepalive() {
	r.sessionsLock.RLock()
	for _, sess := range r.sessions {
		go sess.CheckKeepAlive()
	}
	r.sessionsLock.RUnlock()
}

func (r *room) GetUsedResourceInformation() string {
	return "{['roomid': '111','rs': 73],['roomid': '222','rs': 73],['roomid': '333','rs': 73]}"
}

func (r *room) KickoutFromRoom(sid string) {
	r.sessionsLock.Lock()
	if _, ok := r.sessions[sid]; ok {
		sess := r.sessions[sid]
		isOrigin := sess.DoLeave()
		delete(r.sessions, sess.userid)
		if isOrigin {
			r.notifyLeaveRoom(sid)
		}
		fmt.Printf("client %s heartbeat timeout, kickout now\n", sid)
	}
	r.sessionsLock.Unlock()

	r.sessionsLock.RLock()
	for _, sess := range r.sessions {
		go sess.HandleSubscriberLeaved(sid)
	}
	r.sessionsLock.RUnlock()

	if !r.hasOriginSession() {
		// release room
		go r.parent.ReleaseRoom(r.roomid)
	}
}

func (r *room) hasOriginSession() bool {
	ret := false
	r.sessionsLock.RLock()
	for _, sess := range r.sessions {
		if sess.origin {
			ret = true
			break
		}
	}
	r.sessionsLock.RUnlock()

	return ret
}

func (r *room) OnReceivedAppData(fromSid, fromPid string, buff []byte, len int) {
	r.sessionsLock.RLock()
	for _, s := range r.sessions {
		s.BroadcastAppData(fromSid, fromPid, buff, len)
	}
	r.sessionsLock.RUnlock()
}

func (r *room) notifyNotLogin(command, sid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelCode:   CodeNotLogin,
		LabelDesc:   DescNotLogin,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	r.onSendMessageHandler(sid, string(msg))
}

func (r *room) notifyNotFoundPublishSession(command, sid, ssid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelCode:   CodeNotFoundPublishUser,
		LabelDesc:   DescNotFoundPublishUser + ": " + ssid,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	r.onSendMessageHandler(sid, string(msg))
}

func (r *room) notifyLeaveRoom(sid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdPodLeaveRoom,
		LabelClusterId: r.conf.Clusterid,
		LabelPodId:     r.conf.Uuid,
		LabelRoomId:    r.roomid,
		LabelSessID:    sid,
		LabelSN:        r.conf.GetSn(),
		LabelTimestamp: time.Now().Unix(),
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	PublishMessage2LB(string(msg))
}

func (r *room) notifyStopPush(origin bool, sid, pid string) {
	for _, value := range r.sessions {
		if value.origin {
			msg, err := json.Marshal(map[string]interface{}{
				LabelCmd:    CmdUnpub,
				LabelRoomId: r.roomid,
				LabelSessID: sid,
				LabelPeerID: pid,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			r.onSendMessageHandler(value.userid, string(msg))
		}
	}

	if origin {
		msg, err := json.Marshal(map[string]interface{}{
			LabelCmd:       CmdFakeUnpublish,
			LabelClusterId: r.conf.Clusterid,
			LabelPodId:     r.conf.Uuid,
			LabelRoomId:    r.roomid,
			LabelSessID:    sid,
			LabelPeerID:    pid,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		BroadcastCrossClusterMessage(string(msg))
	}
}
