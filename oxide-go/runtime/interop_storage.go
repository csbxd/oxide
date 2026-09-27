//go:build linux && (amd64 || arm64)

package oxide

// Storage owns only the allocation containing a Rust value. Field/Index views
// do not own that allocation and deliberately have no Free method.
type Storage struct {
	Value
	Size, Align uintptr
}

// HeapAlloc allocates uninitialized value storage independently of a Context.
// Initialize it before calling Drop. Free alone never runs a Rust destructor.
func (t *Type) HeapAlloc() Storage {
	t.checkSized()
	p := RustAlloc(t.Size, t.Align)
	if p == 0 {
		panic("oxide: Rust value storage allocation failed")
	}
	return Storage{Value: Value{Addr: p, Type: t}, Size: t.Size, Align: t.Align}
}

func (s *Storage) Free() {
	RustDealloc(s.Addr, s.Size, s.Align)
	*s = Storage{}
}

// Close consumes an initialized value and then frees its enclosing storage,
// including when its Rust destructor panics. Do not copy an owning Storage.
func (s *Storage) Close(ctx *Context) {
	defer s.Free()
	s.Value.Drop(ctx)
}
