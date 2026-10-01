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

func TestRustFloatMathConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "float_math.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte(fmt.Sprintf("[package]\nname=\"oxide_float_math\"\nversion=\"0.0.0\"\nedition=\"2021\"\n[lib]\npath=%q\n", source)), 0644); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021", "-Copt-level=2", "-Cllvm-args=-global-isel=0", filepath.Join("testdata", "native_float_math.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "float-math-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes", "--cfg=oxide_export", "--check-cfg=cfg(oxide_export)"}
	for _, arch := range []string{"amd64", "arm64"} {
		output := filepath.Join(dir, arch+".json")
		cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)
		cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_float_math::evaluate,oxide_float_math::gamma_sign", "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(cache, "cargo-target"))
		run(t, cmd)
		p, err := mir.Load(output)
		if err != nil {
			t.Fatal(err)
		}
		testGeneratedWithHarness(t, p, expected, arch, "float_math_generated_test.go")
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, arch+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("both targets compile; native %s compares %d records", runtime.GOARCH, len(strings.Split(strings.TrimSpace(string(expected)), "\n")))
}
