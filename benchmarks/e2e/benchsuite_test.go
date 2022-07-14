package zenq_test

import (
	"sync"
	"testing"

	"github.com/alphadose/zenq/v2"
)

// wrapper for chan to have exactly the same api as zenq.
type Chan[T any] struct {
	ch chan T
}

func NewChan[T any]() Chan[T] {
	return Chan[T]{ch: make(chan T, bufferSize)}
}

func (ch Chan[T]) Read() T   { return <-ch.ch }
func (ch Chan[T]) Write(v T) { ch.ch <- v }

func BenchmarkChan_Suite(b *testing.B) {
	type Queue = Chan[int]
	ctor := NewChan[int]

	b.Run("Single", func(b *testing.B) {
		q := ctor()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q.Write(i)
			_ = q.Read()
		}
	})

	b.Run("Uncontended/x100", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			q := ctor()
			for pb.Next() {
				for i := 0; i < 100; i++ {
					q.Write(i)
					_ = q.Read()
				}
			}
		})
	})

	b.Run("Contended/x100", func(b *testing.B) {
		q := ctor()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				for i := 0; i < 100; i++ {
					q.Write(i)
					_ = q.Read()
				}
			}
		})
	})

	b.Run("Multiple/x100", func(b *testing.B) {
		const P = 1000
		qs := [P]Queue{}
		for i := range qs {
			qs[i] = ctor()
		}

		b.ResetTimer()

		var wg sync.WaitGroup
		wg.Add(P * 2)
		for i := 0; i < P; i++ {
			go func(q Queue) {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					var v int
					q.Write(v)
				}
			}(qs[i])
			go func(q Queue) {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					_ = q.Read()
				}

			}(qs[i])
		}
		wg.Wait()
	})

	b.Run("ProducerConsumer/x1", func(b *testing.B) {
		q := ctor()
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				var v int
				q.Write(v)
				work()
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				_ = q.Read()
				work()
			}
		}()
		wg.Wait()
	})

	b.Run("ProducerConsumer/x100", func(b *testing.B) {
		q := ctor()
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for i := 0; i < 100; i++ {
						q.Write(0)
						work()
					}
				}
			})
			wg.Done()
		}()

		go func() {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for i := 0; i < 100; i++ {
						_ = q.Read()
						work()
					}
				}
			})
			wg.Done()
		}()
		wg.Wait()
	})

	b.Run("PingPong/x1", func(b *testing.B) {
		q1 := ctor()
		q2 := ctor()
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			for i := 0; i < b.N; i++ {
				var v int
				q1.Write(v)
				work()
				_ = q2.Read()
			}
			wg.Done()
		}()

		go func() {
			for i := 0; i < b.N; i++ {
				var v int
				_ = q1.Read()
				work()
				q2.Write(v)
			}
			wg.Done()
		}()
		wg.Wait()
	})
}

func BenchmarkZenq_Suite(b *testing.B) {
	type Queue = zenq.ZenQ[int]
	ctor := zenq.New[int]

	b.Run("Single", func(b *testing.B) {
		q := ctor(bufferSize)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q.Write(i)
			_, _ = q.Read()
		}
	})

	b.Run("Uncontended/x100", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			q := ctor(bufferSize)
			for pb.Next() {
				for i := 0; i < 100; i++ {
					q.Write(i)
					_, _ = q.Read()
				}
			}
		})
	})

	b.Run("Contended/x100", func(b *testing.B) {
		q := ctor(bufferSize)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				for i := 0; i < 100; i++ {
					q.Write(i)
					_, _ = q.Read()
				}
			}
		})
	})

	b.Run("Multiple/x100", func(b *testing.B) {
		const P = 1000
		qs := [P]*Queue{}
		for i := range qs {
			qs[i] = ctor(bufferSize)
		}

		b.ResetTimer()

		var wg sync.WaitGroup
		wg.Add(P * 2)
		for i := 0; i < P; i++ {
			go func(q *Queue) {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					var v int
					q.Write(v)
				}
			}(qs[i])
			go func(q *Queue) {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					_, _ = q.Read()
				}

			}(qs[i])
		}
		wg.Wait()
	})

	b.Run("ProducerConsumer/x1", func(b *testing.B) {
		q := ctor(bufferSize)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				var v int
				q.Write(v)
				work()
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				_, _ = q.Read()
				work()
			}
		}()
		wg.Wait()
	})

	b.Run("ProducerConsumer/x100", func(b *testing.B) {
		q := ctor(bufferSize)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for i := 0; i < 100; i++ {
						q.Write(0)
						work()
					}
				}
			})
			wg.Done()
		}()

		go func() {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for i := 0; i < 100; i++ {
						_, _ = q.Read()
						work()
					}
				}
			})
			wg.Done()
		}()
		wg.Wait()
	})

	b.Run("PingPong/x1", func(b *testing.B) {
		q1 := ctor(bufferSize)
		q2 := ctor(bufferSize)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			for i := 0; i < b.N; i++ {
				var v int
				q1.Write(v)
				work()
				_, _ = q2.Read()
			}
			wg.Done()
		}()

		go func() {
			for i := 0; i < b.N; i++ {
				var v int
				_, _ = q1.Read()
				work()
				q2.Write(v)
			}
			wg.Done()
		}()
		wg.Wait()
	})
}

//go:noinline
func work() {
	// really tiny amount of work
}
