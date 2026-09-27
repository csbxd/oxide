//go:build linux && (amd64 || arm64)

package oxide

import (
	"unicode/utf8"
	"unsafe"
)

// Span is a byte range in stable Rust storage, not a Rust slice or str layout.
// Generated typed adapters place it in the compiler's Rust slice or str ABI.
// A Span never owns or frees its storage.
type Span struct {
	Data uintptr
	Len  uintptr
}

// CopyBytes copies Go data into the Context's automatic storage. The returned
// range stays valid until its frame is restored or the Context is closed.
// Even an empty range has the non-null pointer required by Rust slices.
func (c *Context) CopyBytes(value []byte) Span {
	p := c.Alloc(uintptr(len(value)), 1)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(value)), value)
	return Span{Data: p, Len: uintptr(len(value))}
}

// CopyString copies valid UTF-8 into stable automatic storage for a Rust str.
// Invalid UTF-8 panics before changing the Context frame. Unlike a Go pointer
// converted to uintptr, this address remains valid across Go stack movement.
func (c *Context) CopyString(value string) Span {
	if !utf8.ValidString(value) {
		panic("oxide: Rust str requires valid UTF-8")
	}
	p := c.Alloc(uintptr(len(value)), 1)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(value)), value)
	return Span{Data: p, Len: uintptr(len(value))}
}

func (s Span) length() int {
	if s.Len > ^uintptr(0)>>1 || s.Data > ^uintptr(0)-s.Len {
		panic("oxide: byte span exceeds address space")
	}
	if s.Data == 0 && s.Len != 0 {
		panic("oxide: null byte span")
	}
	return int(s.Len)
}

// Bytes borrows the span without copying. The caller must keep its Rust owner
// or Context frame alive and respect Rust's mutability and aliasing rules.
// A mutable view of str/String bytes must also preserve UTF-8 validity.
func (s Span) Bytes() []byte {
	n := s.length()
	if n == 0 {
		// A valid empty Rust slice may use a dangling address such as 1.
		// Never expose that address as a pointer scanned by Go's stack mover.
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(s.Data)), n)
}

// String borrows the bytes as a Go string without copying or UTF-8 validation.
// Do not mutate or release their storage while the returned string is in use.
func (s Span) String() string {
	n := s.length()
	if n == 0 {
		return ""
	}
	return unsafe.String((*byte)(unsafe.Pointer(s.Data)), n)
}
