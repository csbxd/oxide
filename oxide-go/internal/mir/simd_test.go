package mir

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSIMDConstantPadding(t *testing.T) {
	raw := json.RawMessage(`{"Constant":{"const_":{"kind":{"Allocated":{"bytes":[1,0,0,0,null,null,null,null]}}}}}`)
	b, ok := constantBytePrefix(raw, 4)
	if !ok || len(b) != 4 || b[0] != 1 {
		t.Fatalf("initialized lanes rejected: %v %v", b, ok)
	}
	if _, ok := constantBytePrefix(raw, 8); ok {
		t.Fatal("uninitialized lane accepted")
	}
	if _, ok := constantBytePrefix(raw, 12); ok {
		t.Fatal("missing lane accepted")
	}
}

func TestSIMDUnarySnapshotsAndPadding(t *testing.T) {
	g := &generator{types: map[int]*Type{}}
	g.line("package fixture\nimport (\"math\"; \"unsafe\")")
	for _, width := range []int{32, 64} {
		elem, array, vector := width, width+1, width+2
		size := uint64(width / 8)
		g.types[elem] = &Type{ID: elem, Kind: fmt.Sprintf("f%d", width), Size: size, Align: size, Sized: true}
		g.types[array] = &Type{ID: array, Kind: "array", Size: size * 3, Align: size, Sized: true, Element: elem, Length: 3}
		g.types[vector] = &Type{ID: vector, Kind: "aggregate", Size: size * 4, Align: size * 4, Sized: true, ValueABI: "Vector", Fields: []uint64{0}, VariantFieldTypes: [][]int{{array}}}
		g.typeDecl(g.types[vector])
		for _, operation := range []string{"simd_neg", "simd_fabs"} {
			for _, alias := range []bool{false, true} {
				name := fmt.Sprintf("Unary%d_%s", width, operation)
				dst := 0
				if alias {
					name += "_Alias"
					dst = 1
				}
				g.f = &Function{Name: name, Body: &Body{Locals: []Local{{Type: vector}, {Type: vector}}}}
				g.line("func %s(src [4]uint%d) [4]uint%d { v1:=*(*T%d)(unsafe.Pointer(&src))", name, width, width, vector)
				if !alias {
					g.line("var v0 T%d", vector)
				}
				target := 1
				g.intrinsicCall(0, operation, []json.RawMessage{json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`)}, Place{Local: dst}, &target, nil, vector)
				g.line("bb1: return *(*[4]uint%d)(unsafe.Pointer(&v%d)) }", width, dst)
			}
		}
	}
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	testSIMDProgram(t, source, `package fixture
import "testing"
func TestUnaryBits(t *testing.T) {
 for _,tc:=range []struct{f func([4]uint32)[4]uint32; want [4]uint32}{
  {Unary32_simd_neg,[4]uint32{0x80000000,0,0xff800001,0}},
  {Unary32_simd_neg_Alias,[4]uint32{0x80000000,0,0xff800001,0xdeadbeef}},
  {Unary32_simd_fabs,[4]uint32{0,0,0x7f800001,0}},
  {Unary32_simd_fabs_Alias,[4]uint32{0,0,0x7f800001,0xdeadbeef}},
 } { requireNoGoAllocations(t,100,func(){if got:=tc.f([4]uint32{0,0x80000000,0x7f800001,0xdeadbeef});got!=tc.want{t.Fatalf("f32 got %x, want %x",got,tc.want)}}) }
 for _,tc:=range []struct{f func([4]uint64)[4]uint64; want [4]uint64}{
  {Unary64_simd_neg,[4]uint64{0x8000000000000000,0,0x7ff0000000000001,0}},
  {Unary64_simd_neg_Alias,[4]uint64{0x8000000000000000,0,0x7ff0000000000001,0x0123456789abcdef}},
  {Unary64_simd_fabs,[4]uint64{0,0,0x7ff0000000000001,0}},
  {Unary64_simd_fabs_Alias,[4]uint64{0,0,0x7ff0000000000001,0x0123456789abcdef}},
 } { requireNoGoAllocations(t,100,func(){if got:=tc.f([4]uint64{0,0x8000000000000000,0xfff0000000000001,0x0123456789abcdef});got!=tc.want{t.Fatalf("f64 got %x, want %x",got,tc.want)}}) }
}
`)
}

func TestSIMDSqrt(t *testing.T) {
	g := &generator{types: map[int]*Type{}}
	g.line("package fixture\nimport (\"math\"; \"unsafe\")")
	for _, tc := range []struct {
		bits, lanes int
	}{{32, 4}, {64, 2}} {
		elem, array, vector := tc.bits, tc.bits+1, tc.bits+2
		size := uint64(tc.bits / 8)
		g.types[elem] = &Type{ID: elem, Kind: fmt.Sprintf("f%d", tc.bits), Size: size, Align: size, Sized: true}
		g.types[array] = &Type{ID: array, Kind: "array", Size: size * uint64(tc.lanes), Align: size, Sized: true, Element: elem, Length: uint64(tc.lanes)}
		g.types[vector] = &Type{ID: vector, Kind: "aggregate", Size: 16, Align: 16, Sized: true, ValueABI: "Vector", Fields: []uint64{0}, VariantFieldTypes: [][]int{{array}}}
		g.typeDecl(g.types[vector])
		for _, inplace := range []bool{false, true} {
			name := fmt.Sprintf("Sqrt%d", tc.bits)
			dst := 0
			if inplace {
				name += "InPlace"
				dst = 1
			}
			g.f = &Function{Name: name, Body: &Body{Locals: []Local{{Type: vector}, {Type: vector}}}}
			g.line("func %s(x [%d]float%d) [%d]float%d { v1:=*(*T%d)(unsafe.Pointer(&x))", name, tc.lanes, tc.bits, tc.lanes, tc.bits, vector)
			if !inplace {
				g.line("var v0 T%d", vector)
			}
			target := 1
			g.intrinsicCall(0, "simd_fsqrt", []json.RawMessage{json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`)}, Place{Local: dst}, &target, nil, vector)
			g.line("bb1: return *(*[%d]float%d)(unsafe.Pointer(&v%d)) }", tc.lanes, tc.bits, dst)
		}
	}
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatalf("format: %v\n%s", err, g.b.String())
	}
	const check = `package fixture
import ("math"; "testing")
func TestSqrtLanes(t *testing.T) {
 for _, f := range []func([4]float32) [4]float32{Sqrt32, Sqrt32InPlace} {
  for _, tc := range []struct{ in, want [4]uint32 }{
   {[4]uint32{0,0x80000000,0x3f800000,0x40800000}, [4]uint32{0,0x80000000,0x3f800000,0x40000000}},
   {[4]uint32{0x40000000,0x41100000,0x00800000,0x7f800000}, [4]uint32{0x3fb504f3,0x40400000,0x20000000,0x7f800000}},
  } {
   var x [4]float32
   for i,v := range tc.in { x[i]=math.Float32frombits(v) }
   got:=f(x)
   for i,v := range got { if math.Float32bits(v)!=tc.want[i] { t.Fatalf("f32 lane %d: %08x => %08x, want %08x",i,tc.in[i],math.Float32bits(v),tc.want[i]) } }
  }
  got:=f([4]float32{-1,float32(math.Inf(-1)),float32(math.NaN()),25})
  for i:=0;i<3;i++ { if !math.IsNaN(float64(got[i])) { t.Fatalf("f32 NaN lane %d: %v",i,got) } }
  if got[3]!=5 { t.Fatalf("f32 last lane: %v",got) }
 }
 for _, f := range []func([2]float64) [2]float64{Sqrt64, Sqrt64InPlace} {
  for _, tc := range []struct{ in, want [2]uint64 }{
   {[2]uint64{0,0x8000000000000000}, [2]uint64{0,0x8000000000000000}},
   {[2]uint64{0x4000000000000000,0x4022000000000000}, [2]uint64{0x3ff6a09e667f3bcd,0x4008000000000000}},
   {[2]uint64{1,0x7ff0000000000000}, [2]uint64{0x1e60000000000000,0x7ff0000000000000}},
  } {
   x:=[2]float64{math.Float64frombits(tc.in[0]),math.Float64frombits(tc.in[1])}
   got:=f(x)
   for i,v := range got { if math.Float64bits(v)!=tc.want[i] { t.Fatalf("f64 lane %d: %016x => %016x, want %016x",i,tc.in[i],math.Float64bits(v),tc.want[i]) } }
  }
  for _,x:=range [][2]float64{{-1,math.Inf(-1)},{math.NaN(),-4}} { for i,v:=range f(x) { if !math.IsNaN(v) { t.Fatalf("f64 NaN lane %d: %v",i,v) } } }
 }
}
`
	testSIMDProgram(t, source, check)
}

func testSIMDProgram(t *testing.T, source []byte, check string) {
	t.Helper()
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string][]byte{
		"go.mod":  []byte(fmt.Sprintf("module oxide-simd-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module)),
		"simd.go": source, "simd_test.go": []byte(check),
		"allocations_test.go": allocations,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), value, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		args := []string{"test", "-mod=mod", "-count=1", "."}
		if arch != runtime.GOARCH {
			args = []string{"test", "-mod=mod", "-c", "-o", filepath.Join(dir, "simd_"+arch+".test"), "."}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "GOWORK=off", "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generated SIMD %s: %v\n%s\n%s", arch, err, output, source)
		}
	}
}

func TestSIMDSqrtRejectsIntegerLanes(t *testing.T) {
	g := &generator{
		types: map[int]*Type{
			1: {ID: 1, Kind: "u32", Size: 4, Align: 4, Sized: true},
			2: {ID: 2, Kind: "array", Size: 16, Align: 4, Sized: true, Element: 1, Length: 4},
		},
		f: &Function{Body: &Body{Locals: []Local{{Type: 2}, {Type: 2}}}},
	}
	defer func() {
		err, ok := recover().(generationError)
		if !ok || !strings.Contains(string(err), "floating-point lane layout") {
			t.Fatalf("expected float layout error, got %v", err)
		}
	}()
	g.simdIntrinsic("simd_fsqrt", []json.RawMessage{json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`)}, Place{Local: 0})
}
