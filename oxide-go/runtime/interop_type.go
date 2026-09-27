//go:build linux && (amd64 || arm64)

package oxide

import "unicode/utf8"

// Type describes a compiler-produced Rust type, never an inferred Go layout.
// Generated packages share one descriptor per Rust type identity.
// Treat generated descriptors and their slices as read-only.
type Type struct {
	ID                  int
	Name, Kind          string
	Size, Align, Pack   uintptr
	Sized               bool
	NeedsDrop           bool
	Elem                *Type
	Length              uintptr
	PointerKind         string
	Mutable             bool
	Fields              []Field
	Variants            []Variant
	SingleVariant       int
	Tag                 *EnumTag
	Container           *Container
	Drop                func(*Context, uintptr)
	Display, Debug      func(*Context, Value) Value
	JSON                func(*Context, Value) Value
	JSONValue           func(*Context, Value) Value
	FromJSON            func(*Context, Span) Value
	Iterate, IterateMut func(*Context, Value) Iterator
	Default             func(*Context) Value
}

type Field struct {
	Name   string
	Offset uintptr
	Type   *Type
	Public bool
}

type Variant struct {
	Name          string
	Fields        []Field
	Discriminant  U128
	Uninhabited   bool
	NonExhaustive bool
}

type EnumTag struct {
	Offset, Size          uintptr
	Encoding              string
	Start                 U128
	First, Last, Untagged int
}

// Container offsets refer to the Rust value's own storage. Length and capacity
// count elements, not bytes. Element comes from the compiler's type arguments.
type Container struct {
	Kind                                  string
	Element                               *Type
	Key, Value                            *Type
	DataOffset, LenOffset, CapacityOffset uintptr
	MetaOffset                            uintptr
	Metadata                              string
	GlobalAllocator                       bool
}

// Uninit reserves stable automatic storage. Zero bits are not a default Rust
// value: initialize it before reading or dropping it. Restore its Context frame
// only after all values stored there have been consumed or dropped.
func (t *Type) Uninit(ctx *Context) Value {
	t.checkSized()
	return Value{Addr: ctx.Alloc(t.Size, t.Align), Type: t}
}

func (t *Type) checkSized() {
	if t == nil || !t.Sized {
		panic("oxide: operation requires a sized Rust type")
	}
	if _, ok := storageSize(t.Size, t.Align); !ok {
		panic("oxide: invalid Rust type layout")
	}
}

// Enum initializes the named variant and consumes the supplied field values.
// The caller must not use or drop any copied ownership bits afterwards.
func (t *Type) Enum(ctx *Context, name string, fields ...Value) Value {
	t.checkSized()
	index := -1
	for i := range t.Variants {
		if t.Variants[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 || len(fields) != len(t.Variants[index].Fields) {
		panic("oxide: unknown Rust variant or wrong field count")
	}
	if t.Tag == nil && index != t.SingleVariant {
		panic("oxide: uninhabited Rust enum variant")
	}
	if t.Variants[index].Uninhabited {
		panic("oxide: uninhabited Rust enum variant")
	}
	variant := &t.Variants[index]
	for i, f := range variant.Fields {
		if !f.Public || f.Type != fields[i].Type || f.Type == nil || !f.Type.Sized {
			panic("oxide: inaccessible or mismatched Rust variant field")
		}
		checkRange(t.Size, f.Offset, f.Type.Size)
	}
	v := t.Uninit(ctx)
	for i, f := range variant.Fields {
		Value{Addr: addAddress(v.Addr, f.Offset), Type: f.Type}.Init(fields[i])
	}
	v.setVariant(index)
	return v
}

// String allocates an owned Rust String in automatic storage. Drop the value
// before restoring its Context frame. It does not borrow the Go string.
func (t *Type) String(ctx *Context, text string) Value {
	if !utf8.ValidString(text) {
		panic("oxide: Rust String requires valid UTF-8")
	}
	c := t.ownedContainer("string")
	if c.Element == nil || c.Element.Size != 1 || c.Element.Align != 1 {
		panic("oxide: invalid Rust String element layout")
	}
	v := t.newContainer(ctx, uintptr(len(text)))
	copy(v.byteSpanCapacity().Bytes(), text)
	v.putWord(c.LenOffset, uintptr(len(text)))
	return v
}

// Vec allocates capacity for elements but initializes none. Initialize each
// element with InitAt, then explicitly publish the initialized count with SetLen.
func (t *Type) Vec(ctx *Context, capacity uintptr) Value {
	t.ownedContainer("vec")
	return t.newContainer(ctx, capacity)
}

// Bytes constructs owned u8 storage without borrowing Go memory. String checks
// UTF-8; OsString and PathBuf preserve arbitrary Unix bytes, including non-UTF8.
func (t *Type) Bytes(ctx *Context, data []byte) Value {
	if t == nil || t.Container == nil {
		panic("oxide: Rust type is not a byte container")
	}
	kind := t.Container.Kind
	switch kind {
	case "string":
		if !utf8.Valid(data) {
			panic("oxide: Rust String requires valid UTF-8")
		}
	case "vec", "os_string", "path_buf":
	default:
		panic("oxide: Rust type is not an owned byte container")
	}
	c := t.ownedContainer(kind)
	if c.Element.Kind != "u8" || c.Element.Size != 1 || c.Element.Align != 1 {
		panic("oxide: Rust byte container element is not u8")
	}
	v := t.newContainer(ctx, uintptr(len(data)))
	copy(v.byteSpanCapacity().Bytes(), data)
	v.putWord(c.LenOffset, uintptr(len(data)))
	return v
}

func (t *Type) ownedContainer(kind string) *Container {
	t.checkSized()
	c := t.Container
	if c == nil || c.Kind != kind || !c.GlobalAllocator || c.Element == nil || !c.Element.Sized {
		panic("oxide: container construction requires its compiler-described global allocator")
	}
	for _, offset := range [...]uintptr{c.DataOffset, c.LenOffset, c.CapacityOffset} {
		checkRange(t.Size, offset, 8)
	}
	offsets := [...]uintptr{c.DataOffset, c.LenOffset, c.CapacityOffset}
	for i, a := range offsets {
		for _, b := range offsets[:i] {
			if a < b+8 && b < a+8 {
				panic("oxide: overlapping Rust container header fields")
			}
		}
	}
	c.Element.checkSized()
	return c
}

func (t *Type) newContainer(ctx *Context, capacity uintptr) Value {
	c := t.Container
	bytes := arrayBytes(capacity, c.Element.Size)
	mark := ctx.Mark()
	v := t.Uninit(ctx)
	p := c.Element.Align // An empty/ZST allocation uses a non-null aligned dangling pointer.
	storedCapacity := capacity
	if c.Element.Size == 0 {
		storedCapacity = 0
	} else if capacity != 0 {
		p = RustAlloc(bytes, c.Element.Align)
		if p == 0 {
			ctx.Restore(mark)
			panic("oxide: Rust container allocation failed")
		}
	}
	v.putWord(c.DataOffset, p)
	v.putWord(c.LenOffset, 0)
	v.putWord(c.CapacityOffset, storedCapacity)
	return v
}
