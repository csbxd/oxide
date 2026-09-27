//go:build linux && (amd64 || arm64) && !(oxide.heaptrace && memory.counters)

package oxide

func traceHeapAlloc(p, size, align uintptr) {}
func traceHeapFree(p uintptr)               {}
func traceHeapResize(p, size uintptr)       {}
