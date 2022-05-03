package main

import (
	"fmt"
	"runtime"

	_ "unsafe"

	"github.com/alphadose/zenq"
)

type payload struct {
	alpha int
	beta  string
}

// Event types in the trace, args are given in square brackets.
const (
	traceEvGoBlock = 20 // goroutine blocks [timestamp, stack]
)

type mutex struct {
	// Futex-based impl treats it as uint32 key,
	// while sema-based impl as M* waitm.
	// Used to be a union, but unions break precise GC.
	key uintptr
}

//go:linkname lock runtime.lock
func lock(l *mutex)

//go:linkname unlock runtime.unlock
func unlock(l *mutex)

//go:linkname goparkunlock runtime.goparkunlock
func goparkunlock(lock *mutex, reason string, traceEv byte, traceskip int)

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

	// For lowest latency and best performance, allocate the ZenQ.Read() calling goroutine an entire OS thread
	// by calling runtime.LockOSThread()
	// Note:- If you have a single core then doing this will cause a deadlock
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for i := 0; i < 100; i++ {
		var data payload = zq.Read()
		fmt.Printf("%+v\n", data)
	}
}
