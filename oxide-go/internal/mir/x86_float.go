package mir

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// LLVM's SSE intrinsics retain x86-specific FP semantics, including the second
// operand's NaN/zero selection and RCPPS's hardware approximation.
func (g *generator) x86FloatIntrinsic(name string, args []json.RawMessage, dst Place) bool {
	helper, count := "", 1
	switch name {
	case "llvm.x86.sse.max.ps":
		helper, count = "X86MaxF32x4", 2
	case "llvm.x86.sse.min.ps":
		helper, count = "X86MinF32x4", 2
	case "llvm.x86.sse.rcp.ps":
		helper = "X86ReciprocalF32x4"
	case "llvm.x86.sse.cmp.ps":
		helper, count = "X86CompareF32x4", 3
	case "llvm.x86.sse2.cvtps2dq":
		helper = "X86RoundToI32x4"
	case "llvm.x86.sse2.cvttps2dq":
		helper = "X86TruncToI32x4"
	case "llvm.x86.sse2.pause":
		helper, count = "X86Pause", 0
	case "llvm.x86.xgetbv":
		helper = "X86Xgetbv"
	default:
		return false
	}
	if g.p.Target != "x86_64-unknown-linux-gnu" {
		g.fail("x86 intrinsic in target %s", g.p.Target)
	}
	if len(args) != count {
		g.fail("%s arity", name)
	}
	d := g.place(dst)
	if count == 0 {
		if g.typ(d.typ).Size != 0 {
			g.fail("%s return layout", name)
		}
		g.line("oxide.X86Pause()")
		return true
	}
	if helper == "X86Xgetbv" {
		x, t := g.operand(args[0])
		in, out := g.typ(t).Kind, g.typ(d.typ).Kind
		if (in != "u32" && in != "i32") || (out != "u64" && out != "i64") {
			g.fail("%s scalar layout", name)
		}
		g.line("%s=%s(oxide.X86Xgetbv(uint32(%s)))", d.read(), g.goType(d.typ), x)
		return true
	}
	a, at := g.operand(args[0])
	if g.typ(at).Size != 16 || g.typ(d.typ).Size != 16 {
		g.fail("%s vector width", name)
	}
	if count == 1 {
		g.line("{ a := %s; oxide.%s(%s,unsafe.Pointer(&a)) }", a, helper, d.address)
		return true
	}
	b, bt := g.operand(args[1])
	if g.typ(bt).Size != 16 {
		g.fail("%s second vector width", name)
	}
	if count == 2 {
		g.line("{ a,b := %s,%s; oxide.%s(%s,unsafe.Pointer(&a),unsafe.Pointer(&b)) }", a, b, helper, d.address)
		return true
	}
	predicate, ok := constantBytes(args[2])
	if !ok || len(predicate) != 1 || predicate[0] > 31 {
		g.fail("%s requires a constant predicate in 0..31", name)
	}
	g.line("{ a,b := %s,%s; oxide.%s(%s,unsafe.Pointer(&a),unsafe.Pointer(&b),%d) }", a, b, helper, d.address, predicate[0])
	return true
}

// Match the complete stdarch CPUID template, ignoring only source spans. The
// register constraints and operand directions are checked separately below.
func x86CPUIDTemplate(template string) bool {
	if !strings.HasPrefix(template, "[") || !strings.HasSuffix(template, "]") {
		return false
	}
	s := template[1 : len(template)-1]
	var source strings.Builder
	cursor := 0
	for _, span := range asmTemplatePiece.FindAllStringIndex(s, -1) {
		if strings.Trim(s[cursor:span[0]], " ,\n\t") != "" {
			return false
		}
		piece := s[span[0]:span[1]]
		if strings.HasPrefix(piece, "String(") {
			literal, err := strconv.Unquote(piece[7 : len(piece)-1])
			if err != nil {
				return false
			}
			source.WriteString(literal)
		} else if strings.HasPrefix(piece, "Placeholder { operand_idx: 0, modifier: Some('r'), span: ") {
			source.WriteString("{0:r}")
		} else {
			return false
		}
		cursor = span[1]
	}
	return strings.Trim(s[cursor:], " ,\n\t") == "" && source.String() == "mov {0:r}, rbx\ncpuid\nxchg {0:r}, rbx"
}

func (g *generator) x86CPUIDAsm(raw json.RawMessage) bool {
	x := decode[struct {
		Template    string          `json:"template"`
		Destination *int            `json:"destination"`
		Options     string          `json:"options"`
		Unwind      json.RawMessage `json:"unwind"`
		Operands    []struct {
			Input  json.RawMessage `json:"in_value"`
			Output *Place          `json:"out_place"`
			Raw    string          `json:"raw_rpr"`
		} `json:"operands"`
	}](raw)
	if !x86CPUIDTemplate(x.Template) {
		return false
	}
	if g.p.Target != "x86_64-unknown-linux-gnu" {
		g.fail("x86 CPUID asm in target %s", g.p.Target)
	}
	if x.Destination == nil || x.Options != "PRESERVES_FLAGS | NOSTACK" || string(x.Unwind) != `"Unreachable"` || len(x.Operands) != 4 {
		g.fail("unsupported CPUID asm control flow or options")
	}
	var outputs [4]string
	var inputs [2]string
	for i, operand := range x.Operands {
		if operand.Output == nil || len(operand.Output.Projection) != 0 {
			g.fail("unsupported CPUID asm output")
		}
		for _, prior := range x.Operands[:i] {
			if prior.Output.Local == operand.Output.Local {
				g.fail("CPUID asm outputs must be distinct locals")
			}
		}
		out := g.place(*operand.Output)
		if g.typ(out.typ).Kind != "u32" {
			g.fail("CPUID asm output must be u32")
		}
		outputs[i] = out.read()
		output := fmt.Sprintf("_%d", operand.Output.Local)
		var want string
		switch i {
		case 0, 3:
			if string(operand.Input) != "null" {
				g.fail("unexpected CPUID asm input")
			}
			reg := "RegClass(X86(reg))"
			if i == 3 {
				reg = "Reg(X86(dx))"
			}
			want = fmt.Sprintf("Out { reg: %s, late: false, place: Some(%s) }", reg, output)
		case 1, 2:
			kind, data := variant(operand.Input)
			if kind != "Copy" && kind != "Move" {
				g.fail("unsupported CPUID asm input")
			}
			p := decode[Place](data)
			if len(p.Projection) != 0 || g.typ(g.place(p).typ).Kind != "u32" {
				g.fail("CPUID asm input must be a u32 local")
			}
			inputs[i-1], _ = g.operand(operand.Input)
			reg := "ax"
			if i == 2 {
				reg = "cx"
			}
			want = fmt.Sprintf("InOut { reg: Reg(X86(%s)), late: false, in_value: %s _%d, out_place: Some(%s) }", reg, strings.ToLower(kind), p.Local, output)
		}
		if operand.Raw != want {
			g.fail("unsupported CPUID asm register constraint")
		}
	}
	g.line("%s,%s,%s,%s=oxide.X86CPUID(%s,%s)", outputs[1], outputs[0], outputs[2], outputs[3], inputs[0], inputs[1])
	g.line("goto bb%d", *x.Destination)
	return true
}
