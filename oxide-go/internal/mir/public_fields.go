package mir

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func apiMemberName(name string) string {
	if _, err := strconv.ParseUint(name, 10, 64); err == nil {
		return name
	}
	n, err := exportComponent(name)
	if err != nil {
		panic(generationError(err.Error()))
	}
	return n
}

func (g *generator) emitStaticFields(id int) {
	t := g.apiType(id)
	if !t.Sized {
		g.emitDynamicLayout(id)
	}
	if t.AdtKind == "Enum" {
		g.emitStaticEnum(id)
	}
	for variant, fields := range t.Members {
		if t.AdtKind == "Enum" && !g.apiVariantInhabited(t, variant) {
			continue
		}
		for _, f := range fields {
			if !f.Public {
				continue
			}
			name := apiMemberName(f.Name)
			if t.AdtKind == "Enum" {
				name = apiMemberName(t.VariantNames[variant]) + "__" + name
			}
			ft := g.apiType(f.Type)
			unaligned := t.Align < ft.Align || f.Offset%ft.Align != 0
			for _, mutable := range []bool{false, true} {
				// Rust forbids references to unaligned packed fields. A mutable
				// place can move its bits into aligned storage before borrowing.
				if !mutable && unaligned {
					switch ft.Kind {
					case "bool", "char", "usize", "isize", "u8", "u16", "u32", "u64", "u128", "i8", "i16", "i32", "i64", "i128", "f16", "f32", "f64", "f128":
						g.line("func(v %s) Read__%s() %s { return (%s).Get() }", g.apiRefType(id, false), name, g.goType(f.Type), g.apiRefLiteral(f.Type, true, fmt.Sprintf("v.addr+%d", f.Offset), "0"))
					}
					continue
				}
				g.line("func(v %s) Field__%s() %s {", g.apiRefType(id, mutable), name, g.apiRefType(f.Type, mutable))
				if t.AdtKind == "Enum" {
					g.line("if v.variant()!=Variant__%s__%s {panic(\"oxide: wrong Rust enum variant\")}", g.apiName(id), apiMemberName(t.VariantNames[variant]))
				}
				offset := fmt.Sprint(f.Offset)
				if !ft.Sized {
					g.line("align:=(%s).Align()", g.apiRefLiteral(f.Type, false, "v.addr", "v.meta"))
					if t.Pack != 0 {
						g.line("align=min(align,%d)", t.Pack)
					}
					offset = fmt.Sprintf("oxide.AlignUp(%d,align)", f.Offset)
				}
				value := g.apiRefLiteral(f.Type, mutable, "v.addr+"+offset, g.apiMeta("v", id))
				if !mutable && !ft.Sized && t.Pack != 0 {
					// Packed DST tails can have a runtime alignment larger than
					// rustc's static minimum. A shared Rust borrow must satisfy it.
					g.line("child:=%s;if child.addr%%child.Align()!=0 {panic(\"oxide: unaligned Rust reference\")};return child}", value)
				} else {
					g.line("return %s }", value)
				}
			}
		}
	}
}

func (g *generator) emitDynamicLayout(id int) {
	t := g.apiType(id)
	for _, mutable := range []bool{false, true} {
		receiver := g.apiRefType(id, mutable)
		if mutable {
			g.line("func(v %s) Size() uintptr { return (%s).Size() }", receiver, g.apiRefLiteral(id, false, "v.addr", "v.meta"))
			g.line("func(v %s) Align() uintptr { return (%s).Align() }", receiver, g.apiRefLiteral(id, false, "v.addr", "v.meta"))
			continue
		}
		byteDST := t.Kind == "str" || t.Container != nil && (t.Container.Kind == "path" || t.Container.Kind == "os_str")
		switch {
		case byteDST:
			g.line("func(v %s) Size() uintptr {return oxide.ArrayBytes(v.meta,1)}", receiver)
			g.line("func(v %s) Align() uintptr {return %d}", receiver, t.Align)
		case t.Kind == "slice":
			e := g.apiType(t.Element)
			g.line("func(v %s) Size() uintptr {return oxide.ArrayBytes(v.meta,%d)}", receiver, e.Size)
			g.line("func(v %s) Align() uintptr {return %d}", receiver, e.Align)
		case t.Kind == "dynamic":
			g.line("func(v %s) Size() uintptr {return *(*uintptr)(unsafe.Pointer(v.meta+8))}", receiver)
			g.line("func(v %s) Align() uintptr {return *(*uintptr)(unsafe.Pointer(v.meta+16))}", receiver)
		case t.Kind == "aggregate" && len(t.Members) > 0 && len(t.Members[0]) > 0:
			fields := t.Members[0]
			tail := fields[len(fields)-1]
			view := g.apiRefLiteral(tail.Type, false, "v.addr", "v.meta")
			g.line("func(v %s) Align() uintptr {a:=max(uintptr(%d),(%s).Align());", receiver, t.Align, view)
			if t.Pack != 0 {
				g.line("a=min(a,%d)", t.Pack)
			}
			g.line("return a}")
			g.line("func(v %s) Size() uintptr {tail:=%s; a:=tail.Align();", receiver, view)
			if t.Pack != 0 {
				g.line("a=min(a,%d)", t.Pack)
			}
			g.line("offset:=oxide.AlignUp(%d,a);return oxide.AlignUp(oxide.AddAddress(offset,tail.Size()),v.Align())}", tail.Offset)
		default:
			g.fail("unsupported public unsized layout %s", t.Name)
		}
	}
}

func apiUnsigned(value string, size uint64) *big.Int {
	x, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic(generationError("invalid Rust discriminant " + value))
	}
	return x.Mod(x, new(big.Int).Lsh(big.NewInt(1), uint(size*8)))
}

func (g *generator) emitStaticEnum(id int) {
	t := g.apiType(id)
	name := g.apiName(id)
	g.line("type Tag__%s uint32", name)
	for i, v := range t.VariantNames {
		g.line("const Variant__%s__%s Tag__%s = %d", name, apiMemberName(v), name, i)
	}
	g.line("func(v Tag__%s) String() string {switch v {", name)
	for _, v := range t.VariantNames {
		g.line("case Variant__%s__%s:return %q", name, apiMemberName(v), v)
	}
	g.line("};panic(\"oxide: invalid Rust enum variant\")}")
	for _, mutable := range []bool{false, true} {
		r := g.apiRefType(id, mutable)
		g.line("func(v %s) Variant() Tag__%s {return v.variant()}", r, name)
		g.line("func(v %s) variant() Tag__%s {", r, name)
		if mutable {
			g.line("return (%s).variant()}", g.apiRefLiteral(id, false, "v.addr", "0"))
			continue
		}
		total := false
		if tag := t.Tag; tag != nil {
			g.line("raw:=%s", apiReadTag(fmt.Sprintf("v.addr+%d", tag.Offset), tag.Size))
			switch tag.Encoding {
			case "direct":
				g.line("switch raw {")
				for i, d := range t.Discriminants {
					if g.apiVariantInhabited(t, i) {
						g.line("case %s:return Variant__%s__%s", apiTagLiteral(d, tag.Size), name, apiMemberName(t.VariantNames[i]))
					}
				}
				g.line("}")
			case "niche":
				if tag.Size <= 8 {
					g.line("delta:=raw-%s", apiTagLiteral(tag.Start, tag.Size))
					g.line("if uint64(delta)<=%d {switch int(delta)+%d {", tag.Last-tag.First, tag.First)
				} else {
					g.line("delta:=oxide.U128SubValue(raw,%s)", apiTagLiteral(tag.Start, tag.Size))
					g.line("if delta.Hi==0 && delta.Lo<=%d {switch int(delta.Lo)+%d {", tag.Last-tag.First, tag.First)
				}
				for i := tag.First; i <= tag.Last; i++ {
					if g.apiVariantInhabited(t, i) {
						g.line("case %d:return Variant__%s__%s", i, name, apiMemberName(t.VariantNames[i]))
					}
				}
				g.line("};panic(\"oxide: uninhabited Rust variant\")}")
				if g.apiVariantInhabited(t, tag.Untagged) {
					g.line("return Variant__%s__%s", name, apiMemberName(t.VariantNames[tag.Untagged]))
					total = true
				}
			default:
				g.fail("unsupported Rust enum tag %s", tag.Encoding)
			}
		} else if g.apiVariantInhabited(t, t.Variant) {
			g.line("return Variant__%s__%s", name, apiMemberName(t.VariantNames[t.Variant]))
			total = true
		}
		if !total {
			g.line("panic(\"oxide: invalid Rust enum discriminant\")")
		}
		g.line("}")
	}
	for i, variant := range t.VariantNames {
		// A non-exhaustive enum still permits its known variants to be
		// constructed. Only a non-exhaustive variant restricts construction.
		if !g.apiVariantInhabited(t, i) || i < len(t.VariantsInfo) && t.VariantsInfo[i].NonExhaustive {
			continue
		}
		var fields []TypeMember
		if i < len(t.Members) {
			fields = t.Members[i]
		}
		accessible := true
		declarations := []string{"ctx *oxide.Context"}
		for n, f := range fields {
			if !f.Public || !g.apiType(f.Type).Sized {
				accessible = false
				break
			}
			declarations = append(declarations, fmt.Sprintf("a%d %s", n, g.publicGoType(f.Type, false)))
		}
		if !accessible {
			continue
		}
		g.line("func New__%s__%s(%s) Value__%s {", name, apiMemberName(variant), strings.Join(declarations, ","), name)
		g.beginPublicFrame(id)
		for n, f := range fields {
			value := g.publicArgument(fmt.Sprintf("a%d", n), f.Type)
			g.storeValue(g.storageLocation(fmt.Sprintf("unsafe.Pointer(out.addr+%d)", f.Offset), f.Type), value)
		}
		if tag := t.Tag; tag != nil && (tag.Encoding != "niche" || i != tag.Untagged) {
			bits := t.Discriminants[i]
			if tag.Encoding == "niche" {
				x := apiUnsigned(tag.Start, tag.Size)
				bits = x.Add(x, big.NewInt(int64(i-tag.First))).String()
			}
			g.line("%s=%s", apiReadTag(fmt.Sprintf("out.addr+%d", tag.Offset), tag.Size), apiTagLiteral(bits, tag.Size))
		}
		g.line("restore=retained;return out}")
	}
}

func (g *generator) apiVariantInhabited(t *Type, index int) bool {
	return index >= 0 && index < len(t.VariantNames) && (index >= len(t.VariantsInfo) || t.VariantsInfo[index].Inhabited)
}

func apiReadTag(addr string, size uint64) string {
	typ := "oxide.U128"
	if size <= 8 {
		typ = fmt.Sprintf("uint%d", size*8)
	}
	return fmt.Sprintf("*(*%s)(unsafe.Pointer(%s))", typ, addr)
}

func apiTagLiteral(value string, size uint64) string {
	x := apiUnsigned(value, size)
	if size <= 8 {
		return fmt.Sprintf("uint%d(%s)", size*8, x.String())
	}
	return apiBits(x.String())
}
