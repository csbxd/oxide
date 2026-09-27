//go:build linux && (amd64 || arm64)

package mir_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// Deterministic random operands and IEEE/integer boundaries are compared with
// native Rust, including direct integer->f32 double-rounding counterexamples.
func TestRustNumericConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "numeric.rs"))
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native_numeric.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	files := map[string][]byte{"expected.stdout": expected}
	for _, arch := range []string{"amd64", "arm64"} {
		output := filepath.Join(dir, "numeric_"+arch+".json")
		exportCoreFixture(t, frontend, sysroot, source, "oxide_numeric_conformance", arch, output)
		p, err := mir.Load(output)
		if err != nil {
			t.Fatal(err)
		}
		testGenerated(t, p, expected, arch)
		files[arch+".json"], err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
	}
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	files["source.sha256"] = []byte(fmt.Sprintf("%x\n", sha256.Sum256(sourceBytes)))
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "numeric-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	// Keep only exports that passed both target compilation and the native
	// differential/precise-allocation checks, for later runtime-only rechecks.
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
