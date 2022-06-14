package zenq_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/alphadose/zenq"
)

const bufferSize = 1 << 12

type Payload struct {
	first   byte
	second  int64
	third   float64
	fourth  string
	fifth   complex64
	sixth   []rune
	seventh bool
}

type test struct {
	writers   int
	readers   int
	inputSize int
}

var testCases = []test{
	{writers: 1, readers: 1, inputSize: 1e3},
	{writers: 3, readers: 3, inputSize: 3e3},
	{writers: 8, readers: 8, inputSize: 8e3},
	{writers: bufferSize * 2, readers: 1, inputSize: bufferSize * 2 * 4},
	{writers: 1, readers: bufferSize * 2, inputSize: bufferSize * 2 * 4},
	{writers: 100, readers: 100, inputSize: 6e6},
	{writers: 1e3, readers: 1e3, inputSize: 7e6},
}

func init() {
	for _, t := range testCases {
		if t.inputSize%t.writers != 0 {
			panic(fmt.Sprintf("input size %d should be dividable by writers %d", t.inputSize, t.writers))
		}
		if t.inputSize%t.readers != 0 {
			panic(fmt.Sprintf("input size %d should be dividable by readers %d", t.inputSize, t.readers))
		}
	}
}

func BenchmarkChan_ProduceConsume(b *testing.B) {
	for _, t := range testCases {
		t := t
		b.Run(fmt.Sprintf("W%d/R%d/Size%d", t.writers, t.readers, t.inputSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchmarkProduceConsumeChan(b, t)
			}
		})
	}
}

func benchmarkProduceConsumeChan(b *testing.B, t test) {
	q := make(chan Payload, bufferSize)
	defer runtime.KeepAlive(q)

	writesPerProducer := t.inputSize / t.writers
	readsPerConsumer := t.inputSize / t.readers

	var wg sync.WaitGroup
	wg.Add(t.writers)

	// b.ResetTimer()

	for writer := 0; writer < t.writers; writer++ {
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerProducer; i++ {
				q <- Payload{}
			}
		}()
	}

	wg.Add(t.readers)
	for reader := 0; reader < t.readers; reader++ {
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerConsumer; i++ {
				<-q
			}
		}()
	}

	wg.Wait()
}

func BenchmarkZenQ_ProduceConsume(b *testing.B) {
	for _, t := range testCases {
		t := t
		b.Run(fmt.Sprintf("W%d/R%d/Size%d", t.writers, t.readers, t.inputSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchmarkProduceConsumeZenQ(b, t)
			}
		})
	}
}

func benchmarkProduceConsumeZenQ(b *testing.B, t test) {
	q := zenq.New[Payload](bufferSize)
	defer runtime.KeepAlive(q)

	writesPerProducer := t.inputSize / t.writers
	readsPerConsumer := t.inputSize / t.readers

	var wg sync.WaitGroup
	wg.Add(t.writers)

	// b.ResetTimer()

	for writer := 0; writer < t.writers; writer++ {
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerProducer; i++ {
				q.Write(Payload{})
			}
		}()
	}

	wg.Add(t.readers)
	for reader := 0; reader < t.readers; reader++ {
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerConsumer; i++ {
				q.Read()
			}
		}()
	}

	wg.Wait()
}

func BenchmarkChan_New(b *testing.B) {
	b.Run("struct{}", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ch := make(chan struct{}, bufferSize)
			runtime.KeepAlive(ch)
		}
	})
	b.Run("byte", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ch := make(chan byte, bufferSize)
			runtime.KeepAlive(ch)
		}
	})
	b.Run("int64", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ch := make(chan int64, bufferSize)
			runtime.KeepAlive(ch)
		}
	})
}

func BenchmarkZenQ_New(b *testing.B) {
	b.Run("struct{}", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			zq := zenq.New[struct{}](bufferSize)
			runtime.KeepAlive(zq)
		}
	})
	b.Run("byte", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			zq := zenq.New[byte](bufferSize)
			runtime.KeepAlive(zq)
		}
	})
	b.Run("int64", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			zq := zenq.New[int64](bufferSize)
			runtime.KeepAlive(zq)
		}
	})
}

func BenchmarkZenQ_BackgroundSelectWait(b *testing.B) {
	const N = 1e4
	q := zenq.New[struct{}](bufferSize)

	// create background waiters
	for i := 0; i < N; i++ {
		go func() {
			alt := zenq.New[struct{}](bufferSize)
			zenq.Select(q, alt)
		}()
	}

	b.ResetTimer()

	a := zenq.New[int](bufferSize)
	for i := 0; i < b.N; i++ {
		a.Write(i)
		runtime.Gosched()
		a.Read()
	}

	// release background waiters
	for i := 0; i < N; i++ {
		q.Write(struct{}{})
	}
}

func BenchmarkChan_BackgroundSelectWait(b *testing.B) {
	const N = 1e4
	q := make(chan struct{})

	// create background waiters
	for i := 0; i < N; i++ {
		go func() {
			x := make(chan struct{})
			select {
			case <-q:
			case <-x:
			}
		}()
	}

	b.ResetTimer()

	a := make(chan int, bufferSize)
	for i := 0; i < b.N; i++ {
		a <- i
		runtime.Gosched()
		<-a
	}

	// release background waiters
	for i := 0; i < N; i++ {
		q <- struct{}{}
	}
}
