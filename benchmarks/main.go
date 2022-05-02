package main

import (
	"fmt"
	"time"

	"github.com/alphadose/zenq"
)

// Example item which we will be writing to and reading from the queue
type Payload struct {
	first   byte
	second  int64
	third   float64
	fourth  string
	fifth   complex64
	sixth   []rune
	seventh bool
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

const (
	channelBufferSize = 1 << 12

	// Number of writers/producers which will be writing to the queue concurrently
	// For best results set it to num_cpu_cores - 3
	numConcurrentWriters uint64 = 2
)

var (
	pl Payload = *NewPayload(1)

	currSize uint64 = throughput[0]

	throughput = []uint64{60, 600, 6000, 6000000, 600000000}

	// native channel
	ch chan Payload = make(chan Payload, channelBufferSize)

	// ZenQ
	zq *zenq.ZenQ[Payload] = zenq.New[Payload]()
)

func noopPayload(param Payload) {}

func chanProducer() {
	epochs := currSize / numConcurrentWriters
	for i := uint64(0); i < epochs; i++ {
		ch <- pl
	}
}

// func chanForwarder(inChan chan Payload, outChan chan Payload, done chan int) {
// 	for i := uint64(0); i < currSize; i++ {
// 		outChan <- <-inChan
// 	}
// 	done <- 0
// }

func chanConsumer() {
	for i := uint64(0); i < currSize; i++ {
		noopPayload(<-ch)
	}
}

func chanRunner() {
	for i := uint64(0); i < numConcurrentWriters; i++ {
		go chanProducer()
	}
	chanConsumer()
}

func zenqProducer() {
	epochs := currSize / numConcurrentWriters
	for i := uint64(0); i < epochs; i++ {
		zq.Write(pl)
	}
}

// func zenqForwarder(inQ *zenq.ZenQ[Payload], outQ *zenq.ZenQ[Payload], done chan int) {
// 	for i := uint64(0); i < currSize; i++ {
// 		outQ.Write(inQ.Read())
// 	}
// 	done <- 0
// }

func zenqConsumer() {
	for i := uint64(0); i < currSize; i++ {
		// var rez Payload = inQ.Read()
		// if rez.first != pl.first || rez.second != pl.second || rez.third != pl.third || rez.fourth != pl.fourth || rez.fifth != pl.fifth || len(rez.sixth) != len(pl.sixth) || rez.seventh != pl.seventh {
		// 	panic("Loss of data integretiy")
		// }
		// noopPayload(rez)
		noopPayload(zq.Read())
	}
}

func zenqRunner() {
	for i := uint64(0); i < numConcurrentWriters; i++ {
		go zenqProducer()
	}
	zenqConsumer()
}

func measureTime(callback func(), runnerName string) {
	var startTime time.Time = time.Now()
	callback()
	fmt.Printf("%s Runner completed transfer in: %v\n", runnerName, time.Since(startTime))
}

func cleanup() {
	for len(ch) > 0 {
		<-ch
	}
	zq.Reset()
}

func main() {
	for _, tput := range throughput {
		currSize = tput
		fmt.Printf("With Input Batch Size: %d and Num Concurrent Writers: %d\n", currSize, numConcurrentWriters)
		fmt.Print("\n")

		cleanup()

		// Run tests
		measureTime(chanRunner, "Native Channel")
		measureTime(zenqRunner, "ZenQ")
		fmt.Print("====================================================================\n\n")
	}
}
