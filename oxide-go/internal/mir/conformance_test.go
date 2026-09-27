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
	"github.com/csbxd/oxide/oxide-go/internal/translate"
)

// TestMIRConformance uses MIR emitted by the pinned oxide-rs compiler, so the
// default suite exercises generated code without downloading a Rust toolchain.
// Both supported targets are compiled; the host target also executes the Rust
// result comparisons and verifies zero Go allocations for automatic storage.
func TestMIRConformance(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			p, err := mir.Load(filepath.Join("testdata", "core_"+arch+".json"))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata", "core.stdout"))
			if err != nil {
				t.Fatal(err)
			}
			testGenerated(t, p, expected, arch)
		})
	}
}

// TestRustConformance refreshes the entire Rust -> MIR -> Go chain and compares
// against a native executable from the same pinned compiler and source.
// Enable explicitly with OXIDE_RUST_TESTS=1 go test ./internal/mir -run Rust -v.
// Run oxide-rs/build.sh first, or set OXIDE_FRONTEND to the oxide-rs executable.
// To regenerate checked-in snapshots after a source/compiler change, also set
// OXIDE_UPDATE_FIXTURES=1 and select -run '^TestRustConformance$'. Both targets
// must compile and the host must pass differential checks before files change.
func TestRustConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "core.rs"))
	if err != nil {
		t.Fatal(err)
	}
	triple := targetTriple(runtime.GOARCH)
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", triple, "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	fixtures := map[string][]byte{"core.stdout": expected}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "core_" + arch + ".json"
		output := filepath.Join(dir, name)
		exportCoreFixture(t, frontend, sysroot, source, "oxide_conformance", arch, output)
		p, err := mir.Load(output)
		if err != nil {
			t.Fatal(err)
		}
		testGenerated(t, p, expected, arch)
		fixtures[name], err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("OXIDE_UPDATE_FIXTURES") == "1" {
		for name, data := range fixtures {
			if err := os.WriteFile(filepath.Join("testdata", name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Successful-path layout/numeric probes use an actual no_std panic handler.
// Building core supplies every non-generic lang-item MIR body; panic/unwind
// semantics are exercised separately with full Cargo std fixtures.
func exportCoreFixture(t *testing.T, frontend, sysroot, source, crate, arch, output string) {
	t.Helper()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "Cargo.toml")
	cargo := fmt.Sprintf("[package]\nname = %q\nversion = \"0.0.0\"\nedition = \"2021\"\n[lib]\npath = %q\n", crate, source)
	if err := os.WriteFile(manifest, []byte(cargo), 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "core-conformance", "cargo-target"))
	if err != nil {
		t.Fatal(err)
	}
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes", "--cfg=oxide_export", "--check-cfg=cfg(oxide_export)"}
	cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=core",
		"--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)
	cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend,
		"RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS="+crate+"::conformance",
		"RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+cache)
	run(t, cmd)
}

// This fixture exercises the Cargo driver, including rebuilding libstd with
// MIR available for non-generic Vec/String allocation helpers. It requires the
// rust-src component installed by oxide-rs/build.sh and is intentionally gated
// with the other native Rust differential tests.
func TestRustAllocConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	dir := t.TempDir()
	source, err := filepath.Abs(filepath.Join("testdata", "alloc.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "Cargo.toml")
	cargo := fmt.Sprintf("[package]\nname = \"oxide_alloc_conformance\"\nversion = \"0.0.0\"\nedition = \"2021\"\n[lib]\npath = %q\n", source)
	if err := os.WriteFile(manifest, []byte(cargo), 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "alloc-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CARGO_TARGET_DIR", filepath.Join(cache, "cargo-target"))
	out := filepath.Join(cache, "generated")
	_, err = translate.Run(translate.Config{Manifest: manifest, Output: out, Frontend: frontend,
		Roots: "oxide_alloc_conformance::conformance", Target: "linux/" + runtime.GOARCH, OverflowChecks: true})
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(dir, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021",
		"--target", targetTriple(runtime.GOARCH), "-Zmir-opt-level=0", "-Coverflow-checks=yes",
		filepath.Join("testdata", "native_alloc.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	p, err := mir.Load(filepath.Join(out, "oxide.mir.json"))
	if err != nil {
		t.Fatal(err)
	}
	testGenerated(t, p, expected, runtime.GOARCH)
}

func rustFrontend(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("OXIDE_RUST_TESTS") != "1" {
		t.Skip("set OXIDE_RUST_TESTS=1 to run the pinned Rust/Go differential tests")
	}
	path := os.Getenv("OXIDE_FRONTEND")
	if path == "" {
		path = filepath.Join("..", "..", "..", "bin", "oxide-rs")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Rust integration frontend unavailable; run oxide-rs/build.sh or set OXIDE_FRONTEND: %v", err)
	}
	sysroot := strings.TrimSpace(string(run(t, exec.Command(path, "--print-sysroot"))))
	return path, sysroot
}

func targetTriple(arch string) string {
	if arch == "amd64" {
		return "x86_64-unknown-linux-gnu"
	}
	return "aarch64-unknown-linux-gnu"
}

func testGenerated(t *testing.T, p *mir.Program, expected []byte, arch string) {
	t.Helper()
	testGeneratedWithHarness(t, p, expected, arch, "generated_test.go")
}

func testGeneratedWithHarness(t *testing.T, p *mir.Program, expected []byte, arch, harnessName string) {
	t.Helper()
	source, err := mir.Generate(p, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join("testdata", harnessName))
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"go.mod":              []byte(fmt.Sprintf("module oxide-conformance\n\ngo 1.27.1\n\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module)),
		"oxide_gen.go":        source,
		"oxide_gen_test.go":   harness,
		"allocations_test.go": allocations,
		"expected.stdout":     expected,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := mir.WriteAllocations(p, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
		t.Fatal(err)
	}
	args := []string{"test", "-mod=mod", "-count=1", "."}
	if arch != runtime.GOARCH {
		args = []string{"test", "-mod=mod", "-c", "-o", filepath.Join(dir, "fixture.test"), "."}
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "GOWORK=off", "CGO_ENABLED=0")
	run(t, cmd)
}

func run(t *testing.T, cmd *exec.Cmd) []byte {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, output)
	}
	return output
}
