package zenq

import (
	"unsafe"
)

func NewThreadParker() *ThreadParker {
	return &ThreadParker{waitq: NewList()}
}

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by parking excess goroutines and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	waitq *List
}

// used for storing the goroutine pointer *g
func zenqParkCommit(gp, tp unsafe.Pointer) bool {
	ll := (*List)(tp)
	ll.Push(gp)
	return true
}

// Park parks the current calling goroutine
// This keeps only one parked goroutine in state at all times
// the parked goroutine is called with minimal overhead via goready() due to both being in userland
// This ensures there is no thundering herd https://en.wikipedia.org/wiki/Thundering_herd_problem
func (tp *ThreadParker) Park() {
	// gp := GetG()
	// casgstatus(gp, _Grunning, _Gwaiting)
	// tp.waitq.Push(gp)
	// FastPark()
	GoPark(zenqParkCommit, unsafe.Pointer(tp.waitq), waitReasonSleep, traceEvGoBlock, 1)
}

// Ready calls the parked goroutine if any and moves other goroutines up the queue
func (tp *ThreadParker) Ready() {
	if gp, ok := tp.waitq.Pop(); ok {
		GoReady(gp.(unsafe.Pointer), 1)
	}
}
