package mir

import "encoding/json"

type x86IntegerSpec struct {
	helper string
	width  uint64
	sized  bool
}

var x86IntegerOps = map[string]x86IntegerSpec{
	"llvm.x86.avx2.packssdw":         {"X86PackI32ToI16", 32, false},
	"llvm.x86.avx2.packusdw":         {"X86PackI32ToU16", 32, false},
	"llvm.x86.avx2.packuswb":         {"X86PackI16ToU8", 32, false},
	"llvm.x86.avx2.pmadd.ub.sw":      {"X86MaddU8I8", 32, true},
	"llvm.x86.avx2.pmadd.wd":         {"X86MaddI16", 32, true},
	"llvm.x86.avx2.psad.bw":          {"X86SADU8", 32, true},
	"llvm.x86.avx2.pshuf.b":          {"X86ShuffleU8", 32, true},
	"llvm.x86.sse2.pmadd.wd":         {"X86MaddI16", 16, true},
	"llvm.x86.sse2.psad.bw":          {"X86SADU8", 16, true},
	"llvm.x86.sse2.psrl.d":           {"X86ShiftRightU32", 16, false},
	"llvm.x86.ssse3.pmadd.ub.sw.128": {"X86MaddU8I8", 16, true},
	"llvm.x86.ssse3.pshuf.b.128":     {"X86ShuffleU8", 16, true},
	"llvm.x86.pclmulqdq":             {"X86CarrylessMul64", 16, false},
}

func (g *generator) x86IntegerIntrinsic(name string, args []json.RawMessage, dst Place) bool {
	spec, ok := x86IntegerOps[name]
	if !ok {
		return false
	}
	if g.p.Target != "x86_64-unknown-linux-gnu" {
		g.fail("x86 intrinsic in target %s", g.p.Target)
	}
	want := 2
	if name == "llvm.x86.pclmulqdq" {
		want = 3
	}
	if len(args) != want {
		g.fail("%s arity", name)
	}
	d := g.place(dst)
	a, at := g.operand(args[0])
	b, bt := g.operand(args[1])
	if g.typ(d.typ).Size != spec.width || g.typ(at).Size != spec.width || g.typ(bt).Size != spec.width {
		g.fail("%s vector width", name)
	}
	g.line("{ a,b := %s,%s", a, b)
	if name == "llvm.x86.pclmulqdq" {
		// Rust's const-generic i32 is cast to a u8 local in unoptimized
		// MIR. The software operation can consume that value directly.
		immediate, id := g.operand(args[2])
		if kind := g.typ(id).Kind; kind != "u8" && kind != "i8" {
			g.fail("%s requires a byte operand", name)
		}
		g.line("oxide.%s(%s,unsafe.Pointer(&a),unsafe.Pointer(&b),byte(%s))", spec.helper, d.address, immediate)
	} else if spec.sized {
		g.line("oxide.%s(%s,unsafe.Pointer(&a),unsafe.Pointer(&b),%d)", spec.helper, d.address, spec.width)
	} else {
		g.line("oxide.%s(%s,unsafe.Pointer(&a),unsafe.Pointer(&b))", spec.helper, d.address)
	}
	g.line("}")
	return true
}
