package mir

import (
	"go/format"
	"testing"
)

func TestFloatOperationsRoundSeparately(t *testing.T) {
	g := &generator{types: map[int]*Type{
		32: {ID: 32, Kind: "f32", Sized: true, Size: 4, Align: 4},
		64: {ID: 64, Kind: "f64", Sized: true, Size: 8, Align: 8},
	}}
	g.line("package fixture")
	for _, width := range []int{32, 64} {
		g.line("//go:noinline\nfunc Separate%d(a,b,c float%d) float%d {", width, width, width)
		g.line("p := %s; return %s }", g.binary("Mul", "a", "b", width), g.binary("Add", "p", "c", width))
	}
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	testSIMDProgram(t, source, `package fixture
import("math";"testing")
func TestRounding(t *testing.T) {
 // The multiplication rounds to 1 before addition. FMA would instead
 // expose the negative residual, -2^-46 or -2^-104 respectively.
 if x:=Separate32(math.Float32frombits(0x3f800001),math.Float32frombits(0x3f7ffffe),-1); math.Float32bits(x)!=0 { t.Fatalf("f32 operations fused: %08x",math.Float32bits(x)) }
 if x:=Separate64(math.Float64frombits(0x3ff0000000000001),math.Float64frombits(0x3feffffffffffffe),-1); math.Float64bits(x)!=0 { t.Fatalf("f64 operations fused: %016x",math.Float64bits(x)) }
}`)
}
