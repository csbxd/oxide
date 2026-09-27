//go:build linux && (amd64 || arm64)

package mir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// Unwinding a large return must not publish an uninitialized result, and a
// consumed large argument must run its real Rust destructor exactly once.
func TestRustLargePanicConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "large-panic-conformance")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	// Reuse the existing full-std cache; export outputs remain independent.
	target := filepath.Join(root, ".cache", "panic-conformance", "cargo-target")
	manifest := filepath.Join(root, "fixtures", "large-panic", "Cargo.toml")
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	env := append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"),
		"RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1",
		"OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_large_panic_fixture::conformance", "RUSTFLAGS=",
		"CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+target)
	cargo := filepath.Join(sysroot, "bin", "cargo")
	common := []string{"-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest}
	build := exec.Command(cargo, append(append([]string{"build"}, common...),
		"--target", targetTriple(runtime.GOARCH), "--bin", "oxide-large-panic-native")...)
	build.Env = env
	run(t, build)
	expected := run(t, exec.Command(filepath.Join(target, targetTriple(runtime.GOARCH), "debug", "oxide-large-panic-native")))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			stage, err := os.MkdirTemp(cache, "export-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(stage)
			output := filepath.Join(stage, "oxide.mir.json")
			export := exec.Command(cargo, append(append([]string{"rustc"}, common...),
				"--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)...)
			export.Env = env
			run(t, export)
			saved := filepath.Join(cache, arch+".json")
			if err := os.Rename(output, saved); err != nil {
				t.Fatal(err)
			}
			p, err := mir.Load(saved)
			if err != nil {
				t.Fatal(err)
			}
			testGeneratedWithHarness(t, p, expected, arch, "large_generated_test.go")
		})
	}
}
