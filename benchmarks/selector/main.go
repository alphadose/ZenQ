package main

import (
	"fmt"
	"time"

	"github.com/alphadose/zenq"
)

type custom1 struct {
	alpha int
	beta  string
}

type custom2 struct {
	gamma int
}

const (
	channelBufferSize     = 1 << 12
	throughput        int = 6e5 // 6 * 10^5
	numProducers          = 4
)

var (
	zq1 = zenq.New[int]()
	zq2 = zenq.New[string]()
	zq3 = zenq.New[custom1]()
	zq4 = zenq.New[*custom2]()

	ch1 = make(chan int, channelBufferSize)
	ch2 = make(chan string, channelBufferSize)
	ch3 = make(chan custom1, channelBufferSize)
	ch4 = make(chan *custom2, channelBufferSize)
)

func zenqSelector() {
	go looper(intProducer1)
	go looper(stringProducer1)
	go looper(custom1Producer1)
	go looper(custom2Producer1)

	var intCtr, strCtr, cs1Ctr, cs2Ctr = 0, 0, 0, 0
	validCount := throughput / numProducers

	var startTime time.Time = time.Now()
	for i := 0; i < throughput; i++ {

		// Selection occurs here
		if data, ok := zenq.Select(zq1, zq2, zq3, zq4); ok {
			switch data.(type) {
			case int:
				intCtr++
			case string:
				strCtr++
			case custom1:
				cs1Ctr++
			case *custom2:
				cs2Ctr++
			}
		}
		// println(i)
	}
	if intCtr != validCount || strCtr != validCount || cs1Ctr != validCount || cs2Ctr != validCount {
		panic("Data Loss")
	}
	fmt.Printf("ZenQ Select Runner completed transfer in: %v\n", time.Since(startTime))
}

func chanSelector() {
	go looper(intProducer2)
	go looper(stringProducer2)
	go looper(custom1Producer2)
	go looper(custom2Producer2)

	var intCtr, strCtr, cs1Ctr, cs2Ctr = 0, 0, 0, 0
	validCount := throughput / numProducers

	var startTime time.Time = time.Now()
	for i := 0; i < throughput; i++ {
		select {
		case <-ch1:
			intCtr++
		case <-ch2:
			strCtr++
		case <-ch3:
			cs1Ctr++
		case <-ch4:
			cs2Ctr++
		}

	}
	if intCtr != validCount || strCtr != validCount || cs1Ctr != validCount || cs2Ctr != validCount {
		panic("Data Loss")
	}
	fmt.Printf("Chan Select Runner completed transfer in: %v\n", time.Since(startTime))
}

func main() {
	chanSelector()
	zenqSelector()
}

func intProducer1(ctr int) { zq1.Write(ctr) }

func stringProducer1(ctr int) { zq2.Write(fmt.Sprint(ctr * 10)) }

func custom1Producer1(ctr int) { zq3.Write(custom1{alpha: ctr, beta: fmt.Sprint(ctr)}) }

func custom2Producer1(ctr int) { zq4.Write(&custom2{gamma: 1 << ctr}) }

func intProducer2(ctr int) { ch1 <- ctr }

func stringProducer2(ctr int) { ch2 <- fmt.Sprint(ctr * 10) }

func custom1Producer2(ctr int) { ch3 <- custom1{alpha: ctr, beta: fmt.Sprint(ctr)} }

func custom2Producer2(ctr int) { ch4 <- &custom2{gamma: 1 << ctr} }

func looper(producer func(ctr int)) {
	for i := 0; i < throughput/numProducers; i++ {
		producer(i)
	}
}
