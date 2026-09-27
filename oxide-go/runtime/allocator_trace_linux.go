//go:build oxide.heaptrace && memory.counters && linux && (amd64 || arm64)

package oxide

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// HeapTraceAllocation is an allocation created after StartHeapTrace which has
// not been freed. PCs are Go return PCs, resolved using runtime.CallersFrames.
// In-place realloc retains the original allocation stack and updates Size.
type HeapTraceAllocation struct {
	Address uintptr
	Size    uintptr
	Align   uintptr
	PCs     [64]uintptr
}

type heapTraceRecord struct {
	size, align uintptr
	stack       int
}

var heapTrace struct {
	enabled atomic.Bool
	mu      sync.Mutex
	live    map[uintptr]heapTraceRecord
	stacks  map[[64]uintptr]int
	pcs     [][64]uintptr
}

// StartHeapTrace discards previous records and tracks subsequent allocations.
// Call this at a quiescent boundary. Tracing deliberately allocates Go memory
// and is a diagnostic build feature, not an allocation-performance benchmark.
func StartHeapTrace() {
	heapTrace.mu.Lock()
	defer heapTrace.mu.Unlock()
	heapTrace.live = make(map[uintptr]heapTraceRecord)
	heapTrace.stacks = make(map[[64]uintptr]int)
	heapTrace.pcs = nil
	heapTrace.enabled.Store(true)
}

// StopHeapTrace freezes records. Call only after traced workers quiesce.
func StopHeapTrace() {
	heapTrace.mu.Lock()
	heapTrace.enabled.Store(false)
	heapTrace.mu.Unlock()
}

// HeapTraceSnapshot copies tracked live records without using the Rust heap.
func HeapTraceSnapshot() []HeapTraceAllocation {
	heapTrace.mu.Lock()
	defer heapTrace.mu.Unlock()
	result := make([]HeapTraceAllocation, 0, len(heapTrace.live))
	for p, r := range heapTrace.live {
		result = append(result, HeapTraceAllocation{p, r.size, r.align, heapTrace.pcs[r.stack]})
	}
	return result
}

func traceHeapAlloc(p, size, align uintptr) {
	if !heapTrace.enabled.Load() {
		return
	}
	var pcs [64]uintptr
	runtime.Callers(3, pcs[:])
	heapTrace.mu.Lock()
	defer heapTrace.mu.Unlock()
	if !heapTrace.enabled.Load() {
		return
	}
	stack, ok := heapTrace.stacks[pcs]
	if !ok {
		stack = len(heapTrace.pcs)
		heapTrace.stacks[pcs] = stack
		heapTrace.pcs = append(heapTrace.pcs, pcs)
	}
	heapTrace.live[p] = heapTraceRecord{size, align, stack}
}

func traceHeapFree(p uintptr) {
	if !heapTrace.enabled.Load() {
		return
	}
	heapTrace.mu.Lock()
	delete(heapTrace.live, p)
	heapTrace.mu.Unlock()
}

func traceHeapResize(p, size uintptr) {
	if !heapTrace.enabled.Load() {
		return
	}
	heapTrace.mu.Lock()
	if r, ok := heapTrace.live[p]; ok {
		r.size = size
		heapTrace.live[p] = r
	}
	heapTrace.mu.Unlock()
}
