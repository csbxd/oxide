//go:build linux && (amd64 || arm64)

package oxide

import (
	"fmt"
	"syscall"
	"unsafe"

	"modernc.org/libc"
)

// Context owns Rust automatic storage whose address is observable. Addresses
// remain stable when the Go goroutine stack moves. A Context is used by one
// call chain at a time and represents one Rust thread, including its TLS.
// It does not own Rust heap allocations.
type Context struct {
	segments       [][]byte
	segment        int
	offset         uintptr
	panicValue     any
	threadLocals   map[uintptr]threadLocal
	libcTLS        *libc.TLS
	pthreadValues  map[uint32]pthreadValue
	cxaDestructors []threadDestructor
}
type threadLocal struct {
	address uintptr
	mapping []byte
}

type Mark struct {
	segment int
	offset  uintptr
}

func NewContext() *Context {
	c := &Context{segments: make([][]byte, 0, 4)}
	c.grow(1 << 20)
	return c
}

func (c *Context) Mark() Mark     { return Mark{c.segment, c.offset} }
func (c *Context) Restore(m Mark) { c.segment, c.offset = m.segment, m.offset }

func (c *Context) Alloc(size, align uintptr) uintptr {
	n, ok := storageSize(size, align)
	if !ok {
		panic("invalid Rust stack layout")
	}
	size = max(size, 1)
	for {
		data := c.segments[c.segment]
		base := uintptr(unsafe.Pointer(unsafe.SliceData(data)))
		off := c.offset + (-(base + c.offset) & (align - 1))
		if off <= uintptr(len(data)) && size <= uintptr(len(data))-off {
			c.offset = off + size
			return base + off
		}
		c.segment++
		c.offset = 0
		if c.segment == len(c.segments) {
			c.grow(max(n, 1<<20))
		}
	}
}

func (c *Context) grow(size uintptr) {
	b, err := syscall.Mmap(-1, 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		panic(fmt.Errorf("oxide Rust stack: %w", err))
	}
	c.segments = append(c.segments, b)
}

func (c *Context) Close() error {
	var first error
	c.closeThreadDestructors()
	if c.libcTLS != nil {
		c.libcTLS.Close()
	}
	for _, b := range c.segments {
		if err := syscall.Munmap(b); err != nil && first == nil {
			first = err
		}
	}
	for _, local := range c.threadLocals {
		if err := syscall.Munmap(local.mapping); err != nil && first == nil {
			first = err
		}
	}
	*c = Context{}
	return first
}

func (c *Context) ThreadLocal(template, size, align uintptr) uintptr {
	if local, ok := c.threadLocals[template]; ok {
		return local.address
	}
	n, ok := storageSize(size, align)
	if !ok {
		panic("invalid Rust TLS layout")
	}
	b, err := syscall.Mmap(-1, 0, int(n), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		panic(fmt.Errorf("oxide TLS mmap: %w", err))
	}
	base := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	p := (base + align - 1) &^ (align - 1)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), size), unsafe.Slice((*byte)(unsafe.Pointer(template)), size))
	if c.threadLocals == nil {
		c.threadLocals = make(map[uintptr]threadLocal)
	}
	c.threadLocals[template] = threadLocal{p, b}
	return p
}

func (c *Context) Fail(v any)     { c.panicValue = v }
func (c *Context) Failed() bool   { return c.panicValue != nil }
func (c *Context) TakePanic() any { v := c.panicValue; c.panicValue = nil; return v }
