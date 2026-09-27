package mir

import (
	"strings"
	"testing"
)

func TestForeignNamesDoNotSelectRustShims(t *testing.T) {
	for _, name := range []string{"panic", "panic_fmt", "panic_nounwind", "capacity_overflow", "alloc_err", "handle_alloc_error", "__rust_alloc", "__rust_dealloc"} {
		t.Run(name, func(t *testing.T) {
			p := &Program{
				Target:    "aarch64-unknown-linux-gnu",
				Roots:     []Root{{Name: "foreign::" + name, Symbol: "foreign"}},
				Types:     []Type{{ID: 0, Name: "()", Kind: "aggregate", Sized: true, Align: 1}},
				Functions: []Function{{Name: "foreign::" + name, Symbol: "foreign", Kind: "Item", Signature: &Signature{Return: 0}}},
			}
			if _, err := Generate(p, "foreign"); err == nil || !strings.Contains(err.Error(), "external/intrinsic body requires lowering") {
				t.Fatalf("foreign declaration was mistaken for a Rust runtime boundary: %v", err)
			}
		})
	}
}

func TestCompilerRuntimeChecks(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, name := range []string{"OverflowChecks", "UbChecks", "ContractChecks"} {
			g := &generator{p: &Program{RuntimeChecks: map[string]bool{name: enabled}}, types: map[int]*Type{7: {ID: 7, Kind: "bool", Size: 1, Align: 1, Sized: true}}}
			got, ty := g.operand([]byte(`{"RuntimeChecks":"` + name + `"}`))
			if ty != 7 || (got == "true") != enabled {
				t.Fatalf("%s = %s (type %d), want %t", name, got, ty, enabled)
			}
		}
	}
}
