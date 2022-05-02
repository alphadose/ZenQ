# ZenQ

> A low-latency thread-safe queue in golang implemented using a lock-free ringbuffer

## Features

* Much faster than native channels in both SPSC (single-producer-single-consumer) and MPSC (multi-producer-single-consumer) modes in terms of `ns/op`
* More resource efficient in terms of `B/op` and `allocs/op` evident while benchmarking large batch size inputs
* Handles the case where NUM_WRITER_GOROUTINES > NUM_CPU_CORES much better than native channels

Benchmarks to support the above claims [here](#benchmarks)

## Installation

You need Golang [1.18.x](https://go.dev/dl/) or above since this package uses generics

```bash
$ go get github.com/alphadose/zenq@1.2.0
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/alphadose/zenq"
)

type payload struct {
	alpha int
	beta  string
}

func main() {
	zq := zenq.New[payload]()

	for j := 0; j < 5; j++ {
		go func() {
			for i := 0; i < 20; i++ {
				zq.Write(payload{
					alpha: i,
					beta:  fmt.Sprint(i),
				})
			}
		}()
	}

	for i := 0; i < 100; i++ {
        	var data payload = zq.Read()
		fmt.Printf("%+v\n", data)
	}
}
```

## Benchmarks

Benchmarking code available [here](./benchmarks)

Note that if you run the benchmarks with `--race` flag then ZenQ will perform slower because the `--race` flag slows
down the atomic operations in golang. Under normal circumstances, ZenQ will outperform golang native channels.

### Terminology

* NUM_WRITERS -> The number of goroutines concurrently writing to ZenQ/Channel
* INPUT_SIZE -> The number of input payloads to be passed through ZenQ/Channel from producers to consumer

```bash
$ go test -bench=. -benchmem benchmarks/*.go

goos: darwin
goarch: arm64
Benchmark_Chan_NumWriters1_InputSize600-8       48772       23614 ns/op       0 B/op      0 allocs/op
Benchmark_ZenQ_NumWriters1_InputSize600-8       73860       15984 ns/op       0 B/op      0 allocs/op
Benchmark_Chan_NumWriters3_InputSize60000-8       216     5500119 ns/op     109 B/op      0 allocs/op
Benchmark_ZenQ_NumWriters3_InputSize60000-8       411     2835585 ns/op       0 B/op      0 allocs/op
Benchmark_Chan_NumWriters8_InputSize6000000-8       2   703887875 ns/op    1600 B/op      5 allocs/op
Benchmark_ZenQ_NumWriters8_InputSize6000000-8       3   445850153 ns/op       0 B/op      0 allocs/op
Benchmark_Chan_NumWriters100_InputSize6000000-8     1  1593555542 ns/op   39456 B/op    146 allocs/op
Benchmark_ZenQ_NumWriters100_InputSize6000000-8     3   485728444 ns/op    3466 B/op      8 allocs/op
Benchmark_Chan_NumWriters1000_InputSize7000000-8    1  1984590667 ns/op  497344 B/op   1817 allocs/op
Benchmark_ZenQ_NumWriters1000_InputSize7000000-8    2   777197480 ns/op    8736 B/op     21 allocs/op
PASS
ok  	command-line-arguments	19.687s
```

The above results show that ZenQ is more efficient than channels in all 3 metrics i.e `ns/op`, `B/op` and `allocs/op` for the following cases:-

1. SPSC
2. MPSC with NUM_WRITER_GOROUTINES < NUM_CPU_CORES
3. MPSC with NUM_WRITER_GOROUTINES > NUM_CPU_CORES


## Cherry on the Cake

In SPSC mode ZenQ literally blasts native channels out of the picture with a gain of **94 seconds** in case of input size 6 * 10<sup>8</sup>

```bash
$ go run benchmarks/main.go

With Input Batch Size: 60 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 92.25µs
ZenQ Runner completed transfer in: 15.667µs
====================================================================

With Input Batch Size: 600 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 160.708µs
ZenQ Runner completed transfer in: 109.125µs
====================================================================

With Input Batch Size: 6000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 1.419542ms
ZenQ Runner completed transfer in: 815.25µs
====================================================================

With Input Batch Size: 6000000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 1.003736042s
ZenQ Runner completed transfer in: 202.91975ms
====================================================================

With Input Batch Size: 600000000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 1m53.85855875s
ZenQ Runner completed transfer in: 20.466423125s
====================================================================
```
