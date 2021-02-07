// +build !js

package main

import (
	"fmt"

	"github.com/pion/webrtc/v3/examples/media-server/mqttclient"
)

func main() {
	roommgr := CreateRoomMgr()
	// mqtt := mqttclient.GetInstance()
	mqttclient.GetInstance().OnStatusChange(func(s string) {
		fmt.Printf("mqtt status: %s\n", s)
	})
	mqttclient.GetInstance().OnReceivedMessage(func(s string) {
		roommgr.HandleMessage(s)
	})
	if err := mqttclient.GetInstance().Connect(); err != nil {
		panic(err)
	}
}
