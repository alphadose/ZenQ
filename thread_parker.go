package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	StateEmpty uint32 = iota
	StateParked
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	parkedThread unsafe.Pointer
	sync.Mutex
}

func zenqParkCommit(gp unsafe.Pointer, tp unsafe.Pointer) bool {
	obj := (*ThreadParker)(tp)
	atomic.StorePointer(&obj.parkedThread, gp)
	return true
}

// Park parks the current calling goroutine
func (tp *ThreadParker) Park() {
	tp.Lock()
	Gopark(zenqParkCommit, unsafe.Pointer(tp), waitReasonSleep, traceEvGoBlock, 1)
	tp.Unlock()
}

// Ready calls the longest waiting time parked goroutine which in turns unblocks other writer goroutines
func (tp *ThreadParker) Ready() {
	if g := atomic.SwapPointer(&tp.parkedThread, nil); g != nil {
		goready(g, 1)
	}
}
