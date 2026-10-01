//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

func TestRustSoftFloatConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "soft_float.rs"))
	if err != nil {
		t.Fatal(err)
	}
	var expected []byte
	for _, profile := range []string{"debug-default", "debug-selectiondag", "release"} {
		native := filepath.Join(dir, profile)
		args := []string{"--edition=2021", "--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_soft_float.rs"), "-o", native}
		if profile == "debug-selectiondag" {
			args = append(args, "-Cllvm-args=-global-isel=0")
		}
		if profile == "release" {
			args = append(args, "-Copt-level=2")
		}
		run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), args...))
		if profile == "debug-default" {
			t.Logf("native default debug: %s", run(t, exec.Command(native, "--probe")))
			continue
		}
		output := run(t, exec.Command(native))
		if expected != nil && !bytes.Equal(output, expected) {
			t.Fatal("debug SelectionDAG and optimized native references differ")
		}
		expected = output
	}
	files := map[string][]byte{"expected.stdout": expected}
	for _, arch := range []string{"amd64", "arm64"} {
		output := filepath.Join(dir, "soft_float_"+arch+".json")
		exportCoreFixture(t, frontend, sysroot, source, "oxide_soft_float_conformance", arch, output)
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
	cache := filepath.Join("..", "..", "..", ".cache", "soft-float-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
