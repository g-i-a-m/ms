//go:build !js
// +build !js

package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mqttonnection struct {
	client                   mqtt.Client
	mutex                    sync.RWMutex
	onReceivedMessageHandler func([]byte)
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("mqtt connected")
	conf, err := GetConfig()
	if err != nil {
		return
	}
	if token := client.Subscribe(conf.MqttTopic, 2, nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	fmt.Printf("mqtt subscribe %s\n", conf.MqttTopic)
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Println("mqtt connection lost: ", err)
}

// CreateMqtt is
func CreateMqtt() *mqttonnection {
	return &mqttonnection{}
}

func (conn *mqttonnection) Init() {
	mqtt.ERROR = log.New(os.Stdout, "[ERROR] ", 0)
	mqtt.CRITICAL = log.New(os.Stdout, "[CRIT] ", 0)
	mqtt.WARN = log.New(os.Stdout, "[WARN]  ", 0)
	//mqtt.DEBUG = log.New(os.Stdout, "[DEBUG] ", 0)
	config, err := GetConfig()
	if err != nil {
		fmt.Println("mqtt init failed")
		return
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Mqttip, config.Mqttport))
	opts.SetClientID(config.Uuid)
	opts.SetUsername(config.Mqttuser)
	opts.SetPassword(config.Mqttpwd)

	opts.SetConnectRetryInterval(2 * time.Second)
	opts.SetConnectRetry(true)
	opts.SetMaxReconnectInterval(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	opts.SetCleanSession(true)

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		fmt.Printf("Received message from: %s %s\n", msg.Topic(), msg.Payload())
		conn.onReceivedMessageHandler(msg.Payload())
	})
	opts.SetOnConnectHandler(connectHandler)
	opts.SetConnectionLostHandler(connectLostHandler)
	conn.client = mqtt.NewClient(opts)
	if token := conn.client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

}

// OnReceivedMessage
func (conn *mqttonnection) OnReceivedMessage(f func([]byte)) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	conn.onReceivedMessageHandler = f
}

// Publish
func (conn *mqttonnection) Publish(topic string, msg string) {
	token := conn.client.Publish(topic, 0, false, msg)
	token.Wait()
}
