package mir

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

func laneExpr(g *generator, p location, elem, i int) string {
	return fmt.Sprintf("*(*%s)(unsafe.Add(%s,%d))", g.goType(elem), p.address, uint64(i)*g.typ(elem).Size)
}

type neonSpec struct {
	helper        string
	input, output uint64
	binary, shift bool
}

var neonOps = map[string]neonSpec{
	"llvm.aarch64.neon.frecpe.v4f32":       {"NeonReciprocalF32x4", 16, 16, false, false},
	"llvm.aarch64.neon.frecps.v4f32":       {"NeonReciprocalStepF32x4", 16, 16, true, false},
	"llvm.aarch64.neon.fmax.v4f32":         {"NeonMaxF32x4", 16, 16, true, false},
	"llvm.aarch64.neon.fmin.v4f32":         {"NeonMinF32x4", 16, 16, true, false},
	"llvm.aarch64.neon.fcvtns.v4i32.v4f32": {"NeonRoundToI32x4", 16, 16, false, false},
	"llvm.fptosi.sat.v4i32.v4f32":          {"NeonTruncToI32x4", 16, 16, false, false},
	"llvm.roundeven.v4f32":                 {"NeonRoundEvenF32x4", 16, 16, false, false},
	"llvm.aarch64.neon.sqdmulh.v2i32":      {"NeonDoubleMulHighI32x2", 8, 8, true, false},
	"llvm.aarch64.neon.sqdmulh.v4i32":      {"NeonDoubleMulHighI32x4", 16, 16, true, false},
	"llvm.aarch64.neon.sqdmulh.v8i16":      {"NeonDoubleMulHighI16x8", 16, 16, true, false},
	"llvm.aarch64.neon.sqxtn.v4i16":        {"NeonNarrowI32x4", 16, 8, false, false},
	"llvm.aarch64.neon.uqxtn.v8i8":         {"NeonNarrowU16x8", 16, 8, false, false},
	"llvm.aarch64.neon.sqshrun.v4i16":      {"NeonShiftNarrowI32x4", 16, 8, false, true},
	"llvm.aarch64.neon.umaxp.v16i8":        {"NeonPairwiseMaxU8x16", 16, 16, true, false},
	"llvm.aarch64.neon.tbl1.v16i8":         {"NeonTableU8x16", 16, 16, true, false},
}

func (g *generator) neonIntrinsic(name string, args []json.RawMessage, dst Place) {
	if g.p.Target != "aarch64-unknown-linux-gnu" {
		g.fail("ARM intrinsic in target %s", g.p.Target)
	}
	if name == "llvm.aarch64.isb" {
		if len(args) != 1 {
			g.fail("%s arity", name)
		}
		b, ok := constantBytes(args[0])
		if !ok || len(b) != 4 || binary.LittleEndian.Uint32(b) != 15 {
			g.fail("%s requires SY barrier", name)
		}
		g.line("oxide.InstructionBarrier()")
		return
	}
	if name == "llvm.aarch64.crc32b" || name == "llvm.aarch64.crc32x" {
		if len(args) != 2 {
			g.fail("%s arity", name)
		}
		a, _ := g.operand(args[0])
		b, _ := g.operand(args[1])
		if name == "llvm.aarch64.crc32b" {
			g.line("%s=oxide.CRC32Byte(uint32(%s),uint32(%s))", g.place(dst).read(), a, b)
		} else {
			g.line("%s=oxide.CRC32X(uint32(%s),uint64(%s))", g.place(dst).read(), a, b)
		}
		return
	}
	spec, ok := neonOps[name]
	if !ok {
		g.fail("ARM intrinsic %s", name)
	}
	want := 1
	if spec.binary || spec.shift {
		want = 2
	}
	if len(args) != want {
		g.fail("%s arity", name)
	}
	a := g.operandPlace(args[0])
	d := g.place(dst)
	if g.typ(a.typ).Size != spec.input || g.typ(d.typ).Size != spec.output {
		g.fail("%s vector width", name)
	}
	if spec.binary {
		b := g.operandPlace(args[1])
		if g.typ(b.typ).Size != spec.input {
			g.fail("%s second vector width", name)
		}
		g.line("oxide.%s(%s,%s,%s)", spec.helper, d.address, a.address, b.address)
	} else if spec.shift {
		b, ok := constantBytes(args[1])
		if !ok || len(b) != 4 {
			g.fail("%s shift constant", name)
		}
		n := binary.LittleEndian.Uint32(b)
		if n < 1 || n > 16 {
			g.fail("%s shift %d", name, n)
		}
		g.line("oxide.%s(%s,%s,%d)", spec.helper, d.address, a.address, n)
	} else {
		g.line("oxide.%s(%s,%s)", spec.helper, d.address, a.address)
	}
}
