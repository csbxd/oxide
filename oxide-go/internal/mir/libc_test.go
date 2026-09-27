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

// Real Rust extern declarations exercise C variadics, errno, C allocation and
// weak function slots. Results compare validation flags or fixed-content hashes,
// never random bytes, OS thread IDs, pointer values, or temporary filenames.
func TestRustLibcConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	t.Setenv("GOMEMLIMIT", "3GiB")
	t.Setenv("GOMAXPROCS", "2")
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "libc-conformance")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cache, "cargo-target")
	manifest := filepath.Join(root, "fixtures", "libc", "Cargo.toml")
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	env := append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"),
		"RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1",
		"OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_libc_fixture::conformance", "RUSTFLAGS=",
		"CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+target)
	cargo := filepath.Join(sysroot, "bin", "cargo")
	triple := targetTriple(runtime.GOARCH)
	common := []string{"-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest}
	build := exec.Command(cargo, append(append([]string{"build"}, common...), "--target", triple, "--bin", "oxide-libc-native")...)
	build.Env = env
	run(t, build)
	native := exec.Command(filepath.Join(target, triple, "debug", "oxide-libc-native"))
	nativeDir, err := os.MkdirTemp(cache, "native-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(nativeDir)
	native.Dir = nativeDir
	expected := run(t, native)
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
			export := exec.Command(cargo, append(append([]string{"rustc"}, common...), "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)...)
			export.Env = env
			run(t, export)
			saved := filepath.Join(cache, arch+".json")
			if err := os.Rename(output, saved); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(strings.TrimSuffix(output, ".json")+".api.json", filepath.Join(cache, arch+".api.json")); err != nil {
				t.Fatal(err)
			}
			p, err := mir.Load(saved)
			if err != nil {
				t.Fatal(err)
			}
			testGeneratedWithHarness(t, p, expected, arch, "libc_generated_test.go")
		})
	}
}
