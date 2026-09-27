//go:build memory.counters

package memory

import "testing"

func TestFailedMappingDoesNotBecomeLive(t *testing.T) {
	for _, size := range []int{16, maxSlotSize + 1} {
		var a Allocator
		a.FailNextMapping()
		if p, err := a.UintptrMalloc(size); p != 0 || err == nil {
			t.Fatalf("injected failure size=%d: pointer=%#x error=%v", size, p, err)
		}
		if a.Allocs != 0 || a.Mmaps != 0 || a.Bytes != 0 || a.regs != 0 {
			t.Fatalf("failed request changed live counters: %+v", a)
		}
		p, err := a.UintptrMalloc(size)
		if err != nil || p == 0 {
			t.Fatalf("retry failed: %v", err)
		}
		if a.Allocs != 1 || a.Mmaps != 1 || a.Bytes <= 0 {
			t.Fatalf("successful retry not counted: %+v", a)
		}
		if err := a.UintptrFree(p); err != nil {
			t.Fatal(err)
		}
		if a.Allocs != 0 || a.Mmaps != 0 || a.Bytes != 0 || a.regs != 0 {
			t.Fatalf("retry allocation did not drain: %+v", a)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
