//go:build linux && (amd64 || arm64)

package mir_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// serde_json generics are instantiated by compiler metadata. The translated
// library has no project wrapper which invokes either JSON entry point.
func TestRustJSONAPIConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	t.Setenv("GOMEMLIMIT", "3GiB")
	t.Setenv("GOMAXPROCS", "2")
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "json-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "oxide-rs", "tests", "json_api", "Cargo.toml")
	target := filepath.Join(root, ".cache", "frontend-panic", "target")
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	env := append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_ROOTS=oxide_json_api::inspect", "OXIDE_EXPORT=", "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+target)
	cargo := filepath.Join(sysroot, "bin", "cargo")
	common := []string{"-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest}
	cmd := exec.Command(cargo, append(append([]string{"rustc"}, common...), "--target", targetTriple(runtime.GOARCH), "--bin", "oxide-json-api-native", "--", "--cfg", "oxide_native")...)
	cmd.Env = env
	run(t, cmd)
	native := filepath.Join(target, targetTriple(runtime.GOARCH), "debug", "oxide-json-api-native")
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0644); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			metadata := filepath.Join(cache, arch+".json")
			stage, err := os.MkdirTemp(cache, "export-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(stage)
			pending := filepath.Join(stage, "oxide.mir.json")
			cmd := exec.Command(cargo, append(append([]string{"rustc"}, common...), "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+pending)...)
			cmd.Env = env
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
			source, err := mir.Generate(p, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			for _, capability := range []struct {
				declaration string
				present     bool
			}{
				{"func (v Ref__EncodeOnly) JSON(", true},
				{"func (v Ref__EncodeOnly) JSONValue(", true},
				{"func FromJSON__EncodeOnly(", false},
				{"func (v Ref__DecodeOnly) JSON(", false},
				{"func (v Ref__DecodeOnly) JSONValue(", false},
				{"func FromJSON__DecodeOnly(", true},
				{"func (v Ref__Neither) JSON(", false},
				{"func (v Ref__Neither) JSONValue(", false},
				{"func FromJSON__Neither(", false},
			} {
				if strings.Contains(string(source), capability.declaration) != capability.present {
					t.Fatalf("static JSON trait capability %s", capability.declaration)
				}
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			harness, err := os.ReadFile(filepath.Join("testdata", "json_api_generated_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			allocations = []byte(strings.Replace(string(allocations), "package fixture", "package fixture_test", 1))
			files := map[string][]byte{"go.mod": []byte(fmt.Sprintf("module oxide-json-conformance\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))), "oxide_gen_test.go": harness, "allocations_test.go": allocations, "expected.stdout": expected}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			generated, err := mir.WriteGoFiles(dir, source)
			if err != nil {
				t.Fatal(err)
			}
			if err := mir.RemoveStaleGoFiles(dir, generated); err != nil {
				t.Fatal(err)
			}
			if err := mir.WriteAllocations(p, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "json.test")
			build := exec.Command("go", "test", "-mod=mod", "-tags=memory.counters", "-gcflags=-smallframes", "-c", "-o", binary, ".")
			build.Dir = dir
			build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			run(t, build)
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
