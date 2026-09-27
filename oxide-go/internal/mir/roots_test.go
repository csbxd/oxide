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

func TestExportName(t *testing.T) {
	for path, want := range map[string]string{
		"demo::render_svg":                                         "RenderSvg",
		"demo::layout::render_svg":                                 "Layout_RenderSvg",
		"demo::RenderOptions::modern":                              "RenderOptions_Modern",
		"demo::r#type::r#match":                                    "Type_Match",
		"demo::école::résumé":                                      "École_Résumé",
		"<demo::mod_a::Item as core::clone::Clone>::clone":         "ModA_Item_As_Core_Clone_Clone_Clone",
		"<demo::Item as core::convert::From<dep::Other>>::from":    "Item_As_Core_Convert_From_Of_Dep_Other_End_From",
		"<demo::Item as dep::Trait<dep::A, dep::Nested<u8>>>::run": "Item_As_Dep_Trait_Of_Dep_A_And_Dep_Nested_Of_U8_End_End_Run",
	} {
		got, err := ExportName(path)
		if err != nil || got != want {
			t.Errorf("%s: %q, %v; want %q", path, got, err, want)
		}
	}
	for _, path := range []string{"", "demo::", "demo::a::::b", "demo::<impl A>::run", "<demo::A>::run", "<demo::A as >::run", "<demo::A as dep::Trait<u8>::run", "<demo::A as dep::Trait<&str>>::run", "<demo::A as dep::Trait>::a::run"} {
		if name, err := ExportName(path); err == nil {
			t.Errorf("accepted invalid public path %q as %q", path, name)
		}
	}
}

func rootTestProgram() *Program {
	return &Program{
		Target: "aarch64-unknown-linux-gnu",
		Types: []Type{
			{ID: 1, Kind: "u64", Sized: true, Size: 8, Align: 8},
			{ID: 2, Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 1},
			{ID: 3, Kind: "i64", Sized: true, Size: 8, Align: 8},
			{ID: 4, Kind: "aggregate", Sized: true, Size: 0, Align: 1},
		},
		Functions: []Function{
			{Symbol: "identity", Name: "demo::render", Body: &Body{ArgCount: 1, Locals: []Local{{Type: 1}, {Type: 1}}, Blocks: []Block{{Statements: []Statement{{Kind: json.RawMessage(`{"Assign":[{"local":0,"projection":[]},{"Use":[{"Copy":{"local":1,"projection":[]}}]}]}`)}}, Terminator: Statement{Kind: json.RawMessage(`"Return"`)}}}}},
			{Symbol: "constant", Name: "demo::nested::render", Body: &Body{ArgCount: 1, Locals: []Local{{Type: 1}, {Type: 1}}, Blocks: []Block{{Statements: []Statement{{Kind: json.RawMessage(`{"Assign":[{"local":0,"projection":[]},{"Use":[{"Constant":{"const_":{"ty":1,"kind":{"Allocated":{"bytes":[9,0,0,0,0,0,0,0],"align":8}}}}}]}]}`)}}, Terminator: Statement{Kind: json.RawMessage(`"Return"`)}}}}},
			{Symbol: "method", Name: "demo::Result::total", Body: &Body{ArgCount: 1, Locals: []Local{{Type: 1}, {Type: 2}}, Blocks: []Block{{Statements: []Statement{{Kind: json.RawMessage(`{"Assign":[{"local":0,"projection":[]},{"Use":[{"Copy":{"local":1,"projection":["Deref"]}}]}]}`)}}, Terminator: Statement{Kind: json.RawMessage(`"Return"`)}}}}},
			{Symbol: "tracked", Name: "demo::location", TrackCaller: true, CallLocations: map[int]CallLocation{0: {Inherited: true}}, Calls: map[int]string{0: "<intrinsic:caller_location>"}, Body: &Body{Locals: []Local{{Type: 2}}, Blocks: []Block{
				{Terminator: Statement{Kind: json.RawMessage(`{"Call":{"args":[],"destination":{"local":0,"projection":[]},"target":1,"unwind":"Continue"}}`)}},
				{Terminator: Statement{Kind: json.RawMessage(`"Return"`)}},
			}}},
			{Symbol: "syscall", Name: "libc::syscall", Kind: "Item", Signature: &Signature{ABI: "C", Params: []int{3}, Return: 3, Variadic: true, FixedCount: 1}},
		},
	}
}

func TestRootExportCollisionDiagnostics(t *testing.T) {
	for _, paths := range [][2]string{
		{"demo::foo_bar", "demo::fooBar"},
		{"demo::foo::run", "demo::Foo::run"},
		{"<demo::Thing as dep::foo_bar::Trait>::run", "<demo::Thing as dep::fooBar::Trait>::run"},
		{"demo::same", "demo::same"},
	} {
		p := rootTestProgram()
		p.Roots = []Root{{Name: paths[0], Symbol: "identity"}, {Name: paths[1], Symbol: "constant"}}
		_, err := Generate(p, "fixture")
		if err == nil || !strings.Contains(err.Error(), paths[0]) || !strings.Contains(err.Error(), paths[1]) {
			t.Fatalf("collision %v: %v", paths, err)
		}
	}
	p := rootTestProgram()
	p.Roots = []Root{{Name: "demo::t4", Symbol: "identity"}}
	if _, err := Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), "generated Rust layout type") {
		t.Fatalf("type collision: %v", err)
	}
	for _, name := range []string{"demo::rust_type", "demo::rust_exit", "demo::type_foo"} {
		p := rootTestProgram()
		p.PublicTypes = []PublicType{{Name: "demo::Foo", Type: 1}}
		p.Roots = []Root{{Name: name, Symbol: "identity"}}
		if _, err := Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), "generated type API") {
			t.Fatalf("type API collision %s: %v", name, err)
		}
	}
}

func TestPublicRootCalls(t *testing.T) {
	p := rootTestProgram()
	p.Roots = []Root{
		{Name: "demo::render", Symbol: "identity"},
		{Name: "demo::nested::render", Symbol: "constant"},
		{Name: "demo::alias", Symbol: "identity"},
		{Name: "demo::Result::total", Symbol: "method"},
		{Name: "demo::OtherResult::total", Symbol: "method"},
		{Name: "<demo::Result as core::clone::Clone>::clone", Symbol: "method"},
		{Name: "<demo::AliasResult as core::clone::Clone>::clone", Symbol: "method"},
		{Name: "demo::location", Symbol: "tracked"},
		{Name: "demo::system_call", Symbol: "syscall"},
	}
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": fmt.Sprintf("module root-call-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module),
		"roots_test.go": `package fixture
import("os";"syscall";"testing";"unsafe";oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestCalls(t *testing.T){
 c:=oxide.NewContext();defer c.Close()
 if Render(c,7)!=7 || Alias(c,8)!=8 || Nested_Render(c,7)!=9 {t.Fatal("wrong public function binding")}
 p:=c.Alloc(8,8);*(*uint64)(unsafe.Pointer(p))=42
 if Result_Total(c,p)!=42 || OtherResult_Total(c,p)!=42 || Result_As_Core_Clone_Clone_Clone(c,p)!=42 || AliasResult_As_Core_Clone_Clone_Clone(c,p)!=42 {t.Fatal("wrong method/trait/re-export binding")}
 if Location(c,p)!=p {t.Fatal("lost caller location")}
 f,err:=os.CreateTemp(t.TempDir(),"write-");if err!=nil{t.Fatal(err)};defer f.Close()
 input:=c.Alloc(3,1);copy(unsafe.Slice((*byte)(unsafe.Pointer(input)),3),"abc")
 if n:=SystemCall(c,int64(syscall.SYS_WRITE),f.Fd(),input,3);n!=3 {t.Fatalf("variadic forwarded %d bytes",n)}
 b,err:=os.ReadFile(f.Name());if err!=nil || string(b)!="abc"{t.Fatalf("write result %q: %v",b,err)}
}
`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		p.Target = map[string]string{"amd64": "x86_64-unknown-linux-gnu", "arm64": "aarch64-unknown-linux-gnu"}[arch]
		source, err := Generate(p, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		for i, j := 0, len(p.Roots)-1; i < j; i, j = i+1, j-1 {
			p.Roots[i], p.Roots[j] = p.Roots[j], p.Roots[i]
		}
		reordered, err := Generate(p, "fixture")
		if err != nil || !bytes.Equal(source, reordered) {
			t.Fatalf("root order changed output: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "roots.go"), source, 0644); err != nil {
			t.Fatal(err)
		}
		args := []string{"test", "-mod=mod", "-count=1", "."}
		if arch != runtime.GOARCH {
			args = []string{"test", "-mod=mod", "-c", "-o", filepath.Join(dir, "fixture.test"), "."}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOARCH="+arch, "GOOS=linux", "CGO_ENABLED=0", "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, output)
		}
	}
}
