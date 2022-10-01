package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-basic/uuid"
	"gopkg.in/ini.v1"
)

var Gconf *Config

type Config struct {
	Version      int    `ini:"version"`
	Turnip       string `ini:"turn_ip"`
	Turnport     uint32 `ini:"turn_port"`
	Proxyaddr    string
	Proxyport    uint32 `ini:"proxy_port"`
	Localaddr    string
	Mqttip       string `ini:"mqtt_ip"`
	Mqttport     uint32 `ini:"mqtt_port"`
	Mqttuser     string `ini:"mqtt_username"`
	Mqttpwd      string `ini:"mqtt_password"`
	Rmqurl       string `ini:"rabbitmq_url"`
	CenterRmqurl string `ini:"center_rabbitmq_url"`
	Maxpull      uint32 `ini:"max_count_pull_stream"`
	Clusterid    string
	Mqtt_topic   string // for test cross-cluster
	Uuid         string
	ContainerID  string
	sn           uint64
}

var mu sync.Mutex

func GetConfig() (*Config, error) {
	if Gconf != nil {
		return Gconf, nil
	}
	mu.Lock()
	defer mu.Unlock()
	if Gconf != nil {
		return Gconf, nil
	}
	Gconf = initConfig()
	return Gconf, nil
}

func initConfig() *Config {
	iniFile, err := ini.Load("./conf/ms.conf")
	if err != nil {
		return nil
	}

	iniFile.BlockMode = false
	id := uuid.New()
	fmt.Printf("Current podid: %s\n", id)
	containerid := GetContainerID()
	conf := Config{
		Localaddr:   GetLocalAddrV2(),
		Uuid:        id,
		ContainerID: containerid,
		sn:          0,
	}
	if err = iniFile.StrictMapTo(&conf); err != nil {
		return nil
	}
	conf.initFromEnv()
	return &conf
}

func (c *Config) initFromEnv() {
	proxy_ip := os.Getenv("SLB_IP")
	test_clusterid := os.Getenv("TEST_CLUSTER_ID")
	mqtt_topic := os.Getenv("MQTT_TOPIC")
	if len(mqtt_topic) == 0 {
		mqtt_topic = c.Uuid
		fmt.Printf("test mqtt topic: %s\n", c.Uuid)
	}

	c.Proxyaddr = proxy_ip
	c.Clusterid = test_clusterid
	c.Mqtt_topic = mqtt_topic
}

func (c *Config) GetSn() uint64 {
	return atomic.AddUint64(&c.sn, 1)
}

func GetContainerID() string {
	hosts, err := os.Open("/etc/hosts")
	if err != nil {
		return ""
	}
	defer hosts.Close()
	scanner := bufio.NewScanner(hosts)
	for scanner.Scan() {
		line := scanner.Text()
		arr := strings.Fields(line)
		if len(arr) >= 2 && strings.Contains(arr[1], "mediaserver-deployment") {
			return arr[1]
		}
	}
	return ""
}

func GetLocalAddr() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		panic(err)
	}
	for _, addr := range addrs {
		tmp := addr.String()
		ip := strings.Split(tmp, "/")[0]
		if ip != "127.0.0.1" {
			return ip
		}
	}
	return ""
}

func GetLocalAddrV2() string {
	conn, err := net.Dial("udp", "www.baidu.com:80")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	addr := conn.LocalAddr().String()
	ip := strings.Split(addr, ":")[0]
	return ip
}
