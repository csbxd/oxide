//go:build linux && (amd64 || arm64)

package oxide

import "unsafe"

// Generated packages are statically linked into one Go executable. Like the
// executable's crtbegin object, its DSO handle is a static self-address. Runtime
// TLS registrations retain it as the module cookie until their Context exits.
var dsoHandle = newDSOHandle()

func newDSOHandle() uintptr {
	p, _ := staticStorage(8, 8)
	*(*uintptr)(unsafe.Pointer(p)) = p
	return p
}

func DSOHandle() uintptr { return dsoHandle }
