package main

import (
	"fmt"
	"time"

	"github.com/alphadose/zenq"
)

// Example item which we will be writing to and reading from the queue
type Payload struct {
	first  byte
	second int64
	third  float64
	fourth string
	fifth  complex64
	sixth  []rune
	sevent bool
}

func NewPayload(value uint64) *Payload {
	return &Payload{
		first:  1,
		second: 2,
		third:  3.0,
		fourth: "4",
		fifth:  3 + 4i,
		sixth:  []rune("🐈⚔️👍🌏💥🦖"),
	}
}

const channelBufferSize = 4096

var (
	pl Payload = *NewPayload(1)

	currSize uint64 = throughput[0]

	throughput = []uint64{50, 500, 5000, 5000000, 500000000}

	// native channels
	r1, r2 chan Payload

	// ZenQ
	g1, g2 *zenq.ZenQ[Payload]
)

func noopPayload(param Payload) {}

func chanProducer(outChan chan Payload, done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		outChan <- pl
	}
	done <- 0
}

func chanForwarder(inChan chan Payload, outChan chan Payload, done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		var o Payload = <-inChan
		outChan <- o
	}
	defer close(inChan)
	done <- 0
}

func chanConsumer(inChan chan Payload, done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		var o Payload = <-inChan
		noopPayload(o)
	}
	defer close(inChan)
	done <- 0
}

func chanRunner() {
	doneChan := make(chan int)

	var startTime time.Time = time.Now()

	go chanProducer(r1, doneChan)
	go chanForwarder(r1, r2, doneChan)
	go chanConsumer(r2, doneChan)

	<-doneChan
	<-doneChan
	<-doneChan

	fmt.Println("Native Channel Runner completed transfer in:", time.Since(startTime))
}

func zenqProducer(outQ *zenq.ZenQ[Payload], done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		outQ.Write(pl)
	}
	done <- 0
}

func zenqForwarder(inQ *zenq.ZenQ[Payload], outQ *zenq.ZenQ[Payload], done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		var rez Payload = inQ.Read()
		outQ.Write(rez)
	}
	done <- 0
}

func zenqConsumer(inQ *zenq.ZenQ[Payload], done chan int) {
	var i uint64
	for ; i < currSize; i++ {
		var rez Payload = inQ.Read()
		if rez.first != pl.first || rez.second != pl.second || rez.third != pl.third || rez.fourth != pl.fourth || rez.fifth != pl.fifth || len(rez.sixth) != len(pl.sixth) || rez.sevent != pl.sevent {
			panic("Loss of data integretiy")
		}
		noopPayload(rez)
	}
	done <- 0
}

func zenqRunner() {
	doneChan := make(chan int)

	var startTime time.Time = time.Now()

	go zenqProducer(g1, doneChan)
	go zenqForwarder(g1, g2, doneChan)
	go zenqConsumer(g2, doneChan)

	<-doneChan
	<-doneChan
	<-doneChan

	fmt.Println("ZenQ Runner completed transfer in:", time.Since(startTime))
}

func main() {
	for _, tput := range throughput {
		currSize = tput
		fmt.Println("With Input Batch Size: ", currSize)
		fmt.Print("\n")

		// Re-initializing
		r1 = make(chan Payload, channelBufferSize)
		r2 = make(chan Payload, channelBufferSize)

		g1 = zenq.New[Payload]()
		g2 = zenq.New[Payload]()

		// Run tests
		chanRunner()
		zenqRunner()
		fmt.Print("====================================================================\n\n")
	}
}
