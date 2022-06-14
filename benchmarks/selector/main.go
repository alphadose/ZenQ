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
	bufferSize = 8

	numProducers = 4
)

var (
	throughput int

	// input batch size
	testcases = []int{60, 600, 6e3, 6e5}

	zq1 = zenq.New[int](bufferSize)
	zq2 = zenq.New[string](bufferSize)
	zq3 = zenq.New[custom1](bufferSize)
	zq4 = zenq.New[*custom2](bufferSize)

	ch1 = make(chan int, bufferSize)
	ch2 = make(chan string, bufferSize)
	ch3 = make(chan custom1, bufferSize)
	ch4 = make(chan *custom2, bufferSize)
)

func zenqSelector() {
	go looper(intProducer1)
	go looper(stringProducer1)
	go looper(custom1Producer1)
	go looper(custom2Producer1)

	var ctr = 0

	var startTime time.Time = time.Now()
	for i := 0; i < throughput; i++ {
		if _, ok := zenq.Select(zq1, zq2, zq3, zq4); ok {
			ctr++
		}
	}

	if ctr != throughput {
		panic("Data Loss")
	}
	fmt.Printf("ZenQ Select Runner completed transfer in: %v\n", time.Since(startTime))
}

func chanSelector() {
	go looper(intProducer2)
	go looper(stringProducer2)
	go looper(custom1Producer2)
	go looper(custom2Producer2)

	var ctr = 0

	var startTime time.Time = time.Now()
	for i := 0; i < throughput; i++ {
		select {
		case <-ch1:
			ctr++
		case <-ch2:
			ctr++
		case <-ch3:
			ctr++
		case <-ch4:
			ctr++
		}

	}

	if ctr != throughput {
		panic("Data Loss")
	}
	fmt.Printf("Chan Select Runner completed transfer in: %v\n", time.Since(startTime))
}

func main() {
	for _, tput := range testcases {
		throughput = tput
		fmt.Printf("With Input Batch Size: %d and Num Concurrent Writers: %d\n", throughput, numProducers)
		fmt.Print("\n")

		// Run tests
		chanSelector()
		zenqSelector()
		fmt.Print("====================================================================\n\n")
	}
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
