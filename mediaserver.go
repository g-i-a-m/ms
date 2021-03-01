// +build !js

package main

import (
	"fmt"
)

func main() {
	roommgr := CreateRoomMgr()
	mqtt := CreateMqtt()
	mqtt.OnReceivedMessage(roommgr.HandleMessage)
	mqtt.Init()
	roommgr.SetSendMessageHandler(func(topic, msg string) {
		fmt.Printf("send to %s %s\n", topic, msg)
		mqtt.Publish(topic, msg)
	})

	// Block forever
	select {}
}
