package mir

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func (g *generator) unsizeAggregate(dst, src string, st, dt *Type) {
	if len(st.VariantFieldTypes) == 0 || len(dt.VariantFieldTypes) == 0 {
		g.fail("unsize aggregate lacks field metadata: %s -> %s", st.Name, dt.Name)
	}
	sf, df := st.VariantFieldTypes[0], dt.VariantFieldTypes[0]
	if len(sf) != len(df) {
		g.fail("unsize field count: %s -> %s", st.Name, dt.Name)
	}
	for i := range sf {
		so, do := uint64(0), uint64(0)
		if i < len(st.Fields) {
			so = st.Fields[i]
		}
		if i < len(dt.Fields) {
			do = dt.Fields[i]
		}
		if sf[i] == df[i] {
			g.line("copy(unsafe.Slice((*byte)(unsafe.Add(%s,%d)),%d),unsafe.Slice((*byte)(unsafe.Add(%s,%d)),%d))", dst, do, g.typ(df[i]).Size, src, so, g.typ(sf[i]).Size)
		} else {
			g.unsizeValue(fmt.Sprintf("unsafe.Add(%s,%d)", dst, do), fmt.Sprintf("unsafe.Add(%s,%d)", src, so), g.typ(sf[i]), g.typ(df[i]))
		}
	}
}
func (g *generator) unsizeValue(dst, src string, st, dt *Type) {
	if st.Kind == "pointer" && dt.Kind == "pointer" {
		if st.Size != 8 && st.Size != 16 || dt.Size != 8 && dt.Size != 16 {
			g.fail("unsize pointer layout")
		}
		g.line("*(*uintptr)(%s)=*(*uintptr)(%s)", dst, src)
		if dt.Size == 16 {
			old := ""
			if st.Size == 16 {
				old = fmt.Sprintf("*(*uintptr)(unsafe.Add(%s,8))", src)
			}
			g.line("*(*uintptr)(unsafe.Add(%s,8))=%s", dst, g.unsizeMetadata(g.typ(st.Pointee), g.typ(dt.Pointee), old))
		} else if st.Size != 8 {
			g.fail("unsize fat pointer to thin pointer")
		}
		return
	}
	if st.Kind == "aggregate" && dt.Kind == "aggregate" {
		g.unsizeAggregate(dst, src, st, dt)
		return
	}
	g.fail("unsize field %s -> %s", st.Name, dt.Name)
}

func (g *generator) statement(raw json.RawMessage) {
	k, v := variant(raw)
	switch k {
	case "Assign":
		a := decode[[]json.RawMessage](v)
		g.assign(g.place(decode[Place](a[0])), a[1])
	case "StorageLive", "StorageDead", "Nop", "FakeRead", "PlaceMention", "AscribeUserType", "ConstEvalCounter", "Coverage":
	case "Intrinsic":
		name, data := variant(v)
		if name == "Assume" {
			break
		}
		if name == "CopyNonOverlapping" {
			x := decode[struct {
				Dst   json.RawMessage `json:"dst"`
				Src   json.RawMessage `json:"src"`
				Count json.RawMessage `json:"count"`
			}](data)
			d, dt := g.operand(x.Dst)
			s, _ := g.operand(x.Src)
			n, _ := g.operand(x.Count)
			pt := g.typ(dt)
			if pt.Kind != "pointer" {
				g.fail("copy intrinsic pointer")
			}
			g.line("copy(unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d),unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d))", d, n, g.typ(pt.Pointee).Size, s, n, g.typ(pt.Pointee).Size)
			break
		}
		g.fail("statement intrinsic %s", name)
	case "SetDiscriminant":
		x := decode[struct {
			Place   Place `json:"place"`
			Variant int   `json:"variant_index"`
		}](v)
		g.setTag(g.place(x.Place), x.Variant)
	default:
		g.fail("statement %s", k)
	}
}

func (g *generator) assign(dst location, raw json.RawMessage) {
	// A zero-sized MIR rvalue has no side effects or bytes to read/write.
	// In particular Box<ZST> may use address 1, which Go would nil-check.
	if t := g.typ(dst.typ); t.Sized && t.Size == 0 {
		return
	}
	k, v := variant(raw)
	args := func() []json.RawMessage { return decode[[]json.RawMessage](v) }
	store := func(x string) { g.storeValue(dst, x) }
	switch k {
	case "ThreadLocalRef":
		id := decode[uint64](v)
		pt := g.typ(dst.typ)
		if pt.Kind != "pointer" {
			g.fail("thread local destination")
		}
		inner := g.typ(pt.Pointee)
		if _, ok := g.p.ThreadLocals[id]; !ok {
			g.fail("missing thread-local initializer %d", id)
		}
		var meta struct {
			External string `json:"external"`
		}
		_ = json.Unmarshal(g.p.ThreadLocals[id], &meta)
		if meta.External != "" {
			g.fail("external TLS %s", meta.External)
		} else {
			g.line("%s=ctx.ThreadLocal(threadLocal%d,%d,%d)", dst.read(), id, inner.Size, inner.Align)
		}
	case "Use":
		x, _ := g.operand(args()[0])
		store(x)
	case "CopyForDeref":
		p := g.place(decode[Place](v))
		store(p.value())
	case "BinaryOp":
		a := args()
		op := decode[string](a[0])
		l, lt := g.operand(a[1])
		r, _ := g.operand(a[2])
		if (op == "Eq" || op == "Ne") && g.typ(lt).Kind == "pointer" && g.typ(lt).Size == 16 {
			lp, rp := g.operandPlace(a[1]), g.operandPlace(a[2])
			expr := fmt.Sprintf("(*(*uintptr)(%s)==*(*uintptr)(%s) && *(*uintptr)(unsafe.Add(%s,8))==*(*uintptr)(unsafe.Add(%s,8)))", lp.address, rp.address, lp.address, rp.address)
			if op == "Ne" {
				expr = "!(" + expr + ")"
			}
			store(expr)
			break
		}
		if op == "Cmp" {
			g.line("if %s {", g.binary("Lt", l, r, lt))
			g.setTag(dst, 0)
			g.line("} else if %s {", g.binary("Gt", l, r, lt))
			g.setTag(dst, 2)
			g.line("} else {")
			g.setTag(dst, 1)
			g.line("}")
			break
		}
		store(g.binary(op, l, r, lt))
	case "CheckedBinaryOp":
		a := args()
		op := decode[string](a[0])
		l, t := g.operand(a[1])
		r, _ := g.operand(a[2])
		switch op {
		case "Add", "Sub", "Mul":
		default:
			g.fail("checked operation %s", op)
		}
		ty := g.typ(dst.typ)
		if len(ty.Fields) != 2 {
			g.fail("checked result layout %s", ty.Name)
		}
		helper := op + "WithOverflow"
		if g.typ(t).Kind == "u128" {
			helper = "U128" + op
		}
		if g.typ(t).Kind == "i128" {
			helper = "I128" + op
		}
		g.line("{ value, overflow := oxide.%s(%s,%s)", helper, l, r)
		g.line("*(*%s)(unsafe.Add(%s,%d)) = value", g.goType(t), dst.address, ty.Fields[0])
		g.line("*(*bool)(unsafe.Add(%s,%d)) = overflow }", dst.address, ty.Fields[1])
	case "UnaryOp":
		a := args()
		op := decode[string](a[0])
		x, t := g.operand(a[1])
		switch op {
		case "Neg":
			if g.typ(t).Kind == "i128" {
				store("oxide.I128SubValue(oxide.I128{}," + x + ")")
			} else {
				store("-(" + x + ")")
			}
		case "Not":
			if g.typ(t).Kind == "bool" {
				store("!(" + x + ")")
			} else if g.typ(t).Kind == "i128" || g.typ(t).Kind == "u128" {
				store(g.goType(t) + "(oxide.U128Xor(oxide.U128(" + x + "),oxide.U128{Lo:^uint64(0),Hi:^uint64(0)}))")
			} else {
				store("^(" + x + ")")
			}
		case "PtrMetadata":
			if g.typ(dst.typ).Size == 0 {
				store(g.zero(dst.typ))
			} else {
				src := g.operandPlace(a[1])
				if g.typ(src.typ).Size != 16 || g.typ(dst.typ).Size != 8 {
					g.fail("pointer metadata layout")
				}
				g.line("*(*uintptr)(%s)=*(*uintptr)(unsafe.Add(%s,8))", dst.address, src.address)
			}
		default:
			g.fail("unary %s", op)
		}
	case "Ref", "AddressOf":
		a := args()
		p := g.place(decode[Place](a[len(a)-1]))
		if g.typ(dst.typ).Size == 8 {
			store("uintptr(" + p.address + ")")
		} else {
			g.line("*(*uintptr)(%s) = uintptr(%s)", dst.address, p.address)
			g.line("*(*uintptr)(unsafe.Add(%s,8)) = %s", dst.address, g.length(p))
		}
	case "Len":
		store(g.length(g.place(decode[Place](v))))
	case "Discriminant":
		g.discriminant(dst, g.place(decode[Place](v)))
	case "Cast":
		a := args()
		kind, _ := variant(a[0])
		x, from := g.operand(a[1])
		srcType, dstType := g.typ(from), g.typ(dst.typ)
		if kind == "PtrToPtr" && srcType.Size == dstType.Size && srcType.Size > 8 {
			srcPlace := g.operandPlace(a[1])
			g.line("copy(unsafe.Slice((*byte)(%s),%d),unsafe.Slice((*byte)(%s),%d))", dst.address, dstType.Size, srcPlace.address, srcType.Size)
			break
		}
		if kind == "PtrToPtr" && srcType.Size == 16 && dstType.Size == 8 {
			srcPlace := g.operandPlace(a[1])
			store("*(*uintptr)(" + srcPlace.address + ")")
			break
		}
		switch kind {
		case "IntToInt", "IntToFloat", "FloatToFloat", "PtrToPtr", "FnPtrToPtr", "PointerExposeAddress", "PointerExposeProvenance", "PointerWithExposedProvenance":
			if kind == "IntToInt" && srcType.Kind == "bool" {
				g.line("if %s { %s=%s } else { %s=%s }", x, dst.read(), g.integerConstant("1", dstType), dst.read(), g.integerConstant("0", dstType))
				break
			}
			if kind == "IntToFloat" && (srcType.Kind == "u128" || srcType.Kind == "i128") {
				store("oxide." + strings.ToUpper(srcType.Kind[:1]) + "128ToF" + strings.TrimPrefix(dstType.Kind, "f") + "(" + x + ")")
				break
			}
			if kind == "IntToInt" && (dstType.Kind == "u128" || dstType.Kind == "i128") {
				if srcType.Kind == "u128" || srcType.Kind == "i128" {
					store(g.goType(dst.typ) + "(" + x + ")")
				} else if strings.HasPrefix(srcType.Kind, "i") {
					store(g.goType(dst.typ) + "(oxide.I128From64(int64(" + x + ")))")
				} else {
					store(g.goType(dst.typ) + "(oxide.U128From64(uint64(" + x + ")))")
				}
				break
			}
			if kind == "IntToInt" && srcType.Kind == "u128" {
				store(g.goType(dst.typ) + "(oxide.U128To64(" + x + "))")
				break
			}
			if kind == "IntToInt" && srcType.Kind == "i128" {
				store(g.goType(dst.typ) + "(oxide.I128To64(" + x + "))")
				break
			}
			if g.scalar(dstType) == "" || g.scalar(srcType) == "" {
				g.fail("cast %s of %s to %s", kind, srcType.Name, dstType.Name)
			}
			store(g.goType(dst.typ) + "(" + x + ")")
		case "FloatToInt":
			if dstType.Kind == "i128" || dstType.Kind == "u128" {
				store("oxide.FloatTo" + strings.ToUpper(dstType.Kind[:1]) + "128(float64(" + x + "))")
				break
			}
			store(fmt.Sprintf("oxide.FloatToInt[%s](float64(%s))", g.goType(dst.typ), x))
		case "BoxDerefTransmute":
			if srcType.Size != dstType.Size {
				g.fail("box transmute size")
			}
			if g.scalar(srcType) != "" && g.scalar(dstType) != "" {
				store(g.goType(dst.typ) + "(" + x + ")")
			} else {
				src := g.operandPlace(a[1])
				g.line("copy(unsafe.Slice((*byte)(%s),%d),unsafe.Slice((*byte)(%s),%d))", dst.address, dstType.Size, src.address, srcType.Size)
			}
		case "Transmute", "Subtype":
			if srcType.Size != dstType.Size {
				g.fail("%s size mismatch: %s (%d) -> %s (%d)", kind, srcType.Name, srcType.Size, dstType.Name, dstType.Size)
			}
			if g.indirectValue(from) {
				g.copyValue(dst.address, "unsafe.Pointer("+x+")", dstType.Size)
			} else {
				g.line("{ value := %s; copy(unsafe.Slice((*byte)(%s),%d),unsafe.Slice((*byte)(unsafe.Pointer(&value)),%d)) }", x, dst.address, dstType.Size, srcType.Size)
			}
		case "PointerCoercion":
			_, coercionData := variant(a[0])
			coercion, coercionArg := variant(coercionData)
			if coercion == "ReifyFnPointer" || coercion == "ClosureFnPointer" {
				_, sourceTypeID := g.operand(a[1])
				ft := g.typ(sourceTypeID)
				if ft.Function == "" {
					g.fail("function pointer coercion lacks a function instance for %s", ft.Name)
				}
				name := g.names[ft.Function]
				if name == "" {
					g.fail("function pointer coercion references untranslated function %s", ft.Function)
				}
				if g.functions[ft.Function].TrackCaller {
					g.fail("track_caller function pointer coercion requires a rustc reification shim: %s", ft.Function)
				}
				store("oxide.FunctionPointer(" + name + ")")
				_ = coercionArg
				break
			}
			if coercion != "Unsize" {
				g.fail("pointer coercion %s", coercion)
			}
			if g.indirectValue(from) {
				g.unsizeValue(dst.address, "unsafe.Pointer("+x+")", srcType, dstType)
			} else {
				g.line("{ source := %s", x)
				g.unsizeValue(dst.address, "unsafe.Pointer(&source)", srcType, dstType)
				g.line("}")
			}
		default:
			g.fail("cast %s", kind)
		}
	case "Aggregate":
		a := args()
		kind, data := variant(a[0])
		operands := decode[[]json.RawMessage](a[1])
		ty := g.typ(dst.typ)
		if kind == "RawPtr" && ty.Size == 8 {
			if len(operands) == 0 {
				g.fail("empty raw pointer aggregate")
			}
			x, _ := g.operand(operands[0])
			g.line("%s=%s", dst.read(), x)
			return
		}
		offsets := ty.Fields
		variantIndex := 0
		if kind == "Adt" {
			parts := decode[[]json.RawMessage](data)
			variantIndex = decode[int](parts[1])
			if len(ty.Variants) > 0 {
				offsets = ty.Variants[variantIndex]
			}
		}
		if ty.Size == 0 && len(offsets) == 0 {
			if kind == "Adt" {
				g.setTag(dst, variantIndex)
			}
			break
		}
		for i, operand := range operands {
			x, t := g.operand(operand)
			var off uint64
			if kind == "Array" {
				off = uint64(i) * g.typ(t).Size
			} else {
				if i >= len(offsets) {
					g.fail("aggregate fields for %s", ty.Name)
				}
				off = offsets[i]
			}
			g.storeValue(g.storageLocation(fmt.Sprintf("unsafe.Add(%s,%d)", dst.address, off), t), x)
		}
		if kind == "Adt" {
			g.setTag(dst, variantIndex)
		}
	case "Repeat":
		a := args()
		x, t := g.operand(a[0])
		ty := g.typ(dst.typ)
		g.line("{ for i:=uintptr(0); i<%d; i++ {", ty.Length)
		g.storeValue(g.storageLocation(fmt.Sprintf("unsafe.Add(%s,i*%d)", dst.address, g.typ(t).Size), t), x)
		g.line("} }")
	default:
		g.fail("rvalue %s", k)
	}
}

func (g *generator) operandPlace(raw json.RawMessage) location {
	k, v := variant(raw)
	if k != "Copy" && k != "Move" {
		g.fail("operation needs an addressable operand")
	}
	return g.place(decode[Place](v))
}

func (g *generator) binary(op, l, r string, typ int) string {
	t := g.typ(typ)
	if op == "Rem" && (t.Kind == "f32" || t.Kind == "f64") {
		return fmt.Sprintf("%s(math.Mod(float64(%s),float64(%s)))", g.goType(typ), l, r)
	}
	if t.Kind == "u128" || t.Kind == "i128" {
		if op == "Eq" || op == "Ne" || op == "Lt" || op == "Le" || op == "Gt" || op == "Ge" {
			p := "U128"
			if t.Kind == "i128" {
				p = "I128"
			}
			base := map[string]string{"Eq": "Eq", "Ne": "Eq", "Lt": "Lt", "Le": "Le", "Gt": "Le", "Ge": "Lt"}[op]
			expr := "oxide." + p + base + "(" + l + "," + r + ")"
			if op == "Ne" || op == "Gt" || op == "Ge" {
				expr = "!(" + expr + ")"
			}
			return expr
		}
		if op == "Shl" || op == "ShlUnchecked" {
			if t.Kind == "u128" {
				return "oxide.U128Shl(" + l + ",uint64(" + r + "))"
			}
			return "oxide.I128Shl(" + l + ",uint64(" + r + "))"
		}
		if op == "Shr" || op == "ShrUnchecked" {
			if t.Kind == "u128" {
				return "oxide.U128Shr(" + l + ",uint64(" + r + "))"
			}
			return "oxide.I128Shr(" + l + ",uint64(" + r + "))"
		}
		if op == "BitAnd" || op == "BitOr" || op == "BitXor" {
			fn := strings.TrimPrefix(op, "Bit")
			return g.goType(typ) + "(oxide.U128" + fn + "(oxide.U128(" + l + "),oxide.U128(" + r + ")))"
		}
		if op == "Add" || op == "AddUnchecked" {
			if t.Kind == "u128" {
				return "oxide.U128AddValue(" + l + "," + r + ")"
			}
			return "oxide.I128AddValue(" + l + "," + r + ")"
		}
		if op == "Mul" || op == "MulUnchecked" {
			if t.Kind == "u128" {
				return "oxide.U128MulValue(" + l + "," + r + ")"
			}
			return "oxide.I128MulValue(" + l + "," + r + ")"
		}
		if op == "Sub" || op == "SubUnchecked" {
			if t.Kind == "u128" {
				return "oxide.U128SubValue(" + l + "," + r + ")"
			}
			return "oxide.I128SubValue(" + l + "," + r + ")"
		}
		if op == "Div" || op == "Rem" {
			return "oxide." + strings.ToUpper(t.Kind[:1]) + "128" + op + "(" + l + "," + r + ")"
		}
		g.fail("128-bit binary operation %s", op)
	}
	if op == "Offset" {
		return fmt.Sprintf("(%s + uintptr(%s)*%d)", l, r, g.typ(t.Pointee).Size)
	}
	if op == "Shl" || op == "ShlUnchecked" {
		return fmt.Sprintf("(%s << (uint64(%s)&%d))", l, r, t.Size*8-1)
	}
	if op == "Shr" || op == "ShrUnchecked" {
		return fmt.Sprintf("(%s >> (uint64(%s)&%d))", l, r, t.Size*8-1)
	}
	s := map[string]string{"Add": "+", "AddUnchecked": "+", "Sub": "-", "SubUnchecked": "-", "Mul": "*", "MulUnchecked": "*", "Div": "/", "Rem": "%", "BitXor": "^", "BitAnd": "&", "BitOr": "|", "Eq": "==", "Ne": "!=", "Lt": "<", "Le": "<=", "Gt": ">", "Ge": ">="}[op]
	if t.Kind == "bool" {
		switch op {
		case "BitXor":
			s = "!="
		case "BitAnd":
			s = "&&"
		case "BitOr":
			s = "||"
		}
	}
	if s == "" {
		g.fail("binary operation %s", op)
	}
	if g.scalar(t) == "" {
		g.fail("binary operation %s on %s", op, t.Name)
	}
	expr := fmt.Sprintf("(%s %s %s)", l, s, r)
	if (t.Kind == "f32" || t.Kind == "f64") && (s == "+" || s == "-" || s == "*" || s == "/") {
		// Go may fuse multiply/add even across MIR assignments. Its explicit
		// conversion enforces Rust's rounding after each ordinary operation.
		// https://go.dev/ref/spec#Floating-point_operators
		return g.goType(typ) + expr
	}
	return expr
}

func (g *generator) setTag(dst location, index int) {
	t := g.typ(dst.typ)
	tag := t.Tag
	if tag == nil {
		return
	}
	var value string
	if tag.Encoding == "direct" {
		if index < 0 || index >= len(t.Discriminants) {
			g.fail("missing discriminant %s #%d", t.Name, index)
		}
		value = t.Discriminants[index]
	} else if tag.Encoding == "niche" {
		if index == tag.Untagged {
			return
		}
		if index < tag.First || index > tag.Last {
			g.fail("invalid niche variant %s #%d", t.Name, index)
		}
		// Addition wraps at the tag's width; the start can occupy all 128 bits.
		st := g.tagType(tag, false)
		start := g.integerConstant(tag.Start, st)
		offset := g.integerConstant(strconv.Itoa(index-tag.First), st)
		value = "(" + start + "+" + offset + ")"
		if tag.Size == 16 {
			value = "oxide.U128AddValue(" + start + "," + offset + ")"
		}
		g.line("*(*%s)(unsafe.Add(%s,%d)) = %s", g.scalar(st), dst.address, tag.Offset, value)
		return
	} else {
		g.fail("unknown tag encoding %s", tag.Encoding)
	}
	st := g.tagType(tag, false)
	g.line("*(*%s)(unsafe.Add(%s,%d)) = %s", g.scalar(st), dst.address, tag.Offset, g.integerConstant(value, st))
}

func (g *generator) discriminant(dst, src location) {
	t := g.typ(src.typ)
	tag := t.Tag
	if tag == nil {
		value := "0"
		if t.Variant < len(t.Discriminants) {
			value = t.Discriminants[t.Variant]
		}
		g.line("%s = %s", dst.read(), g.integerConstant(value, g.typ(dst.typ)))
		return
	}
	if tag.Encoding == "direct" {
		st := g.tagType(tag, tag.Primitive.Int.Signed)
		read := fmt.Sprintf("*(*%s)(unsafe.Add(%s,%d))", g.scalar(st), src.address, tag.Offset)
		g.line("%s = %s", dst.read(), g.integerConvert(read, st, g.typ(dst.typ)))
		return
	}
	if tag.Encoding != "niche" {
		g.fail("unknown tag encoding %s", tag.Encoding)
	}
	g.nicheDiscriminant(dst, src)
}

func (g *generator) terminator(bb int, raw json.RawMessage, ret int) {
	k, v := variant(raw)
	switch k {
	case "Goto":
		t := decode[struct {
			Target int `json:"target"`
		}](v)
		g.line("goto bb%d", t.Target)
	case "Return":
		g.returnValue(ret)
	case "Unreachable":
		g.line("panic(\"unreachable Rust MIR\")")
	case "Resume":
		g.line("ctx.Fail(unwind); return %s", g.returnZero(ret))
	case "Abort":
		g.line("oxide.Abort(); return %s", g.returnZero(ret))
	case "SwitchInt":
		// The exporter stores u128 branch bit patterns as decimal strings.
		var rawSwitch struct {
			Discr   json.RawMessage `json:"discr"`
			Targets struct {
				Branches  []json.RawMessage `json:"branches"`
				Otherwise int               `json:"otherwise"`
			} `json:"targets"`
		}
		_ = json.Unmarshal(v, &rawSwitch)
		discr := rawSwitch.Discr
		d, t := g.operand(discr)
		g.line("switch %s {", d)
		for _, branch := range rawSwitch.Targets.Branches {
			pair := decode[[]json.RawMessage](branch)
			value := decode[string](pair[0])
			if g.typ(t).Kind == "bool" {
				value = strconv.FormatBool(value != "0")
			} else {
				value = g.integerConstant(value, g.typ(t))
			}
			g.line("case %s: goto bb%d", value, decode[int](pair[1]))
		}
		g.line("default: goto bb%d }", rawSwitch.Targets.Otherwise)
	case "Assert":
		x := decode[struct {
			Cond     json.RawMessage `json:"cond"`
			Expected bool            `json:"expected"`
			Msg      json.RawMessage `json:"msg"`
			Target   int             `json:"target"`
			Unwind   json.RawMessage `json:"unwind"`
		}](v)
		cond, _ := g.operand(x.Cond)
		call, ok := g.f.AssertCalls[bb]
		if !ok {
			g.fail("missing compiler assertion call at bb%d", bb)
		}
		if call.Optional {
			enabled, known := g.p.RuntimeChecks["OverflowChecks"]
			if !known {
				g.fail("missing compiler overflow-check configuration")
			}
			if !enabled {
				g.line("goto bb%d", x.Target)
				return
			}
		}
		callee := g.functions[call.Symbol]
		if callee == nil {
			g.fail("missing assertion panic function %s", call.Symbol)
		}
		args := []string{"ctx"}
		for _, raw := range call.Args {
			a, _ := g.operand(raw)
			args = append(args, a)
		}
		if callee.TrackCaller {
			args = append(args, g.callerLocation(bb))
		}
		g.line("if %s != %t {", cond, x.Expected)
		g.line("_=%s(%s)", g.names[call.Symbol], strings.Join(args, ","))
		g.line("if !ctx.Failed() { panic(\"Rust assertion panic returned\") }")
		g.unwind(x.Unwind, ret)
		g.line("}; goto bb%d", x.Target)
	case "Call":
		x := decode[struct {
			Func        json.RawMessage   `json:"func"`
			Args        []json.RawMessage `json:"args"`
			Destination Place             `json:"destination"`
			Target      *int              `json:"target"`
			Unwind      json.RawMessage   `json:"unwind"`
		}](v)
		symbol, ok := g.f.Calls[bb]
		if !ok {
			g.fail("indirect/virtual call at bb%d", bb)
		}
		if strings.HasPrefix(symbol, "<intrinsic:") {
			g.intrinsicCall(bb, strings.TrimSuffix(strings.TrimPrefix(symbol, "<intrinsic:"), ">"), x.Args, x.Destination, x.Target, x.Unwind, ret)
			return
		}
		if symbol == "<indirect>" {
			g.indirectCall(bb, x.Func, x.Args, x.Destination, x.Target, x.Unwind, ret)
			return
		}
		if strings.HasPrefix(symbol, "<virtual:") {
			if _, tracked := g.f.CallLocations[bb]; tracked {
				g.fail("track_caller virtual call requires a rustc ABI shim")
			}
			g.virtualCall(bb, strings.TrimSuffix(strings.TrimPrefix(symbol, "<virtual:"), ">"), x.Args, x.Destination, x.Target, x.Unwind, ret)
			return
		}
		callee := g.names[symbol]
		if callee == "" {
			g.fail("untranslated function: %s", symbol)
		}
		args := []string{"ctx"}
		for i, arg := range g.callArguments(bb, x.Args) {
			a, typ := arg.value, arg.typ
			if sig := g.functions[symbol].Signature; sig != nil && sig.Variadic && i >= sig.FixedCount {
				a = g.variadicWord(a, typ)
			}
			args = append(args, a)
		}
		if g.functions[symbol].TrackCaller {
			args = append(args, g.callerLocation(bb))
		}
		g.callResult(g.place(x.Destination), callee, args)
		g.line("if ctx.Failed() {")
		g.unwind(x.Unwind, ret)
		g.line("}")
		if x.Target == nil {
			g.line("panic(\"Rust diverging call returned\")")
		} else {
			g.line("goto bb%d", *x.Target)
		}
	case "Drop":
		x := decode[struct {
			Place  Place           `json:"place"`
			Target int             `json:"target"`
			Unwind json.RawMessage `json:"unwind"`
		}](v)
		symbol := g.f.Calls[bb]
		callee := g.functions[symbol]
		if callee == nil {
			g.fail("untranslated drop glue: %s", symbol)
		}
		p := g.place(x.Place)
		if g.typ(p.typ).Kind == "dynamic" {
			_, dropReturn := g.signature(callee)
			if p.meta == "" || g.typ(dropReturn).Size != 0 || callee.TrackCaller {
				g.fail("dynamic drop ABI")
			}
			g.line("{ descriptor:=*(*uintptr)(unsafe.Pointer(%s)); if descriptor!=0 {", p.meta)
			g.line("f:=*(*func(*oxide.Context,uintptr) %s)(unsafe.Pointer(&descriptor)); _=f(ctx,uintptr(%s)); if ctx.Failed() {", g.goType(dropReturn), p.address)
			g.unwind(x.Unwind, ret)
			g.line("} } }; goto bb%d", x.Target)
			return
		}
		if !callee.EmptyDrop {
			params, _ := g.signature(callee)
			if len(params) != 1 || g.typ(params[0]).Kind != "pointer" {
				g.fail("drop glue pointer ABI")
			}
			argument := fmt.Sprintf("uintptr(%s)", p.address)
			g.line("{")
			if g.typ(params[0]).Size == 16 {
				if p.meta == "" {
					g.fail("unsized drop lacks pointer metadata")
				}
				g.line("var dropArg %s; *(*uintptr)(unsafe.Pointer(&dropArg))=uintptr(%s); *(*uintptr)(unsafe.Add(unsafe.Pointer(&dropArg),8))=%s", g.goType(params[0]), p.address, p.meta)
				argument = "dropArg"
			} else if g.typ(params[0]).Size != 8 {
				g.fail("drop glue pointer width")
			}
			args := []string{"ctx", argument}
			if callee.TrackCaller {
				args = append(args, g.callerLocation(bb))
			}
			g.line("_ = %s(%s); if ctx.Failed() {", g.names[symbol], strings.Join(args, ","))
			g.unwind(x.Unwind, ret)
			g.line("}")
			g.line("}")
		}
		g.line("goto bb%d", x.Target)
	case "InlineAsm":
		g.inlineAsm(v)
	default:
		g.fail("terminator %s", k)
	}
}

func (g *generator) variadicWord(value string, typ int) string {
	t := g.typ(typ)
	if t.Size > 8 || g.scalar(t) == "" || strings.HasPrefix(t.Kind, "f") || t.Kind == "bool" {
		g.fail("unsupported C variadic argument type %s", t.Kind)
	}
	return "uintptr(" + value + ")"
}

func (g *generator) indirectCall(bb int, funcRaw json.RawMessage, args []json.RawMessage, dst Place, target *int, unwind json.RawMessage, ret int) {
	fn, ft := g.operand(funcRaw)
	typ := g.typ(ft)
	if typ.Kind == "fn" && typ.Function != "" {
		name := g.names[typ.Function]
		if name == "" {
			g.fail("untranslated function item: %s", typ.Function)
		}
		call := []string{"ctx"}
		for _, arg := range g.callArguments(bb, args) {
			call = append(call, arg.value)
		}
		if g.functions[typ.Function].TrackCaller {
			call = append(call, g.callerLocation(bb))
		}
		g.callResult(g.place(dst), name, call)
		g.line("if ctx.Failed(){")
		g.unwind(unwind, ret)
		g.line("}")
		if target == nil {
			g.fail("function item diverged")
		} else {
			g.line("goto bb%d", *target)
		}
		return
	}
	if typ.Kind != "fnptr" {
		g.fail("indirect call of %s", typ.Name)
	}
	a := []string{"ctx"}
	types := []string{"*oxide.Context"}
	if g.indirectValue(g.place(dst).typ) {
		types = append(types, "uintptr")
	}
	for i, arg := range g.callArguments(bb, args) {
		x, t := arg.value, arg.typ
		if typ.FnVariadic && i >= typ.FnFixedCount {
			a = append(a, g.variadicWord(x, t))
			continue
		}
		a = append(a, x)
		types = append(types, g.argumentType(t))
	}
	if typ.FnVariadic {
		types = append(types, "...uintptr")
	}
	sig := "func(" + strings.Join(types, ",") + ") " + g.returnType(g.place(dst).typ)
	g.line("{ f:=*(*%s)(unsafe.Pointer(&struct{P uintptr}{%s}));", sig, fn)
	g.callResult(g.place(dst), "f", a)
	g.line("}")
	g.line("if ctx.Failed(){")
	g.unwind(unwind, ret)
	g.line("}")
	if target == nil {
		g.fail("indirect diverging call")
	} else {
		g.line("goto bb%d", *target)
	}
}

func (g *generator) unsizeMetadata(st, dt *Type, old string) string {
	key := fmt.Sprintf("%d/%d", st.ID, dt.ID)
	if id, ok := g.p.VTables[key]; ok {
		return fmt.Sprintf("oxideGlobals.Get(%d)", id)
	}
	if st.Kind == "array" && dt.Kind == "slice" {
		return strconv.FormatUint(st.Length, 10)
	}
	if st.ID == dt.ID && old != "" {
		return old
	}
	if slot, ok := g.p.Upcasts[key]; ok {
		if old == "" {
			g.fail("trait upcast without source metadata")
		}
		if slot < 0 {
			return old
		}
		return fmt.Sprintf("*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),%d))", old, slot*8)
	}
	if st.Kind == "aggregate" && dt.Kind == "aggregate" && len(st.VariantFieldTypes) == 1 && len(dt.VariantFieldTypes) == 1 {
		sf, df := st.VariantFieldTypes[0], dt.VariantFieldTypes[0]
		if len(sf) > 0 && len(sf) == len(df) {
			return g.unsizeMetadata(g.typ(sf[len(sf)-1]), g.typ(df[len(df)-1]), old)
		}
	}
	g.fail("missing compiler unsize metadata %s -> %s", st.Name, dt.Name)
	return ""
}

func (g *generator) virtualCall(bb int, slot string, args []json.RawMessage, dst Place, target *int, unwind json.RawMessage, ret int) {
	if len(args) == 0 {
		g.fail("virtual call without receiver")
	}
	receiver := g.operandPlace(args[0])
	if receiver.typ == 0 && len(receiver.address) == 0 {
		g.fail("virtual receiver")
	}
	slotN, err := strconv.ParseUint(slot, 10, 64)
	if err != nil {
		g.fail("virtual slot %s", slot)
	}
	data := fmt.Sprintf("*(*uintptr)(%s)", receiver.address)
	vt := fmt.Sprintf("*(*uintptr)(unsafe.Add(%s,8))", receiver.address)
	fnword := fmt.Sprintf("*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),%d))", vt, slotN*8)
	argExpr := []string{"ctx", data}
	argTypes := []string{"uintptr"}
	if g.indirectValue(g.place(dst).typ) {
		argTypes = append(argTypes, "uintptr")
	}
	for _, arg := range g.callArguments(bb, args)[1:] {
		x, t := arg.value, arg.typ
		argExpr = append(argExpr, x)
		argTypes = append(argTypes, g.argumentType(t))
	}
	retType := g.returnType(g.place(dst).typ)
	fnType := "func(*oxide.Context," + strings.Join(argTypes, ",") + ") " + retType
	g.line("{ fn := *(*%s)(unsafe.Pointer(&struct{ P uintptr }{%s}));", fnType, fnword)
	g.callResult(g.place(dst), "fn", argExpr)
	g.line("}")
	g.line("if ctx.Failed() {")
	g.unwind(unwind, ret)
	g.line("}")
	if target == nil {
		g.line("panic(\"Rust virtual call returned\")")
	} else {
		g.line("goto bb%d", *target)
	}
}

func (g *generator) intrinsicCall(bb int, name string, args []json.RawMessage, dst Place, target *int, unwind json.RawMessage, ret int) {
	goToTarget := func() {
		if target == nil {
			g.fail("non-returning intrinsic %s", name)
		}
		g.line("goto bb%d", *target)
	}
	if g.x86IntegerIntrinsic(name, args, dst) || g.x86FloatIntrinsic(name, args, dst) {
		goToTarget()
		return
	}
	if strings.HasPrefix(name, "llvm.aarch64.") || strings.HasPrefix(name, "llvm.roundeven.") || strings.HasPrefix(name, "llvm.fptosi.sat.") {
		g.neonIntrinsic(name, args, dst)
		goToTarget()
		return
	}
	switch name {
	case "disjoint_bitor":
		if len(args) != 2 {
			g.fail("disjoint_bitor arity")
		}
		x, t := g.operand(args[0])
		y, _ := g.operand(args[1])
		g.line("%s=%s", g.place(dst).read(), g.binary("BitOr", x, y, t))
		goToTarget()
	case "type_id_eq":
		if len(args) != 2 {
			g.fail("type_id_eq arity")
		}
		a, b := g.operandPlace(args[0]), g.operandPlace(args[1])
		if g.typ(a.typ).Size != 16 || g.typ(b.typ).Size != 16 {
			g.fail("type_id_eq layout")
		}
		g.line("%s=*(*oxide.U128)(%s)==*(*oxide.U128)(%s)", g.place(dst).read(), a.address, b.address)
		goToTarget()
	case "carrying_mul_add":
		g.carryingMulAdd(args, g.place(dst))
		goToTarget()
	case "simd_splat", "simd_add", "simd_sub", "simd_mul", "simd_div", "simd_shl", "simd_shr", "simd_or", "simd_and", "simd_xor", "simd_eq", "simd_ne", "simd_lt", "simd_le", "simd_gt", "simd_ge", "simd_select", "simd_reduce_all", "simd_reduce_any", "simd_reduce_max", "simd_reduce_min", "simd_reduce_or", "simd_extract", "simd_insert", "simd_shuffle", "simd_bitmask", "simd_cast", "simd_fsqrt":
		g.simdIntrinsic(name, args, dst)
		goToTarget()
	case "assume", "cold_path":
		goToTarget()
	case "black_box":
		if len(args) != 1 {
			g.fail("black_box arity")
		}
		x, _ := g.operand(args[0])
		g.storeValue(g.place(dst), x)
		goToTarget()
	case "catch_unwind":
		g.catchUnwind(args, g.place(dst))
		goToTarget()
	case "is_val_statically_known":
		g.line("%s=false", g.place(dst).read())
		goToTarget()
	case "log", "log2", "sin", "cos", "sqrt", "sqrtf32", "sqrtf64", "floorf32", "floorf64", "ceilf32", "ceilf64", "roundf32", "roundf64", "truncf32", "floor", "ceil", "round":
		if len(args) != 1 {
			g.fail("%s arity", name)
		}
		x, _ := g.operand(args[0])
		fn := map[string]string{"log": "Log", "log2": "Log2", "sin": "Sin", "cos": "Cos", "sqrt": "Sqrt", "sqrtf32": "Sqrt", "sqrtf64": "Sqrt", "floorf32": "Floor", "floorf64": "Floor", "ceilf32": "Ceil", "ceilf64": "Ceil", "roundf32": "Round", "roundf64": "Round", "truncf32": "Trunc", "floor": "Floor", "ceil": "Ceil", "round": "Round"}[name]
		g.line("%s=%s(math.%s(float64(%s)))", g.place(dst).read(), g.goType(g.place(dst).typ), fn, x)
		goToTarget()
	case "fabs", "fabsf32", "fabsf64":
		if len(args) != 1 {
			g.fail("%s arity", name)
		}
		x, t := g.operand(args[0])
		width := g.typ(t).Size * 8
		g.line("%s=math.Float%dfrombits(math.Float%dbits(%s)&^ (uint%d(1)<<%d))", g.place(dst).read(), width, width, x, width, width-1)
		goToTarget()
	case "copysignf32", "copysignf64":
		if len(args) != 2 {
			g.fail("%s arity", name)
		}
		x, t := g.operand(args[0])
		y, _ := g.operand(args[1])
		width := g.typ(t).Size * 8
		g.line("%s=math.Float%dfrombits((math.Float%dbits(%s)&^(uint%d(1)<<%d)) | (math.Float%dbits(%s)&(uint%d(1)<<%d)))", g.place(dst).read(), width, width, x, width, width-1, width, y, width, width-1)
		goToTarget()
	case "powif32", "powif64", "powf32", "powf64":
		if len(args) != 2 {
			g.fail("%s arity", name)
		}
		x, _ := g.operand(args[0])
		y, _ := g.operand(args[1])
		g.line("%s=%s(math.Pow(float64(%s),float64(%s)))", g.place(dst).read(), g.goType(g.place(dst).typ), x, y)
		goToTarget()
	case "minimum_number_nsz_f32", "maximum_number_nsz_f32", "minimum_number_nsz_f64", "maximum_number_nsz_f64":
		if len(args) != 2 {
			g.fail("%s arity", name)
		}
		x, _ := g.operand(args[0])
		y, _ := g.operand(args[1])
		fn := "min"
		if strings.HasPrefix(name, "maximum") {
			fn = "max"
		}
		g.line("{ a,b:=%s,%s; if math.IsNaN(float64(a)) { %s=b } else if math.IsNaN(float64(b)) { %s=a } else { %s=%s(a,b) } }", x, y, g.place(dst).read(), g.place(dst).read(), g.place(dst).read(), fn)
		goToTarget()
	case "assert_inhabited", "assert_zero_valid", "assert_mem_uninitialized_valid":
		valid, ok := g.f.IntrinsicValidity[bb]
		if !ok {
			g.fail("missing compiler validity result for %s", name)
		}
		if !valid {
			// rustc lowers invalid-value assertions to panic_nounwind; a
			// catch handler must never receive a fabricated panic payload.
			g.line("oxide.Abort(); return %s", g.returnZero(ret))
		} else {
			goToTarget()
		}
	case "unreachable":
		g.line("panic(\"Rust unreachable intrinsic\")")
	case "copy_nonoverlapping":
		if len(args) != 3 {
			g.fail("copy_nonoverlapping arity")
		}
		srcv, dt := g.operand(args[0])
		dstv, _ := g.operand(args[1])
		n, _ := g.operand(args[2])
		pt := g.typ(dt)
		if pt.Kind != "pointer" {
			g.fail("copy_nonoverlapping dst")
		}
		g.line("copy(unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d),unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d))", dstv, n, g.typ(pt.Pointee).Size, srcv, n, g.typ(pt.Pointee).Size)
		goToTarget()
	case "copy":
		if len(args) != 3 {
			g.fail("copy arity")
		}
		srcv, dt := g.operand(args[0])
		dstv, _ := g.operand(args[1])
		n, _ := g.operand(args[2])
		pt := g.typ(dt)
		if pt.Kind != "pointer" {
			g.fail("copy pointer")
		}
		g.line("copy(unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d),unsafe.Slice((*byte)(unsafe.Pointer(%s)),int(%s)*%d))", dstv, n, g.typ(pt.Pointee).Size, srcv, n, g.typ(pt.Pointee).Size)
		goToTarget()
	case "write_bytes":
		if len(args) != 3 {
			g.fail("write_bytes arity")
		}
		dstv, dt := g.operand(args[0])
		value, _ := g.operand(args[1])
		n, _ := g.operand(args[2])
		pt := g.typ(dt)
		if pt.Kind != "pointer" {
			g.fail("write_bytes pointer")
		}
		g.line("oxide.WriteBytes(unsafe.Pointer(%s),byte(%s),uintptr(%s)*%d)", dstv, value, n, g.typ(pt.Pointee).Size)
		goToTarget()
	case "ctlz", "ctlz_nonzero":
		x, t := g.operand(args[0])
		if g.typ(t).Kind == "u128" || g.typ(t).Kind == "i128" {
			g.fail("128-bit %s is not yet lowered", name)
		}
		g.line("%s=oxide.LeadingZeros(%s)", g.place(dst).read(), x)
		goToTarget()
	case "cttz", "cttz_nonzero":
		x, t := g.operand(args[0])
		if g.typ(t).Kind == "u128" || g.typ(t).Kind == "i128" {
			g.fail("128-bit %s is not yet lowered", name)
		}
		g.line("%s=oxide.TrailingZeros(%s)", g.place(dst).read(), x)
		goToTarget()
	case "ctpop":
		x, t := g.operand(args[0])
		if g.typ(t).Kind == "u128" || g.typ(t).Kind == "i128" {
			g.fail("128-bit ctpop is not yet lowered")
		}
		g.line("%s=oxide.OnesCount(%s)", g.place(dst).read(), x)
		goToTarget()
	case "bswap":
		x, t := g.operand(args[0])
		helper := "ByteSwap"
		if k := g.typ(t).Kind; k == "u128" || k == "i128" {
			helper = strings.ToUpper(k[:1]) + "128ByteSwap"
		}
		g.line("%s=oxide.%s(%s)", g.place(dst).read(), helper, x)
		goToTarget()
	case "bitreverse":
		x, t := g.operand(args[0])
		helper := "BitReverse"
		if g.typ(t).Kind == "u128" {
			helper = "U128ReverseBits"
		} else if g.typ(t).Kind == "i128" {
			helper = "I128ReverseBits"
		}
		g.line("%s=oxide.%s(%s)", g.place(dst).read(), helper, x)
		goToTarget()
	case "rotate_left", "rotate_right":
		x, _ := g.operand(args[0])
		n, _ := g.operand(args[1])
		fn := "RotateLeft"
		if name == "rotate_right" {
			fn = "RotateRight"
		}
		g.line("%s=oxide.%s(%s,uint32(%s))", g.place(dst).read(), fn, x, n)
		goToTarget()
	case "integer_min", "integer_max":
		a, _ := g.operand(args[0])
		b, _ := g.operand(args[1])
		fn := "IntegerMin"
		if name == "integer_max" {
			fn = "IntegerMax"
		}
		g.line("%s=oxide.%s(%s,%s)", g.place(dst).read(), fn, a, b)
		goToTarget()
	case "saturating_add", "saturating_sub":
		a, at := g.operand(args[0])
		b, _ := g.operand(args[1])
		fn := "SaturatingAdd"
		if name == "saturating_sub" {
			fn = "SaturatingSub"
		}
		switch g.scalar(g.typ(at)) {
		case "oxide.U128":
			fn = "U128" + fn
		case "oxide.I128":
			fn = "I128" + fn
		}
		g.line("%s=oxide.%s(%s,%s)", g.place(dst).read(), fn, a, b)
		goToTarget()
	case "select_unpredictable":
		cond, _ := g.operand(args[0])
		yes, _ := g.operand(args[1])
		no, _ := g.operand(args[2])
		if g.indirectValue(g.place(dst).typ) {
			g.line("if %s {", cond)
			g.storeValue(g.place(dst), yes)
			g.line("} else {")
			g.storeValue(g.place(dst), no)
			g.line("}")
		} else {
			g.line("%s=oxide.Select(%s,%s,%s)", g.place(dst).read(), cond, yes, no)
		}
		goToTarget()
	case "arith_offset", "ptr_offset_from", "ptr_offset_from_unsigned":
		p, pt := g.operand(args[0])
		n, _ := g.operand(args[1])
		if g.typ(pt).Kind != "pointer" {
			g.fail("%s pointer", name)
		}
		sz := g.typ(g.typ(pt).Pointee).Size
		if name == "arith_offset" {
			g.line("%s=uintptr(unsafe.Add(unsafe.Pointer(%s),int64(%s)*%d))", g.place(dst).read(), p, n, sz)
		} else {
			q, _ := g.operand(args[1])
			if name == "ptr_offset_from_unsigned" {
				g.line("%s=(%s-%s)/%d", g.place(dst).read(), p, q, sz)
			} else {
				g.line("%s=int64(%s-%s)/%d", g.place(dst).read(), p, q, sz)
			}
		}
		goToTarget()
	case "raw_eq", "compare_bytes":
		a, at := g.operand(args[0])
		b, _ := g.operand(args[1])
		if name == "raw_eq" {
			if len(args) != 2 || g.typ(at).Kind != "pointer" {
				g.fail("raw_eq arguments")
			}
			g.line("%s=oxide.MemoryEqual(unsafe.Pointer(%s),unsafe.Pointer(%s),%d)", g.place(dst).read(), a, b, g.typ(g.typ(at).Pointee).Size)
		} else {
			if len(args) != 3 {
				g.fail("compare_bytes arguments")
			}
			n, _ := g.operand(args[2])
			g.line("%s=oxide.CompareMemory(unsafe.Pointer(%s),unsafe.Pointer(%s),uintptr(%s))", g.place(dst).read(), a, b, n)
		}
		goToTarget()
	case "caller_location":
		g.line("%s = %s", g.place(dst).read(), g.callerLocation(bb))
		goToTarget()
	case "abort":
		g.line("oxide.Abort(); return %s", g.returnZero(ret))
	case "atomic_fence", "atomic_singlethreadfence":
		g.line("oxide.AtomicFence()")
		goToTarget()
	case "size_of_val", "align_of_val":
		if len(args) != 1 {
			g.fail("%s arity", name)
		}
		p := g.operandPlace(args[0])
		pt := g.typ(p.typ)
		if pt.Kind != "pointer" {
			g.fail("%s pointer", name)
		}
		inner := g.typ(pt.Pointee)
		if inner.Sized {
			if name == "size_of_val" {
				g.line("%s=%d", g.place(dst).read(), inner.Size)
			} else {
				g.line("%s=%d", g.place(dst).read(), inner.Align)
			}
		} else if inner.Kind == "slice" || inner.Kind == "str" {
			meta := fmt.Sprintf("*(*uintptr)(unsafe.Add(%s,8))", p.address)
			size, align := uint64(1), uint64(1)
			if inner.Kind == "slice" {
				size, align = g.typ(inner.Element).Size, g.typ(inner.Element).Align
			}
			if name == "size_of_val" {
				g.line("%s=%s*%d", g.place(dst).read(), meta, size)
			} else {
				g.line("%s=%d", g.place(dst).read(), align)
			}
		} else if inner.Kind == "dynamic" {
			vt := fmt.Sprintf("*(*uintptr)(unsafe.Add(%s,8))", p.address)
			idx := 1
			if name == "align_of_val" {
				idx = 2
			}
			g.line("%s=*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),%d))", g.place(dst).read(), vt, idx*8)
		} else if inner.Kind == "aggregate" {
			size, align, ok := g.dstSizeAlign(inner, fmt.Sprintf("*(*uintptr)(unsafe.Add(%s,8))", p.address))
			if !ok {
				g.fail("%s for unsized %s", name, inner.Name)
			}
			if name == "size_of_val" {
				g.line("%s=%s", g.place(dst).read(), size)
			} else {
				g.line("%s=%s", g.place(dst).read(), align)
			}
		} else {
			g.fail("%s for unsized %s", name, inner.Name)
		}
		goToTarget()
	case "vtable_size", "vtable_align":
		v, _ := g.operand(args[0])
		idx := 8
		if name == "vtable_align" {
			idx = 16
		}
		g.line("%s=*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),%d))", g.place(dst).read(), v, idx)
		goToTarget()
	case "atomic_load", "atomic_store", "atomic_xchg", "atomic_xadd", "atomic_xsub", "atomic_and", "atomic_or", "atomic_xor", "atomic_nand", "atomic_min", "atomic_max", "atomic_umin", "atomic_umax", "atomic_cxchg", "atomic_cxchgweak":
		if len(args) < 1 {
			g.fail("%s arity", name)
		}
		p, pt := g.operand(args[0])
		if g.typ(pt).Kind != "pointer" {
			g.fail("%s pointer", name)
		}
		width := g.typ(g.typ(pt).Pointee).Size
		if width != 1 && width != 2 && width != 4 && width != 8 {
			g.fail("atomic width %d", width)
		}
		ptr := fmt.Sprintf("unsafe.Pointer(%s)", p)
		dstTypeID := g.f.Body.Locals[dst.Local].Type
		result := func(v string, id int) string {
			if g.typ(id).Kind == "bool" {
				return "(" + v + " != 0)"
			}
			return g.goType(id) + "(" + v + ")"
		}
		value := func(raw json.RawMessage, id int) string {
			x, _ := g.operand(raw)
			if g.typ(id).Kind == "bool" {
				return "func() uint64 { if " + x + " { return 1 }; return 0 }()"
			}
			return "uint64(" + x + ")"
		}
		switch name {
		case "atomic_load":
			g.line("%s=%s", g.place(dst).read(), result(fmt.Sprintf("oxide.AtomicLoad(%s,%d)", ptr, width), dstTypeID))
		case "atomic_store":
			if len(args) != 2 {
				g.fail("atomic_store arity")
			}
			_, vt := g.operand(args[1])
			g.line("oxide.AtomicStore(%s,%s,%d)", ptr, value(args[1], vt), width)
		case "atomic_xchg":
			if len(args) != 2 {
				g.fail("atomic_xchg arity")
			}
			_, vt := g.operand(args[1])
			g.line("%s=%s", g.place(dst).read(), result(fmt.Sprintf("oxide.AtomicSwap(%s,%s,%d)", ptr, value(args[1], vt), width), dstTypeID))
		case "atomic_xadd", "atomic_xsub", "atomic_and", "atomic_or", "atomic_xor", "atomic_nand", "atomic_min", "atomic_max", "atomic_umin", "atomic_umax":
			if len(args) != 2 {
				g.fail("%s arity", name)
			}
			_, vt := g.operand(args[1])
			fn := map[string]string{"atomic_xadd": "AtomicAdd", "atomic_xsub": "AtomicSub", "atomic_and": "AtomicAnd", "atomic_or": "AtomicOr", "atomic_xor": "AtomicXor", "atomic_nand": "AtomicNand", "atomic_min": "AtomicMin", "atomic_max": "AtomicMax", "atomic_umin": "AtomicUMin", "atomic_umax": "AtomicUMax"}[name]
			g.line("%s=%s", g.place(dst).read(), result(fmt.Sprintf("oxide.%s(%s,%s,%d)", fn, ptr, value(args[1], vt), width), dstTypeID))
		case "atomic_cxchg", "atomic_cxchgweak":
			if len(args) != 3 {
				g.fail("%s arity", name)
			}
			_, oldt := g.operand(args[1])
			_, newt := g.operand(args[2])
			old := fmt.Sprintf("oxide.AtomicCompareExchange(%s,%s,%s,%d)", ptr, value(args[1], oldt), value(args[2], newt), width)
			d := g.place(dst)
			if len(g.typ(dstTypeID).Fields) < 2 {
				g.fail("%s result layout", name)
			}
			g.line("{ old,ok:= %s; *(*%s)(unsafe.Add(%s,%d))=%s; *(*bool)(unsafe.Add(%s,%d))=ok }", old, g.goType(oldt), d.address, g.typ(dstTypeID).Fields[0], result("old", oldt), d.address, g.typ(dstTypeID).Fields[1])
		}
		goToTarget()
	case "typed_swap_nonoverlapping":
		if len(args) != 2 {
			g.fail("typed_swap_nonoverlapping arity")
		}
		x, t := g.operand(args[0])
		y, _ := g.operand(args[1])
		pt := g.typ(t)
		if pt.Kind != "pointer" {
			g.fail("typed_swap_nonoverlapping pointer")
		}
		gt := g.goType(pt.Pointee)
		g.line("{ x,y:=(*%s)(unsafe.Pointer(%s)),(*%s)(unsafe.Pointer(%s)); *x,*y=*y,*x }", gt, x, gt, y)
		g.line("goto bb%d", *target)
	case "volatile_load", "volatile_store":
		g.fail("%s requires a target volatile access implementation", name)
	case "exact_div":
		if len(args) != 2 {
			g.fail("exact_div arity")
		}
		l, _ := g.operand(args[0])
		r, _ := g.operand(args[1])
		g.line("%s = %s / %s", g.place(dst).read(), l, r)
		g.line("goto bb%d", *target)
	case "unchecked_add", "unchecked_sub", "unchecked_mul":
		if len(args) != 2 {
			g.fail("%s arity", name)
		}
		l, t := g.operand(args[0])
		r, _ := g.operand(args[1])
		op := map[string]string{"unchecked_add": "AddUnchecked", "unchecked_sub": "SubUnchecked", "unchecked_mul": "MulUnchecked"}[name]
		g.line("%s = %s", g.place(dst).read(), g.binary(op, l, r, t))
		g.line("goto bb%d", *target)
	case "transmute":
		if len(args) != 1 {
			g.fail("transmute arity")
		}
		src := g.operandPlace(args[0])
		dt := g.place(dst)
		st := g.typ(src.typ)
		if st.Size != g.typ(dt.typ).Size {
			g.fail("transmute size")
		}
		g.line("copy(unsafe.Slice((*byte)(%s),%d),unsafe.Slice((*byte)(%s),%d))", dt.address, st.Size, src.address, st.Size)
		g.line("goto bb%d", *target)
	default:
		g.fail("intrinsic %s", name)
	}
}

func (g *generator) dstSizeAlign(t *Type, meta string) (size, align string, ok bool) {
	if t.Kind == "slice" || t.Kind == "str" {
		es, ea := uint64(1), uint64(1)
		if t.Kind == "slice" {
			es = g.typ(t.Element).Size
			ea = g.typ(t.Element).Align
		}
		return fmt.Sprintf("(%s)*%d", meta, es), fmt.Sprintf("uintptr(%d)", ea), true
	}
	if t.Kind == "dynamic" {
		return fmt.Sprintf("*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),8))", meta), fmt.Sprintf("*(*uintptr)(unsafe.Add(unsafe.Pointer(%s),16))", meta), true
	}
	if t.Kind != "aggregate" || len(t.VariantFieldTypes) == 0 || len(t.VariantFieldTypes[0]) == 0 || len(t.Fields) != len(t.VariantFieldTypes[0]) {
		return "", "", false
	}
	i := len(t.VariantFieldTypes[0]) - 1
	tail := g.typ(t.VariantFieldTypes[0][i])
	ts, ta, ok := g.dstSizeAlign(tail, meta)
	if !ok {
		return "", "", false
	}
	ta = dstFieldAlign(t, ta)
	full := fmt.Sprintf("max(uintptr(%d),%s)", t.Align, ta)
	offset := dstAlignUp(fmt.Sprint(t.Fields[i]), ta)
	return dstAlignUp(fmt.Sprintf("(%s)+(%s)", offset, ts), full), full, true
}

// A DST field's compiler offset is the prefix before its runtime alignment.
// For example ArcInner<dyn Trait> has a 16-byte header, but an aligned payload
// can start at 32 or 64. Packed parents cap that field alignment.
func dstFieldAlign(parent *Type, align string) string {
	if parent.Pack != 0 {
		return fmt.Sprintf("min(uintptr(%d),%s)", parent.Pack, align)
	}
	return align
}

func dstAlignUp(offset, align string) string {
	return fmt.Sprintf("((uintptr(%s)+(%s)-1)&^((%s)-1))", offset, align, align)
}

func (g *generator) unwind(raw json.RawMessage, ret int) {
	k, v := variant(raw)
	switch k {
	case "Continue":
		g.line("return %s", g.returnZero(ret))
	case "Cleanup":
		g.line("unwind=ctx.TakePanic(); goto bb%d", decode[int](v))
	case "Terminate":
		g.line("oxide.Abort(); return %s", g.returnZero(ret))
	case "Unreachable":
		g.line("panic(\"unreachable Rust unwind\")")
	default:
		g.fail("unwind %s", k)
	}
}
