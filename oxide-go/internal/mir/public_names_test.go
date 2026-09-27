package mir

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func staticNameGenerator(t *testing.T, p *Program) (*generator, []int) {
	t.Helper()
	g, err := newGenerator(p)
	if err != nil {
		t.Fatal(err)
	}
	g.apiTypes = make(map[int]*Type)
	var ids []int
	for i := range p.Types {
		g.apiTypes[p.Types[i].ID] = &p.Types[i]
		ids = append(ids, p.Types[i].ID)
	}
	return g, ids
}

func TestStaticTypeDisplayNames(t *testing.T) {
	known := map[string]string{"demo::Node": "Node"}
	for input, want := range map[string]string{
		"u64": "U64", "()": "Unit", "!": "Never",
		"alloc::vec::Vec<demo::Node>":       "Alloc_Vec_Vec__Of__Node__End",
		"&'_ mut demo::Node":                "MutRef__Of__Node__End",
		"*const [u8; 3]":                    "ConstPtr__Of__Array__Of__U8__Len__3__End__End",
		"(demo::Node, [u8])":                "Tuple__Of__Node__And__Slice__Of__U8__End__End",
		"core::option::Option<(u32, bool)>": "Core_Option_Option__Of__Tuple__Of__U32__And__Bool__End__End",
	} {
		if got, ok := staticTypeName(input, known, 0); !ok || got != want {
			t.Fatalf("%s: %s, %t; want %s", input, got, ok, want)
		}
	}
	if _, ok := staticTypeName("unsafe extern \"C\" fn(u8) -> u64", known, 0); ok {
		t.Fatal("anonymous function syntax was treated as a named path")
	}
}

func TestStaticAPINames(t *testing.T) {
	makeProgram := func(a, b int) *Program {
		return &Program{Target: "aarch64-unknown-linux-gnu", Types: []Type{
			{ID: a, Name: "demo::Node", Kind: "aggregate", Sized: true, Size: 8, Align: 8},
			{ID: b, Name: "core::option::Option<demo::Node>", Kind: "aggregate", Sized: true, Size: 16, Align: 8},
		}, PublicTypes: []PublicType{{Name: "demo::nested::A", Type: a}, {Name: "demo::Z", Type: a}, {Name: "demo::Node", Type: a}}}
	}
	first, ids := staticNameGenerator(t, makeProgram(1, 2))
	first.initAPINames(ids)
	second, ids := staticNameGenerator(t, makeProgram(200, 100))
	second.initAPINames(ids)
	if first.apiName(1) != "Node" || first.apiName(1) != second.apiName(200) || first.apiName(2) != second.apiName(100) {
		t.Fatal("public names depend on traversal IDs or alias ordering")
	}
	p := makeProgram(1, 2)
	p.Types[1].Name = "core::Marker"
	p.PublicTypes = append(p.PublicTypes, PublicType{Name: "demo::core::Marker", Type: 1})
	g, ids := staticNameGenerator(t, p)
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(string(r.(generationError)), "Core_Marker") {
			t.Fatalf("secondary public alias collision was not diagnosed: %v", r)
		}
	}()
	g.initAPINames(ids)
}

func TestStaticLayoutTypes(t *testing.T) {
	var scalar Primitive
	if err := json.Unmarshal([]byte(`{"Int":{"length":"I64","signed":false}}`), &scalar); err != nil {
		t.Fatal(err)
	}
	types := []Type{
		{ID: 1, Name: "demo::A", Kind: "u64", Sized: true, Size: 8, Align: 8},
		{ID: 2, Name: "demo::B", Kind: "aggregate", Sized: true, Size: 8, Align: 8, ValueABI: "Scalar", ABIScalar: &scalar},
		{ID: 3, Name: "demo::Packed", Kind: "aggregate", Sized: true, Size: 8, Align: 1, Pack: 1, ValueABI: "Scalar", ABIScalar: &scalar},
		{ID: 4, Name: "demo::Aligned", Kind: "aggregate", Sized: true, Size: 64, Align: 64},
		{ID: 5, Name: "demo::Zero", Kind: "aggregate", Sized: true, Size: 0, Align: 64},
		{ID: 6, Name: "demo::Tail", Kind: "aggregate", Align: 8},
		{ID: 7, Name: "usize", Kind: "usize", Sized: true, Size: 8, Align: 8},
		{ID: 8, Name: "char", Kind: "char", Sized: true, Size: 4, Align: 4},
	}
	for _, arch := range []string{"amd64", "arm64"} {
		g, ids := staticNameGenerator(t, &Program{Target: map[string]string{"amd64": "x86_64-unknown-linux-gnu", "arm64": "aarch64-unknown-linux-gnu"}[arch], Types: types, PublicTypes: []PublicType{{Name: "demo::Word", Type: 7}}})
		g.initAPINames(ids)
		if g.apiName(7) != "Word" || g.apiName(8) != "Char" {
			t.Fatal("primitive alias or char name changed")
		}
		g.line("package layout\nimport \"unsafe\"")
		for _, id := range ids {
			g.emitLayoutType(id)
		}
		source := g.b.String()
		if strings.Contains(source, "type Rust__Demo_Tail") || strings.Contains(source, "RustSize__Demo_Tail") {
			t.Fatal("DST acquired a fictitious sized representation")
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module layout\n\ngo 1.27.1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "layout.go")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		compile := func() ([]byte, error) {
			cmd := exec.Command("go", "test", ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			return cmd.CombinedOutput()
		}
		if output, err := compile(); err != nil {
			t.Fatalf("%s Go size/alignment assertions: %v\n%s", arch, err, output)
		}
		if err := os.WriteFile(path, []byte(source+"\nvar _ Rust__Demo_A = Rust__Demo_B(0)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if output, err := compile(); err == nil || !strings.Contains(string(output), "cannot use") {
			t.Fatalf("nominal Rust identities were assignable: %v\n%s", err, output)
		}
	}
}

// A sidecar-only audit also supplies exact generated names to external test
// consumers. It never reads the large MIR body or starts a Rust compiler.
func TestStaticAPINamesManifest(t *testing.T) {
	input := os.Getenv("OXIDE_STATIC_NAMES_INPUT")
	if input == "" {
		t.Skip("set OXIDE_STATIC_NAMES_INPUT to audit a compiler API sidecar")
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var p Program
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	p.Types = p.APITypes
	g, ids := staticNameGenerator(t, &p)
	g.initAPINames(ids)
	output := map[string]string{}
	for _, id := range ids {
		output[g.apiType(id).Name] = g.apiName(id)
	}
	for _, alias := range p.PublicTypes {
		output[alias.Name] = g.apiName(alias.Type)
	}
	before := g.apiNames
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	g.initAPINames(ids)
	if !reflect.DeepEqual(before, g.apiNames) {
		t.Fatal("manifest type ordering changed public names")
	}
	if name := os.Getenv("OXIDE_STATIC_NAMES_OUTPUT"); name != "" {
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d canonical names, %d public aliases", len(g.apiNames), len(p.PublicTypes))
}
