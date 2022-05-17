package zenq

import (
	"sync/atomic"
	"unsafe"
)

func NewThreadParker() *ThreadParker {
	return &ThreadParker{sema: 1}
}

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	sema         uint32
	waiters      int64
	parkedThread unsafe.Pointer
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	atomic.AddInt64(&tp.waiters, 1)
	runtime_SemacquireMutex(&tp.sema, false, 1)
	gp := GetG()
	casgstatus(gp, _Grunning, _Gwaiting)
	atomic.StorePointer(&tp.parkedThread, gp)
	FastPark()
	runtime_Semrelease(&tp.sema, atomic.AddInt64(&tp.waiters, -1) > 0, 1)
}

// Ready calls the parked goroutine if any and moves other goroutines up the queue
func (tp *ThreadParker) Ready() bool {
	if g := atomic.SwapPointer(&tp.parkedThread, nil); g != nil {
		GoReady(g, 1)
		return true
	}
	return false
}
