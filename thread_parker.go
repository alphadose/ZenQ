package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	parkedThread unsafe.Pointer
	sync.Mutex
}

// used for storing the goroutine pointer *g
func zenqParkCommit(gp unsafe.Pointer, tp unsafe.Pointer) bool {
	obj := (*ThreadParker)(tp)
	atomic.StorePointer(&obj.parkedThread, gp)
	return true
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	tp.Lock()
	gopark(zenqParkCommit, unsafe.Pointer(tp), waitReasonSleep, traceEvGoBlock, 1)
	tp.Unlock()
}

// Ready calls the parked goroutine if any and moves other goroutines up the queue
func (tp *ThreadParker) Ready() {
	if g := atomic.SwapPointer(&tp.parkedThread, nil); g != nil {
		goready(g, 1)
	}
}
