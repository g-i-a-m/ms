package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
)

type Producer interface {
	MsgContent() string
}

type Receiver interface {
	Consumer([]byte) error
}

type PublishMsg struct {
	excname string
	key     string
	msg     string
}

type QueueExchange struct {
	parent        *RabbitMQ
	conn          *amqp.Connection
	channel       *amqp.Channel
	queueName     string
	routingKey    string
	exchangeName  string
	exchangeType  string
	feature       string
	msgChan       chan PublishMsg
	exitChan      chan int
	waitGroup     *sync.WaitGroup
	notifyClose   chan *amqp.Error
	notifyConfirm chan amqp.Confirmation
}

var exc_pub_dr *QueueExchange
var exc_proxy_bc *QueueExchange
var exc_cluster_bc *QueueExchange
var exc_cross_cluster_dr *QueueExchange
var exc_cross_cluster_bc *QueueExchange
var connected bool
var center_connected bool

type RabbitMQ struct {
	exitChan                 chan int
	waitGroup                *sync.WaitGroup
	conf                     *Config
	conn                     *amqp.Connection
	url                      string
	mu                       sync.RWMutex
	exs                      []*QueueExchange
	onReceivedMessageHandler func([]byte)
}

func CreateRabbitMQ(url string) *RabbitMQ {
	config, err := GetConfig()
	if err != nil {
		fmt.Print("Media server startup failed, read configure:", err)
		return nil
	}

	return &RabbitMQ{
		exitChan:                 make(chan int),
		waitGroup:                &sync.WaitGroup{},
		conf:                     config,
		conn:                     nil,
		url:                      url,
		onReceivedMessageHandler: nil,
	}
}

func (r *RabbitMQ) CreateQueueExchange(conn *amqp.Connection, exc, excType, queue, key, features string) *QueueExchange {
	return &QueueExchange{
		parent:       nil,
		conn:         conn,
		channel:      nil,
		queueName:    queue,
		routingKey:   key,
		exchangeName: exc,
		exchangeType: excType,
		feature:      features,
		msgChan:      nil,
		exitChan:     make(chan int),
		waitGroup:    &sync.WaitGroup{},
	}
}

func (r *RabbitMQ) InterClusterStartup(f func([]byte)) {
	exc_pub_dr = nil
	exc_proxy_bc = nil
	exc_cluster_bc = nil
	connected = false
	r.onReceivedMessageHandler = f
	r.lanuchRetryTimer(0)
}

func (r *RabbitMQ) CrossClusterStartup(f func([]byte)) {
	exc_cross_cluster_dr = nil
	exc_cross_cluster_bc = nil
	center_connected = false
	r.onReceivedMessageHandler = f
	r.CenterRMQConnectTimer(0)
}

func (r *RabbitMQ) mqConnect() bool {
	var err error
	RabbitUrl := fmt.Sprintf("amqp://%s/", r.url)
	r.conn, err = amqp.DialConfig(RabbitUrl, amqp.Config{Vhost: "wb"})
	if err != nil {
		fmt.Printf("rabbitmq connect failed:%s \n", err)
		r.cancelRetryTimer()

		// try again until connect success
		r.lanuchRetryTimer(5)
		return false
	}
	r.cancelRetryTimer()
	fmt.Println("rabbitmq connect successed")

	go func() {
		errChan := make(chan *amqp.Error)
		e := <-r.conn.NotifyClose(errChan)
		if e != nil {
			fmt.Printf("rabbitmq got error: %s\n", e.Reason)
			connected = false
			r.Stop()
			r.lanuchRetryTimer(3)
		}
	}()

	go func() {
		blkChan := make(chan amqp.Blocking)
		e := <-r.conn.NotifyBlocked(blkChan)

		fmt.Printf("rabbitmq blocked: %t %s\n", e.Active, e.Reason)
	}()

	// regist to LB
	exc_pub_dr = r.RegisterProducer("WB_", "direct", "", "111", "d")
	// recv LB reboot event
	r.RegisterReceiver("WB_broadcast", "fanout", "bc_ms_"+r.conf.Uuid, "container_broadcast", "d")
	// recv client's request that relayed by LB
	r.RegisterReceiver("WB_", "direct", "direct_ms_"+r.conf.Uuid, r.conf.Uuid, "d")

	// publish iceinfo
	exc_proxy_bc = r.RegisterProducer("udpProxy", "fanout", "", "broadcast", "ad")

	// recv & send inter cluster broadcast message
	exc_cluster_bc = r.RegisterProducer("inter-cluster_broadcast", "fanout", "", "inter-cluster", "ad")
	r.RegisterReceiver("inter-cluster_broadcast", "fanout", "inter-cluster_"+r.conf.Uuid, "inter-cluster", "ad")

	connected = true
	return true
}

func (r *RabbitMQ) centerRMQConnect() bool {
	var err error
	RabbitUrl := fmt.Sprintf("amqp://%s/", r.url)
	r.conn, err = amqp.DialConfig(RabbitUrl, amqp.Config{Vhost: "wb"})
	if err != nil {
		fmt.Printf("connect to center rabbitmq failed:%s \n", err)
		r.cancelRetryTimer()

		// try again until connect success
		r.CenterRMQConnectTimer(5)
		return false
	}
	r.cancelRetryTimer()
	fmt.Println("connect to center rabbitmq successed")

	go func() {
		errChan := make(chan *amqp.Error)
		e := <-r.conn.NotifyClose(errChan)
		if e != nil {
			fmt.Printf("center rabbitmq got error: %s\n", e.Reason)
			center_connected = false
			r.CenterRMQStop()
			r.CenterRMQConnectTimer(3)
		}
	}()

	go func() {
		blkChan := make(chan amqp.Blocking)
		e := <-r.conn.NotifyBlocked(blkChan)

		fmt.Printf(" center rabbitmq blocked: %t %s\n", e.Active, e.Reason)
	}()

	exc_cross_cluster_dr = r.RegisterProducer("WB_", "direct", "", "", "d")
	r.RegisterReceiver("WB_", "direct", "direct_ms_"+r.conf.Uuid, r.conf.Uuid, "d")

	// recv & send cross cluster broadcast message
	exc_cross_cluster_bc = r.RegisterProducer("cross-cluster_broadcast", "fanout", "", "cross-cluster", "ad")
	r.RegisterReceiver("cross-cluster_broadcast", "fanout", "cross-cluster_"+r.conf.Uuid, "cross-cluster", "ad")

	center_connected = true
	return true
}

func (r *RabbitMQ) lanuchRetryTimer(sec int64) {
	r.waitGroup.Add(1)
	go func() {
		defer r.waitGroup.Done()
		timer := time.NewTimer(time.Duration(sec) * time.Second)
		for {
			select {
			case <-r.exitChan:
				return
			case <-timer.C:
				go r.mqConnect()
			}
		}
	}()
}

func (r *RabbitMQ) CenterRMQConnectTimer(sec int64) {
	r.waitGroup.Add(1)
	go func() {
		defer r.waitGroup.Done()
		timer := time.NewTimer(time.Duration(sec) * time.Second)
		for {
			select {
			case <-r.exitChan:
				return
			case <-timer.C:
				go r.centerRMQConnect()
			}
		}
	}()
}

// if the timer has not triggered, cancel it. otherwise return directly.
func (r *RabbitMQ) cancelRetryTimer() {
	go func() {
		r.exitChan <- 0
	}()
	r.waitGroup.Wait()
}

func (r *RabbitMQ) Stop() {
	if exc_pub_dr != nil {
		exc_pub_dr.destroyEXC()
	}
	if exc_proxy_bc != nil {
		exc_proxy_bc.destroyEXC()
	}
	if exc_cluster_bc != nil {
		exc_cluster_bc.destroyEXC()
	}

	for _, ex := range r.exs {
		ex.destroyEXC()
	}
	r.exs = r.exs[0:0]

	err := r.conn.Close()
	if err != nil {
		fmt.Printf("rabbitmq close connection failed:%s \n", err)
	}
	r.conn = nil
}

func (r *RabbitMQ) CenterRMQStop() {
	if exc_cross_cluster_dr != nil {
		exc_cross_cluster_dr.destroyEXC()
	}
	if exc_cross_cluster_bc != nil {
		exc_cross_cluster_bc.destroyEXC()
	}

	for _, ex := range r.exs {
		ex.destroyEXC()
	}
	r.exs = r.exs[0:0]

	err := r.conn.Close()
	if err != nil {
		fmt.Printf("center rabbitmq close connection failed:%s \n", err)
	}
	r.conn = nil
}

func (ex *QueueExchange) destroyEXC() {
	if ex.channel != nil {
		ex.channel.Close()
		ex.channel = nil
	}
	ex.exitChan <- 0
	ex.waitGroup.Wait()
}

func (r *RabbitMQ) RegisterProducer(exc, excType, queue, key, feature string) *QueueExchange {
	r.mu.Lock()
	ex := r.CreateQueueExchange(r.conn, exc, excType, queue, key, feature)
	ex.addProducer()
	r.mu.Unlock()

	return ex
}

func (ex *QueueExchange) addProducer() {
	var err error
	ex.channel, err = ex.conn.Channel()
	if err != nil {
		fmt.Printf("rabbitmq create channel failed:%s \n", err)
	}

	go func() {
		select {
		case err := <-ex.notifyClose:
			fmt.Print(err)
		case c := <-ex.notifyConfirm:
			fmt.Print(c)
		}
	}()

	// name:交换机名称,kind:交换机类型,durable:是否持久化,队列存盘,true服务重启后信息不会丢失,影响性能;autoDelete:是否自动删除;
	// noWait:是否非阻塞, true为是,不等待RMQ返回信息;args:参数,传nil即可; internal:是否为内部
	var opts = [4]bool{}
	if ex.feature == "d" {
		opts[0] = true
		opts[1] = false
	}
	if ex.feature == "ad" {
		opts[0] = false
		opts[1] = true
	}

	err = ex.channel.ExchangeDeclare(ex.exchangeName, ex.exchangeType, opts[0], opts[1], false, true, nil)
	if err != nil {
		fmt.Printf("rabbitmq declare exchange failed:%s \n", err)
		return
	}

	if len(ex.queueName) != 0 {
		_, err = ex.channel.QueueDeclare(ex.queueName, false, true, false, true, nil)
		if err != nil {
			fmt.Printf("rabbitmq declare queue failed:%s \n", err)
			return
		}

		err = ex.channel.QueueBind(ex.queueName, ex.routingKey, ex.exchangeName, true, nil)
		if err != nil {
			fmt.Printf("rabbitmq bind queue failed:%s \n", err)
			return
		}
	}

	ex.msgChan = make(chan PublishMsg)
	ex.waitGroup.Add(1)
	go func() {
		defer ex.waitGroup.Done()
		for {
			select {
			case <-ex.exitChan:
				return
			case msg := <-ex.msgChan:
				err = ex.channel.Publish(msg.excname, msg.key, false, false, amqp.Publishing{
					ContentType: "text/plain",
					Body:        []byte(msg.msg),
				})
				if err != nil {
					fmt.Printf("rabbitmq publish message failed:%s \n", err)
				}
			}
		}
	}()
}

func (r *RabbitMQ) RegisterReceiver(exc, excType, queue, key, feature string) {
	r.mu.Lock()
	ex := r.CreateQueueExchange(r.conn, exc, excType, queue, key, feature)
	ex.addReceiver(r)
	r.exs = append(r.exs, ex)
	r.mu.Unlock()
}

func (ex *QueueExchange) addReceiver(r *RabbitMQ) {
	var err error
	ex.parent = r
	ex.channel, err = ex.conn.Channel()
	if err != nil {
		fmt.Printf("rabbitmq create channel failed:%s \n", err)
	}

	var opts = [4]bool{}
	if ex.feature == "d" {
		opts[0] = true
		opts[1] = false
	}
	if ex.feature == "ad" {
		opts[0] = false
		opts[1] = true
	}

	err = ex.channel.ExchangeDeclare(ex.exchangeName, ex.exchangeType, opts[0], opts[1], false, true, nil)
	if err != nil {
		fmt.Printf("rabbitmq declare exchange failed:%s \n", err)
		return
	}

	_, err = ex.channel.QueueDeclare(ex.queueName, false, true, false, true, nil)
	if err != nil {
		fmt.Printf("rabbitmq declare queue failed:%s \n", err)
		return
	}

	err = ex.channel.QueueBind(ex.queueName, ex.routingKey, ex.exchangeName, true, nil)
	if err != nil {
		fmt.Printf("rabbitmq bind queue failed:%s \n", err)
		return
	}

	if err = ex.channel.Qos(1, 0, true); err != nil {
		fmt.Printf("rabbitmq set channel qos failed:%s \n", err)
	}

	msgList, err := ex.channel.Consume(ex.queueName, "", true, false, false, false, nil)
	if err != nil {
		fmt.Printf("rabbitmq consume failed:%s \n", err)
		return
	}

	ex.waitGroup.Add(1)
	go func() {
		ex.waitGroup.Done()
		for {
			select {
			case <-ex.exitChan:
				return
			case msg := <-msgList:
				if ex.parent != nil && msg.Body != nil {
					ex.parent.onReceivedMessageHandler(msg.Body)
					fmt.Printf("received message:%s \n", string(msg.Body))
				}
			}
		}
	}()
}

func PublishMessage2LB(msg string) {
	if !connected {
		return
	}
	m := PublishMsg{
		excname: "WB_",
		key:     "111",
		msg:     msg,
	}
	fmt.Printf("send message to LB:%s\n", msg)
	exc_pub_dr.msgChan <- m
}

func PublishMessage2Pod(key, msg string) {
	if !connected {
		return
	}
	m := PublishMsg{
		excname: "WB_",
		key:     key,
		msg:     msg,
	}
	fmt.Printf("send msg to pod-%s: %s\n", key, msg)
	exc_pub_dr.msgChan <- m
}

func BroadcastMessage2Proxy(msg string) {
	if !connected {
		return
	}
	m := PublishMsg{
		excname: "udpProxy",
		key:     "broadcast",
		msg:     msg,
	}
	fmt.Printf("broadcast msg to proxy: %s\n", msg)
	exc_proxy_bc.msgChan <- m
}

func BroadcastInterClusterMessage(msg string) {
	if !connected {
		return
	}
	m := PublishMsg{
		excname: "inter-cluster_broadcast",
		key:     "inter-cluster",
		msg:     msg,
	}
	fmt.Printf("inter cluster broadcast msg: %s\n", msg)
	exc_cluster_bc.msgChan <- m
}

func PublishMessage2CrossClusterPod(key, msg string) {
	if !connected {
		return
	}
	m := PublishMsg{
		excname: "WB_",
		key:     key,
		msg:     msg,
	}
	fmt.Printf("send msg to cross-cluster pod-%s: %s\n", key, msg)
	exc_cross_cluster_dr.msgChan <- m
}

func BroadcastCrossClusterMessage(msg string) {
	if !center_connected {
		return
	}
	m := PublishMsg{
		excname: "cross-cluster_broadcast",
		key:     "cross-cluster",
		msg:     msg,
	}
	fmt.Printf("cross cluster broadcast msg: %s\n", msg)
	exc_cross_cluster_bc.msgChan <- m
}
