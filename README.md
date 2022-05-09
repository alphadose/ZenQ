# ZenQ

> A low-latency thread-safe queue in golang implemented using a lock-free ringbuffer

## Features

* Much faster than native channels in both SPSC (single-producer-single-consumer) and MPSC (multi-producer-single-consumer) modes in terms of `time/op`
* More resource efficient in terms of `memory_allocation/op` and `num_allocations/op` evident while benchmarking large batch size inputs
* Handles the case where NUM_WRITER_GOROUTINES > NUM_CPU_CORES much better than native channels
* Selection from multiple ZenQs just like golang's `select{}` ensuring fair selection and no starvation

Benchmarks to support the above claims [here](#benchmarks)

## Installation

You need Golang [1.18.x](https://go.dev/dl/) or above since this package uses generics

```bash
$ go get github.com/alphadose/zenq@1.4.0
```

## Usage

1. Simple Read/Write
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

2. **Selection** from multiple ZenQs just like golang's native `select{}`. The selection process is fair i.e no single ZenQ gets starved
```go
package main

import (
	"fmt"

	"github.com/alphadose/zenq"
)

type custom1 struct {
	alpha int
	beta  string
}

type custom2 struct {
	gamma int
}

var (
	zq1 = zenq.New[int]()
	zq2 = zenq.New[string]()
	zq3 = zenq.New[custom1]()
	zq4 = zenq.New[*custom2]()
)

func main() {
	go looper(intProducer)
	go looper(stringProducer)
	go looper(custom1Producer)
	go looper(custom2Producer)

	for i := 0; i < 40; i++ {

		// Selection occurs here
		switch data := zenq.Select(zq1, zq2, zq3, zq4).(type) {
		case int:
			fmt.Printf("Received int %d\n", data)
		case string:
			fmt.Printf("Received string %s\n", data)
		case custom1:
			fmt.Printf("Received custom data type number 1 %#v\n", data)
		case *custom2:
			fmt.Printf("Received pointer %#v\n", data)
		}
	}
}

func intProducer(ctr int) { zq1.Write(ctr) }

func stringProducer(ctr int) { zq2.Write(fmt.Sprint(ctr * 10)) }

func custom1Producer(ctr int) { zq3.Write(custom1{alpha: ctr, beta: fmt.Sprint(ctr)}) }

func custom2Producer(ctr int) { zq4.Write(&custom2{gamma: 1 << ctr}) }

func looper(producer func(ctr int)) {
	for i := 0; i < 10; i++ {
		producer(i)
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
Computed from benchstat of 30 benchmarks each via go test -benchmem -bench=. benchmarks/simple/*.go

name                                     time/op
_Chan_NumWriters1_InputSize600-8          23.3µs ± 1%
_ZenQ_NumWriters1_InputSize600-8          17.9µs ± 1%
_Chan_NumWriters3_InputSize60000-8        5.48ms ± 3%
_ZenQ_NumWriters3_InputSize60000-8        2.67ms ± 6%
_Chan_NumWriters8_InputSize6000000-8       679ms ± 1%
_ZenQ_NumWriters8_InputSize6000000-8       313ms ± 5%
_Chan_NumWriters100_InputSize6000000-8     1.58s ± 1%
_ZenQ_NumWriters100_InputSize6000000-8     516ms ± 2%
_Chan_NumWriters1000_InputSize7000000-8    1.97s ± 1%
_ZenQ_NumWriters1000_InputSize7000000-8    445ms ± 9%
_Chan_Million_Blocking_Writers-8           10.8s ± 1%
_ZenQ_Million_Blocking_Writers-8           11.0s ± 4%

name                                     alloc/op
_Chan_NumWriters1_InputSize600-8           0.00B
_ZenQ_NumWriters1_InputSize600-8           0.00B
_Chan_NumWriters3_InputSize60000-8         95.0B ±65%
_ZenQ_NumWriters3_InputSize60000-8        34.8B ±127%
_Chan_NumWriters8_InputSize6000000-8       878B ±272%
_ZenQ_NumWriters8_InputSize6000000-8       671B ±222%
_Chan_NumWriters100_InputSize6000000-8    46.0kB ±31%
_ZenQ_NumWriters100_InputSize6000000-8   13.3kB ±100%
_Chan_NumWriters1000_InputSize7000000-8    488kB ± 8%
_ZenQ_NumWriters1000_InputSize7000000-8  2.37kB ±210%
_Chan_Million_Blocking_Writers-8           553MB ± 0%
_ZenQ_Million_Blocking_Writers-8          95.5MB ± 0%

name                                     allocs/op
_Chan_NumWriters1_InputSize600-8            0.00
_ZenQ_NumWriters1_InputSize600-8            0.00
_Chan_NumWriters3_InputSize60000-8          0.00
_ZenQ_NumWriters3_InputSize60000-8          0.00
_Chan_NumWriters8_InputSize6000000-8       2.77 ±225%
_ZenQ_NumWriters8_InputSize6000000-8       1.40 ±186%
_Chan_NumWriters100_InputSize6000000-8       164 ±20%
_ZenQ_NumWriters100_InputSize6000000-8     31.8 ±100%
_Chan_NumWriters1000_InputSize7000000-8    1.79k ± 3%
_ZenQ_NumWriters1000_InputSize7000000-8    5.50 ±227%
_Chan_Million_Blocking_Writers-8           2.00M ± 0%
_ZenQ_Million_Blocking_Writers-8            995k ± 0%
```

The above results show that ZenQ is more efficient than channels in all 3 metrics i.e `time/op`, `mem_alloc/op` and `num_allocs/op` for the following tested cases:-

1. SPSC
2. MPSC with NUM_WRITER_GOROUTINES < NUM_CPU_CORES
3. MPSC with NUM_WRITER_GOROUTINES > NUM_CPU_CORES


## Cherry on the Cake

In SPSC mode ZenQ is faster than channels by **98 seconds** in case of input size 6 * 10<sup>8</sup>

```bash
❯ go run benchmarks/simple/main.go

With Input Batch Size: 60 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 64.875µs
ZenQ Runner completed transfer in: 9µs
====================================================================

With Input Batch Size: 600 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 70.958µs
ZenQ Runner completed transfer in: 44.958µs
====================================================================

With Input Batch Size: 6000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 967.417µs
ZenQ Runner completed transfer in: 518.916µs
====================================================================

With Input Batch Size: 6000000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 1.191589458s
ZenQ Runner completed transfer in: 144.895583ms
====================================================================

With Input Batch Size: 600000000 and Num Concurrent Writers: 1

Native Channel Runner completed transfer in: 1m52.671809708s
ZenQ Runner completed transfer in: 14.356517042s
====================================================================
```

For a `select{}` based transfer experiment these are the results
```bash
❯ go run benchmarks/selector/main.go

Chan Select Runner completed transfer in: 2m42.313942333s
ZenQ Select Runner completed transfer in: 41.938121583s
```

Code available [here](./benchmarks/selector/main.go)
