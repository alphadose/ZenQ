package main

import (
	"time"
	"unsafe"

	"github.com/alphadose/zenq"
)

type payload struct {
	alpha int
	beta  string
}

var p unsafe.Pointer

func main() {
	go func() {
		p = zenq.GetG()
		println(p)
		// l := &zenq.Mutex{}
		// zenq.Lock(l)
		// zenq.Goparkunlock(l, 19, 25, 1)
		zenq.FastPark()
		println("meow")
	}()
	time.Sleep(2 * time.Second)
	zenq.GoReady(p, 1)
	time.Sleep(2 * time.Second)
	// zq := zenq.New[payload]()

	// for j := 0; j < 5; j++ {
	// 	go func() {
	// 		for i := 0; i < 20; i++ {
	// 			zq.Write(payload{
	// 				alpha: i,
	// 				beta:  fmt.Sprint(i),
	// 			})
	// 		}
	// 	}()
	// }

	// // For lowest latency and best performance, allocate the ZenQ.Read() calling goroutine an entire OS thread
	// // by calling runtime.LockOSThread()
	// // Note:- If you have a single core then doing this will cause a deadlock
	// runtime.LockOSThread()
	// defer runtime.UnlockOSThread()

	// for i := 0; i < 100; i++ {
	// 	var data payload = zq.Read()
	// 	fmt.Printf("%+v\n", data)
	// }
}
