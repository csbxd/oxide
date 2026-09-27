//go:build linux && (amd64 || arm64)

package oxide

import (
	"syscall"
	"unsafe"
)

func MonotonicTimespec() (int64, int32) {
	var ts syscall.Timespec
	const clockMonotonic = 1
	_, _, err := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME, clockMonotonic, uintptr(unsafe.Pointer(&ts)), 0)
	if err != 0 {
		panic("oxide: clock_gettime(CLOCK_MONOTONIC)")
	}
	return ts.Sec, int32(ts.Nsec)
}

func InstantElapsed(p uintptr) (uint64, uint32) {
	startSec := *(*int64)(unsafe.Pointer(p))
	startNS := int64(*(*uint32)(unsafe.Add(unsafe.Pointer(p), 8)))
	nowSec, nowNS := MonotonicTimespec()
	if nowSec < startSec || nowSec == startSec && int64(nowNS) < startNS {
		return 0, 0
	}
	sec, ns := nowSec-startSec, int64(nowNS)-startNS
	if ns < 0 {
		sec--
		ns += 1_000_000_000
	}
	return uint64(sec), uint32(ns)
}
