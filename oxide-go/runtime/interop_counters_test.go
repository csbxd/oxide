//go:build memory.counters && linux && (amd64 || arm64)

package oxide

import "testing"

func TestInteropOwnershipCountsAndFailure(t *testing.T) {
	c := NewContext()
	defer c.Close()
	text, vector := interopContainers()
	baseline := HeapStats().LiveAllocations
	storage := text.HeapAlloc()
	storage.Init(text.String(c, "owned payload"))
	if live := HeapStats().LiveAllocations; live != baseline+2 {
		t.Fatalf("storage + String payload: live=%d baseline=%d", live, baseline)
	}
	storage.Drop(c)
	if live := HeapStats().LiveAllocations; live != baseline+1 {
		t.Fatal("Rust Drop unexpectedly freed outer storage or retained payload")
	}
	storage.Free()
	mark := c.Mark()
	requireNoGoAllocations(t, 100, func() {
		defer c.Restore(mark)
		v := vector.Vec(c, 2)
		v.InitAt(0, text.String(c, "first"))
		v.InitAt(1, text.String(c, "second"))
		v.SetLen(2)
		if HeapStats().LiveAllocations != baseline+3 {
			t.Fatal("vector and its two payloads were not observed")
		}
		v.Drop(c)
		if HeapStats().LiveAllocations != baseline {
			t.Fatal("container Drop leaked an owned allocation")
		}
	})
	// More than the largest spare class forces a mapping attempt, so this is
	// deterministic even with a warm allocator. A failed constructor owns none.
	rustHeap.mu.Lock()
	rustHeap.pages.FailNextMapping()
	rustHeap.mu.Unlock()
	before := HeapStats()
	interopMustPanic(t, func() { vector.Vec(c, (2<<20)/text.Size+1) })
	if after := HeapStats(); after != before || c.Mark() != mark {
		t.Fatalf("failed Vec constructor changed heap/frame: before=%+v after=%+v", before, after)
	}
	var drops int
	raw := &Type{Kind: "aggregate", Size: 8, Align: 8, Sized: true, Drop: func(*Context, uintptr) { drops++ }}
	s := raw.HeapAlloc()
	s.Free()
	if drops != 0 || HeapStats().LiveAllocations != baseline {
		t.Fatal("Free ran a destructor or retained storage")
	}
}
