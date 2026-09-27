//go:build linux && (amd64 || arm64)

package oxide

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

func TestRustAllocationAlignment(t *testing.T) {
	for _, align := range []uintptr{1, 2, 8, 16, 64, 4096, 1 << 16} {
		for _, size := range []uintptr{0, 1, 7, 1024, 4097} {
			p := RustAlloc(size, align)
			if p == 0 || p%align != 0 || p%16 != 0 {
				t.Fatalf("size %d align %d: got %#x", size, align, p)
			}
			b := unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
			for i, v := range b {
				if v != 0 {
					t.Fatalf("alloc_zeroed byte %d: %d", i, v)
				}
				b[i] = byte(i)
			}
			RustDealloc(p, size, align)
		}
	}
}

func BenchmarkRustAllocation(b *testing.B) {
	for _, layout := range [][2]uintptr{{32, 16}, {4096, 16}, {65536, 16}, {32, 65536}} {
		b.Run(fmt.Sprintf("size%d-align%d", layout[0], layout[1]), func(b *testing.B) {
			p := RustAlloc(layout[0], layout[1])
			RustDealloc(p, layout[0], layout[1])
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p := RustAlloc(layout[0], layout[1])
				if p == 0 {
					b.Fatal("allocation failed")
				}
				RustDealloc(p, layout[0], layout[1])
			}
		})
	}
}

func BenchmarkRustReallocation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := RustAlloc(32, 16)
		q := RustRealloc(p, 32, 16, 64)
		if q == 0 {
			b.Fatal("reallocation failed")
		}
		RustDealloc(q, 64, 16)
	}
}

func TestRustReallocPreservesBytes(t *testing.T) {
	for _, align := range []uintptr{1, 16, 1 << 16} {
		p := RustAlloc(128, align)
		if p == 0 {
			t.Fatal("allocation failed")
		}
		for i := uintptr(0); i < 128; i++ {
			*(*byte)(unsafe.Pointer(p + i)) = byte(i ^ 0xa5)
		}
		oldSize := uintptr(128)
		for _, size := range []uintptr{8192, 32} {
			q := RustRealloc(p, oldSize, align, size)
			if q == 0 || q%align != 0 {
				RustDealloc(p, oldSize, align)
				t.Fatalf("reallocation failed for align %d", align)
			}
			p, oldSize = q, size
			for i := uintptr(0); i < min(size, 128); i++ {
				if got := *(*byte)(unsafe.Pointer(p + i)); got != byte(i^0xa5) {
					t.Fatalf("realloc byte %d: %d", i, got)
				}
			}
		}
		if q := RustRealloc(p, oldSize, align, ^uintptr(0)); q != 0 {
			t.Fatal("oversized reallocation succeeded")
		}
		if *(*byte)(unsafe.Pointer(p)) != 0xa5 {
			t.Fatal("failed realloc changed the original allocation")
		}
		RustDealloc(p, oldSize, align)
	}
}

func TestRustAllocNoGoAllocs(t *testing.T) {
	requireNoGoAllocations(t, 100, func() {
		p := RustAlloc(32, 16)
		if p == 0 {
			panic("allocation failed")
		}
		q := RustRealloc(p, 32, 16, 64)
		if q == 0 {
			panic("reallocation failed")
		}
		RustDealloc(q, 64, 16)
	})
}

func TestRustAllocRejectsInvalidLayouts(t *testing.T) {
	const maxInt = ^uintptr(0) >> 1
	for _, layout := range [][2]uintptr{
		{8, 0}, {8, 3}, {^uintptr(0), 1}, {1 << 63, 1}, {1, 1 << 63}, {maxInt, 16},
		{maxInt, 1}, {maxInt - 16, 1}, {maxInt - 65536, 16}, {maxInt - 131072, 1}, {maxInt - (1 << 20), 1},
	} {
		if p := RustAlloc(layout[0], layout[1]); p != 0 {
			RustDealloc(p, layout[0], layout[1])
			t.Errorf("accepted size %d, align %d", layout[0], layout[1])
		}
	}
}

func TestRustReusedStorageIsZero(t *testing.T) {
	for _, layout := range [][2]uintptr{{0, 1}, {32, 1}, {4096, 16}, {65536, 16}, {32, 65536}, {(1 << 20) - 64, 16}} {
		size, align := layout[0], layout[1]
		p := RustAlloc(size, align)
		if p == 0 {
			t.Fatal("allocation failed")
		}
		for i := range unsafe.Slice((*byte)(unsafe.Pointer(p)), max(size, 1)) {
			*(*byte)(unsafe.Pointer(p + uintptr(i))) = 0xa5
		}
		RustDealloc(p, size, align)
		q := RustAlloc(size, align)
		if q != p {
			t.Fatalf("size %d align %d did not reuse its cached allocation", size, align)
		}
		for i, v := range unsafe.Slice((*byte)(unsafe.Pointer(q)), max(size, 1)) {
			if v != 0 {
				t.Fatalf("reused size %d align %d byte %d is %d", size, align, i, v)
			}
		}
		RustDealloc(q, size, align)
	}
}

func TestRustReallocClearsGrowth(t *testing.T) {
	p := RustAlloc(64, 16)
	if p == 0 {
		t.Fatal("allocation failed")
	}
	for i := range unsafe.Slice((*byte)(unsafe.Pointer(p)), 64) {
		*(*byte)(unsafe.Pointer(p + uintptr(i))) = 0x39
	}
	q := RustRealloc(p, 64, 16, 32)
	if q != p || allocationSize(q) != 32 {
		t.Fatal("shrink should retain the allocation and update its size")
	}
	q = RustRealloc(p, 32, 16, 64)
	if q != p || allocationSize(q) != 64 {
		t.Fatal("growth within capacity should retain the allocation")
	}
	for i, v := range unsafe.Slice((*byte)(unsafe.Pointer(q)), 64) {
		want := byte(0)
		if i < 32 {
			want = 0x39
		}
		if v != want {
			t.Fatalf("regrowth byte %d: %d, want %d", i, v, want)
		}
	}
	r := RustRealloc(q, 64, 16, 8192)
	if r == 0 {
		t.Fatal("larger reallocation failed")
	}
	for i, v := range unsafe.Slice((*byte)(unsafe.Pointer(r)), 8192) {
		want := byte(0)
		if i < 32 {
			want = 0x39
		}
		if v != want {
			t.Fatalf("moved regrowth byte %d: %d, want %d", i, v, want)
		}
	}
	RustDealloc(r, 8192, 16)
}

func TestRustAllocationConcurrentLiveRanges(t *testing.T) {
	var mu sync.Mutex
	live := make(map[uintptr]uintptr)
	var workers sync.WaitGroup
	for worker := range 12 {
		workers.Go(func() {
			for round := range 40 {
				var pointers, sizes, aligns [8]uintptr
				for i := range pointers {
					size := uintptr(17 + (worker*97+round*113+i*337)%8192)
					align := uintptr(1) << ((worker + round + i) % 17)
					p := RustAlloc(size, align)
					if p == 0 || p%align != 0 || p%16 != 0 {
						t.Errorf("concurrent allocation failed: size %d align %d address %#x", size, align, p)
						return
					}
					mu.Lock()
					for old, end := range live {
						if p < end && old < p+size {
							t.Errorf("live allocations overlap: [%#x,%#x) and [%#x,%#x)", p, p+size, old, end)
						}
					}
					live[p] = p + size
					mu.Unlock()
					for j := range unsafe.Slice((*byte)(unsafe.Pointer(p)), size) {
						*(*byte)(unsafe.Pointer(p + uintptr(j))) = byte(worker + i + 1)
					}
					pointers[i], sizes[i], aligns[i] = p, size, align
				}
				runtime.Gosched()
				for i, p := range pointers {
					for _, v := range unsafe.Slice((*byte)(unsafe.Pointer(p)), sizes[i]) {
						if v != byte(worker+i+1) {
							t.Errorf("concurrent allocation content changed for worker %d", worker)
							break
						}
					}
					mu.Lock()
					delete(live, p)
					mu.Unlock()
					RustDealloc(p, sizes[i], aligns[i])
				}
			}
		})
	}
	workers.Wait()
	if len(live) != 0 {
		t.Fatalf("%d live allocations remain", len(live))
	}
}

func TestHeapIdleRetentionBound(t *testing.T) {
	var a heapAllocator
	defer func() {
		if err := a.pages.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	for log := 6; log <= heapCacheMaxLog+2; log++ {
		var bases [4]uintptr
		for i := range bases {
			bases[i], _ = a.alloc(1 << log)
			if bases[i] == 0 {
				t.Fatal("allocation failed")
			}
		}
		for _, p := range bases {
			a.free(p, 1<<log)
		}
	}
	for log := 6; log <= heapCacheMaxLog; log++ {
		if a.spare[log] == 0 {
			t.Fatalf("size class %d was not retained", log)
		}
	}
	// The upstream counters are enabled by -tags=memory.counters. Run this
	// configuration too to verify actual retained mapping bytes, not payloads.
	if a.pages.Bytes >= 4<<20 {
		t.Fatalf("idle allocator retained %d bytes", a.pages.Bytes)
	}
	if a.pages.Allocs != 0 && a.pages.Allocs != heapCacheMaxLog-6+1 {
		t.Fatalf("idle allocator retained %d allocations", a.pages.Allocs)
	}
	if a.pages.Bytes != 0 {
		t.Logf("idle mapped bytes: %d; retained allocations: %d", a.pages.Bytes, a.pages.Allocs)
	}
}
