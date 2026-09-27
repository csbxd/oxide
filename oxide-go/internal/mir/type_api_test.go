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

// Public calls operate on original library types; no Rust observer or release
// wrapper is part of the translated graph. The native binary is only an oracle.
func TestRustTypeAPIConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	t.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -tags=memory.counters"))
	t.Setenv("GOMEMLIMIT", "3GiB")
	t.Setenv("GOMAXPROCS", "2")
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "direct-type-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(cache, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2024", "-Awarnings", "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_type_api.rs"), "-o", native))
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
			cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", filepath.Join(root, "oxide-rs", "tests", "type_api", "Cargo.toml"), "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+pending)
			flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
			cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_ROOTS=", "OXIDE_EXPORT=", "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(root, ".cache", "frontend-panic", "target"), "CARGO_BUILD_JOBS=2")
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
			testDirectTypeAPI(t, p, expected, root, cache, arch)
		})
	}
}

func testDirectTypeAPI(t *testing.T, p *mir.Program, expected []byte, root, cache, arch string) {
	t.Helper()
	source, err := mir.Generate(p, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "go-"+arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join("testdata", "type_api_generated_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	allocations = []byte(strings.Replace(string(allocations), "package fixture", "package fixture_test", 1))
	files := map[string][]byte{
		"go.mod":            []byte(fmt.Sprintf("module oxide-type-api-conformance\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))),
		"oxide_gen_test.go": harness, "allocations_test.go": allocations, "expected.stdout": expected,
	}
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
	binary := filepath.Join(dir, "type-api.test")
	cmd := exec.Command("go", "test", "-mod=mod", "-gcflags=-smallframes", "-c", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
	run(t, cmd)
	testTypeAPICompile(t, dir, arch)
	if arch == runtime.GOARCH {
		cmd := exec.Command(binary, "-test.v", "-test.count=1")
		cmd.Dir = dir
		log := run(t, cmd)
		if err := os.WriteFile(filepath.Join(cache, arch+".log"), log, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
