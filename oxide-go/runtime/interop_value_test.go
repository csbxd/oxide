//go:build linux && (amd64 || arm64)

package oxide

import "testing"

func interopScalar(kind string, size, align uintptr) *Type {
	return &Type{Name: kind, Kind: kind, Size: size, Align: align, Sized: true}
}

// These deliberately different header orders ensure the runtime consumes
// descriptor offsets. Actual Rust compiler layouts are tested by the MIR gate.
func interopContainers() (*Type, *Type) {
	u8 := interopScalar("u8", 1, 1)
	text := &Type{Name: "String", Kind: "aggregate", Size: 24, Align: 8, Sized: true,
		Container: &Container{Kind: "string", Element: u8, DataOffset: 16, LenOffset: 8, CapacityOffset: 0, GlobalAllocator: true}}
	text.Drop = func(_ *Context, address uintptr) {
		v := Value{Addr: address, Type: text}
		if v.Cap() != 0 {
			RustDealloc(v.word(16), v.Cap(), 1)
		}
	}
	vec := &Type{Name: "Vec<String>", Kind: "aggregate", Size: 24, Align: 8, Sized: true,
		Container: &Container{Kind: "vec", Element: text, DataOffset: 8, LenOffset: 16, CapacityOffset: 0, GlobalAllocator: true}}
	vec.Drop = func(ctx *Context, address uintptr) {
		v := Value{Addr: address, Type: vec}
		for i := uintptr(0); i < v.Len(); i++ {
			v.Index(i).Drop(ctx)
		}
		if v.Cap() != 0 {
			RustDealloc(v.word(8), v.Cap()*text.Size, text.Align)
		}
	}
	return text, vec
}

func interopMustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected invalid interop operation to panic")
		}
	}()
	fn()
}

func TestInteropOwnedContainers(t *testing.T) {
	c := NewContext()
	defer c.Close()
	text, vector := interopContainers()
	mark := c.Mark()
	requireNoGoAllocations(t, 100, func() {
		defer c.Restore(mark)
		v := vector.Vec(c, 3)
		for i, s := range [...]string{"雪", "", "hello\x00"} {
			v.InitAt(uintptr(i), text.String(c, s))
			v.SetLen(uintptr(i + 1))
		}
		if v.Len() != 3 || v.Cap() != 3 || v.Index(0).String() != "雪" || v.Index(1).String() != "" || v.Index(2).String() != "hello\x00" {
			t.Fatal("container content/layout mismatch")
		}
		v.Drop(c)
		storage := text.HeapAlloc()
		storage.Init(text.String(c, "persistent"))
		if storage.String() != "persistent" {
			t.Fatal("heap value initialization")
		}
		storage.Close(c)
		if storage.Addr != 0 {
			t.Fatal("freed storage retained owner address")
		}
	})
	if c.Mark() != mark {
		t.Fatal("frame not restored")
	}
	value := text.String(c, "copy survives Rust drop")
	copy := value.StringCopy()
	value.Drop(c)
	replacement := text.String(c, "replacement bytes")
	replacement.Drop(c)
	if copy != "copy survives Rust drop" {
		t.Fatal("StringCopy retained a Rust borrow")
	}
	mark = c.Mark()
	interopMustPanic(t, func() { text.String(c, "\xff") })
	if c.Mark() != mark {
		t.Fatal("bad UTF-8 consumed automatic storage")
	}
	other := *text
	other.Container = &Container{Kind: "string", Element: text.Container.Element, DataOffset: 16, LenOffset: 8, CapacityOffset: 0}
	interopMustPanic(t, func() { other.String(c, "custom allocator") })
}

func TestInteropPackedFieldsAndScalars(t *testing.T) {
	c := NewContext()
	defer c.Close()
	u64 := interopScalar("u64", 8, 8)
	packed := &Type{Kind: "aggregate", Sized: true, Size: 9, Align: 1, Pack: 1, Fields: []Field{{Name: "value", Offset: 1, Type: u64, Public: true}, {Name: "hidden", Type: u64}}}
	v := packed.Uninit(c)
	v.Field("value").SetUint(0x123456789abcdef0)
	if v.Field("value").Uint() != 0x123456789abcdef0 {
		t.Fatal("unaligned field access")
	}
	interopMustPanic(t, func() { v.Field("hidden") })
	for _, kind := range []string{"i8", "i16", "i32", "i64", "i128"} {
		size := map[string]uintptr{"i8": 1, "i16": 2, "i32": 4, "i64": 8, "i128": 16}[kind]
		x := interopScalar(kind, size, min(size, 8)).Uninit(c)
		x.SetInt(-7)
		if x.Int128() != I128From64(-7) {
			t.Fatalf("signed scalar %s", kind)
		}
	}
	x := interopScalar("u128", 16, 16).Uninit(c)
	want := U128{Lo: 3, Hi: 1 << 40}
	x.SetUint128(want)
	if x.Uint128() != want {
		t.Fatal("u128 scalar")
	}
	bit := interopScalar("bool", 1, 1).Uninit(c)
	bit.SetBool(true)
	if !bit.Bool() {
		t.Fatal("bool scalar")
	}
	f := interopScalar("f32", 4, 4).Uninit(c)
	f.SetFloat(1.25)
	if f.Float() != 1.25 {
		t.Fatal("float scalar")
	}
	r := interopScalar("char", 4, 4).Uninit(c)
	r.SetChar('雪')
	if r.Char() != '雪' {
		t.Fatal("char scalar")
	}
	interopMustPanic(t, func() { r.SetChar(0xd800) })
	alias := *u64
	interopMustPanic(t, func() { u64.Uninit(c).Init(alias.Uninit(c)) })
}

func TestInteropEnums(t *testing.T) {
	c := NewContext()
	defer c.Close()
	u8 := interopScalar("u8", 1, 1)
	niche := &Type{Kind: "aggregate", Size: 1, Align: 1, Sized: true, Tag: &EnumTag{Size: 1, Encoding: "niche", Start: U128{Lo: 255}, First: 0, Last: 1, Untagged: 2}, Variants: []Variant{{Name: "A"}, {Name: "B"}, {Name: "Some", Fields: []Field{{Name: "0", Type: u8, Public: true}}}}}
	x := u8.Uninit(c)
	x.SetUint(7)
	for _, name := range []string{"A", "B", "Some"} {
		var v Value
		if name == "Some" {
			v = niche.Enum(c, name, x)
		} else {
			v = niche.Enum(c, name)
		}
		if v.Variant() != name {
			t.Fatalf("niche variant %s", name)
		}
		if name == "Some" && v.Field("0").Uint() != 7 {
			t.Fatal("niche payload overwritten")
		}
	}
	direct := &Type{Kind: "aggregate", Size: 16, Align: 16, Sized: true, Tag: &EnumTag{Size: 16, Encoding: "direct"}, Variants: []Variant{{Name: "Negative", Discriminant: U128(I128From64(-7))}, {Name: "High", Discriminant: U128{Lo: 3, Hi: 1 << 40}}}}
	for _, name := range []string{"Negative", "High"} {
		if direct.Enum(c, name).Variant() != name {
			t.Fatal("wide enum tag")
		}
	}
	single := &Type{Kind: "aggregate", Size: 0, Align: 1, Sized: true, SingleVariant: 2, Variants: []Variant{{Name: "Dead0"}, {Name: "Dead1"}, {Name: "Live"}}}
	if single.Enum(c, "Live").Variant() != "Live" {
		t.Fatal("single variant index lost")
	}
	mark := c.Mark()
	interopMustPanic(t, func() { single.Enum(c, "Dead0") })
	if c.Mark() != mark {
		t.Fatal("invalid enum construction changed frame")
	}
}

func TestInteropDSTAndPointers(t *testing.T) {
	c := NewContext()
	defer c.Close()
	u64 := interopScalar("u64", 8, 8)
	slice := &Type{Kind: "slice", Elem: u64, Align: 8}
	outer := &Type{Kind: "aggregate", Align: 8, Fields: []Field{{Name: "tail", Offset: 8, Type: slice, Public: true}}}
	v := Value{Addr: c.Alloc(24, 8), Type: outer, Meta: 2}
	v.Field("tail").Index(1).SetUint(42)
	if v.Size() != 24 || v.Align() != 8 || v.Field("tail").Len() != 2 || v.Field("tail").Index(1).Uint() != 42 {
		t.Fatal("unsized slice tail")
	}
	ptr := &Type{Kind: "pointer", Sized: true, Size: 16, Align: 8, Elem: slice, Container: &Container{Kind: "slice_ref", Element: u64, DataOffset: 0, LenOffset: 8}}
	r := ptr.Uninit(c)
	r.SetRef(v.Field("tail"))
	if r.Deref().Index(1).Uint() != 42 || r.Len() != 2 {
		t.Fatal("fat reference")
	}
	parts := r.Elements(u64)
	if parts.Data != v.Addr+8 || parts.Len != 2 {
		t.Fatal("typed slice arguments lost data/element count")
	}
	interopMustPanic(t, func() { r.Elements(interopScalar("u64", 8, 8)) })
	vtable := c.Alloc(24, 8)
	writeInteger(vtable+8, 8, U128{Lo: 64})
	writeInteger(vtable+16, 8, U128{Lo: 64})
	dyn := &Type{Kind: "dynamic", Align: 1}
	dst := &Type{Kind: "aggregate", Align: 8, Fields: []Field{{Name: "tail", Offset: 8, Type: dyn, Public: true}}}
	v = Value{Addr: c.Alloc(128, 64), Type: dst, Meta: vtable}
	if v.Field("tail").Addr != v.Addr+64 || v.Size() != 128 || v.Align() != 64 {
		t.Fatal("dynamic tail alignment")
	}
	packed := *dst
	packed.Pack = 1
	packed.Align = 1
	v.Type = &packed
	if v.Field("tail").Addr != v.Addr+8 || v.Size() != 72 || v.Align() != 1 {
		t.Fatal("packed dynamic tail alignment")
	}
}

func TestInteropStorageDropAddressAndPanic(t *testing.T) {
	c := NewContext()
	defer c.Close()
	var got uintptr
	typ := &Type{Kind: "aggregate", Size: 64, Align: 64, Sized: true, Drop: func(ctx *Context, address uintptr) { got = address; ctx.Fail("drop panic") }}
	s := typ.HeapAlloc()
	address := s.Addr
	interopMustPanic(t, func() { s.Close(c) })
	if got != address || s.Addr != 0 || c.Failed() {
		t.Fatal("Drop moved receiver, leaked storage or retained panic")
	}
}

func TestInteropZSTVector(t *testing.T) {
	c := NewContext()
	defer c.Close()
	drops := 0
	element := &Type{Kind: "aggregate", Size: 0, Align: 64, Sized: true, Drop: func(_ *Context, address uintptr) {
		if address%64 != 0 {
			t.Fatal("unaligned ZST element")
		}
		drops++
	}}
	typ := &Type{Kind: "aggregate", Size: 24, Align: 8, Sized: true, Container: &Container{Kind: "vec", Element: element, DataOffset: 8, LenOffset: 16, CapacityOffset: 0, GlobalAllocator: true}}
	v := typ.Vec(c, ^uintptr(0))
	if v.Cap() != ^uintptr(0) || v.word(8) != 64 || v.word(0) != 0 {
		t.Fatal("ZST vector capacity/dangling representation")
	}
	for i := uintptr(0); i < 3; i++ {
		v.InitAt(i, element.Uninit(c))
		v.SetLen(i + 1)
	}
	for i := uintptr(0); i < v.Len(); i++ {
		v.Index(i).Drop(c)
	}
	v.SetLen(0)
	if drops != 3 {
		t.Fatal("ZST element destruction count")
	}
}

func TestInteropOSBytesAndBoxes(t *testing.T) {
	c := NewContext()
	defer c.Close()
	text, _ := interopContainers()
	path := *text
	layout := *text.Container
	layout.Kind = "path_buf"
	path.Container = &layout
	path.Drop = func(_ *Context, addr uintptr) {
		v := Value{Addr: addr, Type: &path}
		if v.Cap() > 0 {
			RustDealloc(v.word(layout.DataOffset), v.Cap(), 1)
		}
	}
	value := path.Bytes(c, []byte{'/', 0xff, 'x'})
	if value.Len() != 3 || value.Bytes()[1] != 0xff || value.Span().Len != 3 {
		t.Fatal("PathBuf lost Unix byte contents")
	}
	interopMustPanic(t, func() { value.String() })
	value.Drop(c)
	unsized := &Type{Kind: "aggregate", Align: 1, Container: &Container{Kind: "path", Element: layout.Element}}
	span := c.CopyBytes([]byte{'/', 0xfe})
	view := Value{Addr: span.Data, Type: unsized, Meta: span.Len}
	if view.Len() != 2 || view.Size() != 2 || view.Align() != 1 || view.Bytes()[1] != 0xfe {
		t.Fatal("unsized Path borrow")
	}
	box := &Type{Kind: "aggregate", Size: 24, Align: 8, Sized: true, Container: &Container{Kind: "box", Element: unsized, DataOffset: 16, MetaOffset: 0, Metadata: "length"}}
	boxed := box.Uninit(c)
	boxed.putWord(16, view.Addr)
	boxed.putWord(0, view.Meta)
	if boxed.Deref().Span() != span {
		t.Fatal("Box DST descriptor offsets")
	}
	interopMustPanic(t, func() { boxed.Deref().Drop(c) })
	missing := &Type{Kind: "aggregate", Size: 1, Align: 1, Sized: true, NeedsDrop: true}
	interopMustPanic(t, func() { missing.Uninit(c).Drop(c) })
}

func TestInteropReplaceZSTOwnersAndNonzeroAlias(t *testing.T) {
	c := NewContext()
	defer c.Close()
	drops := 0
	zero := &Type{Kind: "aggregate", Size: 0, Align: 8, Sized: true, NeedsDrop: true, Drop: func(*Context, uintptr) { drops++ }}
	// Two logical Rust ZST owners may share a non-null aligned dangling address.
	left := Value{Addr: 8, Type: zero}
	right := Value{Addr: 8, Type: zero}
	mark := c.Mark()
	requireNoGoAllocations(t, 100, func() {
		drops = 0
		left.Replace(c, right)
		if drops != 1 || c.Mark() != mark {
			t.Fatal("same-address ZST skipped the old destructor or leaked a frame")
		}
		left.Drop(c)
		if drops != 2 {
			t.Fatal("remaining ZST owner was not dropped once")
		}
	})
	nonzero := &Type{Kind: "aggregate", Size: 8, Align: 8, Sized: true, NeedsDrop: true, Drop: func(*Context, uintptr) { drops++ }}
	v := nonzero.Uninit(c)
	before := drops
	v.Replace(c, v)
	if drops != before {
		t.Fatal("nonzero self-alias replacement changed ownership")
	}
	v.Drop(c)
}
