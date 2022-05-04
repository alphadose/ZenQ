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

func NewPayload() *Payload {
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
)

var (
	pl Payload = *NewPayload()

	currSize uint64 = throughput[0]

	// input batch size
	throughput = []uint64{60, 600, 6000, 6000000, 600000000}
	// throughput = []uint64{5}

	// Number of writers/producers which will be writing to the queue concurrently
	numConcurrentWriters uint64 = 1

	// native channel
	ch chan Payload = make(chan Payload, channelBufferSize)

	// ZenQ
	zq *zenq.ZenQ[Payload] = zenq.New[Payload]()
)

func validatePayload(param Payload) {
	if param.first != pl.first || param.second != pl.second || param.third != pl.third || param.fourth != pl.fourth || param.fifth != pl.fifth || len(param.sixth) != len(pl.sixth) || param.seventh != pl.seventh {
		panic("Loss of data integrity")
	}
}

func chanProducer() {
	epochs := currSize / numConcurrentWriters
	for i := uint64(0); i < epochs; i++ {
		ch <- pl
	}
}

func chanConsumer() {
	for i := uint64(0); i < currSize; i++ {
		validatePayload(<-ch)
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

func zenqConsumer() {
	for i := uint64(0); i < currSize; i++ {
		validatePayload(zq.Read())
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

// drain the channel and zenQ
func cleanup() {
	for len(ch) > 0 {
		<-ch
	}
	zq.Reset()
}

func main() {
	cleanup()
	for _, tput := range throughput {
		currSize = tput
		fmt.Printf("With Input Batch Size: %d and Num Concurrent Writers: %d\n", currSize, numConcurrentWriters)
		fmt.Print("\n")

		// Run tests
		measureTime(chanRunner, "Native Channel")
		measureTime(zenqRunner, "ZenQ")
		fmt.Print("====================================================================\n\n")
	}
}
