//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

func TestRustSIMD128Masks(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "simd_masks.rs"))
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021", "--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes", "-Cllvm-args=-global-isel=0", filepath.Join("testdata", "native_simd_masks.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if bytes.Count(expected, []byte{'\n'}) != 32*32 {
		t.Fatal("incomplete SIMD mask reference")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			output := filepath.Join(dir, arch+".json")
			exportCoreFixture(t, frontend, sysroot, source, "oxide_simd_masks", arch, output)
			p, err := mir.Load(output)
			if err != nil {
				t.Fatal(err)
			}
			calls := map[string]bool{}
			for _, f := range p.Functions {
				for _, callee := range f.Calls {
					calls[callee] = true
				}
			}
			for _, op := range []string{"simd_lt", "simd_select", "simd_reduce_all", "simd_reduce_any", "simd_bitmask", "simd_reduce_or", "simd_reduce_min", "simd_reduce_max", "simd_and", "simd_or", "simd_xor"} {
				if !calls["<intrinsic:"+op+">"] {
					t.Fatalf("missing actual intrinsic %s", op)
				}
			}
			testGenerated(t, p, expected, arch)
		})
	}
}
