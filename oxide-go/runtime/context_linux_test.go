//go:build linux && (amd64 || arm64)

package oxide

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestContextRustStackDoesNotMove(t *testing.T) {
	c := NewContext()
	defer c.Close()
	m := c.Mark()
	p := c.Alloc(16, 16)
	if p%16 != 0 {
		t.Fatalf("address %x is not aligned", p)
	}
	*(*uintptr)(unsafe.Pointer(p)) = p
	*(*uint64)(unsafe.Pointer(p + 8)) = 0x123456789abcdef0
	// A new goroutine starts with a small Go stack. Force it to grow while
	// keeping the Rust address alive only as an integer, as generated MIR does.
	done := make(chan bool)
	go func() {
		done <- rustStorageSurvivesStackGrowth(p)
	}()
	if !<-done {
		t.Fatal("Rust storage changed or the Go stack did not move")
	}
	c.Restore(m)
	if c.offset != m.offset || c.segment != m.segment {
		t.Fatal("context restore did not release the frame")
	}
	if q := c.Alloc(16, 16); q != p {
		t.Fatalf("restored frame was not reused: %#x != %#x", q, p)
	}
}

// checkptr=2 forces stack pointers used by unsafe operations onto the heap.
// Disable it only for the marker so the test still exercises a moving stack.
//
//go:nocheckptr
//go:noinline
func rustStorageSurvivesStackGrowth(p uintptr) bool {
	var marker byte
	before := uintptr(unsafe.Pointer(&marker))
	ok := growGoStack(128, p)
	after := uintptr(unsafe.Pointer(&marker))
	return ok && before != after
}

//go:noinline
func growGoStack(depth int, p uintptr) bool {
	var pad [4096]byte
	pad[0], pad[len(pad)-1] = byte(depth), byte(depth^0xff)
	ok := true
	if depth > 0 {
		ok = growGoStack(depth-1, p)
	} else {
		runtime.GC()
	}
	ok = ok && *(*uintptr)(unsafe.Pointer(p)) == p && *(*uint64)(unsafe.Pointer(p + 8)) == 0x123456789abcdef0
	ok = ok && pad[0] == byte(depth) && pad[len(pad)-1] == byte(depth^0xff)
	runtime.KeepAlive(&pad)
	return ok
}

func TestContextNestedFrames(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := c.Alloc(8, 8)
	*(*uint64)(unsafe.Pointer(p)) = 42
	m := c.Mark()
	const size = 2 << 20
	q := c.Alloc(size, 1<<16)
	if q%(1<<16) != 0 || c.segment == m.segment {
		t.Fatal("large aligned frame did not grow the Rust stack")
	}
	*(*byte)(unsafe.Pointer(q)) = 1
	*(*byte)(unsafe.Pointer(q + size - 1)) = 2
	c.Restore(m)
	if *(*uint64)(unsafe.Pointer(p)) != 42 {
		t.Fatal("nested frame overwrote its caller")
	}
	requireNoGoAllocations(t, 100, func() {
		mark := c.Mark()
		defer c.Restore(mark)
		if r := c.Alloc(size, 1<<16); r != q {
			panic("nested frame was not reused")
		}
	})
}

func TestContextThreadLocalIsolation(t *testing.T) {
	a, b := NewContext(), NewContext()
	defer a.Close()
	defer b.Close()
	template := RustAlloc(16, 16)
	if template == 0 {
		t.Fatal("cannot allocate TLS template")
	}
	defer RustDealloc(template, 16, 16)
	*(*uint64)(unsafe.Pointer(template)) = 42
	p, q := a.ThreadLocal(template, 16, 16), b.ThreadLocal(template, 16, 16)
	if p == q || p%16 != 0 || q%16 != 0 {
		t.Fatal("TLS instances must be distinct and aligned")
	}
	*(*uint64)(unsafe.Pointer(p)) = 99
	if *(*uint64)(unsafe.Pointer(q)) != 42 || *(*uint64)(unsafe.Pointer(template)) != 42 {
		t.Fatal("TLS write changed another thread or its template")
	}
	requireNoGoAllocations(t, 100, func() {
		if a.ThreadLocal(template, 16, 16) != p {
			panic("TLS address changed")
		}
	})
}

func TestContextRejectsInvalidLayouts(t *testing.T) {
	c := NewContext()
	defer c.Close()
	for _, layout := range [][2]uintptr{{8, 0}, {8, 3}, {^uintptr(0), 1}, {1 << 63, 1}, {1, 1 << 63}} {
		for _, kind := range []string{"stack", "TLS", "static"} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("%s accepted size %d, align %d", kind, layout[0], layout[1])
					}
				}()
				switch kind {
				case "stack":
					c.Alloc(layout[0], layout[1])
				case "TLS":
					c.ThreadLocal(0, layout[0], layout[1])
				case "static":
					staticStorage(layout[0], layout[1])
				}
			}()
		}
	}
}
