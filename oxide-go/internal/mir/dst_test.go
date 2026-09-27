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

// Trait-object tail alignment comes from the vtable, including nested DSTs.
// Compare actual Arc/Box access and destruction with native Rust; packed tails
// use raw unaligned reads rather than constructing invalid Rust references.
func TestRustDSTConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "dst.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "Cargo.toml")
	config := fmt.Sprintf("[package]\nname = \"oxide_dst_conformance\"\nversion = \"0.0.0\"\nedition = \"2021\"\n[lib]\npath = %q\n", source)
	if err := os.WriteFile(manifest, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "dst-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native_dst.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			output := filepath.Join(cache, arch+".json")
			cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind",
				"--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)
			cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend,
				"RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_dst_conformance::conformance",
				"RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(cache, "cargo-target"))
			run(t, cmd)
			p, err := mir.Load(output)
			if err != nil {
				t.Fatal(err)
			}
			testGenerated(t, p, expected, arch)
		})
	}
}
