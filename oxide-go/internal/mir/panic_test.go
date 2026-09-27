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

// This full-std fixture compares actual Rust panic payloads, hooks, catch_unwind
// and dynamic Drop against native Rust. No panic is emulated in the Go harness.
func TestRustPanicConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "panic-conformance")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cache, "cargo-target")
	manifest := filepath.Join(root, "fixtures", "panic", "Cargo.toml")
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	env := append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"),
		"RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1",
		"OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_panic_fixture::conformance", "RUSTFLAGS=",
		"CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+target)
	cargo := filepath.Join(sysroot, "bin", "cargo")
	triple := targetTriple(runtime.GOARCH)
	common := []string{"-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest, "--target", triple}
	build := exec.Command(cargo, append(append([]string{"build"}, common...), "--bin", "oxide-panic-native")...)
	build.Env = env
	run(t, build)
	native := exec.Command(filepath.Join(target, triple, "debug", "oxide-panic-native"))
	expected := run(t, native)
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(cache, "export-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	output := filepath.Join(stage, "oxide.mir.json")
	export := exec.Command(cargo, append(append([]string{"rustc"}, common...), "--lib", "--", "--oxide-export="+output)...)
	export.Env = env
	run(t, export)
	saved := filepath.Join(cache, "oxide.mir.json")
	if err := os.Rename(output, saved); err != nil {
		t.Fatal(err)
	}
	p, err := mir.Load(saved)
	if err != nil {
		t.Fatal(err)
	}
	testGeneratedWithHarness(t, p, expected, runtime.GOARCH, "panic_generated_test.go")
}
