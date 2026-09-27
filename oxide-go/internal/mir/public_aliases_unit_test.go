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

func TestStaticUnitDefaultAlias(t *testing.T) {
	unit := Type{ID: 1, Name: "()", Kind: "aggregate", Sized: true, Align: 1, DefaultSymbol: "unit_default"}
	p := &Program{
		Target: "aarch64-unknown-linux-gnu", Types: []Type{unit}, APITypes: []Type{unit},
		PublicTypes: []PublicType{{Name: "test::A", Type: 1}, {Name: "test::B", Type: 1}},
		Functions:   []Function{{Symbol: "unit_default", Name: "core::default::Default::default", Kind: "Item", Body: &Body{Locals: []Local{{Type: 1}}, Blocks: []Block{{Terminator: Statement{Kind: json.RawMessage(`"Return"`)}}}}}},
	}
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"go.mod": []byte(fmt.Sprintf("module unit-alias-test\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module)),
		"alias_test.go": []byte(`package fixture
import("testing";oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestUnitAliases(t *testing.T){
 ctx:=oxide.NewContext();defer ctx.Close();mark:=ctx.Mark()
 requireNoGoAllocations(t,100,func(){Default__A(ctx);Default__B(ctx);if ctx.Mark()!=mark || ctx.Failed(){t.Fatal("unit Default changed context")}})
}
`),
	}
	files["allocations_test.go"], err = os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"arm64", "amd64"} {
		p.Target = map[string]string{"arm64": "aarch64-unknown-linux-gnu", "amd64": "x86_64-unknown-linux-gnu"}[arch]
		source, err := Generate(p, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "alias.go"), source, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"test", "-mod=mod", "-count=1"}
		if arch != runtime.GOARCH {
			args = append(args, "-c", "-o", filepath.Join(dir, arch+".test"))
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, output)
		}
	}
}
