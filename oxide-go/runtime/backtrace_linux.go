//go:build linux && (amd64 || arm64)

package oxide

import (
	"runtime"
	"unsafe"
)

type unwindFrame struct{ pc, stackOffset uintptr }
type UnwindTraceCallback func(*Context, uintptr, uintptr, uintptr) uint32

// captureFrames walks Go's physical frame-pointer chain without calling Go or
// allowing a stack move while it reads that chain. Both supported Go targets
// store the previous FP and return PC in consecutive words (runtime/tracestack.go).
//
//go:noescape
func captureFrames(dst *unwindFrame, capacity uintptr) uintptr

// frameStackPointer converts a captured offset relative to g.stack.hi into the
// current stack address. Go can relocate the stack between callback invocations.
//
//go:noescape
func frameStackPointer(offset uintptr) uintptr

// UnwindGetCFA resolves the saved offset in assembly so reading stack.hi and
// returning the frame address cannot be separated by Go stack relocation.
//
//go:noescape
func UnwindGetCFA(c *Context, frame uintptr) uintptr

// UnwindBacktrace exposes the executable's real Go code addresses. Rust source
// symbol/line mapping is a separate compiler debug-info task. Frame stack
// addresses follow Go stack relocations, but are not pointers into Rust's
// off-heap automatic storage. Frame contexts belong to this goroutine and call.
func UnwindBacktrace(c *Context, descriptor, data uintptr, invoke UnwindTraceCallback) uint32 {
	mark := c.Mark()
	defer c.Restore(mark)
	capacity := uintptr(64)
	var frames uintptr
	var n uintptr
	for {
		frames = c.Alloc(capacity*16, 8)
		n = captureFrames((*unwindFrame)(unsafe.Pointer(frames)), capacity)
		if n < capacity {
			break
		}
		c.Restore(mark)
		capacity *= 2
	}
	for i := uintptr(0); i < n; i++ {
		frame := frames + i*16
		pc := (*unwindFrame)(unsafe.Pointer(frame)).pc
		f := runtime.FuncForPC(pc - 1)
		// Skip this runtime boundary and its generated ABI wrapper. The
		// remaining PCs are the translated and calling Go frames themselves.
		if f != nil && (f.Name() == "github.com/csbxd/oxide/oxide-go/runtime.UnwindBacktrace" || f.Name() == "github.com/csbxd/oxide/oxide-go/runtime.captureFrames") {
			continue
		}
		result := invoke(c, descriptor, frame, data)
		if c.Failed() {
			Abort()
		}
		if result != 0 {
			return result
		}
	}
	return 5 // _URC_END_OF_STACK
}

func UnwindGetIP(_ *Context, frame uintptr) uintptr {
	return (*unwindFrame)(unsafe.Pointer(frame)).pc
}
func UnwindFindEnclosingFunction(_ *Context, pc uintptr) uintptr {
	if pc != 0 {
		if f := runtime.FuncForPC(pc - 1); f != nil {
			return f.Entry()
		}
	}
	return 0
}
