# ZenQ

> A low-latency thread-safe queue in golang implemented using a lock-free ringbuffer

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

ZenQ is even faster than golang native channels making it suitable for low-latency operations

Benchmarking code available [here](./benchmarks)

Note that if you run the benchmarks with `--race` flag then ZenQ will perform slower because the `--race` flag slows
down the atomic operations in golang. Under normal circumstances, ZenQ will outperform golang native channels.

```bash
$ go run benchmarks/main.go

With Input Batch Size:  50

Native Channel Runner completed transfer in: 69.167µs
ZenQ Runner completed transfer in: 10.209µs
====================================================================

With Input Batch Size:  500

Native Channel Runner completed transfer in: 106.417µs
ZenQ Runner completed transfer in: 63.542µs
====================================================================

With Input Batch Size:  5000

Native Channel Runner completed transfer in: 2.080167ms
ZenQ Runner completed transfer in: 584.042µs
====================================================================

With Input Batch Size:  5000000

Native Channel Runner completed transfer in: 557.59225ms
ZenQ Runner completed transfer in: 391.962042ms
====================================================================

With Input Batch Size:  500000000

Native Channel Runner completed transfer in: 1m12.552570459s
ZenQ Runner completed transfer in: 41.473401041s
====================================================================
```

In terms of operational efficiency
```bash
$ go test -benchmem -bench . benchmarks/*.go

goos: darwin
goarch: arm64
BenchmarkChanInputSize50-8       	  251517	      4766 ns/op	      80 B/op	       3 allocs/op
BenchmarkZenQInputSize50-8       	  260270	      4581 ns/op	      80 B/op	       3 allocs/op
BenchmarkChanInputSize5000-8     	    3072	    511490 ns/op	      80 B/op	       3 allocs/op
BenchmarkZenQInputSize5000-8     	    3223	    369790 ns/op	      80 B/op	       3 allocs/op
BenchmarkChanInputSize500000-8   	      21	  59785744 ns/op	      80 B/op	       3 allocs/op
BenchmarkZenQInputSize500000-8   	      28	  40053103 ns/op	      80 B/op	       3 allocs/op
PASS
ok  	command-line-arguments	8.879s
```
