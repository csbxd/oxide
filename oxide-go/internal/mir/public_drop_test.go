package mir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func publicDropTestProgram() *Program {
	p := &Program{
		Target: "aarch64-unknown-linux-gnu",
		Types: []Type{
			{ID: 1, Kind: "u64", Sized: true, Size: 8, Align: 8},
			{ID: 2, Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 1},
			{ID: 3, Kind: "aggregate", Sized: true, Align: 1},
			{ID: 4, Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 5},
			{ID: 5, Kind: "aggregate", Sized: true, Size: 64, Align: 64, Fields: []uint64{0}, VariantFieldTypes: [][]int{{2}}},
			{ID: 6, Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 7},
			{ID: 7, Kind: "aggregate", Sized: true, Size: 256 << 10, Align: 64, Fields: []uint64{0}, VariantFieldTypes: [][]int{{2}}},
		},
		PublicDropTypes: []PublicDropType{{Type: 5, Name: "demo::Aligned", Symbol: "small"}, {Type: 7, Name: "demo::Large", Symbol: "large"}},
		PublicTypes:     []PublicType{{Name: "demo::Aligned", Type: 5}, {Name: "demo::Large", Type: 7}},
	}
	for _, f := range []struct {
		symbol string
		param  int
	}{{"small", 4}, {"large", 6}} {
		p.Functions = append(p.Functions, Function{Symbol: f.symbol, Name: "core::ptr::drop_glue", Kind: "Shim", Body: &Body{
			ArgCount: 1, Locals: []Local{{Type: 3}, {Type: f.param}}, Blocks: []Block{{
				Statements: []Statement{{Kind: json.RawMessage(`{"Assign":[{"local":1,"projection":["Deref",{"Field":[0,2]},"Deref"]},{"Use":[{"Constant":{"const_":{"ty":1,"kind":{"Allocated":{"bytes":[42,0,0,0,0,0,0,0],"align":8}}}}}]}]}`)}},
				Terminator: Statement{Kind: json.RawMessage(`"Return"`)},
			}},
		}})
	}
	return p
}

func TestPublicDropValidation(t *testing.T) {
	for _, tt := range []struct {
		name, want string
		change     func(*Program)
	}{
		{"duplicate", "duplicate public drop type", func(p *Program) { p.PublicDropTypes = append(p.PublicDropTypes, p.PublicDropTypes[0]) }},
		{"missing type", "no sized Rust layout", func(p *Program) { p.PublicDropTypes[0].Type = 99999 }},
		{"unsized", "no sized Rust layout", func(p *Program) { p.Types[4].Sized = false }},
		{"missing destructor", "missing function", func(p *Program) { p.PublicDropTypes[0].Symbol = "missing" }},
		{"wrong pointee", "invalid destructor signature", func(p *Program) { p.PublicDropTypes[0].Type = 7; p.PublicDropTypes = p.PublicDropTypes[:1] }},
		{"tracked destructor", "invalid destructor signature", func(p *Program) { p.Functions[0].TrackCaller = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := publicDropTestProgram()
			tt.change(p)
			if _, err := Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
		})
	}
}

func TestPublicDropStorage(t *testing.T) {
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"allocations_test.go": bytes.Replace(allocations, []byte("package fixture"), []byte("package fixture_test"), 1),
		"go.mod":              []byte(fmt.Sprintf("module public-drop-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module)),
		"drop_test.go": []byte(`package fixture_test
import("testing";"unsafe"; f "public-drop-test"; oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestDropStorage(t *testing.T) {
 ctx:=oxide.NewContext();defer ctx.Close()
 counter:=ctx.Alloc(8,8)
 small:=f.TypeAligned.Uninit(ctx); *(*uintptr)(unsafe.Pointer(small.Addr))=counter
 large:=f.TypeLarge.Uninit(ctx);*(*uintptr)(unsafe.Pointer(large.Addr))=counter
 mark:=ctx.Mark()
 requireNoGoAllocations(t,100,func(){
  *(*uint64)(unsafe.Pointer(counter))=0;small.Drop(ctx)
  if *(*uint64)(unsafe.Pointer(counter))!=42 {t.Fatal("small drop target")}
  *(*uint64)(unsafe.Pointer(counter))=0;large.Drop(ctx)
  if *(*uint64)(unsafe.Pointer(counter))!=42 {t.Fatal("large drop target")}
  if ctx.Mark()!=mark||ctx.Failed(){t.Fatal("destructor leaked frame/panic")}
 })
}
`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		p := publicDropTestProgram()
		p.Target = map[string]string{"amd64": "x86_64-unknown-linux-gnu", "arm64": "aarch64-unknown-linux-gnu"}[arch]
		source, err := Generate(p, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		p.PublicDropTypes[0], p.PublicDropTypes[1] = p.PublicDropTypes[1], p.PublicDropTypes[0]
		reordered, err := Generate(p, "fixture")
		if err != nil || !bytes.Equal(source, reordered) {
			t.Fatalf("drop order changed output: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "drop.go"), source, 0644); err != nil {
			t.Fatal(err)
		}
		args := []string{"test", "-mod=mod", "-gcflags=-smallframes", "-count=1", "."}
		if arch != runtime.GOARCH {
			args = []string{"test", "-mod=mod", "-gcflags=-smallframes", "-c", "-o", filepath.Join(dir, "drop.test"), "."}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOARCH="+arch, "GOOS=linux", "CGO_ENABLED=0", "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, output)
		}
	}
}
