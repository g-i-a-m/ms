// +build !js

package main

import (
	"github.com/pion/webrtc/v3/examples/media-server/mqttclient"
)

func main() {
	roommgr := CreateRoomMgr()
	mqtt := mqttclient.CreateMqtt()
	mqtt.OnReceivedMessage(roommgr.HandleMessage)
	mqtt.Init()
	roommgr.SetSendMessageHandler(func(topic, msg string) {
		go mqtt.Publish(topic, msg)
	})

	// Block forever
	select {}
}
