package mir

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLargeValueRootsAndConstructor(t *testing.T) {
	const size = 256 << 10
	variant := 0
	p := &Program{
		Target: "aarch64-unknown-linux-gnu",
		Types: []Type{
			{ID: 1, Name: "u8", Kind: "u8", Sized: true, Size: 1, Align: 1},
			{ID: 2, Name: "[u8; 262144]", Kind: "array", Sized: true, Size: size, Align: 1, Element: 1, Length: size},
			{ID: 3, Name: "demo::Wrapped", Kind: "aggregate", Sized: true, Size: size, Align: 1, Fields: []uint64{0}, VariantFieldTypes: [][]int{{2}}},
		},
		PublicTypes: []PublicType{{Name: "demo::Buffer", Type: 2}, {Name: "demo::Wrapped", Type: 3}},
		Roots:       []Root{{Name: "mutate", Symbol: "mutate"}, {Name: "construct", Symbol: "construct"}},
		Functions: []Function{
			{Symbol: "mutate", Name: "mutate", Body: &Body{ArgCount: 1, Locals: []Local{{Type: 2}, {Type: 2}}, Blocks: []Block{{
				Statements: []Statement{
					{Kind: json.RawMessage(`{"Assign":[{"local":1,"projection":[{"ConstantIndex":{"offset":0,"from_end":false}}]},{"Use":[{"Constant":{"const_":{"ty":1,"kind":{"Allocated":{"bytes":[99],"align":1}}}}}]}]}`)},
					{Kind: json.RawMessage(`{"Assign":[{"local":0,"projection":[]},{"Use":[{"Copy":{"local":1,"projection":[]}}]}]}`)},
				}, Terminator: Statement{Kind: json.RawMessage(`"Return"`)},
			}}}},
			{Symbol: "construct", Name: "construct", Kind: "Item", Constructor: &variant, Signature: &Signature{Params: []int{2}, Return: 3}},
		},
	}
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
		"allocations_test.go": allocations,
		"go.mod":              []byte(fmt.Sprintf("module large-value-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module)),
		"large_test.go": []byte(`package fixture
import("testing";"unsafe"; oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestRoots(t *testing.T) {
 ctx:=oxide.NewContext();defer ctx.Close()
 input:=New__Buffer(ctx)
 bytes:=unsafe.Slice((*byte)(unsafe.Pointer(input.Addr())),256<<10)
 for i:=range bytes {bytes[i]=byte(i+7)}
 mark:=ctx.Mark()
 check:=func(){
  wrapped:=Construct(ctx,input)
  for i,b:=range unsafe.Slice((*byte)(unsafe.Pointer(wrapped.Addr())),len(bytes)) {if b!=bytes[i]{t.Fatalf("constructor byte %d",i)}}
  output:=Mutate(ctx,input)
  if bytes[0]!=7 || *(*byte)(unsafe.Pointer(output.Addr()))!=99 {t.Fatal("by-value source changed")}
  for i:=1;i<len(bytes);i++ {if *(*byte)(unsafe.Pointer(output.Addr()+uintptr(i)))!=bytes[i]{t.Fatalf("return byte %d",i)}}
  ctx.Restore(mark)
  if ctx.Mark()!=mark {t.Fatal("frame leaked")}
 }
 check()
 requireNoGoAllocations(t,100,check)
 input.Init(Mutate(ctx,input));ctx.Restore(mark)
 if bytes[0]!=99 || ctx.Mark()!=mark {t.Fatal("aliased result storage")}
}
`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		p.Target = map[string]string{"amd64": "x86_64-unknown-linux-gnu", "arm64": "aarch64-unknown-linux-gnu"}[arch]
		source, err := Generate(p, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "large.go"), source, 0o644); err != nil {
			t.Fatal(err)
		}
		args := []string{"test", "-mod=mod", "-gcflags=-smallframes", "-count=1", "."}
		if arch != runtime.GOARCH {
			args = []string{"test", "-mod=mod", "-gcflags=-smallframes", "-c", "-o", filepath.Join(dir, "large.test"), "."}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOARCH="+arch, "GOOS=linux", "GOWORK=off", "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, output)
		}
	}
}
