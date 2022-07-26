package main

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
	_ "unsafe"
)

/*
#include <stdlib.h>
*/
import "C"

//go:linkname noescape runtime.noescape
func noescape(p unsafe.Pointer) unsafe.Pointer

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
func memclrNoHeapPointers(ptr unsafe.Pointer, n uintptr)

//go:linkname mallocgc runtime.mallocgc
func mallocgc(size uintptr, typ unsafe.Pointer, needzero bool) unsafe.Pointer

func alloc[T any](sample T, size uintptr) unsafe.Pointer {
	length := unsafe.Sizeof(sample) * size
	return mallocgc(length, nil, true)
}

func getIndexAt[T any](ptr unsafe.Pointer, offset uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(ptr) + offset)
}

type block struct {
	Data  int
	Kooky string
	Endy  float64
	// Last  *uint
}

func main() {
	// a := make([]int32, 0, 3)
	// a = append(a, 10, 20, 30)
	// t := unsafe.Pointer(&a[0])
	// fmt.Println(*(*int32)(unsafe.Pointer(uintptr(t) + 2*unsafe.Sizeof(int32(0)))))
	// return

	const n = uintptr(100)
	t := make([]block, n, n)
	k := unsafe.Pointer(&t[0])
	// k := alloc(block{Data: 1, Kooky: "2", Endy: 3.2, Last: new(uint)}, n)
	// k := C.calloc(C.ulong(unsafe.Sizeof(block{})), C.ulong(n))
	// unsafe.Slice(k, n)
	// memclrNoHeapPointers(k, n)
	// t := (*[]block)(k)
	// runtime.KeepAlive((*[n]block)(k))
	// for i := uintptr(0); i < n; i++ {
	// 	slot := getIndexAt[block](k, i*unsafe.Sizeof(block{}))
	// 	slot.Data = int(i)
	// 	slot.Kooky = fmt.Sprintf("wutface%d", i)
	// 	slot.Endy = float64(i)
	// 	slot.Last = new(uint)
	// 	*slot.Last = uint(i)
	// }
	// for i := uintptr(0); i < n; i++ {
	// 	fmt.Printf("%#v\n", t[i])
	// }
	// return
	var wg sync.WaitGroup
	wg.Add(int(n))
	for i := uintptr(0); i < n; i++ {
		slot := unsafe.Pointer(uintptr(k) + i*unsafe.Sizeof(block{}))
		(*block)(slot).Data = int(i)
		(*block)(slot).Kooky = fmt.Sprintf("wutface%d", i)
		(*block)(slot).Endy = float64(i)
		// (*block)(slot).Last = new(uint)
		// *(*block)(slot).Last = uint(i)
	}
	for i := uintptr(0); i < n; i++ {
		j := i
		go func() {
			slot := unsafe.Pointer(uintptr(k) + j*unsafe.Sizeof(block{}))
			// *(*block)(slot).Last++
			fmt.Println(uintptr(slot), "  ", *(*block)(slot))
			runtime.GC()
			wg.Done()
		}()
	}
	wg.Wait()
}
