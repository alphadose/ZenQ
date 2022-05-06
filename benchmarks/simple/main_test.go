package main

import (
	"testing"
)

func zenqTestRunner(numWriters uint64, size uint64, b *testing.B) {
	currSize = size
	numConcurrentWriters = numWriters

	cleanup()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		zenqRunner()
	}
}

func chanTestRunner(numWriters uint64, size uint64, b *testing.B) {
	currSize = size
	numConcurrentWriters = numWriters

	cleanup()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		chanRunner()
	}
}

func Benchmark_Chan_NumWriters1_InputSize600(b *testing.B) { chanTestRunner(1, 600, b) }

func Benchmark_ZenQ_NumWriters1_InputSize600(b *testing.B) { zenqTestRunner(1, 600, b) }

func Benchmark_Chan_NumWriters3_InputSize60000(b *testing.B) { chanTestRunner(3, 60000, b) }

func Benchmark_ZenQ_NumWriters3_InputSize60000(b *testing.B) { zenqTestRunner(3, 60000, b) }

func Benchmark_Chan_NumWriters8_InputSize6000000(b *testing.B) { chanTestRunner(8, 6000000, b) }

func Benchmark_ZenQ_NumWriters8_InputSize6000000(b *testing.B) { zenqTestRunner(8, 6000000, b) }

func Benchmark_Chan_NumWriters100_InputSize6000000(b *testing.B) { chanTestRunner(100, 6000000, b) }

func Benchmark_ZenQ_NumWriters100_InputSize6000000(b *testing.B) { zenqTestRunner(100, 6000000, b) }

func Benchmark_Chan_NumWriters1000_InputSize7000000(b *testing.B) { chanTestRunner(1000, 7000000, b) }

func Benchmark_ZenQ_NumWriters1000_InputSize7000000(b *testing.B) { zenqTestRunner(1000, 7000000, b) }

// Test resource efficiency when there are large number of blocked goroutine writers
const million uint64 = 1000000

func Benchmark_Chan_Million_Blocking_Writers(b *testing.B) {
	chanTestRunner(million, 10*million, b)
}

func Benchmark_ZenQ_Million_Blocking_Writers(b *testing.B) {
	zenqTestRunner(million, 10*million, b)
}
