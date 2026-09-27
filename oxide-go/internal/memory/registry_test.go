//go:build linux && (amd64 || arm64)

package memory

import (
	"fmt"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func checkRegistry(t *testing.T, a *Allocator, want int) {
	t.Helper()
	var previous uintptr
	count := 0
	for p := a.regs; p != 0; {
		pg := (*page)(unsafe.Pointer(p))
		if pg.prev != previous || count >= want {
			t.Fatalf("page registry: previous=%#x, stored=%#x, count=%d", previous, pg.prev, count)
		}
		previous, p = p, pg.next
		count++
	}
	if count != want {
		t.Fatalf("page registry has %d entries, want %d", count, want)
	}
}

func TestPageRegistryRemovalAndClose(t *testing.T) {
	var a Allocator
	defer a.Close()
	var pointers [64]uintptr
	for i := range pointers {
		p, err := a.UintptrMalloc(maxSlotSize + 1 + i*16)
		if err != nil {
			t.Fatal(err)
		}
		if p%mallocAllign != 0 {
			t.Fatalf("unaligned allocation %#x", p)
		}
		pointers[i] = p
	}
	checkRegistry(t, &a, len(pointers))
	// An odd stride visits every slot and removes head, tail and middle pages.
	for i := 0; i < len(pointers)/2; i++ {
		if err := a.UintptrFree(pointers[i*17%len(pointers)]); err != nil {
			t.Fatal(err)
		}
		checkRegistry(t, &a, len(pointers)-i-1)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if a != (Allocator{}) {
		t.Fatal("Close did not restore the allocator's zero value")
	}
	for _, p := range pointers {
		var resident byte
		_, _, errno := syscall.RawSyscall(syscall.SYS_MINCORE, p&^uintptr(pageMask), uintptr(osPageSize), uintptr(unsafe.Pointer(&resident)))
		if errno != syscall.ENOMEM {
			t.Fatalf("page %#x still mapped after Close: %v", p, errno)
		}
	}
}

func churnAllocatorPages() error {
	var a Allocator
	defer a.Close()
	var pointers [96]uintptr
	for round := 0; round < 4; round++ {
		for i := range pointers {
			size := 16 << (i % 13)
			p, err := a.UintptrMalloc(size)
			if err != nil {
				return err
			}
			if p%mallocAllign != 0 {
				return fmt.Errorf("unaligned allocation %#x", p)
			}
			pointers[i] = p
			*(*byte)(unsafe.Pointer(p)) = byte(i)
			*(*byte)(unsafe.Pointer(p + uintptr(size-1))) = byte(i + 1)
		}
		for i := range pointers {
			j := i * 17 % len(pointers)
			p, size := pointers[j], 16<<(j%13)
			if *(*byte)(unsafe.Pointer(p)) != byte(j) || *(*byte)(unsafe.Pointer(p + uintptr(size-1))) != byte(j+1) {
				return fmt.Errorf("corrupted allocation %d", j)
			}
			if err := a.UintptrFree(p); err != nil {
				return err
			}
		}
		if a.regs != 0 {
			return fmt.Errorf("empty allocator retained registered pages")
		}
	}
	return nil
}

func TestColdAllocatorHasNoGoHeap(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	// Each call constructs a fresh allocator; there is deliberately no warm-up.
	for i := 0; i < 3; i++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		err := churnAllocatorPages()
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatal(err)
		}
		allocations, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc
		if allocations != 0 || bytes != 0 {
			t.Fatalf("cold allocator: %d Go allocations, %d bytes", allocations, bytes)
		}
	}
}
