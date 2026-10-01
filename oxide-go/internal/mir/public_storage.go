package mir

// Storage handles contain stable Rust addresses, never Go copies of the Rust
// layout. Each canonical Rust type gets distinct Go types; layout-equivalent
// owners must not share an API or a destructor.
func (g *generator) emitStaticStorage(id int) {
	id = g.apiIdentity(id)
	t := g.apiType(id)
	name := g.apiName(id)
	metadata := ""
	arguments := "addr uintptr"
	values := "addr:addr"
	if !t.Sized {
		metadata = "; meta uintptr"
		arguments += ",meta uintptr"
		values += ",meta:meta"
	}
	for _, prefix := range []string{"Ref", "Mut"} {
		// A distinct zero-sized first field blocks ordinary Go conversions
		// between Rust identities or capabilities, without tail-ZST padding.
		g.line("type brand__%s__%s struct{}", prefix, name)
		g.line("type %s__%s struct { _ brand__%s__%s; addr uintptr%s }", prefix, name, prefix, name, metadata)
		size := 8
		if !t.Sized {
			size = 16
		}
		g.line("var _ [%d-int(unsafe.Sizeof(%s__%s{}))]byte", size, prefix, name)
		g.line("var _ [int(unsafe.Sizeof(%s__%s{}))-%d]byte", prefix, name, size)
		g.line("func (v %s__%s) Addr() uintptr { return v.addr }", prefix, name)
		if !t.Sized {
			g.line("func (v %s__%s) Meta() uintptr { return v.meta }", prefix, name)
		}
		g.line("// Unsafe%s__%s borrows caller-provided Rust storage; its lifetime and aliasing are the caller's responsibility.", prefix, name)
		g.line("func Unsafe%s__%s(%s) %s__%s {", prefix, name, arguments, prefix, name)
		g.line("if addr==0 { panic(\"oxide: null Rust storage\") }")
		if prefix == "Ref" && t.Sized && t.Align > 1 {
			g.line("if addr%%%d!=0 { panic(\"oxide: unaligned Rust reference\") }", t.Align)
		}
		if prefix == "Ref" && !t.Sized {
			g.line("v:=Ref__%s{%s}; if addr%%v.Align()!=0 { panic(\"oxide: unaligned Rust reference\") }; return v }", name, values)
		} else {
			g.line("return %s__%s{%s} }", prefix, name, values)
		}
	}
	refValues := "addr:v.addr"
	if !t.Sized {
		refValues += ",meta:v.meta"
	}
	g.line("func (v Mut__%s) Ref() Ref__%s {", name, name)
	g.line("if v.addr==0 { panic(\"oxide: null Rust storage\") }")
	if t.Sized && t.Align > 1 {
		g.line("if v.addr%%%d!=0 { panic(\"oxide: unaligned Rust reference\") }", t.Align)
	} else if !t.Sized {
		g.line("if v.addr%%v.Align()!=0 { panic(\"oxide: unaligned Rust reference\") }")
	}
	g.line("return Ref__%s{%s} }", name, refValues)
	if !t.Sized {
		return // An unsized borrow does not own its sized Box/Arc allocation.
	}
	const maxSize = uint64(1<<63 - 1)
	if t.Align == 0 || t.Align > maxSize || t.Align&(t.Align-1) != 0 || t.Size > maxSize-(t.Align-1) {
		g.fail("invalid public Rust storage layout for %s", t.Name)
	}
	g.line("// Value__%s owns one initialized Rust value after construction/Init; copying the handle never clones that owner.", name)
	g.line("type brand__Value__%s struct{}", name)
	g.line("type Value__%s struct { _ brand__Value__%s; addr uintptr }", name, name)
	g.line("var _ [8-int(unsafe.Sizeof(Value__%s{}))]byte", name)
	g.line("var _ [int(unsafe.Sizeof(Value__%s{}))-8]byte", name)
	g.line("func (v Value__%s) Addr() uintptr { return v.addr }", name)
	g.line("func (v Value__%s) Ref() Ref__%s { return Ref__%s{addr:v.addr} }", name, name, name)
	g.line("func (v Value__%s) Mut() Mut__%s { return Mut__%s{addr:v.addr} }", name, name, name)
	for _, prefix := range []string{"Ref", "Mut", "Value"} {
		g.line("func (v %s__%s) Size() uintptr { return %d }", prefix, name, t.Size)
		g.line("func (v %s__%s) Align() uintptr { return %d }", prefix, name, t.Align)
	}
	g.line("// New__%s reserves uninitialized automatic storage; initialize it before reading or dropping it.", name)
	g.line("func New__%s(ctx *oxide.Context) Value__%s { return Value__%s{addr:ctx.Alloc(%d,%d)} }", name, name, name, t.Size, t.Align)
	g.line("// UnsafeValue__%s adopts an initialized Rust value; the caller transfers ownership and must preserve its storage lifetime.", name)
	g.line("func UnsafeValue__%s(addr uintptr) Value__%s {", name, name)
	g.line("if addr==0 || addr%%%d!=0 { panic(\"oxide: invalid Rust owner address\") }; return Value__%s{addr:addr} }", t.Align, name)
	g.line("// Init consumes rhs into uninitialized storage; it never drops previous destination bits.")
	g.line("func (v Mut__%s) Init(rhs Value__%s) {", name, name)
	g.line("if v.addr==0 || rhs.addr==0 { panic(\"oxide: null Rust storage\") }")
	g.staticCopy("v.addr", "rhs.addr", t.Size)
	g.line("}")
	g.line("func (v Value__%s) Init(rhs Value__%s) { v.Mut().Init(rhs) }", name, name)
	g.line("// Move consumes this place into new aligned automatic storage, leaving the original place uninitialized.")
	g.line("func (v Mut__%s) Move(ctx *oxide.Context) Value__%s {", name, name)
	g.line("if v.addr==0 { panic(\"oxide: null Rust storage\") }")
	g.line("out:=New__%s(ctx)", name)
	g.staticCopy("out.addr", "v.addr", t.Size)
	g.line("return out }")
	drop := g.staticDropSymbol(id)
	g.line("// Drop consumes the initialized value, retaining its enclosing storage.")
	g.line("func (v Value__%s) Drop(ctx *oxide.Context) {", name)
	g.line("if v.addr==0 { panic(\"oxide: null Rust storage\") }")
	if drop != "" {
		g.line("%s(ctx,v.addr); if ctx.Failed(){panic(ctx.TakePanic())}", drop)
	}
	g.line("}")
	g.line("// Replace consumes rhs using Rust assignment semantics, including when the old destructor panics.")
	g.line("func (v Mut__%s) Replace(ctx *oxide.Context,rhs Value__%s) {", name, name)
	g.line("if v.addr==0 || rhs.addr==0 { panic(\"oxide: null Rust storage\") }")
	if t.Size != 0 {
		g.line("if v.addr==rhs.addr { return }")
	}
	g.line("mark:=ctx.Mark(); next:=New__%s(ctx); next.Init(rhs)", name)
	g.line("defer func(){v.Init(next);ctx.Restore(mark)}()")
	if drop != "" {
		g.line("old:=Value__%s{addr:v.addr}", name)
		if t.Align > 1 {
			g.line("if v.addr%%%d!=0 { old=v.Move(ctx) }", t.Align)
		}
		g.line("old.Drop(ctx)")
	}
	g.line("}")
	g.line("func (v Value__%s) Replace(ctx *oxide.Context,rhs Value__%s) { v.Mut().Replace(ctx,rhs) }", name, name)
	g.line("// Storage__%s owns only the enclosing heap slot; Free does not run a Rust destructor.", name)
	g.line("type Storage__%s struct { Value__%s }", name, name)
	g.line("func Alloc__%s() Storage__%s {", name, name)
	g.line("addr:=oxide.RustAlloc(%d,%d); if addr==0 { panic(\"oxide: Rust value storage allocation failed\") }", t.Size, t.Align)
	g.line("return Storage__%s{Value__%s:Value__%s{addr:addr}} }", name, name, name)
	g.line("func (s *Storage__%s) Free() { oxide.RustDealloc(s.addr,%d,%d); *s=Storage__%s{} }", name, t.Size, t.Align, name)
	g.line("// Close consumes the initialized value and frees its slot, also when its Rust destructor panics.")
	g.line("func (s *Storage__%s) Close(ctx *oxide.Context) { defer s.Free(); s.Value__%s.Drop(ctx) }", name, name)
	g.emitStaticScalar(id)
}

func (g *generator) staticCopy(destination, source string, size uint64) {
	if size != 0 {
		g.line("copy(unsafe.Slice((*byte)(unsafe.Pointer(%s)),%d),unsafe.Slice((*byte)(unsafe.Pointer(%s)),%d))", destination, size, source, size)
	}
}

func (g *generator) staticDropSymbol(id int) string {
	t := g.apiType(id)
	symbol := t.DropSymbol
	for _, d := range g.p.PublicDropTypes {
		if g.apiIdentity(d.Type) != id {
			continue
		}
		if symbol != "" && symbol != d.Symbol {
			g.fail("ambiguous Rust destructor identity %d", id)
		}
		symbol = d.Symbol
	}
	if symbol == "" {
		if t.NeedsDrop {
			g.fail("missing public Rust destructor for %s", t.Name)
		}
		return ""
	}
	f := g.functions[symbol]
	if f == nil || g.names[symbol] == "" {
		g.fail("missing public Rust destructor function %s", symbol)
	}
	params, ret := g.signature(f)
	if len(params) != 1 || f.TrackCaller || f.Signature != nil && f.Signature.Variadic || g.apiType(ret).Size != 0 {
		g.fail("invalid public Rust destructor signature for %s", t.Name)
	}
	pointer := g.apiType(params[0])
	if pointer.Kind != "pointer" || !g.apiType(pointer.Pointee).Sized || g.apiIdentity(pointer.Pointee) != id {
		g.fail("mismatched public Rust destructor receiver for %s", t.Name)
	}
	return g.names[symbol]
}

// Copy through bytes instead of forming an unaligned Go scalar pointer. These
// small local scalars never expose a Go address as uintptr or enter Rust storage.
func (g *generator) emitStaticScalar(id int) {
	t := g.apiType(id)
	var scalar string
	var size uint64
	switch t.Kind {
	case "bool":
		scalar, size = "bool", 1
	case "char":
		scalar, size = "uint32", 4
	case "usize":
		scalar, size = "uintptr", 8
	case "isize":
		scalar, size = "int64", 8
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f16", "f32", "f64":
		scalar = g.scalar(t)
		switch t.Kind[1:] {
		case "8":
			size = 1
		case "16":
			size = 2
		case "32":
			size = 4
		case "64":
			size = 8
		}
	case "u128", "i128", "f128":
		scalar, size = g.scalar(t), 16
	default:
		return
	}
	if t.Size != size {
		g.fail("invalid public Rust scalar size for %s", t.Name)
	}
	name := g.apiName(id)
	for _, prefix := range []string{"Ref", "Mut"} {
		g.line("func (v %s__%s) Get() %s {", prefix, name, scalar)
		g.line("if v.addr==0 { panic(\"oxide: null Rust storage\") }")
		if t.Kind == "bool" {
			g.line("value:=*(*byte)(unsafe.Pointer(v.addr)); if value>1 { panic(\"oxide: invalid Rust bool\") }; return value!=0")
		} else {
			g.line("var value %s", scalar)
			g.staticCopy("&value", "v.addr", size)
			if t.Kind == "char" {
				g.staticCheckChar("value")
			}
			g.line("return value")
		}
		g.line("}")
	}
	g.line("func (v Mut__%s) Set(value %s) {", name, scalar)
	g.line("if v.addr==0 { panic(\"oxide: null Rust storage\") }")
	if t.Kind == "bool" {
		g.line("var b byte; if value { b=1 }; *(*byte)(unsafe.Pointer(v.addr))=b")
	} else {
		if t.Kind == "char" {
			g.staticCheckChar("value")
		}
		g.staticCopy("v.addr", "&value", size)
	}
	g.line("}")
}

func (g *generator) staticCheckChar(value string) {
	g.line("if uint32(%s)>0x10ffff || (uint32(%s)>=0xd800 && uint32(%s)<=0xdfff) { panic(%q) }", value, value, value, "oxide: invalid Rust char")
}
