package mir

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const cpuidTemplate = `[String("mov "), Placeholder { operand_idx: 0, modifier: Some('r'), span: cpuid.rs:89:18: 89:23 (#0) }, String(", rbx"), String("\n"), String("cpuid"), String("\n"), String("xchg "), Placeholder { operand_idx: 0, modifier: Some('r'), span: cpuid.rs:91:19: 91:24 (#0) }, String(", rbx")]`

func TestX86CPUIDTemplate(t *testing.T) {
	if !x86CPUIDTemplate(cpuidTemplate) {
		t.Fatal("rejected stdarch CPUID template")
	}
	for _, source := range []string{
		strings.Replace(cpuidTemplate, `String("cpuid")`, `String("rdtsc")`, 1),
		strings.Replace(cpuidTemplate, "operand_idx: 0", "operand_idx: 1", 1),
		strings.Replace(cpuidTemplate, "Some('r')", "None", 1),
		strings.Replace(cpuidTemplate, `String("cpuid")`, `String("cpuid; syscall")`, 1),
		strings.Replace(cpuidTemplate, `String("cpuid")`, `Unknown("cpuid")`, 1),
		strings.Replace(cpuidTemplate, "rbx", "ebx", 1),
		cpuidTemplate + `, String("syscall")`,
	} {
		if x86CPUIDTemplate(source) {
			t.Errorf("accepted unsupported CPUID template: %s", source)
		}
	}
}

func x86TestGenerator() *generator {
	g := &generator{p: &Program{Target: "x86_64-unknown-linux-gnu"}, types: map[int]*Type{
		0: {ID: 0, Kind: "u32", Size: 4, Align: 4, Sized: true},
		1: {ID: 1, Kind: "aggregate", Size: 16, Align: 16, Sized: true},
		2: {ID: 2, Kind: "u64", Size: 8, Align: 8, Sized: true},
		3: {ID: 3, Kind: "aggregate", Sized: true},
		4: {ID: 4, Kind: "i64", Size: 8, Align: 8, Sized: true},
	}}
	g.f = &Function{Name: "x86-test", Body: &Body{Locals: make([]Local, 5)}}
	return g
}

func cpuidAsmMetadata() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
 "template":%q,"options":"PRESERVES_FLAGS | NOSTACK","destination":4,"unwind":"Unreachable",
 "operands":[
 {"in_value":null,"out_place":{"local":0,"projection":[]},"raw_rpr":"Out { reg: RegClass(X86(reg)), late: false, place: Some(_0) }"},
 {"in_value":{"Move":{"local":4,"projection":[]}},"out_place":{"local":1,"projection":[]},"raw_rpr":"InOut { reg: Reg(X86(ax)), late: false, in_value: move _4, out_place: Some(_1) }"},
 {"in_value":{"Copy":{"local":3,"projection":[]}},"out_place":{"local":2,"projection":[]},"raw_rpr":"InOut { reg: Reg(X86(cx)), late: false, in_value: copy _3, out_place: Some(_2) }"},
 {"in_value":null,"out_place":{"local":3,"projection":[]},"raw_rpr":"Out { reg: Reg(X86(dx)), late: false, place: Some(_3) }"}]}`, cpuidTemplate))
}

func TestX86CPUIDAsm(t *testing.T) {
	g := x86TestGenerator()
	if !g.x86CPUIDAsm(cpuidAsmMetadata()) {
		t.Fatal("CPUID asm not handled")
	}
	if want := "v1,v0,v2,v3=oxide.X86CPUID(v4,v3)\ngoto bb4\n"; g.b.String() != want {
		t.Fatalf("CPUID register mapping: %q, want %q", g.b.String(), want)
	}
	for _, tc := range []struct{ from, to string }{
		{"X86(cx)", "X86(ax)"},
		{"late: false", "late: true"},
		{"PRESERVES_FLAGS | NOSTACK", "NOSTACK"},
		{`"Unreachable"`, `"Continue"`},
		{`"destination":4`, `"destination":null`},
	} {
		t.Run(tc.to, func(t *testing.T) {
			expectX86Rejection(t, func() {
				x86TestGenerator().x86CPUIDAsm(json.RawMessage(strings.Replace(string(cpuidAsmMetadata()), tc.from, tc.to, 1)))
			})
		})
	}
	expectX86Rejection(t, func() {
		g := x86TestGenerator()
		g.p.Target = "aarch64-unknown-linux-gnu"
		g.x86CPUIDAsm(cpuidAsmMetadata())
	})
}

func expectX86Rejection(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if _, ok := recover().(generationError); !ok {
			t.Fatal("expected x86 lowering rejection")
		}
	}()
	fn()
}

func TestX86FloatIntrinsic(t *testing.T) {
	for name, count := range map[string]int{
		"llvm.x86.sse.max.ps": 2, "llvm.x86.sse.min.ps": 2,
		"llvm.x86.sse.rcp.ps": 1, "llvm.x86.sse.cmp.ps": 3,
		"llvm.x86.sse2.cvtps2dq": 1, "llvm.x86.sse2.cvttps2dq": 1,
		"llvm.x86.sse2.pause": 0, "llvm.x86.xgetbv": 1,
	} {
		t.Run(name, func(t *testing.T) {
			g := x86TestGenerator()
			for i := range g.f.Body.Locals {
				g.f.Body.Locals[i].Type = 1
			}
			if count == 0 {
				g.f.Body.Locals[0].Type = 3
			} else if name == "llvm.x86.xgetbv" {
				g.f.Body.Locals[0].Type = 4
				g.f.Body.Locals[1].Type = 0
			}
			args := make([]json.RawMessage, count)
			for i := range args {
				args[i] = json.RawMessage(fmt.Sprintf(`{"Copy":{"local":%d,"projection":[]}}`, i+1))
			}
			if count == 3 {
				args[2] = json.RawMessage(`{"Constant":{"const_":{"kind":{"Allocated":{"bytes":[7]}}}}}`)
			}
			if !g.x86FloatIntrinsic(name, args, Place{Local: 0}) || !strings.Contains(g.b.String(), "oxide.X86") {
				t.Fatal("x86 intrinsic not handled")
			}
			g.p.Target = "aarch64-unknown-linux-gnu"
			expectX86Rejection(t, func() { g.x86FloatIntrinsic(name, args, Place{Local: 0}) })
		})
	}
	if x86TestGenerator().x86FloatIntrinsic("llvm.x86.unrecognized", nil, Place{}) {
		t.Fatal("unrecognized intrinsic accepted")
	}
	for _, predicate := range []string{"[32]", "[0,0,0,0]"} {
		expectX86Rejection(t, func() {
			g := x86TestGenerator()
			for i := range g.f.Body.Locals {
				g.f.Body.Locals[i].Type = 1
			}
			g.x86FloatIntrinsic("llvm.x86.sse.cmp.ps", []json.RawMessage{
				json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`),
				json.RawMessage(`{"Copy":{"local":2,"projection":[]}}`),
				json.RawMessage(`{"Constant":{"const_":{"kind":{"Allocated":{"bytes":` + predicate + `}}}}}`),
			}, Place{Local: 0})
		})
	}
}

func TestX86FloatConstantOperands(t *testing.T) {
	for _, name := range []string{"llvm.x86.sse.rcp.ps", "llvm.x86.sse.max.ps", "llvm.x86.sse.cmp.ps"} {
		g := x86TestGenerator()
		g.f.Body.Locals[0].Type = 1
		value := json.RawMessage(`{"Constant":{"const_":{"ty":1,"kind":{"Allocated":{"bytes":[0,0,128,63,0,0,0,64,0,0,64,64,0,0,128,64]}}}}}`)
		args := []json.RawMessage{value}
		if name != "llvm.x86.sse.rcp.ps" {
			args = append(args, value)
		}
		if name == "llvm.x86.sse.cmp.ps" {
			args = append(args, json.RawMessage(`{"Constant":{"const_":{"kind":{"Allocated":{"bytes":[0]}}}}}`))
		}
		if !g.x86FloatIntrinsic(name, args, Place{Local: 0}) || !strings.Contains(g.b.String(), "unsafe.Pointer(&a)") {
			t.Fatalf("constant vector not materialized for %s: %s", name, g.b.String())
		}
	}
}

func TestX86FloatComparisonPredicates(t *testing.T) {
	for predicate := range 32 {
		g := x86TestGenerator()
		for i := range g.f.Body.Locals {
			g.f.Body.Locals[i].Type = 1
		}
		args := []json.RawMessage{
			json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`),
			json.RawMessage(`{"Copy":{"local":2,"projection":[]}}`),
			json.RawMessage(fmt.Sprintf(`{"Constant":{"const_":{"kind":{"Allocated":{"bytes":[%d]}}}}}`, predicate)),
		}
		if !g.x86FloatIntrinsic("llvm.x86.sse.cmp.ps", args, Place{Local: 0}) || !strings.Contains(g.b.String(), fmt.Sprintf(",%d)", predicate)) {
			t.Fatalf("predicate %d rejected or changed: %s", predicate, g.b.String())
		}
	}
}
