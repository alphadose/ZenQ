package main

import (
	"testing"

	"github.com/alphadose/zenq"
)

func zenqTestRunner(size uint64, epochs int) {
	doneChan := make(chan int)
	currSize = size
	g1 = zenq.New[Payload]()
	g2 = zenq.New[Payload]()

	for n := 0; n < epochs; n++ {
		go zenqProducer(g1, doneChan)
		go zenqForwarder(g1, g2, doneChan)
		go zenqConsumer(g2, doneChan)

		<-doneChan
		<-doneChan
		<-doneChan
	}
}

func chanTestRunner(size uint64, epochs int) {
	doneChan := make(chan int)
	currSize = size
	r1 = make(chan Payload, channelBufferSize)
	r2 = make(chan Payload, channelBufferSize)

	for n := 0; n < epochs; n++ {
		go chanProducer(r1, doneChan)
		go chanForwarder(r1, r2, doneChan)
		go chanConsumer(r2, doneChan)

		<-doneChan
		<-doneChan
		<-doneChan
	}
}

func BenchmarkNativeChanSize50(b *testing.B) { chanTestRunner(50, b.N) }

func BenchmarkZenQSize50(b *testing.B) { zenqTestRunner(50, b.N) }

func BenchmarkNativeChanSize5000(b *testing.B) { chanTestRunner(5000, b.N) }

func BenchmarkZenQSize5000(b *testing.B) { zenqTestRunner(5000, b.N) }

func BenchmarkNativeChanSize500000(b *testing.B) { chanTestRunner(500000, b.N) }

func BenchmarkZenQSize500000(b *testing.B) { zenqTestRunner(500000, b.N) }
