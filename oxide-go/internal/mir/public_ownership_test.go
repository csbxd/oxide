//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

var ownershipKinds = []string{"string", "vector", "box", "dynamic", "aligned", "large", "result", "zero", "boxed_string", "borrowed_vector"}

// The Go caller is in a separate package. It consumes public Rust return values
// only through compiler-generated DropTn helpers, without Rust release roots.
func TestRustPublicOwnershipConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "public-ownership-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("testdata", "public_ownership.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(cache, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte(fmt.Sprintf("[package]\nname=\"oxide_public_ownership\"\nversion=\"0.0.0\"\nedition=\"2021\"\n[lib]\npath=%q\n", source)), 0644); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(cache, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021", "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_public_ownership.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0644); err != nil {
		t.Fatal(err)
	}
	paths := []string{"init", "reset", "drop_state", "make_panic", "catch_callback", "borrow_box", "borrow_dynamic"}
	for _, kind := range ownershipKinds {
		paths = append(paths, "make_"+kind, "inspect_"+kind)
	}
	for i := range paths {
		paths[i] = "oxide_public_ownership::" + paths[i]
	}
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			metadata := filepath.Join(cache, arch+".json")
			stage, err := os.MkdirTemp(cache, "export-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(stage)
			pending := filepath.Join(stage, "oxide.mir.json")
			cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+pending)
			cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS="+strings.Join(paths, ","), "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(root, ".cache", "process-conformance", "cargo-target"))
			run(t, cmd)
			if err := os.Rename(pending, metadata); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(strings.TrimSuffix(pending, ".json")+".api.json", filepath.Join(cache, arch+".api.json")); err != nil {
				t.Fatal(err)
			}
			p, err := mir.Load(metadata)
			if err != nil {
				t.Fatal(err)
			}
			testPublicOwnership(t, root, cache, arch, p, expected)
		})
	}
}

func testPublicOwnership(t *testing.T, root, cache, arch string, p *mir.Program, expected []byte) {
	t.Helper()
	functions := map[string]*mir.Function{}
	types := map[int]*mir.Type{}
	drops := map[int]mir.PublicDropType{}
	for _, f := range p.Functions {
		functions[f.Symbol] = &f
	}
	for _, typ := range p.Types {
		types[typ.ID] = &typ
	}
	for _, d := range p.PublicDropTypes {
		if _, ok := drops[d.Type]; ok {
			t.Fatalf("duplicate owned public type %d", d.Type)
		}
		drops[d.Type] = d
	}
	rootFunction := func(name string) *mir.Function {
		for _, r := range p.Roots {
			if r.Name == "oxide_public_ownership::"+name {
				f := functions[r.Symbol]
				if r.Return != f.Body.Locals[0].Type || len(r.Params) != f.Body.ArgCount {
					t.Fatalf("root %s lost original Rust signature types", name)
				}
				for i, id := range r.Params {
					if id != f.Body.Locals[i+1].Type {
						t.Fatalf("root %s parameter %d lost Rust type identity", name, i)
					}
				}
				return f
			}
		}
		t.Fatalf("missing root %s", name)
		return nil
	}
	if len(drops) != len(ownershipKinds)+1 {
		t.Fatalf("public drop types = %d, want %d", len(drops), len(ownershipKinds)+1)
	}
	for _, name := range []string{"borrow_box", "borrow_dynamic"} {
		f := rootFunction(name)
		for _, local := range f.Body.Locals[:2] {
			if _, ok := drops[local.Type]; ok {
				t.Fatalf("non-owning %s signature incorrectly has DropT%d", name, local.Type)
			}
		}
	}
	generated, err := mir.Generate(p, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "generated.go", generated, 0)
	if err != nil {
		t.Fatal(err)
	}
	goReturns := map[string]string{}
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Type.Results != nil && len(f.Type.Results.List) == 1 {
			var b bytes.Buffer
			if err := format.Node(&b, fset, f.Type.Results.List[0].Type); err != nil {
				t.Fatal(err)
			}
			goReturns[f.Name.Name] = b.String()
		}
	}
	var cases strings.Builder
	for index, kind := range ownershipKinds {
		f := rootFunction("make_" + kind)
		id := f.Body.Locals[0].Type
		if _, ok := drops[id]; !ok {
			t.Fatalf("missing public drop for make_%s return type %d", kind, id)
		}
		makeName, _ := mir.ExportName("oxide_public_ownership::make_" + kind)
		inspectName, _ := mir.ExportName("oxide_public_ownership::inspect_" + kind)
		fmt.Fprintf(&cases, "case %d:\n", index)
		if ret := goReturns[makeName]; ret != "" {
			if strings.HasPrefix(ret, "T") {
				ret = "f." + ret
			}
			fmt.Fprintf(&cases, "v:=f.%s(ctx,seed);*(*%s)(unsafe.Pointer(slot))=v;sum=f.%s(ctx,slot);f.DropT%d(ctx,v)\n", makeName, ret, inspectName, id)
		} else {
			fmt.Fprintf(&cases, "f.%s(ctx,slot,seed);sum=f.%s(ctx,slot);f.DropT%d(ctx,slot)\n", makeName, inspectName, id)
		}
		if types[id].Size > (256<<10)+64 || types[id].Align > 64 {
			t.Fatalf("test slot too small for type %d", id)
		}
		// Borrowed arguments/results are not implicit ownership transfers.
		for _, local := range rootFunction("inspect_" + kind).Body.Locals[:2] {
			if _, ok := drops[local.Type]; ok {
				t.Fatalf("borrowed inspector signature incorrectly owns type %d", local.Type)
			}
		}
	}
	panicType := rootFunction("make_panic").Body.Locals[0].Type
	if _, ok := drops[panicType]; !ok {
		t.Fatalf("missing panic destructor %d", panicType)
	}
	callbackType := types[rootFunction("catch_callback").Body.Locals[1].Type]
	unit := callbackType.FnOutput
	harness := fmt.Sprintf(`package fixture_test
import("fmt";"os";"strings";"testing";"unsafe";f "public-ownership-test";oxide "github.com/csbxd/oxide/oxide-go/runtime")
var callbackSeed uint64
var propagatedPanics uint64
func panicDrop(ctx *oxide.Context)(unit f.T%d) {
 defer func(){if value:=recover();value!=nil{propagatedPanics++;ctx.Fail(value)}}()
 value:=f.MakePanic(ctx,callbackSeed);f.DropT%d(ctx,value);return
}
func TestPublicOwnership(t *testing.T) {
 expected,err:=os.ReadFile("expected.stdout");if err!=nil{t.Fatal(err)}
 beforeContext:=oxide.HeapStats().LiveAllocations
 ctx:=oxide.NewContext();defer ctx.Close();f.Init(ctx)
 slot:=ctx.Alloc((256<<10)+64,64);mark:=ctx.Mark()
 // Initialize Rust's thread panic state before taking the ownership baseline.
 callbackSeed=0;if !f.CatchCallback(ctx,oxide.FunctionPointer(panicDrop),0){t.Fatal("panic warm-up")}
 baseline:=oxide.HeapStats().LiveAllocations
 for _,line:=range strings.Split(strings.TrimSpace(string(expected)),"\n") {
  var caseID,seed,want uint64;if _,err:=fmt.Sscanf(line,"%%d %%d %%d",&caseID,&seed,&want);err!=nil{t.Fatal(err)}
  t.Run(fmt.Sprintf("case_%%d/seed_%%d",caseID,seed),func(t *testing.T){
   requireNoGoAllocations(t,100,func(){
    f.Reset(ctx);sum:=uint64(0)
    switch caseID {
     %s
     case 10:callbackSeed=seed;panics:=propagatedPanics;if f.CatchCallback(ctx,oxide.FunctionPointer(panicDrop),seed){sum=1};if propagatedPanics!=panics+1{t.Fatal("drop panic did not cross the Go API boundary")}
     default:t.Fatal("unknown case")
    }
    got:=sum+f.DropState(ctx)
    if got!=want{t.Fatalf("got %%d, native %%d",got,want)}
    if ctx.Failed()||ctx.Mark()!=mark{t.Fatal("panic/frame escaped destructor")}
    if live:=oxide.HeapStats().LiveAllocations;live!=baseline{t.Fatalf("owned return leaked: baseline=%%d live=%%d",baseline,live)}
   })
  })
 }
 if err:=ctx.Close();err!=nil{t.Fatal(err)}
 if live:=oxide.HeapStats().LiveAllocations;live!=beforeContext{t.Fatalf("after Context.Close: live=%%d baseline=%%d",live,beforeContext)}
}
`, unit, panicType, cases.String())
	dir := filepath.Join(cache, "go-"+arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"go.mod":                   []byte(fmt.Sprintf("module public-ownership-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))),
		"allocations_test.go":      bytes.Replace(allocations, []byte("package fixture"), []byte("package fixture_test"), 1),
		"public_ownership_test.go": []byte(harness),
		"expected.stdout":          expected,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	generatedFiles, err := mir.WriteGoFiles(dir, generated)
	if err != nil {
		t.Fatal(err)
	}
	if err := mir.RemoveStaleGoFiles(dir, generatedFiles); err != nil {
		t.Fatal(err)
	}
	if err := mir.WriteAllocations(p, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "public-ownership.test")
	cmd := exec.Command("go", "test", "-mod=mod", "-tags=memory.counters", "-gcflags=-smallframes", "-c", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
	run(t, cmd)
	if arch == runtime.GOARCH {
		cmd := exec.Command(binary, "-test.v", "-test.count=1")
		cmd.Dir = dir
		log := run(t, cmd)
		if err := os.WriteFile(filepath.Join(cache, arch+".log"), log, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
