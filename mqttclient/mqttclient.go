// +build !js

package mqttclient

import (
	"fmt"
	"sync"

	"golang.org/x/net/websocket"
)

type mqttonnection struct {
	ws                       *websocket.Conn
	mutex                    sync.RWMutex
	onReceivedMessageHandler func(string)
	onConnectStatusHandler   func(string)
}

var instance *mqttonnection
var once sync.Once

func GetInstance() *mqttonnection {
	once.Do(func() {
		instance = &mqttonnection{
			ws:                       nil,
			onReceivedMessageHandler: nil,
			onConnectStatusHandler:   nil,
		}
	})
	return instance
}

// CreateMqtt is create a room manager object
// func CreateMqtt() *mqttonnection {
// 	return &mqttonnection{
// 		ws: nil,
// 	}
// }

// Connect
func (mqtt *mqttonnection) Connect() error {
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
			mqtt.mutex.RLock()
			handler := mqtt.onReceivedMessageHandler
			mqtt.mutex.RUnlock()
			if handler != nil {
				go handler(string(reply))
			}
		}
	}()
	mqtt.mutex.RLock()
	handler := mqtt.onConnectStatusHandler
	mqtt.mutex.RUnlock()
	if handler != nil {
		go handler(string("Service is already running" +
			" and currently does not support cross-network segment"))
	}
	return nil
}

// OnReceivedMessage
func (mqtt *mqttonnection) OnStatusChange(f func(string)) {
	mqtt.mutex.Lock()
	defer mqtt.mutex.Unlock()
	mqtt.onConnectStatusHandler = f
}

// OnReceivedMessage
func (mqtt *mqttonnection) OnReceivedMessage(f func(string)) {
	mqtt.mutex.Lock()
	defer mqtt.mutex.Unlock()
	mqtt.onReceivedMessageHandler = f
}

// ReplyMessage
func (mqtt *mqttonnection) ReplyMessage(msg string) {
	var err error
	if err = websocket.Message.Send(mqtt.ws, msg); err != nil {
		fmt.Println("send failed:", err)
		return
	}
	fmt.Println("send message: ", msg)
}
