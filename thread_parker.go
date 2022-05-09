package zenq

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// ThreadParker is a data-structure used for sleeping and waking up goroutines on user call
// useful for saving up resources by putting excess goroutines to sleep and pre-empt them when required with minimal latency overhead
type ThreadParker struct {
	parkedThread unsafe.Pointer
	sema         uint32
	sync.Mutex
}

func (tp *ThreadParker) Chanparkcommit(gp unsafe.Pointer, chanLock unsafe.Pointer) bool {
	// gp.activeStackChans = true
	// atomic.Store8(&gp.parkingOnChan, 0)
	// Make sure we unlock after setting activeStackChans and
	// unsetting parkingOnChan. The moment we unlock chanLock
	// we risk gp getting readied by a channel operation and
	// so gp could continue running before everything before
	// the unlock is visible (even to gp itself).
	// unlock((*mutex)(chanLock))
	// println("meow")
	tp.parkedThread = gp
	atomic.AddUint32(&tp.sema, 1)
	return true
}

// Park parks the current calling goroutine
// Edge Case:- when semaCount is 0, the first calling goroutine needs to call this twice to be parked
func (tp *ThreadParker) Park() {
	tp.Lock()
	// println("there2")
	Gopark(tp.Chanparkcommit, nil, waitReasonSleep, traceEvGoBlock, 1)
	// println("here")
	tp.Unlock()
	// println("final")
}

// Ready wakes up all sleeping goroutines associated with this ThreadParker object
// Underlying implementation depends on the OS, for linux its futex, for BSD/MacOS its sema_wakeup etc
func (tp *ThreadParker) Ready() {
start:
	ctr := atomic.LoadUint32(&tp.sema)
	if ctr > 0 {
		if atomic.CompareAndSwapUint32(&tp.sema, ctr, ctr-1) {
			// println("here2")
			goready(tp.parkedThread, 1)
			// println("exdee")
		} else {
			goto start
		}
	}

}
