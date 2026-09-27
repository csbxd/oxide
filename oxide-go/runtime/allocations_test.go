package oxide

import (
	"runtime"
	"testing"
)

// Check raw totals. AllocsPerRun divides integers and can hide occasional
// allocations when their count is smaller than the number of calls.
func requireNoGoAllocations(t testing.TB, calls int, run func()) {
	t.Helper()
	allocations, bytes := measureGoAllocations(calls, run)
	if allocations != 0 || bytes != 0 {
		t.Fatalf("%d warmed calls allocated %d Go objects / %d bytes", calls, allocations, bytes)
	}
}

func measureGoAllocations(calls int, run func()) (uint64, uint64) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	run()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range calls {
		run()
	}
	runtime.ReadMemStats(&after)
	return after.Mallocs - before.Mallocs, after.TotalAlloc - before.TotalAlloc
}

var allocationProbe []byte

func TestAllocationCounterDoesNotRound(t *testing.T) {
	calls := 0
	allocations, bytes := measureGoAllocations(100, func() {
		calls++
		if calls == 50 {
			allocationProbe = make([]byte, 32)
		}
	})
	if allocations != 1 || bytes != 32 {
		t.Fatalf("single allocation hidden or miscounted: %d objects / %d bytes", allocations, bytes)
	}
}
