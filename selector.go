package zenq

import "runtime"

const uintMaxSize uint32 = 1<<32 - 1

// Selectable is an an interface for getting selected among many others
type Selectable interface {
	Check() (uint32, bool)
	Poll() (any, any)
}

// Select selects a single element out of multiple ZenQs
// The return value is determined by which ZenQ has the current least number of reads
// This ensures fairness and equal distribution of selection, and ensurses no single ZenQ starves
// TODO: remove polling implementation
func Select(streams ...Selectable) any {
	leastReads := uintMaxSize
	var mostDeserving Selectable

	for {
		for _, currStream := range streams {
			if numReads, ready := currStream.Check(); ready && numReads < leastReads {
				leastReads = numReads
				mostDeserving = currStream
			}
		}
		if mostDeserving != nil {
			val, _ := mostDeserving.Poll()
			return val
		}
		// No streams are ready for reading, context switch and then loop again after getting back the context
		// This is required for making this function non-blocking so that on single core systems the CPU doesnt get blocked
		runtime.Gosched()
	}
}
