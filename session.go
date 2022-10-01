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
	conf                 *Config
	userid               string
	clusterid            string
	podid                string
	origin               bool
	publishers           map[string]*peer
	subscribers          map[string]*peer
	onSendMessageHandler func(topic, msg string)
	exitChan             chan int
	waitGroup            *sync.WaitGroup
	parent               *room
	alivetime            int64
}

// CreateSession is create a session object
func CreateSession(r *room, id string, c *Config, f func(topic, msg string)) *session {
	return &session{
		conf:                 c,
		userid:               id,
		origin:               true,
		publishers:           make(map[string]*peer),
		subscribers:          make(map[string]*peer),
		onSendMessageHandler: f,
		exitChan:             make(chan int),
		waitGroup:            &sync.WaitGroup{},
		parent:               r,
		alivetime:            time.Now().Unix(),
	}
}

func (sess *session) SetFakeSession(clusterid, podid string) {
	sess.origin = false
	sess.clusterid = clusterid
	sess.podid = podid
}

func (sess *session) HandleMessage(j jsonparser) {
	command := GetValue(j, LabelCmd)
	userID := GetValue(j, LabelSessID)
	remoteUserID := GetValue(j, LabelSrcSessID)
	peerid := GetValue(j, LabelPeerID)

	if command == CmdHeartbeat {
		sess.alivetime = time.Now().Unix()
	} else if command == CmdPush {
		if value, ok := sess.publishers[peerid]; ok {
			sess.pubsmux.Lock()
			if value.ShouldNotify() {
				sess.parent.notifyStopPush(sess.origin, sess.userid, peerid)
			}
			value.Destroy()
			delete(sess.publishers, peerid)
			sess.pubsmux.Unlock()
		}
		peer, err := CreatePublishPeer(sess, userID, peerid, "", sess.conf, false)
		if err != nil {
			fmt.Println("publish to create peer failed", err)
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.pubsmux.Lock()
		sess.publishers[peerid] = peer
		sess.pubsmux.Unlock()

		msg, err := json.Marshal(map[string]interface{}{
			LabelCmd:    CmdPush,
			LabelSessID: userID,
			LabelPeerID: peerid,
			LabelCode:   0,
		})
		if err != nil {
			fmt.Println("generate json error:", err)
		}
		sess.onSendMessageHandler(userID, string(msg))
	} else if command == CmdStopPush {
		if value, ok := sess.publishers[peerid]; ok {
			sess.pubsmux.Lock()
			if value.ShouldNotify() {
				sess.parent.notifyStopPush(sess.origin, sess.userid, peerid)
			}
			value.Destroy()
			delete(sess.publishers, peerid)
			sess.pubsmux.RUnlock()

			msg, err := json.Marshal(map[string]interface{}{
				LabelCmd:    CmdStopPush,
				LabelSessID: userID,
				LabelPeerID: peerid,
				LabelCode:   0,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}
			sess.onSendMessageHandler(userID, string(msg))
			sess.notifyReleasePublish(peerid)
		} else {
			fmt.Printf("%s:%s is not exist, can not stop push stream\n", userID, peerid)
		}
	} else if command == CmdSub {
		if _, ok := sess.publishers[peerid]; !ok {
			sess.notifyNotFoundPublishPeer(command, userID, peerid)
			return
		}

		sess.pubsmux.RLock()
		sdp := sess.publishers[peerid].peerConnection.CurrentLocalDescription()
		acodec := sess.publishers[peerid].GetAudioCodec()
		vcodec := sess.publishers[peerid].GetVideoCodec()
		sess.pubsmux.RUnlock()

		if _, ok := sess.subscribers[userID+remoteUserID+peerid]; ok {
			sess.subsmux.Lock()
			sess.subscribers[userID+remoteUserID+peerid].Destroy()
			delete(sess.subscribers, userID+remoteUserID+peerid)
			sess.subsmux.Unlock()
		}

		peer, err := CreateSubscribePeer(sess, userID, remoteUserID, peerid, "", sess.conf, sdp, acodec, vcodec, false)
		if err != nil {
			sess.notifyCreatePeerFailed(command, userID, peerid)
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.subsmux.Lock()
		sess.subscribers[userID+remoteUserID+peerid] = peer
		sess.subsmux.Unlock()
		peer.HandleMessage(j)
	} else if command == CmdStopSub {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[userID+remoteUserID+peerid]; ok {
			go sess.ReleaseSubscriberPeer(userID, remoteUserID, peerid)
		}
		sess.subsmux.RUnlock()
	} else if command == CmdOffer {
		sess.subsmux.RLock()
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
		} else {
			sess.notifyNotFoundPublishPeer(command, userID, peerid)
		}
		sess.subsmux.RUnlock()
	} else if command == CmdAnswer {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[userID+remoteUserID+peerid]; ok {
			peer := sess.subscribers[userID+remoteUserID+peerid]
			peer.HandleMessage(j)
		} else {
			sess.notifyNotFoundSubscribePeer(command, userID, peerid)
		}
		sess.subsmux.RUnlock()
	} else if command == CmdCandidate {
		/* sess.pubsmux.RLock()
		if _, ok := sess.publishers[peerid]; ok {
			peer := sess.publishers[peerid]
			peer.HandleMessage(j)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock() */

		/* sess.subsmux.RLock()
		if _, ok := sess.subscribers[userID+remoteUserID+peerid]; ok {
			peer := sess.subscribers[userID+remoteUserID+peerid]
			peer.HandleMessage(j)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()
		*/
	} else if command == CmdRoomCreated {
		clusterid := GetValue(j, LabelClusterId)
		podid := GetValue(j, LabelPodId)
		for _, peer := range sess.publishers {
			msg, err := json.Marshal(map[string]interface{}{
				LabelCmd:       CmdFakePublish,
				LabelClusterId: sess.conf.Clusterid,
				LabelPodId:     sess.conf.Uuid,
				LabelRoomId:    sess.parent.roomid,
				LabelSessID:    peer.userid,
				LabelPeerID:    peer.peerid,
			})
			if err != nil {
				fmt.Println("generate json error:", err)
			}

			if sess.clusterid == clusterid {
				PublishMessage2Pod(podid, string(msg))
			} else {
				PublishMessage2CrossClusterPod(podid, string(msg))
			}
		}
	} else if command == CmdFakePublish {
		// TODO: repeated publish
		sess.FakeSubscribe(userID, peerid, CLUSTER_RECONNECTION_MAXTIMES)
	} else if command == CmdFakeSubscribe {
		podid := GetValue(j, LabelPodId)
		if _, ok := sess.publishers[peerid]; !ok {
			fmt.Println("not found the publisher when subscribe")
			return
		}

		sess.pubsmux.RLock()
		sdp := sess.publishers[peerid].peerConnection.CurrentLocalDescription()
		acodec := sess.publishers[peerid].GetAudioCodec()
		vcodec := sess.publishers[peerid].GetVideoCodec()
		sess.pubsmux.RUnlock()

		if _, ok := sess.subscribers[podid+remoteUserID+peerid]; ok {
			sess.subsmux.Lock()
			sess.subscribers[podid+remoteUserID+peerid].Destroy()
			delete(sess.subscribers, podid+remoteUserID+peerid)
			sess.subsmux.Unlock()
		}

		peer, err := CreateSubscribePeer(sess, podid, remoteUserID, peerid, podid, sess.conf, sdp, acodec, vcodec, true)
		if err != nil {
			fmt.Println("sub to create peer failed")
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.subsmux.Lock()
		sess.subscribers[podid+remoteUserID+peerid] = peer
		sess.subsmux.Unlock()
		peer.HandleMessage(j)
	} else if command == CmdFakeOffer {
		podid := GetValue(j, LabelPodId)
		if value, ok := sess.publishers[peerid]; ok {
			sess.pubsmux.Lock()
			if value.ShouldNotify() {
				sess.parent.notifyStopPush(sess.origin, sess.userid, peerid)
			}
			value.Destroy()
			delete(sess.publishers, peerid)
			sess.pubsmux.Unlock()
		}
		peer, err := CreatePublishPeer(sess, userID, peerid, podid, sess.conf, true)
		if err != nil {
			fmt.Println("fakeoffer to create peer failed", err)
			return
		}
		peer.SetSendMessageHandler(sess.onSendMessageHandler)
		sess.pubsmux.Lock()
		sess.publishers[peerid] = peer
		sess.pubsmux.Unlock()

		peer.HandleMessage(j)
	} else if command == CmdFakeAnswer {
		podid := GetValue(j, LabelPodId)
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[podid+remoteUserID+peerid]; ok {
			peer := sess.subscribers[podid+remoteUserID+peerid]
			peer.HandleMessage(j)
		} else {
			fmt.Printf("can't handle %s not subscribe yet\n", command)
		}
		sess.subsmux.RUnlock()
	} else if command == CmdFakeCandidate {
		if !sess.origin {
			/*sess.pubsmux.RLock()
			if _, ok := sess.publishers[peerid]; ok {
				peer := sess.publishers[peerid]
				peer.HandleMessage(j)
				sess.pubsmux.RUnlock()
				return
			}
			sess.pubsmux.RUnlock()*/
		} else {
			podid := GetValue(j, LabelPodId)
			sess.subsmux.RLock()
			if _, ok := sess.subscribers[podid+remoteUserID+peerid]; ok {
				peer := sess.subscribers[podid+remoteUserID+peerid]
				peer.HandleMessage(j)
			} else {
				fmt.Printf("can't handle %s not subscribe yet\n", command)
			}
			sess.subsmux.RUnlock()
		}
	} else {
		fmt.Println("session unsupport msg type:", command)
	}
}

func (sess *session) hasPublisher() (ret bool, sid, pid string) {
	sess.pubsmux.RLock()
	defer sess.pubsmux.RUnlock()
	for _, publisher := range sess.publishers {
		if ret := publisher.IsReady(); ret {
			return true, publisher.userid, publisher.peerid
		}
	}
	return false, "", ""
}

func (sess *session) HasOriginPublisher() (ret bool, sid, pid string) {
	if !sess.origin {
		return false, "", ""
	}
	sess.pubsmux.RLock()
	defer sess.pubsmux.RUnlock()
	for _, publisher := range sess.publishers {
		if ret := publisher.IsReady(); ret {
			return true, publisher.userid, publisher.peerid
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

			if sess.origin {
				msg, err := json.Marshal(map[string]interface{}{
					LabelCmd:       CmdFakePublish,
					LabelClusterId: sess.conf.Clusterid,
					LabelPodId:     sess.conf.Uuid,
					LabelRoomId:    sess.parent.roomid,
					LabelSessID:    sid,
					LabelPeerID:    pid,
				})
				if err != nil {
					fmt.Println("generate json error:", err)
				}
				BroadcastCrossClusterMessage(string(msg))
			}
			return
		}
	}

	if role == 2 {
		if _, ok := sess.publishers[pid]; ok {
			peer := sess.publishers[pid]
			go peer.RequestKeyframe()
		}
	}
}

// 1>. clean all publish peer if exist
// 2>. clean all subscribe peer if exist
// 3>. clean watch other people' publish peer if exist (by return sid, in last callstack)
func (sess *session) DoLeave() bool {
	sess.pubsmux.Lock()
	for _, value := range sess.publishers {
		if value.ShouldNotify() {
			sess.parent.notifyStopPush(sess.origin, sess.userid, value.peerid)
		}
		value.Destroy()
	}
	sess.publishers = make(map[string]*peer)
	sess.pubsmux.Unlock()

	sess.subsmux.Lock()
	for _, value := range sess.subscribers {
		value.Destroy()
	}
	sess.subscribers = make(map[string]*peer)
	sess.subsmux.Unlock()

	go func() { sess.exitChan <- 0 }()
	sess.waitGroup.Wait()

	return sess.origin
}

func (sess *session) OnIceDisconnected(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if value, ok := sess.publishers[pid]; ok {
			if value.ShouldNotify() {
				sess.parent.notifyStopPush(sess.origin, sess.userid, pid)
			}
			//go sess.parent.OnPublisherPeerDisconnected(sid, pid)
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

func (sess *session) OnIceFailed(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[pid]; ok {
			go sess.parent.OnPublisherPeerFailed(sid, pid)
		}
		sess.pubsmux.RUnlock()

		if !sess.origin {
			sess.FakePublisherReConnect(sid, pid, CLUSTER_RECONNECTION_MAXTIMES)
			fmt.Printf("cross-cluster ice failed, start reconnect, %d.\n", CLUSTER_RECONNECTION_MAXTIMES)
		}
		return
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

func (sess *session) OnIceClosed(role int, sid, ssid, pid string) {
	if role == 1 {
		sess.pubsmux.RLock()
		if _, ok := sess.publishers[pid]; ok {
			go sess.parent.OnPublisherPeerClosed(sid, pid)
			sess.pubsmux.RUnlock()
			return
		}
		sess.pubsmux.RUnlock()
	}

	if role == 2 {
		sess.subsmux.RLock()
		if _, ok := sess.subscribers[pid]; ok {
			go sess.parent.OnSubscriberPeerClosed(sid, ssid, pid)
			sess.subsmux.RUnlock()
			return
		}
		sess.subsmux.RUnlock()
	}
}

func (sess *session) FakeSubscribe(sid, pid string, curtime uint32) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdFakeSubscribe,
		LabelClusterId: sess.conf.Clusterid,
		LabelPodId:     sess.conf.Uuid,
		LabelRoomId:    sess.parent.roomid,
		LabelSessID:    sid,
		LabelSrcSessID: sid,
		LabelPeerID:    pid,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	PublishMessage2CrossClusterPod(sess.podid, string(msg))

	//startup a timer for check subscribe result
	sess.waitGroup.Add(1)
	go func() {
		defer sess.waitGroup.Done()
		timer := time.NewTimer(time.Second * CLUSTER_RECONNECTION_INTERVAL)
		for {
			select {
			case <-sess.exitChan:
				return
			case <-timer.C:
				// check cross-cluster subscribe result
				if _, ok := sess.publishers[pid]; ok {
					if sess.publishers[pid].IsReady() {
						// successed, wait for exitChan
						continue
					}
				}

				// try again
				if curtime > 0 {
					curtime -= 1
					go sess.FakePublisherReConnect(sid, pid, curtime)
					fmt.Printf("cross-cluster reconnect again, %d.\n", curtime)
				} else {
					fmt.Printf("cross-cluster exceeded the max number of reconnections.\n")
				}

			}
		}
	}()
}

func (sess *session) FakePublisherReConnect(sid, pid string, curtime uint32) {
	if sess.origin {
		return
	}

	// make sure that the check result goroutine exit
	go func() { sess.exitChan <- 0 }()
	sess.waitGroup.Wait()

	//cross cluster subscribe
	sess.FakeSubscribe(sid, pid, curtime)
}

func (sess *session) ReleasePublisher(sid, pid string) {
	sess.pubsmux.Lock()
	if _, ok := sess.publishers[pid]; ok {
		peer := sess.publishers[pid]
		if peer.ShouldNotify() {
			sess.parent.notifyStopPush(sess.origin, sess.userid, pid)
		}
		peer.Destroy()
		delete(sess.publishers, pid)
	}
	sess.pubsmux.Unlock()

	sess.subsmux.Lock()
	for _, value := range sess.subscribers {
		if value.peerid == pid {
			value.Destroy()
			key := value.userid + value.remoteuserid + value.peerid
			delete(sess.subscribers, key)
		}
	}
	sess.subsmux.Unlock()
	sess.notifyReleasePublish(pid)
}

func (sess *session) ReleaseSubscriberPeer(sid, ssid, pid string) {
	sess.subsmux.Lock()
	if _, ok := sess.subscribers[sid+ssid+pid]; ok {
		peer := sess.subscribers[sid+ssid+pid]
		peer.Destroy()
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
		if _, ok := sess.subscribers[sid+sess.userid+it.Value.(string)]; ok {
			peer := sess.subscribers[sid+sess.userid+it.Value.(string)]
			peer.Destroy()
			delete(sess.subscribers, sid+sess.userid+it.Value.(string))
		}
	}
	sess.subsmux.Unlock()
}

func (sess *session) RequestPli(pid string) {
	if _, ok := sess.publishers[pid]; ok {
		peer := sess.publishers[pid]
		go peer.RequestKeyframe()
	}
}

func (sess *session) UpdateAudioCodec(codec webrtc.RTPCodecParameters) {
	sess.subsmux.RLock()
	for _, sub := range sess.subscribers {
		sub.SetAudioCodec(codec)
	}
	sess.subsmux.RUnlock()
}

func (sess *session) UpdateVideoCodec(codec webrtc.RTPCodecParameters) {
	sess.subsmux.RLock()
	for _, sub := range sess.subscribers {
		sub.SetVideoCodec(codec)
	}
	sess.subsmux.RUnlock()
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
		if value.userid != fromSid || value.peerid != fromPid {
			value.deliverAppData(buff, len)
		}
	}
	sess.pubsmux.RUnlock()

	sess.subsmux.RLock()
	for _, value := range sess.subscribers {
		if value.userid != fromSid || value.peerid != fromPid {
			value.deliverAppData(buff, len)
		}
	}
	sess.subsmux.RUnlock()
}

func (sess *session) CheckKeepAlive() {
	if !sess.origin {
		return
	}
	if (time.Now().Unix() - sess.alivetime) > 60 {
		fmt.Printf("user %s heartbeat timeout\n", sess.userid)
		go sess.parent.KickoutFromRoom(sess.userid)
	}
}

func (sess *session) notifyNotFoundPublishPeer(command, sid, peerid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelPeerID: peerid,
		LabelCode:   CodeNotFoundPublishPeer,
		LabelDesc:   DescNotFoundPublishPeer + ": " + sess.userid + ":" + peerid,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	sess.onSendMessageHandler(sid, string(msg))
}

func (sess *session) notifyNotFoundSubscribePeer(command, sid, peerid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelPeerID: peerid,
		LabelCode:   CodeNotFoundSubscribePeer,
		LabelDesc:   DescNotFoundSubscribePeer + ": " + sess.userid + ":" + peerid,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	sess.onSendMessageHandler(sid, string(msg))
}

func (sess *session) notifyCreatePeerFailed(command, sid, peerid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:    command,
		LabelSessID: sid,
		LabelPeerID: peerid,
		LabelCode:   CodeCreatePeerError,
		LabelDesc:   DescCreatePeerError + ": " + sid + "<-" + sess.userid + ":" + peerid,
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	sess.onSendMessageHandler(sid, string(msg))
}

func (sess *session) notifyReleasePublish(pid string) {
	msg, err := json.Marshal(map[string]interface{}{
		LabelCmd:       CmdPodReleasePublish,
		LabelClusterId: sess.conf.Clusterid,
		LabelPodId:     sess.conf.Uuid,
		LabelRoomId:    sess.parent.roomid,
		LabelSessID:    sess.userid,
		LabelPeerID:    pid,
		LabelSN:        sess.conf.GetSn(),
		LabelTimestamp: time.Now().Unix(),
	})
	if err != nil {
		fmt.Println("generate json error:", err)
	}
	PublishMessage2LB(string(msg))
}
