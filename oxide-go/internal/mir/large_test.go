//go:build linux && (amd64 || arm64)

package mir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// Large automatic values must not turn into per-call Go heap allocations.
// The shared runner checks native results, frame restoration and 100 warmed
// calls for each direct, by-value, returned, function-pointer and virtual path.
func TestRustLargeValueConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "large.rs"))
	if err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "large-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native_large.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			output := filepath.Join(cache, arch+".json")
			exportCoreFixture(t, frontend, sysroot, source, "oxide_large_conformance", arch, output)
			p, err := mir.Load(output)
			if err != nil {
				t.Fatal(err)
			}
			testGeneratedWithHarness(t, p, expected, arch, "large_generated_test.go")
		})
	}
}
