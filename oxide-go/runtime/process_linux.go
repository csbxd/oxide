//go:build linux && (amd64 || arm64)

package oxide

import (
	"os"
	"strings"
	"sync"
	"unsafe"

	"modernc.org/libc"
)

// Retain the startup values even if the embedding Go main later edits os.Args.
var startupArguments = append([]string(nil), os.Args...)

var processArguments = sync.OnceValues(func() (int64, uintptr) {
	argc, argv := makeProcessArguments(startupArguments)
	startupArguments = nil
	return argc, argv
})

// ProcessArguments supplies the process-lifetime C argv used by Rust's Unix
// startup getter. Rust still copies and iterates arguments in its own MIR.
func ProcessArguments() (int64, uintptr) { return processArguments() }

func makeProcessArguments(args []string) (int64, uintptr) {
	const limit = ^uintptr(0) >> 1
	if uintptr(len(args)) > limit/8-1 {
		panic("oxide: too many process arguments")
	}
	size := uintptr(len(args)+1) * 8
	for _, arg := range args {
		if strings.IndexByte(arg, 0) >= 0 {
			panic("oxide: NUL in process argument")
		}
		if uintptr(len(arg)) >= limit-size {
			panic("oxide: process arguments too large")
		}
		size += uintptr(len(arg) + 1)
	}
	argv, storage := staticStorage(size, 8)
	offset := uintptr(len(args)+1) * 8
	for i, arg := range args {
		*(*uintptr)(unsafe.Pointer(argv + uintptr(i)*8)) = argv + offset
		copy(storage[offset:], arg)
		offset += uintptr(len(arg) + 1)
	}
	return int64(len(args)), argv
}

// GNU process exit runs the current thread's C++ TLS destructors, not pthread
// key destructors or Rust stack drops. Rust's rt::cleanup runs before this leaf.
func LibcExit(c *Context, code int32) {
	c.closeCxaDestructors()
	libc.Xexit(c.libc(), code)
}
