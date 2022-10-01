// +build !js

package main

import (
	"fmt"
	"sync"
	"time"
)

type KeyframeFilter struct {
	minInterval         time.Duration // unit: nanosecond
	maxInterval         time.Duration
	mutex               sync.Mutex
	exitChan            chan int
	waitGroup           *sync.WaitGroup
	timer               *time.Timer
	cached              bool
	lasttime            int64
	tick                bool // origin publisher is true
	sendRequestKeyframe func()
}

func CreateKeyframeFilter(tick bool) *KeyframeFilter {
	return &KeyframeFilter{
		minInterval:         time.Duration(time.Second * 2),
		maxInterval:         time.Duration(time.Second * 30),
		exitChan:            make(chan int),
		waitGroup:           &sync.WaitGroup{},
		timer:               nil,
		cached:              false,
		lasttime:            0,
		tick:                tick,
		sendRequestKeyframe: nil,
	}
}

func (filter *KeyframeFilter) Startup(f func()) {
	filter.sendRequestKeyframe = f
	if filter.tick {
		filter.startDelayTimer(filter.maxInterval)
	}
}

func (filter *KeyframeFilter) startDelayTimer(d time.Duration) {
	filter.timer = time.NewTimer(d)
	filter.waitGroup.Add(1)
	go func() {
		fmt.Println("#### 🍺🍺🍺🍺 KeyframeFilter goroutine start")
		defer filter.waitGroup.Done()
		for {
			select {
			case <-filter.exitChan:
				fmt.Println("#### 🍺🍺🍺🍺 KeyframeFilter goroutine exit")
				return
			case <-filter.timer.C:
				go filter.requestKeyframeNow()
			}
		}
	}()
}

// tips: multi-threads unsafe
func (filter *KeyframeFilter) Shutdown() {
	if filter.timer != nil {
		filter.sendRequestKeyframe = nil
		go func() {
			fmt.Println("#### 🍺🍺🍺🍺 KeyframeFilter exitChan <- 0")
			filter.exitChan <- 0
		}()
		filter.waitGroup.Wait()
		filter.timer = nil
	}
	filter.mutex.Lock()
	defer filter.mutex.Unlock()
	filter.cached = false
	filter.lasttime = 0
}

func (filter *KeyframeFilter) TryRequestKeyframe() {
	filter.mutex.Lock()
	defer filter.mutex.Unlock()
	if !filter.cached {
		curr := time.Now().UnixNano()
		if curr-filter.lasttime >= filter.minInterval.Nanoseconds() {
			// It has been more than minimum interval since the last request keyframe, send immediately
			go filter.requestKeyframeNow()
		} else {
			// less than minimum interval since the last request keyframe
			filter.cached = true
			filter.refreshMaxIntervalTimer(time.Duration(filter.minInterval.Nanoseconds() - curr + filter.lasttime))
		}
	} else {
		//fmt.Printf("#### 🍺过于频繁的PLI请求, 直接丢弃不处理\n")
		// already exists in the cache, drop it and do nothing.
		return
	}
}

func (filter *KeyframeFilter) requestKeyframeNow() {
	filter.mutex.Lock()
	defer filter.mutex.Unlock()
	curr := time.Now().UnixNano()
	dur := time.Duration(curr - filter.lasttime)
	strtime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("#### %s send PLI right now, real-interval:%f consume:%t\n", strtime, dur.Seconds(), filter.cached)
	filter.sendRequestKeyframe()
	filter.lasttime = curr
	if filter.cached {
		filter.cached = false
	}
	if filter.tick {
		filter.refreshMaxIntervalTimer(filter.maxInterval)
	}
}

func (filter *KeyframeFilter) refreshMaxIntervalTimer(d time.Duration) {
	/* fmt.Printf("#### 试图重设关键帧过滤器的 timer\n")
	if filter.timer.Stop() {
		<-filter.timer.C
	} */

	if filter.sendRequestKeyframe == nil {
		fmt.Println("#### 停止重设 timer")
		return
	}

	if filter.timer == nil {
		filter.startDelayTimer(d)
		fmt.Printf("#### 首次启动关键帧过滤器的 timer 间隔为:%f seconds\n", d.Seconds())
	} else {
		filter.timer.Reset(d)
		//fmt.Printf("#### 重设关键帧过滤器的 timer 间隔为:%f seconds\n", d.Seconds())
	}
}
