//go:build linux && (amd64 || arm64)

package oxide

import (
	"math/bits"
	"sync"
	"unsafe"

	"github.com/csbxd/oxide/oxide-go/internal/memory"
)

// storageSize includes the padding needed to align a mapping. Go slices and
// Rust allocations both require sizes representable by a signed pointer.
func storageSize(size, align uintptr) (uintptr, bool) {
	const maxSize = ^uintptr(0) >> 1
	if align == 0 || align&(align-1) != 0 || align > maxSize {
		return 0, false
	}
	size = max(size, 1)
	if size > maxSize-(align-1) {
		return 0, false
	}
	return size + align - 1, true
}

const heapCacheMaxLog = 20

// memory.Allocator manages offheap slabs, but unmaps a slab when its last
// allocation is freed. Retain one allocation per power-of-two size class up to
// 1 MiB so a short-lived object does not repeatedly map/unmap an entire slab.
// With memory v1.11.0's 64 KiB slab alignment this retains less than 4 MiB when
// no user allocations remain. Larger allocations are returned immediately.
type heapAllocator struct {
	mu    sync.Mutex
	pages memory.Allocator
	spare [heapCacheMaxLog + 1]uintptr
}

var rustHeap heapAllocator

// The header remains offheap and separate from the user's layout. Its size is
// a multiple of 16, preserving memory.Allocator's minimum pointer alignment.
type allocationHeader struct {
	base     uintptr
	capacity uintptr
	size     uintptr
	_        uintptr
}

const allocationHeaderSize = unsafe.Sizeof(allocationHeader{})

func allocation(p uintptr) *allocationHeader {
	return (*allocationHeader)(unsafe.Pointer(p - allocationHeaderSize))
}

func allocationSize(p uintptr) uintptr { return allocation(p).size }

func allocationStorageSize(size, align uintptr) (uintptr, bool) {
	n, ok := storageSize(size, align)
	// memory adds its page header and rounds/aligned-maps in 64 KiB units.
	// Leave room for those additions before converting our size to int.
	if !ok || n > (^uintptr(0)>>1)-allocationHeaderSize-((2<<16)+64) {
		return 0, false
	}
	return n + allocationHeaderSize, true
}

func (a *heapAllocator) alloc(size uintptr) (base, capacity uintptr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if size <= 1<<heapCacheMaxLog {
		log := bits.Len64(uint64(size - 1))
		size = 1 << log
		if p := a.spare[log]; p != 0 {
			a.spare[log] = 0
			return p, size
		}
	}
	p, err := a.pages.UintptrMalloc(int(size))
	if err != nil {
		return 0, 0
	}
	return p, size
}

func (a *heapAllocator) free(base, capacity uintptr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if capacity <= 1<<heapCacheMaxLog {
		log := bits.Len64(uint64(capacity - 1))
		if a.spare[log] == 0 {
			a.spare[log] = base
			return
		}
	}
	if err := a.pages.UintptrFree(base); err != nil {
		panic("oxide: Rust allocation free failed")
	}
}

// Rust allocations are opaque byte storage to Go's collector. Clear reused
// storage as well as fresh storage, retaining the alloc_zeroed contract.
func RustAlloc(size, align uintptr) uintptr {
	n, ok := allocationStorageSize(size, align)
	if !ok {
		return 0
	}
	base, capacity := rustHeap.alloc(n)
	if base == 0 {
		return 0
	}
	p := (base + allocationHeaderSize + align - 1) &^ (align - 1)
	*allocation(p) = allocationHeader{base: base, capacity: capacity, size: size}
	clear(unsafe.Slice((*byte)(unsafe.Pointer(p)), max(size, 1)))
	traceHeapAlloc(p, size, align)
	return p
}

func RustDealloc(p, size, align uintptr) {
	if p == 0 {
		return
	}
	traceHeapFree(p)
	h := allocation(p)
	rustHeap.free(h.base, h.capacity)
}

func RustRealloc(p, oldSize, align, newSize uintptr) uintptr {
	if _, ok := allocationStorageSize(newSize, align); !ok {
		return 0
	}
	if p != 0 {
		h := allocation(p)
		if p%align == 0 && newSize <= h.capacity-(p-h.base) {
			if newSize > oldSize {
				clear(unsafe.Slice((*byte)(unsafe.Pointer(p+oldSize)), newSize-oldSize))
			}
			h.size = newSize
			traceHeapResize(p, newSize)
			return p
		}
	}
	q := RustAlloc(newSize, align)
	if q == 0 {
		return 0
	}
	if p != 0 {
		n := min(oldSize, newSize)
		copy(unsafe.Slice((*byte)(unsafe.Pointer(q)), n), unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	}
	RustDealloc(p, oldSize, align)
	return q
}
