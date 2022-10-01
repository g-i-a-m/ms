//go:build !js

package main

import (
	"fmt"
)

func main() {
	c, err := GetConfig()
	if err != nil {
		return
	}

	roommgr := CreateRoomMgr()
	rmq := CreateRabbitMQ(c.Rmqurl)
	rmq.InterClusterStartup(roommgr.HandleMessage)
	defer rmq.Stop()

	crmq := CreateRabbitMQ(c.CenterRmqurl)
	crmq.CrossClusterStartup(roommgr.HandleMessage)
	defer crmq.CenterRMQStop()

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
