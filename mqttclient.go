// +build !js

package main

import (
	"fmt"
	"sync"

	"golang.org/x/net/websocket"
)

type mqttonnection struct {
	ws                       *websocket.Conn
	mu                       sync.RWMutex
	onReceivedMessageHandler func(string)
	onConnectStatusHandler   func(string)
}

// CreateMqtt is create a room manager object
func CreateMqtt() *mqttonnection {
	return &mqttonnection{
		ws: nil,
	}
}

// connect (This note is for shit rules)
func (mqtt *mqttonnection) connect() error {
	var wsurl = "wss://node.offcncloud.com:8010"
	var origin = "http://api.huobi.pro/"
	var err error
	mqtt.ws, err = websocket.Dial(wsurl, "", origin)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			var reply []byte
			if err := websocket.Message.Receive(mqtt.ws, &reply); err != nil {
				fmt.Println("error receive failed: ", err)
			}
			fmt.Println("reveived from client: ", reply)
			if mqtt.onReceivedMessageHandler != nil {
				go mqtt.onReceivedMessageHandler(string(reply))
			}
		}
	}()
	if mqtt.onReceivedMessageHandler != nil {
		go mqtt.onReceivedMessageHandler(string("Service is already running" +
			" and currently does not support cross-network segment"))
	}
	return nil
}

// OnReceivedMessage (This note is for shit rules)
func (mqtt *mqttonnection) OnStatusChange(f func(string)) {
	mqtt.mu.Lock()
	defer mqtt.mu.Unlock()
	mqtt.onConnectStatusHandler = f
}

// OnReceivedMessage (This note is for shit rules)
func (mqtt *mqttonnection) OnReceivedMessage(f func(string)) {
	mqtt.mu.Lock()
	defer mqtt.mu.Unlock()
	mqtt.onReceivedMessageHandler = f
}

// ResponseMessage (This note is for shit rules)
func (mqtt *mqttonnection) ReplyMessage(msg string) {
	var err error
	if err = websocket.Message.Send(mqtt.ws, msg); err != nil {
		fmt.Println("send failed:", err)
		return
	}
	fmt.Println("send message: ", msg)
}
