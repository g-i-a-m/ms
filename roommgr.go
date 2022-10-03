//go:build !js
// +build !js

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type jsonparser map[string]interface{}

type roommgr struct {
	rooms                map[string]*room
	roomsLock            sync.RWMutex
	regist               bool
	stopRegist           chan int
	onSendMessageHandler func(topic, msg string)
	conf                 *Config
}

// CreateRoomMgr is create a room manager object
func CreateRoomMgr() *roommgr {
	c, err := GetConfig()
	if err != nil {
		return nil
	}
	return &roommgr{
		rooms:      make(map[string]*room),
		regist:     false,
		stopRegist: make(chan int),
		conf:       c,
	}
}

func (rm *roommgr) SetSendMessageHandler(f func(topic, msg string)) {
	rm.onSendMessageHandler = f

	go func() {
		ticker := time.NewTicker(time.Second * 20)
		for range ticker.C {
			rm.roomsLock.RLock()
			for _, r := range rm.rooms {
				r.OnCheckKeepalive()
			}
			rm.roomsLock.RUnlock()
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		for range ticker.C {
			rm.roomsLock.RLock()
			for _, r := range rm.rooms {
				r.GetUsedResourceInformation()
			}
			rm.roomsLock.RUnlock()
		}
	}()

	if len(rm.conf.MqttTopic) == 36 {
		go rm.StartRegistToLB()
	} else {
		rm.regist = true
	}
}

func (rm *roommgr) HandleMessage(msg []byte) {
	m, err := CreateJSONParser(msg)
	if err != nil {
		fmt.Println("json.Unmarshal failed:", err)
	}
	command := GetValue(m, LabelCmd)
	roomid := GetValue(m, LabelRoomId)
	if !rm.regist {
		if command == CmdPodRegist {
			code := GetIntValue(m, LabelCode)
			if code == 0 {
				clusterid := GetValue(m, LabelClusterId)
				if len(clusterid) != 0 {
					rm.conf.Clusterid = clusterid
				} else {
					if rm.conf.Clusterid == "" {
						return
					}
					fmt.Println("*** using test cluster id")
				}
				rm.regist = true
				rm.stopRegist <- 0
			}
		} else if command == CmdPodRegistNow {
			if rm.regist {
				// must be LB restart, startup goroutine to regist
				rm.regist = false
				go rm.StartRegistToLB()
			}
		} else {
			sid := GetValue(m, LabelSessID)
			rm.notifyServiceNotAvailable(command, sid)
		}
		return
	}

	if command == CmdLogin {
		_, ok := rm.rooms[roomid]
		if !ok {
			r := CreateRoom(roomid, rm)
			r.SetSendMessageHandler(rm.onSendMessageHandler)
			rm.roomsLock.Lock()
			rm.rooms[roomid] = r
			rm.roomsLock.Unlock()
			userid := GetValue(m, LabelSessID)
			msg, err := json.Marshal(map[string]interface{}{
				LabelCmd:       CmdRoomCreated,
				LabelClusterId: rm.conf.Clusterid,
				LabelPodId:     rm.conf.Uuid,
				LabelRoomId:    roomid,
				LabelSessID:    userid,
				LabelSN:        rm.conf.GetSn(),
				LabelTimestamp: time.Now().Unix(),
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			BroadcastCrossClusterMessage(string(msg))
		}

		rm.roomsLock.RLock()
		r := rm.rooms[roomid]
		rm.roomsLock.RUnlock()
		r.HandleMessage(m)
	} else if command == CmdHeartbeat ||
		command == CmdLogout ||
		command == CmdPush ||
		command == CmdStopPush ||
		command == CmdSub ||
		command == CmdStopSub ||
		command == CmdOffer ||
		command == CmdAnswer ||
		command == CmdCandidate {
		if _, ok := rm.rooms[roomid]; ok {
			r := rm.rooms[roomid]
			r.HandleMessage(m)
		} else {
			sid := GetValue(m, LabelSessID)
			rm.notifyNotLogin(command, sid)
			fmt.Printf("cmd %s room %s not created yet\n", command, roomid)
		}
	} else if command == CmdPodRegist {
		code := GetIntValue(m, LabelCode)
		if code == 0 {
			clusterid := GetValue(m, LabelClusterId)
			if len(clusterid) != 0 {
				rm.conf.Clusterid = clusterid
			} else {
				if rm.conf.Clusterid == "" {
					return
				}
				fmt.Println("*** using test cluster id")
			}
			rm.regist = true
			rm.stopRegist <- 0
		}
	} else if command == CmdPodRegistNow {
		if rm.regist {
			// must be LB restart, startup goroutine to regist
			rm.regist = false
			go rm.StartRegistToLB()
		}
	} else if command == CmdRoomCreated ||
		command == CmdFakePublish ||
		command == CmdFakeUnpublish {
		podid := GetValue(m, LabelPodId)
		if podid != rm.conf.Uuid {
			if _, ok := rm.rooms[roomid]; ok {
				r := rm.rooms[roomid]
				r.HandleMessage(m)
			}
		}
	} else if command == CmdFakeSubscribe ||
		command == CmdFakeOffer ||
		command == CmdFakeAnswer ||
		command == CmdFakeCandidate {
		if _, ok := rm.rooms[roomid]; ok {
			r := rm.rooms[roomid]
			r.HandleMessage(m)
		}
	} else {
		fmt.Println("roommgr unsupport msg type:", command)
	}
}

func (rm *roommgr) StartRegistToLB() {
	ticker := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-rm.stopRegist:
			return
		case <-ticker.C:
			if rm.regist {
				return
			}
			msg, err := json.Marshal(map[string]interface{}{
				LabelCmd:       CmdPodRegist,
				LabelClusterId: rm.conf.Clusterid,
				LabelPodId:     rm.conf.Uuid,
				LabelResource:  rm.conf.Maxpull,
				LabelSN:        rm.conf.GetSn(),
				LabelTimestamp: time.Now().Unix(),
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			PublishMessage2LB(string(msg))
		}
	}
}

func (rm *roommgr) ReleaseRoom(rid string) {
	rm.roomsLock.Lock()
	defer rm.roomsLock.Unlock()
	if _, ok := rm.rooms[rid]; ok {
		rm.rooms[rid].DestroyRoom()
		delete(rm.rooms, rid)
	}
	fmt.Printf("room %s released\n", rid)
}

func (rm *roommgr) notifyServiceNotAvailable(cmd, sid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    cmd,
		LabelSessID: sid,
		LabelCode:   CodeNotAvailable,
		LabelDesc:   DescNotAvailable,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	rm.onSendMessageHandler(sid, string(msg))
}

func (rm *roommgr) notifyNotLogin(command, sid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelCode:   CodeNotLogin,
		LabelDesc:   DescNotLogin,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	rm.onSendMessageHandler(sid, string(msg))
}
