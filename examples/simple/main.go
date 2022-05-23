package main

import (
	"fmt"
	"runtime"

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

	// For lowest latency and best performance, allocate the ZenQ.Read() calling goroutine an entire OS thread
	// by calling runtime.LockOSThread()
	// Note:- If you have a single core then doing this will cause a deadlock
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for i := 0; i < 100; i++ {
		if data, queueOpen := zq.Read(); queueOpen {
			fmt.Printf("%+v\n", data)
		}
	}
}
