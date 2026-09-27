package oxide

import "unsafe"

// RustException is an owned pointer to panic_unwind's actual Exception. Its
// payload, vtable, canary and cleanup all belong to translated Rust code.
type RustException unsafe.Pointer

func (c *Context) RaiseException(pointer uintptr) {
	if pointer == 0 || c.Failed() {
		Abort()
	}
	// A pointer is stored directly in an interface. Boxing uintptr here
	// would add a Go allocation to every Rust exception propagation.
	c.Fail(RustException(unsafe.Pointer(pointer)))
}

func (c *Context) TakeException() uintptr {
	v := c.TakePanic()
	e, ok := v.(RustException)
	if !ok {
		// A Go runtime fault or a diagnostic string cannot be handed to
		// Rust's Box<dyn Any> cleanup as though it were an exception object.
		panic(v)
	}
	return uintptr(unsafe.Pointer(e))
}
