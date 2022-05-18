package zenq

import (
	"unsafe"
	_ "unsafe"
)

// Linking ZenQ with golang internal runtime library will allow the usage of getg() and goready()
// function to schedule all goroutines without spinning
// use goparkunlock() to park a goroutine, then preempt it using goready()
// fetch the params *g used in goready() by using getg()
// with this there will be significant improvement in performance

// Alternative method is using assembly stubs to load the goroutine
// stack pointer as demonstrated in https://github.com/sitano/gsysint

type Mutex struct {
	// Futex-based impl treats it as uint32 key,
	// while sema-based impl as M* waitm.
	// Used to be a union, but unions break precise GC.
	key uintptr
}

// The functions below are used for scheduling goroutines with exclusive control
// Shifting to the below flow will remove the spinning and mutex lock implementations

//go:linkname Lock runtime.lock
func Lock(l *Mutex)

//go:linkname Nanotime runtime.nanotime
func Nanotime() int64

//go:linkname Unlock runtime.unlock
func Unlock(l *Mutex)

//go:linkname Goparkunlock runtime.goparkunlock
func Goparkunlock(lock *Mutex, reason waitReason, traceEv byte, traceskip int)

// func getg() any

func GetG() unsafe.Pointer

//go:linkname Fastrand runtime.fastrand
func Fastrand() uint32

//go:linkname Fastlog2 runtime.fastlog2
func Fastlog2(x float64) float64

//go:linkname GoReady runtime.goready
func GoReady(goroutinePtr unsafe.Pointer, traceskip int)

//go:linkname GoPark runtime.gopark
func GoPark(unlockf func(unsafe.Pointer, unsafe.Pointer) bool, lock unsafe.Pointer, reason waitReason, traceEv byte, traceskip int)

func Chanparkcommit(gp unsafe.Pointer, chanLock unsafe.Pointer) bool {
	// gp.activeStackChans = true
	// atomic.Store8(&gp.parkingOnChan, 0)
	// Make sure we unlock after setting activeStackChans and
	// unsetting parkingOnChan. The moment we unlock chanLock
	// we risk gp getting readied by a channel operation and
	// so gp could continue running before everything before
	// the unlock is visible (even to gp itself).
	// unlock((*mutex)(chanLock))
	return true
}

// Active spinning runtime support.
// runtime_canSpin reports whether spinning makes sense at the moment.
//go:linkname runtime_canSpin sync.runtime_canSpin
func runtime_canSpin(i int) bool

// runtime_doSpin does active spinning.
//go:linkname runtime_doSpin sync.runtime_doSpin
func runtime_doSpin()

//go:linkname runtime_nanotime sync.runtime_nanotime
func runtime_nanotime() int64

// Semacquire waits until *s > 0 and then atomically decrements it.
// It is intended as a simple sleep primitive for use by the synchronization
// library and should not be used directly.
//go:linkname runtime_Semacquire sync.runtime_Semacquire
func runtime_Semacquire(s *uint32)

// SemacquireMutex is like Semacquire, but for profiling contended Mutexes.
// If lifo is true, queue waiter at the head of wait queue.
// skipframes is the number of frames to omit during tracing, counting from
// runtime_SemacquireMutex's caller.
//go:linkname runtime_SemacquireMutex sync.runtime_SemacquireMutex
func runtime_SemacquireMutex(s *uint32, lifo bool, skipframes int)

// Semrelease atomically increments *s and notifies a waiting goroutine
// if one is blocked in Semacquire.
// It is intended as a simple wakeup primitive for use by the synchronization
// library and should not be used directly.
// If handoff is true, pass count directly to the first waiter.
// skipframes is the number of frames to omit during tracing, counting from
// runtime_Semrelease's caller.
//go:linkname runtime_Semrelease sync.runtime_Semrelease
func runtime_Semrelease(s *uint32, handoff bool, skipframes int)

//go:linkname goyield runtime.goyield
func goyield()

//go:linkname mcall runtime.mcall
func mcall(fn func(unsafe.Pointer))

//go:linkname park_m runtime.park_m
func park_m(gp unsafe.Pointer)

//go:linkname Readgstatus runtime.readgstatus
func Readgstatus(gp unsafe.Pointer) uint32

//go:linkname casgstatus runtime.casgstatus
func casgstatus(gp unsafe.Pointer, oldval, newval uint32)

//go:linkname dropg runtime.dropg
func dropg()

//go:linkname schedule runtime.schedule
func schedule()

//go:linkname mallocgc runtime.mallocgc
func mallocgc(size uintptr, typ unsafe.Pointer, needzero bool) unsafe.Pointer

//go:linkname sysFree runtime.sysFree
func sysFree(v unsafe.Pointer, n uintptr, sysStat unsafe.Pointer)

//go:linkname sysFreeOS runtime.sysFreeOS
func sysFreeOS(v unsafe.Pointer, n uintptr)

func fast_park(gp unsafe.Pointer) {
	dropg()
	casgstatus(gp, _Grunning, _Gwaiting)
	schedule()
}

func FastPark() {
	mcall(fast_park)
}

type waitReason uint8

const (
	waitReasonZero                  waitReason = iota // ""
	waitReasonGCAssistMarking                         // "GC assist marking"
	waitReasonIOWait                                  // "IO wait"
	waitReasonChanReceiveNilChan                      // "chan receive (nil chan)"
	waitReasonChanSendNilChan                         // "chan send (nil chan)"
	waitReasonDumpingHeap                             // "dumping heap"
	waitReasonGarbageCollection                       // "garbage collection"
	waitReasonGarbageCollectionScan                   // "garbage collection scan"
	waitReasonPanicWait                               // "panicwait"
	waitReasonSelect                                  // "select"
	waitReasonSelectNoCases                           // "select (no cases)"
	waitReasonGCAssistWait                            // "GC assist wait"
	waitReasonGCSweepWait                             // "GC sweep wait"
	waitReasonGCScavengeWait                          // "GC scavenge wait"
	waitReasonChanReceive                             // "chan receive"
	waitReasonChanSend                                // "chan send"
	waitReasonFinalizerWait                           // "finalizer wait"
	waitReasonForceGCIdle                             // "force gc (idle)"
	waitReasonSemacquire                              // "semacquire"
	waitReasonSleep                                   // "sleep"
	waitReasonSyncCondWait                            // "sync.Cond.Wait"
	waitReasonTimerGoroutineIdle                      // "timer goroutine (idle)"
	waitReasonTraceReaderBlocked                      // "trace reader (blocked)"
	waitReasonWaitForGCCycle                          // "wait for GC cycle"
	waitReasonGCWorkerIdle                            // "GC worker (idle)"
	waitReasonPreempted                               // "preempted"
	waitReasonDebugCall                               // "debug call"
)

// Event types in the trace, args are given in square brackets.
const (
	traceEvNone              = 0  // unused
	traceEvBatch             = 1  // start of per-P batch of events [pid, timestamp]
	traceEvFrequency         = 2  // contains tracer timer frequency [frequency (ticks per second)]
	traceEvStack             = 3  // stack [stack id, number of PCs, array of {PC, func string ID, file string ID, line}]
	traceEvGomaxprocs        = 4  // current value of GOMAXPROCS [timestamp, GOMAXPROCS, stack id]
	traceEvProcStart         = 5  // start of P [timestamp, thread id]
	traceEvProcStop          = 6  // stop of P [timestamp]
	traceEvGCStart           = 7  // GC start [timestamp, seq, stack id]
	traceEvGCDone            = 8  // GC done [timestamp]
	traceEvGCSTWStart        = 9  // GC STW start [timestamp, kind]
	traceEvGCSTWDone         = 10 // GC STW done [timestamp]
	traceEvGCSweepStart      = 11 // GC sweep start [timestamp, stack id]
	traceEvGCSweepDone       = 12 // GC sweep done [timestamp, swept, reclaimed]
	traceEvGoCreate          = 13 // goroutine creation [timestamp, new goroutine id, new stack id, stack id]
	traceEvGoStart           = 14 // goroutine starts running [timestamp, goroutine id, seq]
	traceEvGoEnd             = 15 // goroutine ends [timestamp]
	traceEvGoStop            = 16 // goroutine stops (like in select{}) [timestamp, stack]
	traceEvGoSched           = 17 // goroutine calls Gosched [timestamp, stack]
	traceEvGoPreempt         = 18 // goroutine is preempted [timestamp, stack]
	traceEvGoSleep           = 19 // goroutine calls Sleep [timestamp, stack]
	traceEvGoBlock           = 20 // goroutine blocks [timestamp, stack]
	traceEvGoUnblock         = 21 // goroutine is unblocked [timestamp, goroutine id, seq, stack]
	traceEvGoBlockSend       = 22 // goroutine blocks on chan send [timestamp, stack]
	traceEvGoBlockRecv       = 23 // goroutine blocks on chan recv [timestamp, stack]
	traceEvGoBlockSelect     = 24 // goroutine blocks on select [timestamp, stack]
	traceEvGoBlockSync       = 25 // goroutine blocks on Mutex/RWMutex [timestamp, stack]
	traceEvGoBlockCond       = 26 // goroutine blocks on Cond [timestamp, stack]
	traceEvGoBlockNet        = 27 // goroutine blocks on network [timestamp, stack]
	traceEvGoSysCall         = 28 // syscall enter [timestamp, stack]
	traceEvGoSysExit         = 29 // syscall exit [timestamp, goroutine id, seq, real timestamp]
	traceEvGoSysBlock        = 30 // syscall blocks [timestamp]
	traceEvGoWaiting         = 31 // denotes that goroutine is blocked when tracing starts [timestamp, goroutine id]
	traceEvGoInSyscall       = 32 // denotes that goroutine is in syscall when tracing starts [timestamp, goroutine id]
	traceEvHeapAlloc         = 33 // gcController.heapLive change [timestamp, heap_alloc]
	traceEvHeapGoal          = 34 // gcController.heapGoal (formerly next_gc) change [timestamp, heap goal in bytes]
	traceEvTimerGoroutine    = 35 // not currently used; previously denoted timer goroutine [timer goroutine id]
	traceEvFutileWakeup      = 36 // denotes that the previous wakeup of this goroutine was futile [timestamp]
	traceEvString            = 37 // string dictionary entry [ID, length, string]
	traceEvGoStartLocal      = 38 // goroutine starts running on the same P as the last event [timestamp, goroutine id]
	traceEvGoUnblockLocal    = 39 // goroutine is unblocked on the same P as the last event [timestamp, goroutine id, stack]
	traceEvGoSysExitLocal    = 40 // syscall exit on the same P as the last event [timestamp, goroutine id, real timestamp]
	traceEvGoStartLabel      = 41 // goroutine starts running with label [timestamp, goroutine id, seq, label string id]
	traceEvGoBlockGC         = 42 // goroutine blocks on GC assist [timestamp, stack]
	traceEvGCMarkAssistStart = 43 // GC mark assist start [timestamp, stack]
	traceEvGCMarkAssistDone  = 44 // GC mark assist done [timestamp]
	traceEvUserTaskCreate    = 45 // trace.NewContext [timestamp, internal task id, internal parent task id, stack, name string]
	traceEvUserTaskEnd       = 46 // end of a task [timestamp, internal task id, stack]
	traceEvUserRegion        = 47 // trace.WithRegion [timestamp, internal task id, mode(0:start, 1:end), stack, name string]
	traceEvUserLog           = 48 // trace.Log [timestamp, internal task id, key string id, stack, value string]
	traceEvCount             = 49
	// Byte is used but only 6 bits are available for event type.
	// The remaining 2 bits are used to specify the number of arguments.
	// That means, the max event type value is 63.
)

// defined constants
const (
	// G status
	//
	// Beyond indicating the general state of a G, the G status
	// acts like a lock on the goroutine's stack (and hence its
	// ability to execute user code).
	//
	// If you add to this list, add to the list
	// of "okay during garbage collection" status
	// in mgcmark.go too.
	//
	// TODO(austin): The _Gscan bit could be much lighter-weight.
	// For example, we could choose not to run _Gscanrunnable
	// goroutines found in the run queue, rather than CAS-looping
	// until they become _Grunnable. And transitions like
	// _Gscanwaiting -> _Gscanrunnable are actually okay because
	// they don't affect stack ownership.

	// _Gidle means this goroutine was just allocated and has not
	// yet been initialized.
	_Gidle = iota // 0

	// _Grunnable means this goroutine is on a run queue. It is
	// not currently executing user code. The stack is not owned.
	_Grunnable // 1

	// _Grunning means this goroutine may execute user code. The
	// stack is owned by this goroutine. It is not on a run queue.
	// It is assigned an M and a P (g.m and g.m.p are valid).
	_Grunning // 2

	// _Gsyscall means this goroutine is executing a system call.
	// It is not executing user code. The stack is owned by this
	// goroutine. It is not on a run queue. It is assigned an M.
	_Gsyscall // 3

	// _Gwaiting means this goroutine is blocked in the runtime.
	// It is not executing user code. It is not on a run queue,
	// but should be recorded somewhere (e.g., a channel wait
	// queue) so it can be ready()d when necessary. The stack is
	// not owned *except* that a channel operation may read or
	// write parts of the stack under the appropriate channel
	// lock. Otherwise, it is not safe to access the stack after a
	// goroutine enters _Gwaiting (e.g., it may get moved).
	_Gwaiting // 4

	// _Gmoribund_unused is currently unused, but hardcoded in gdb
	// scripts.
	_Gmoribund_unused // 5

	// _Gdead means this goroutine is currently unused. It may be
	// just exited, on a free list, or just being initialized. It
	// is not executing user code. It may or may not have a stack
	// allocated. The G and its stack (if any) are owned by the M
	// that is exiting the G or that obtained the G from the free
	// list.
	_Gdead // 6

	// _Genqueue_unused is currently unused.
	_Genqueue_unused // 7

	// _Gcopystack means this goroutine's stack is being moved. It
	// is not executing user code and is not on a run queue. The
	// stack is owned by the goroutine that put it in _Gcopystack.
	_Gcopystack // 8

	// _Gpreempted means this goroutine stopped itself for a
	// suspendG preemption. It is like _Gwaiting, but nothing is
	// yet responsible for ready()ing it. Some suspendG must CAS
	// the status to _Gwaiting to take responsibility for
	// ready()ing this G.
	_Gpreempted // 9

	// _Gscan combined with one of the above states other than
	// _Grunning indicates that GC is scanning the stack. The
	// goroutine is not executing user code and the stack is owned
	// by the goroutine that set the _Gscan bit.
	//
	// _Gscanrunning is different: it is used to briefly block
	// state transitions while GC signals the G to scan its own
	// stack. This is otherwise like _Grunning.
	//
	// atomicstatus&~Gscan gives the state the goroutine will
	// return to when the scan completes.
	_Gscan          = 0x1000
	_Gscanrunnable  = _Gscan + _Grunnable  // 0x1001
	_Gscanrunning   = _Gscan + _Grunning   // 0x1002
	_Gscansyscall   = _Gscan + _Gsyscall   // 0x1003
	_Gscanwaiting   = _Gscan + _Gwaiting   // 0x1004
	_Gscanpreempted = _Gscan + _Gpreempted // 0x1009
)

// Comparison before and after linking with this
// name                                     old time/op    new time/op    delta
// _ZenQ_NumWriters1_InputSize600-8           16.5µs ± 1%    17.9µs ± 1%   +8.65%  (p=0.000 n=28+29)
// _ZenQ_NumWriters3_InputSize60000-8         2.85ms ± 0%    2.67ms ± 6%   -6.11%  (p=0.000 n=23+30)
// _ZenQ_NumWriters8_InputSize6000000-8        417ms ± 0%     313ms ± 5%  -24.83%  (p=0.000 n=23+29)
// _ZenQ_NumWriters100_InputSize6000000-8      741ms ± 3%     516ms ± 2%  -30.40%  (p=0.000 n=29+30)
// _ZenQ_NumWriters1000_InputSize7000000-8     1.05s ± 1%     0.45s ± 9%  -57.58%  (p=0.000 n=28+30)
// _ZenQ_Million_Blocking_Writers-8            7.01s ±44%    10.98s ± 4%  +56.54%  (p=0.000 n=30+28)

// name                                     old alloc/op   new alloc/op   delta
// _ZenQ_NumWriters1_InputSize600-8            0.00B          0.00B          ~     (all equal)
// _ZenQ_NumWriters3_InputSize60000-8         28.9B ±111%    34.8B ±127%     ~     (p=0.268 n=30+29)
// _ZenQ_NumWriters8_InputSize6000000-8        885B ±163%     671B ±222%     ~     (p=0.208 n=30+30)
// _ZenQ_NumWriters100_InputSize6000000-8     16.2kB ±66%   13.3kB ±100%     ~     (p=0.072 n=30+30)
// _ZenQ_NumWriters1000_InputSize7000000-8    62.4kB ±82%    2.4kB ±210%  -96.20%  (p=0.000 n=30+30)
// _ZenQ_Million_Blocking_Writers-8           95.9MB ± 0%    95.5MB ± 0%   -0.41%  (p=0.000 n=28+30)

// name                                     old allocs/op  new allocs/op  delta
// _ZenQ_NumWriters1_InputSize600-8             0.00           0.00          ~     (all equal)
// _ZenQ_NumWriters3_InputSize60000-8           0.00           0.00          ~     (all equal)
// _ZenQ_NumWriters8_InputSize6000000-8        2.07 ±142%     1.40 ±186%     ~     (p=0.081 n=30+30)
// _ZenQ_NumWriters100_InputSize6000000-8       53.5 ±50%     31.8 ±100%  -40.60%  (p=0.000 n=30+30)
// _ZenQ_NumWriters1000_InputSize7000000-8       525 ±39%        6 ±227%  -98.95%  (p=0.000 n=30+30)
// _ZenQ_Million_Blocking_Writers-8            1.00M ± 0%     0.99M ± 0%   -0.41%  (p=0.000 n=28+29)
