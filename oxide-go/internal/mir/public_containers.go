package mir

import "fmt"

// Container operations use rustc's concrete offsets and element identity. They
// never infer a Vec/String representation or dispatch through type descriptors.
func (g *generator) emitStaticContainer(id int) {
	t := g.apiType(id)
	if t.Kind == "pointer" || t.Container != nil && t.Container.Kind == "box" {
		g.emitStaticPointer(id)
		return
	}
	name := g.apiName(id)
	c := t.Container
	data, length, capacity := "", "", ""
	element := -1
	owned := false
	stringType := t.Kind == "str"
	byteDST := t.Kind == "str" || c != nil && (c.Kind == "path" || c.Kind == "os_str")
	switch {
	case t.Kind == "array":
		data, length, element = "v.addr", fmt.Sprint(t.Length), t.Element
	case t.Kind == "slice":
		data, length, element = "v.addr", "v.meta", t.Element
	case byteDST:
		data, length = "v.addr", "v.meta"
	case c != nil && (c.Kind == "vec" || c.Kind == "string" || c.Kind == "path_buf" || c.Kind == "os_string"):
		owned = true
		element = c.Element
		g.checkStaticContainer(id)
		data = staticWord("v.addr", c.DataOffset)
		length = staticWord("v.addr", c.LenOffset)
		if g.apiType(element).Size == 0 {
			capacity = "^uintptr(0)"
		} else {
			capacity = staticWord("v.addr", c.CapacityOffset)
		}
		stringType = c.Kind == "string"
	default:
		return
	}
	for _, mutable := range []bool{false, true} {
		receiver := g.apiRefType(id, mutable)
		g.line("func(v %s) Len() uintptr { return %s }", receiver, length)
		if capacity != "" {
			g.line("func(v %s) Cap() uintptr { return %s }", receiver, capacity)
		}
		indexable := t.Kind == "array" || t.Kind == "slice" || owned && c.Kind == "vec"
		if indexable {
			e := g.apiType(element)
			if !e.Sized {
				g.fail("unsized Rust sequence element for %s", t.Name)
			}
			g.line("func(v %s) Index(index uintptr) %s {", receiver, g.apiRefType(element, mutable))
			g.line("if index>=v.Len(){panic(\"oxide: Rust index out of bounds\")}")
			address := fmt.Sprintf("oxide.AddAddress(%s,oxide.ArrayBytes(index,%d))", data, e.Size)
			g.line("return %s }", g.apiRefLiteral(element, mutable, address, "0"))
			if slice, ok := g.staticSliceType(element); ok {
				g.line("func(v %s) Slice() %s { return %s }", receiver, g.apiRefType(slice, mutable), g.apiRefLiteral(slice, mutable, data, "v.Len()"))
			}
		}
		bytes := byteDST || element >= 0 && g.apiType(element).Kind == "u8" && g.apiType(element).Size == 1
		if bytes {
			g.line("// Span borrows stable bytes; preserve this Rust borrow's mutability and lifetime.")
			g.line("func(v %s) Span() oxide.Span { return oxide.Span{Data:%s,Len:v.Len()} }", receiver, data)
			g.line("func(v %s) Bytes() []byte { return v.Span().Bytes() }", receiver)
			if stringType {
				g.line("func(v %s) String() string { return v.Span().String() }", receiver)
			}
		}
		if owned {
			kind := ""
			switch c.Kind {
			case "string":
				kind = "str"
			case "path_buf":
				kind = "path"
			case "os_string":
				kind = "os_str"
			}
			if borrowed, ok := g.staticByteDSTType(kind); ok {
				g.line("func(v %s) Borrow() %s { return %s }", receiver, g.apiRefType(borrowed, mutable), g.apiRefLiteral(borrowed, mutable, data, "v.Len()"))
			}
		}
	}
	if byteDST {
		g.line("// Borrow__%s borrows a byte span; the caller must preserve its lifetime and Rust aliasing rules.", name)
		g.line("func Borrow__%s(input oxide.Span) Ref__%s {", name, name)
		if t.Kind == "str" {
			g.line("if !utf8.Valid(input.Bytes()){panic(\"oxide: Rust str requires valid UTF-8\")}")
		} else {
			g.line("_ = input.Bytes()") // Validate the byte range even for non-UTF8 OS strings.
		}
		g.line("if input.Data==0 { if input.Len!=0 {panic(\"oxide: null Rust byte range\")}; input.Data=%d }", t.Align)
		g.line("return UnsafeRef__%s(input.Data,input.Len) }", name)
	}
	if !owned {
		return
	}
	if c.Kind == "vec" {
		g.line("// InitAt consumes one element into spare capacity; SetLen publishes initialized elements.")
		g.line("func(v Mut__%s) InitAt(index uintptr,rhs Value__%s) {", name, g.apiName(element))
		g.line("if index<v.Len() || index>=v.Cap(){panic(\"oxide: Rust Vec initialization is outside spare capacity\")}")
		address := fmt.Sprintf("oxide.AddAddress(%s,oxide.ArrayBytes(index,%d))", data, g.apiType(element).Size)
		g.line("target:=Mut__%s{addr:%s}; target.Init(rhs) }", g.apiName(element), address)
		g.line("// SetLen requires every new element initialized and every removed owner already consumed.")
		g.line("func(v Mut__%s) SetLen(length uintptr) { if length>v.Cap(){panic(\"oxide: invalid Rust Vec length\")};", name)
		g.staticPutWord("v.addr", c.LenOffset, "length")
		g.line("}")
	}
	if !c.GlobalAllocator {
		return
	}
	g.emitStaticContainerConstructors(id)
}

func staticWord(address string, offset uint64) string {
	return fmt.Sprintf("*(*uintptr)(unsafe.Pointer(%s+%d))", address, offset)
}

func (g *generator) staticPutWord(address string, offset uint64, value string) {
	g.line("*(*uintptr)(unsafe.Pointer(%s+%d))=uintptr(%s)", address, offset, value)
}

func (g *generator) checkStaticContainer(id int) {
	t := g.apiType(id)
	c := t.Container
	e := g.apiType(c.Element)
	if !t.Sized || !e.Sized || e.Align == 0 || e.Align&(e.Align-1) != 0 {
		g.fail("invalid Rust container layout for %s", t.Name)
	}
	offsets := [...]uint64{c.DataOffset, c.LenOffset, c.CapacityOffset}
	for i, offset := range offsets {
		if offset > t.Size || t.Size-offset < 8 {
			g.fail("Rust container header exceeds %s", t.Name)
		}
		for _, previous := range offsets[:i] {
			if offset < previous+8 && previous < offset+8 {
				g.fail("overlapping Rust container header fields for %s", t.Name)
			}
		}
	}
}

func (g *generator) emitStaticContainerConstructors(id int) {
	t := g.apiType(id)
	c := t.Container
	e := g.apiType(c.Element)
	name := g.apiName(id)
	g.line("func oxideNewContainer__%s(ctx *oxide.Context,capacity uintptr) Value__%s {", name, name)
	g.line("bytes:=oxide.ArrayBytes(capacity,%d); mark:=ctx.Mark(); out:=New__%s(ctx)", e.Size, name)
	g.line("data:=uintptr(%d); storedCapacity:=capacity", e.Align)
	if e.Size == 0 {
		g.line("_ = bytes; _ = mark; storedCapacity=0")
	} else {
		g.line("if capacity!=0 { data=oxide.RustAlloc(bytes,%d); if data==0 {ctx.Restore(mark);panic(\"oxide: Rust container allocation failed\")} }", e.Align)
	}
	g.staticPutWord("out.addr", c.DataOffset, "data")
	g.staticPutWord("out.addr", c.LenOffset, "0")
	g.staticPutWord("out.addr", c.CapacityOffset, "storedCapacity")
	g.line("return out }")
	if c.Kind == "vec" {
		g.line("// Vec__%s allocates spare capacity without initializing elements.", name)
		g.line("func Vec__%s(ctx *oxide.Context,capacity uintptr) Value__%s { return oxideNewContainer__%s(ctx,capacity) }", name, name, name)
	}
	if e.Kind != "u8" || e.Size != 1 || e.Align != 1 {
		if c.Kind != "vec" {
			g.fail("invalid Rust byte container element for %s", t.Name)
		}
		return
	}
	if c.Kind == "string" {
		g.line("func String__%s(ctx *oxide.Context,text string) Value__%s {", name, name)
		g.line("if !utf8.ValidString(text){panic(\"oxide: Rust String requires valid UTF-8\")}")
		g.line("out:=oxideNewContainer__%s(ctx,uintptr(len(text)))", name)
		g.line("copy((oxide.Span{Data:%s,Len:uintptr(len(text))}).Bytes(),text)", staticWord("out.addr", c.DataOffset))
		g.staticPutWord("out.addr", c.LenOffset, "uintptr(len(text))")
		g.line("return out }")
	}
	g.line("func Bytes__%s(ctx *oxide.Context,data []byte) Value__%s {", name, name)
	if c.Kind == "string" {
		g.line("if !utf8.Valid(data){panic(\"oxide: Rust String requires valid UTF-8\")}")
	}
	g.line("out:=oxideNewContainer__%s(ctx,uintptr(len(data)))", name)
	g.line("copy((oxide.Span{Data:%s,Len:uintptr(len(data))}).Bytes(),data)", staticWord("out.addr", c.DataOffset))
	g.staticPutWord("out.addr", c.LenOffset, "uintptr(len(data))")
	g.line("return out }")
}

func (g *generator) staticSliceType(element int) (int, bool) {
	id := -1
	for candidate := range g.apiTypes {
		t := g.apiType(candidate)
		if t.Kind == "slice" && g.apiIdentity(t.Element) == g.apiIdentity(element) {
			canonical := g.apiIdentity(candidate)
			if id < 0 || canonical < id {
				id = canonical
			}
		}
	}
	return id, id >= 0
}

func (g *generator) staticByteDSTType(kind string) (int, bool) {
	if kind == "" {
		return 0, false
	}
	id := -1
	for candidate := range g.apiTypes {
		t := g.apiType(candidate)
		if t.Kind == "str" && kind == "str" || !t.Sized && t.Container != nil && t.Container.Kind == kind {
			canonical := g.apiIdentity(candidate)
			if id < 0 || canonical < id {
				id = canonical
			}
		}
	}
	return id, id >= 0
}

func (g *generator) emitStaticPointer(id int) {
	t := g.apiType(id)
	name := g.apiName(id)
	element, offset, metaOffset := t.Pointee, uint64(0), uint64(0)
	box := t.Container != nil && t.Container.Kind == "box"
	if box {
		element = t.Container.Element
		offset = t.Container.DataOffset
	}
	e := g.apiType(element)
	if !e.Sized {
		c := t.Container
		if c == nil {
			g.fail("missing Rust pointer metadata for %s", t.Name)
		}
		metadata := c.Metadata
		switch c.Kind {
		case "str_ref", "slice_ref", "path_ref", "os_str_ref":
			metadata = "length"
		}
		if metadata != "length" && metadata != "vtable" {
			g.fail("missing Rust pointer metadata for %s", t.Name)
		}
		offset = c.DataOffset
		metaOffset = c.MetaOffset
		if !box && metadata == "length" {
			metaOffset = c.LenOffset
		}
	}
	if !t.Sized || offset > t.Size || t.Size-offset < 8 || !e.Sized && (metaOffset > t.Size || t.Size-metaOffset < 8) {
		g.fail("invalid Rust pointer storage layout for %s", t.Name)
	}
	for _, mutable := range []bool{false, true} {
		pointeeMutable := mutable && (box || t.Mutable)
		g.line("func(v %s) Deref() %s {", g.apiRefType(id, mutable), g.apiRefType(element, pointeeMutable))
		g.line("data:=%s; if data==0 {panic(\"oxide: null Rust pointer dereference\")}", staticWord("v.addr", offset))
		meta := "0"
		if !e.Sized {
			g.line("metadata:=%s", staticWord("v.addr", metaOffset))
			meta = "metadata"
		}
		g.line("out:=%s; if data%%out.Align()!=0 {panic(\"oxide: unaligned Rust pointer dereference\")}; return out }", g.apiRefLiteral(element, pointeeMutable, "data", meta))
	}
	if box {
		return // Borrowing a pointee cannot construct ownership of a Box.
	}
	g.line("// SetRef initializes a pointer header; it does not extend the pointee's lifetime.")
	g.line("func(v Mut__%s) SetRef(other %s) {", name, g.apiRefType(element, t.Mutable))
	g.line("if v.addr==0 || other.addr==0 {panic(\"oxide: null Rust pointer storage\")}")
	if t.PointerKind == "ref" {
		g.line("if other.addr%%other.Align()!=0 {panic(\"oxide: unaligned Rust reference\")}")
	}
	g.staticPutWord("v.addr", offset, "other.addr")
	if !e.Sized {
		g.staticPutWord("v.addr", metaOffset, "other.meta")
	}
	g.line("}")
}
