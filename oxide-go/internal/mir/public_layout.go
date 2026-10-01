package mir

// These are nominal Go storage types with the compiler's byte size. Stable
// Ref__/Mut__/Value__ storage supplies Rust alignment beyond Go's maximum of 8;
// no address of an ordinary Go object is promised to have that alignment.
func (g *generator) emitLayoutType(id int) {
	id = g.apiIdentity(id)
	t := g.apiType(id)
	name := g.apiName(id)
	if t.Align == 0 || t.Align&(t.Align-1) != 0 {
		g.fail("invalid public Rust alignment for %s", t.Name)
	}
	g.line("const RustName__%s = %q", name, t.Name)
	g.line("const RustAlign__%s uintptr = %d", name, t.Align)
	if !t.Sized {
		g.line("// %s is unsized; RustAlign__%s is its minimum alignment. Its typed view resolves the dynamic layout.", name, name)
		return
	}
	if t.Size%t.Align != 0 {
		g.fail("invalid public Rust size/alignment for %s", t.Name)
	}
	g.line("const RustSize__%s uintptr = %d", name, t.Size)
	align := min(t.Align, 8)
	if scalar := g.publicLayoutScalar(t); scalar != "" {
		g.line("type Rust__%s %s", name, scalar)
	} else {
		g.line("type Rust__%s struct { _ [0]uint%d; data [%d]byte }", name, align*8, t.Size)
	}
	g.line("var _ [int(RustSize__%s)-int(unsafe.Sizeof(*new(Rust__%s)))]byte", name, name)
	g.line("var _ [int(unsafe.Sizeof(*new(Rust__%s)))-int(RustSize__%s)]byte", name, name)
	g.line("var _ [%d-int(unsafe.Alignof(*new(Rust__%s)))]byte", align, name)
	g.line("var _ [int(unsafe.Alignof(*new(Rust__%s)))-%d]byte", name, align)
}

func (g *generator) publicLayoutScalar(t *Type) string {
	scalar := g.scalar(t)
	var size, align uint64
	switch scalar {
	case "bool", "int8", "uint8":
		size, align = 1, 1
	case "int16", "uint16", "oxide.F16":
		size, align = 2, 2
	case "int32", "uint32", "float32":
		size, align = 4, 4
	case "int64", "uint64", "float64", "uintptr":
		size, align = 8, 8
	case "oxide.I128", "oxide.U128", "oxide.F128":
		size, align = 16, 8
	case "":
		return ""
	default:
		g.fail("unsupported public Go scalar %s", scalar)
	}
	if size != t.Size {
		g.fail("public scalar size mismatch for %s", t.Name)
	}
	// A packed scalar/newtype must not acquire the builtin Go scalar alignment.
	if align != min(t.Align, 8) {
		return ""
	}
	return scalar
}
