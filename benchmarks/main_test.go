package main

import (
	"testing"

	"github.com/alphadose/zenq"
)

func zenqTestRunner(size uint64, b *testing.B) {
	doneChan := make(chan int)
	currSize = size
	g1 = zenq.New[Payload]()
	g2 = zenq.New[Payload]()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		go zenqProducer(g1, doneChan)
		go zenqForwarder(g1, g2, doneChan)
		go zenqConsumer(g2, doneChan)

		<-doneChan
		<-doneChan
		<-doneChan
	}
}

func chanTestRunner(size uint64, b *testing.B) {
	doneChan := make(chan int)
	currSize = size
	r1 = make(chan Payload, channelBufferSize)
	r2 = make(chan Payload, channelBufferSize)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		go chanProducer(r1, doneChan)
		go chanForwarder(r1, r2, doneChan)
		go chanConsumer(r2, doneChan)

		<-doneChan
		<-doneChan
		<-doneChan
	}
}

func BenchmarkChanInputSize50(b *testing.B) { chanTestRunner(50, b) }

func BenchmarkZenQInputSize50(b *testing.B) { zenqTestRunner(50, b) }

func BenchmarkChanInputSize5000(b *testing.B) { chanTestRunner(5000, b) }

func BenchmarkZenQInputSize5000(b *testing.B) { zenqTestRunner(5000, b) }

func BenchmarkChanInputSize500000(b *testing.B) { chanTestRunner(500000, b) }

func BenchmarkZenQInputSize500000(b *testing.B) { zenqTestRunner(500000, b) }
