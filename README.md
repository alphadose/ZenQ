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

### Hardware Specs

```
❯ neofetch
                    'c.          alphadose@ReiEki.local
                 ,xNMM.          ----------------------
               .OMMMMo           OS: macOS 12.3 21E230 arm64
               OMMM0,            Host: MacBookAir10,1
     .;loddo:' loolloddol;.      Kernel: 21.4.0
   cKMMMMMMMMMMNWMMMMMMMMMM0:    Uptime: 6 hours, 41 mins
 .KMMMMMMMMMMMMMMMMMMMMMMMWd.    Packages: 86 (brew)
 XMMMMMMMMMMMMMMMMMMMMMMMX.      Shell: zsh 5.8
;MMMMMMMMMMMMMMMMMMMMMMMM:       Resolution: 1440x900
:MMMMMMMMMMMMMMMMMMMMMMMM:       DE: Aqua
.MMMMMMMMMMMMMMMMMMMMMMMMX.      WM: Rectangle
 kMMMMMMMMMMMMMMMMMMMMMMMMWd.    Terminal: iTerm2
 .XMMMMMMMMMMMMMMMMMMMMMMMMMMk   Terminal Font: FiraCodeNerdFontComplete-Medium 16 (normal)
  .XMMMMMMMMMMMMMMMMMMMMMMMMK.   CPU: Apple M1
    kMMMMMMMMMMMMMMMMMMMMMMd     GPU: Apple M1
     ;KMMMMMMMWXXWMMMMMMMk.      Memory: 1370MiB / 8192MiB
       .cooc,.    .,coo:.

```

### Terminology

* NUM_WRITERS -> The number of goroutines concurrently writing to ZenQ/Channel
* INPUT_SIZE -> The number of input payloads to be passed through ZenQ/Channel from producers to consumer

```bash
Computed from benchstat of 20 benchmarks each via go test -benchmem -bench=. benchmarks/*.go

name                                     time/op
_Chan_NumWriters1_InputSize600-8          23.2µs ± 0%
_ZenQ_NumWriters1_InputSize600-8          16.4µs ± 1%
_Chan_NumWriters3_InputSize60000-8        3.61ms ± 3%
_ZenQ_NumWriters3_InputSize60000-8        2.96ms ± 2%
_Chan_NumWriters8_InputSize6000000-8       701ms ± 1%
_ZenQ_NumWriters8_InputSize6000000-8       458ms ± 1%
_Chan_NumWriters100_InputSize6000000-8     1.57s ± 2%
_ZenQ_NumWriters100_InputSize6000000-8     460ms ± 1%
_Chan_NumWriters1000_InputSize7000000-8    1.96s ± 0%
_ZenQ_NumWriters1000_InputSize7000000-8    548ms ± 1%

name                                     alloc/op
_Chan_NumWriters1_InputSize600-8           0.00B
_ZenQ_NumWriters1_InputSize600-8           0.00B
_Chan_NumWriters3_InputSize60000-8         74.5B ±62%
_ZenQ_NumWriters3_InputSize60000-8        23.1B ±125%
_Chan_NumWriters8_InputSize6000000-8       666B ±200%
_ZenQ_NumWriters8_InputSize6000000-8       570B ±265%
_Chan_NumWriters100_InputSize6000000-8    44.6kB ±21%
_ZenQ_NumWriters100_InputSize6000000-8   4.25kB ±135%
_Chan_NumWriters1000_InputSize7000000-8    480kB ± 7%
_ZenQ_NumWriters1000_InputSize7000000-8  2.33kB ±239%

name                                     allocs/op
_Chan_NumWriters1_InputSize600-8            0.00
_ZenQ_NumWriters1_InputSize600-8            0.00
_Chan_NumWriters3_InputSize60000-8          0.00
_ZenQ_NumWriters3_InputSize60000-8          0.00
_Chan_NumWriters8_InputSize6000000-8        3.24 ±85%
_ZenQ_NumWriters8_InputSize6000000-8       1.30 ±285%
_Chan_NumWriters100_InputSize6000000-8       172 ±12%
_ZenQ_NumWriters100_InputSize6000000-8     10.0 ±140%
_Chan_NumWriters1000_InputSize7000000-8    1.78k ± 4%
_ZenQ_NumWriters1000_InputSize7000000-8    5.47 ±247%
```

The above results show that ZenQ is more efficient than channels in all 3 metrics i.e `ns/op`, `B/op` and `allocs/op` for the following tested cases:-

1. SPSC
2. MPSC with NUM_WRITER_GOROUTINES < NUM_CPU_CORES
3. MPSC with NUM_WRITER_GOROUTINES > NUM_CPU_CORES


## Cherry on the Cake

In SPSC mode ZenQ is faster than channels by **94 seconds** in case of input size 6 * 10<sup>8</sup>

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
