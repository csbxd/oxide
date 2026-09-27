//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

func TestRustCallbackABIConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "callback-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(cache, "native-"+runtime.GOARCH)
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021", "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_callback.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0644); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			metadata := filepath.Join(cache, arch+".json")
			exportCoreFixture(t, frontend, sysroot, filepath.Join(root, "oxide-go/internal/mir/testdata/callback.rs"), "oxide_callback", arch, metadata)
			p, err := mir.Load(metadata)
			if err != nil {
				t.Fatal(err)
			}
			types := map[int]*mir.Type{}
			for i := range p.Types {
				types[p.Types[i].ID] = &p.Types[i]
			}
			var callbacks []*mir.Type
			for _, alias := range p.PublicTypes {
				want := -2
				if strings.HasSuffix(alias.Name, "::RustCall") {
					want = 0
				}
				if strings.HasSuffix(alias.Name, "::Ordinary") {
					want = -1
				}
				if want == -2 {
					continue
				}
				ty := types[alias.Type]
				if ty.Kind != "fnptr" || ty.FnSpreadArg == nil || *ty.FnSpreadArg != want || len(ty.FnInputs) != 1 || len(types[ty.FnInputs[0]].FieldTypes) != 3 {
					t.Fatalf("compiler callback ABI metadata: %+v", ty)
				}
				callbacks = append(callbacks, ty)
			}
			if len(callbacks) != 2 {
				t.Fatal("missing callback types")
			}
			source, err := mir.Generate(p, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			// Missing compiler authority must fail, never resurrect the former
			// assumption that every callback has its logical tuple ABI.
			for i := range p.APITypes {
				if p.APITypes[i].Kind == "fnptr" {
					p.APITypes[i].FnSpreadArg = nil
				}
			}
			if _, err := mir.Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), "missing compiler function-pointer spread ABI") {
				t.Fatalf("accepted missing callback ABI: %v", err)
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			harness, err := os.ReadFile(filepath.Join("testdata", "callback_generated_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"go.mod": []byte(fmt.Sprintf("module oxide-callback-conformance\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))), "callback_test.go": harness, "allocations_test.go": bytes.Replace(allocations, []byte("package fixture"), []byte("package fixture_test"), 1), "expected.stdout": expected}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			names, err := mir.WriteGoFiles(dir, source)
			if err != nil {
				t.Fatal(err)
			}
			if err := mir.RemoveStaleGoFiles(dir, names); err != nil {
				t.Fatal(err)
			}
			if err := mir.WriteAllocations(p, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "callback.test")
			cmd := exec.Command("go", "test", "-mod=mod", "-gcflags=-smallframes", "-c", "-o", binary, ".")
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
		})
	}
}
