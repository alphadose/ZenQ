package main

import (
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
		ptr := zenq.GetG()
		println(ptr)
		zenq.Goparkunlock(&zenq.Mutex{}, 19, 25, 1)
		println("meow")
	}()
	zenq.GoReady(p, 1)
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
