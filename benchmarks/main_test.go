package main

import (
	"testing"
)

func zenqTestRunner(size uint64, b *testing.B) {
	currSize = size

	cleanup()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := uint64(0); i < numConcurrentWriters; i++ {
			go zenqProducer()
		}
		zenqConsumer()
	}
}

func chanTestRunner(size uint64, b *testing.B) {
	currSize = size

	cleanup()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := uint64(0); i < numConcurrentWriters; i++ {
			go chanProducer()
		}
		chanConsumer()
	}
}

func BenchmarkChanInputSize600(b *testing.B) { chanTestRunner(600, b) }

func BenchmarkZenQInputSize600(b *testing.B) { zenqTestRunner(600, b) }

func BenchmarkChanInputSize60000(b *testing.B) { chanTestRunner(60000, b) }

func BenchmarkZenQInputSize60000(b *testing.B) { zenqTestRunner(60000, b) }

func BenchmarkChanInputSize6000000(b *testing.B) { chanTestRunner(6000000, b) }

func BenchmarkZenQInputSize6000000(b *testing.B) { zenqTestRunner(6000000, b) }
