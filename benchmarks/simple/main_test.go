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

func Benchmark_Chan_NumWriters1_InputSize600(b *testing.B) { chanTestRunner(1, 6e2, b) }

func Benchmark_ZenQ_NumWriters1_InputSize600(b *testing.B) { zenqTestRunner(1, 6e2, b) }

func Benchmark_Chan_NumWriters3_InputSize60000(b *testing.B) { chanTestRunner(3, 6e4, b) }

func Benchmark_ZenQ_NumWriters3_InputSize60000(b *testing.B) { zenqTestRunner(3, 6e4, b) }

func Benchmark_Chan_NumWriters8_InputSize6000000(b *testing.B) { chanTestRunner(8, 6e6, b) }

func Benchmark_ZenQ_NumWriters8_InputSize6000000(b *testing.B) { zenqTestRunner(8, 6e6, b) }

func Benchmark_Chan_NumWriters100_InputSize6000000(b *testing.B) { chanTestRunner(100, 6e6, b) }

func Benchmark_ZenQ_NumWriters100_InputSize6000000(b *testing.B) { zenqTestRunner(100, 6e6, b) }

func Benchmark_Chan_NumWriters1000_InputSize7000000(b *testing.B) { chanTestRunner(1e3, 7e6, b) }

func Benchmark_ZenQ_NumWriters1000_InputSize7000000(b *testing.B) { zenqTestRunner(1e3, 7e6, b) }

func Benchmark_Chan_Million_Blocking_Writers(b *testing.B) { chanTestRunner(1e6, 1e7, b) }

func Benchmark_ZenQ_Million_Blocking_Writers(b *testing.B) { zenqTestRunner(1e6, 1e7, b) }
