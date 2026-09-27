//go:build oxide.heaptrace && memory.counters && linux && (amd64 || arm64)

package oxide

import (
	"sync"
	"testing"
)

func TestHeapTraceOwnership(t *testing.T) {
	old := RustAlloc(64, 16)
	defer RustDealloc(old, 64, 16)
	StartHeapTrace()
	defer StopHeapTrace()
	p := RustAlloc(32, 16)
	initial := HeapTraceSnapshot()
	if len(initial) != 1 || initial[0].Address != p || initial[0].Size != 32 || initial[0].PCs[0] == 0 {
		t.Fatalf("initial trace: %+v", initial)
	}
	q := RustRealloc(p, 32, 16, 64)
	grown := HeapTraceSnapshot()
	if q != p || len(grown) != 1 || grown[0].Size != 64 || grown[0].PCs != initial[0].PCs {
		t.Fatalf("in-place realloc trace: %+v", grown)
	}
	r := RustRealloc(q, 64, 16, 4096)
	moved := HeapTraceSnapshot()
	if r == 0 || r == q || len(moved) != 1 || moved[0].Address != r || moved[0].Size != 4096 {
		t.Fatalf("moved realloc trace: %+v", moved)
	}
	RustDealloc(r, 4096, 16)
	if left := HeapTraceSnapshot(); len(left) != 0 {
		t.Fatalf("freed allocation still traced: %+v", left)
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				p := RustAlloc(32, 16)
				RustDealloc(p, 32, 16)
			}
		})
	}
	workers.Wait()
	if left := HeapTraceSnapshot(); len(left) != 0 {
		t.Fatalf("address reuse left traces: %+v", left)
	}
}
