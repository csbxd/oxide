//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// Compare each SIMD lane's bits with the pinned native compiler. The private
// repr(simd) types reach Go only through the public scalar conformance root.
func TestRustSIMDInsertConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	t.Setenv("GOMEMLIMIT", "3GiB")
	t.Setenv("GOMAXPROCS", "2")
	t.Setenv("CARGO_BUILD_JOBS", "2")
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "simd-insert-conformance")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "oxide-go", "internal", "mir", "testdata", "simd_insert.rs")
	native := filepath.Join(cache, "native-"+runtime.GOARCH)
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native_simd_insert.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if bytes.Count(expected, []byte{'\n'}) != 96*8 {
		t.Fatal("incomplete native SIMD insert reference")
	}
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			metadata := filepath.Join(cache, arch+".json")
			exportCoreFixture(t, frontend, sysroot, source, "oxide_simd_insert", arch, metadata)
			program, err := mir.Load(metadata)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			for _, function := range program.Functions {
				for _, symbol := range function.Calls {
					if symbol == "<intrinsic:simd_insert>" {
						calls++
					}
				}
			}
			if calls != 12 {
				t.Fatalf("expected twelve actual SIMD insert calls, got %d", calls)
			}
			generated, err := mir.Generate(program, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{
				"go.mod":          []byte(fmt.Sprintf("module oxide-simd-insert-conformance\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))),
				"expected.stdout": expected,
			}
			for _, name := range []string{"generated_test.go", "allocations_test.go"} {
				files[name], err = os.ReadFile(filepath.Join("testdata", name))
				if err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			names, err := mir.WriteGoFiles(dir, generated)
			if err != nil {
				t.Fatal(err)
			}
			if err := mir.RemoveStaleGoFiles(dir, names); err != nil {
				t.Fatal(err)
			}
			if err := mir.WriteAllocations(program, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "simd-insert.test")
			cmd := exec.Command("go", "test", "-mod=mod", "-gcflags=-smallframes", "-c", "-o", binary, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			run(t, cmd)
			if arch == runtime.GOARCH {
				cmd := exec.Command(binary, "-test.v", "-test.count=1")
				cmd.Dir = dir
				log := run(t, cmd)
				if err := os.WriteFile(filepath.Join(cache, arch+".log"), log, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "source.sha256"), []byte(fmt.Sprintf("%x\n", sha256.Sum256(sourceBytes))), 0o644); err != nil {
		t.Fatal(err)
	}
}
