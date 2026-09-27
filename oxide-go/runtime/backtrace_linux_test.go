//go:build linux && (amd64 || arm64)

package oxide

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// checkptr=2 would heap-allocate the address marker and defeat this stack test.
//
//go:nocheckptr
//go:noinline
func backtraceCaller(c *Context, callback UnwindTraceCallback) uint32 {
	var local [128]byte
	local[127] = 17
	result := UnwindBacktrace(c, 0, uintptr(unsafe.Pointer(&local)), callback)
	runtime.KeepAlive(&local)
	return result
}

func TestBacktraceRealFrames(t *testing.T) {
	c := NewContext()
	defer c.Close()
	mark := c.Mark()
	count, found := 0, false
	result := backtraceCaller(c, func(c *Context, descriptor, frame, data uintptr) uint32 {
		pc, sp := UnwindGetIP(c, frame), UnwindGetCFA(c, frame)
		f := runtime.FuncForPC(pc - 1)
		if pc == 0 || sp == 0 || f == nil {
			t.Fatalf("invalid captured frame pc=%x sp=%x", pc, sp)
		}
		if UnwindFindEnclosingFunction(c, pc) != f.Entry() {
			t.Fatal("enclosing function mismatch")
		}
		if strings.HasSuffix(f.Name(), ".backtraceCaller") {
			found = true
			if data < sp || data-sp > 4096 {
				t.Fatalf("local=%x outside caller frame at %x", data, sp)
			}
		}
		count++
		return 0
	})
	if result != 5 || count < 2 || !found || c.Mark() != mark {
		t.Fatalf("result=%d count=%d caller=%v restored=%v", result, count, found, c.Mark() == mark)
	}
	count = 0
	result = backtraceCaller(c, func(*Context, uintptr, uintptr, uintptr) uint32 { count++; return 3 })
	if result != 3 || count != 1 {
		t.Fatalf("callback early exit = %d / %d", result, count)
	}
}

//go:noinline
func deepBacktrace(c *Context, depth int) uint32 {
	if depth > 0 {
		return deepBacktrace(c, depth-1)
	}
	count := uint32(0)
	UnwindBacktrace(c, 0, 0, func(*Context, uintptr, uintptr, uintptr) uint32 { count++; return 0 })
	return count
}

func TestBacktraceBeyondInitialBuffer(t *testing.T) {
	c := NewContext()
	defer c.Close()
	mark := c.Mark()
	if n := deepBacktrace(c, 200); n < 200 {
		t.Fatalf("truncated native frame chain: %d", n)
	}
	if c.Mark() != mark {
		t.Fatal("backtrace leaked Rust stack storage")
	}
}

//go:noinline
func growBacktraceStack(c *Context, frame uintptr, depth int) (uintptr, uintptr) {
	var pad [4096]byte
	pad[0], pad[len(pad)-1] = byte(depth), byte(depth^0xff)
	var top, sp uintptr
	if depth != 0 {
		top, sp = growBacktraceStack(c, frame, depth-1)
	} else {
		sp = UnwindGetCFA(c, frame)
		top = frameStackPointer(0)
	}
	runtime.KeepAlive(&pad)
	return top, sp
}

func TestBacktraceCFAFollowsStackGrowth(t *testing.T) {
	type result struct {
		beforeTop, beforeSP, grownTop, grownSP, afterTop, afterSP uintptr
	}
	done := make(chan result, 1)
	go func() {
		c := NewContext()
		defer c.Close()
		var got result
		UnwindBacktrace(c, 0, 0, func(c *Context, _ uintptr, frame uintptr, _ uintptr) uint32 {
			got.beforeSP = UnwindGetCFA(c, frame)
			got.beforeTop = frameStackPointer(0)
			got.grownTop, got.grownSP = growBacktraceStack(c, frame, 128)
			// GC is also allowed to shrink a stack after the deep calls return.
			runtime.GC()
			got.afterSP = UnwindGetCFA(c, frame)
			got.afterTop = frameStackPointer(0)
			return 3
		})
		done <- got
	}()
	got := <-done
	if got.beforeTop == got.grownTop {
		t.Fatal("test did not move the Go stack")
	}
	offset := got.beforeTop - got.beforeSP
	if offset == 0 || got.grownTop-got.grownSP != offset || got.afterTop-got.afterSP != offset {
		t.Fatalf("CFA did not follow stack relocation: %+v", got)
	}
}

func TestBacktraceCaptureCapacity(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := c.Alloc(48, 8)
	*(*uint64)(unsafe.Pointer(p + 32)) = 0x123456789abcdef0
	if n := captureFrames(nil, 0); n != 0 {
		t.Fatalf("zero-capacity capture returned %d frames", n)
	}
	if n := captureFrames((*unwindFrame)(unsafe.Pointer(p)), 2); n != 2 {
		t.Fatalf("capture returned %d frames, want 2", n)
	}
	if *(*uint64)(unsafe.Pointer(p + 32)) != 0x123456789abcdef0 {
		t.Fatal("capture wrote past its capacity")
	}
}

func TestBacktraceCallbackCannotUnwind(t *testing.T) {
	const child = "OXIDE_BACKTRACE_ABORT_TEST"
	if os.Getenv(child) == "1" {
		c := NewContext()
		UnwindBacktrace(c, 0, 0, func(c *Context, _, _, _ uintptr) uint32 {
			c.RaiseException(c.Alloc(64, 16))
			return 3 // Early termination must not hide a panic across the C ABI.
		})
		t.Fatal("callback panic did not abort")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBacktraceCallbackCannotUnwind$")
	cmd.Env = append(os.Environ(), child+"=1")
	err := cmd.Run()
	if status, ok := err.(*exec.ExitError); !ok || status.ExitCode() != 134 {
		t.Fatalf("callback panic: expected exit 134, got %v", err)
	}
}
