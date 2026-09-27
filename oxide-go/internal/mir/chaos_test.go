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

// This gate compiles actual Rust ownership/panic churn and checks its live
// offheap allocation ledger after randomized repeated calls and Context.Close.
func TestRustOwnershipChaos(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(root, "fixtures", "chaos")
	cache := filepath.Join(root, ".cache", "ownership-chaos")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, ".cache", "panic-conformance", "cargo-target")
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	env := append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"),
		"RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=",
		"OXIDE_ROOTS=oxide_chaos_fixture::cycle,oxide_chaos_fixture::retain,oxide_chaos_fixture::reclaim",
		"RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+target)
	cargo := filepath.Join(sysroot, "bin", "cargo")
	common := []string{"-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", filepath.Join(fixture, "Cargo.toml")}
	cmd := exec.Command(cargo, append(append([]string{"build"}, common...), "--target", targetTriple(runtime.GOARCH), "--bin", "oxide-chaos-native")...)
	cmd.Env = env
	run(t, cmd)
	expected := run(t, exec.Command(filepath.Join(target, targetTriple(runtime.GOARCH), "debug", "oxide-chaos-native")))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0o644); err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join(fixture, "generated_test.go"))
	if err != nil {
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
			cmd := exec.Command(cargo, append(append([]string{"rustc"}, common...), "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+output)...)
			cmd.Env = env
			result := run(t, cmd)
			if err := os.WriteFile(filepath.Join(cache, arch+"-export.log"), result, 0o644); err != nil {
				t.Fatal(err)
			}
			saved := filepath.Join(cache, arch+".json")
			if err := os.Rename(output, saved); err != nil {
				t.Fatal(err)
			}
			program, err := mir.Load(saved)
			if err != nil {
				t.Fatal(err)
			}
			source, err := mir.Generate(program, "chaosfixture")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			files, err := mir.WriteGoFiles(dir, source)
			if err != nil {
				t.Fatal(err)
			}
			if err := mir.RemoveStaleGoFiles(dir, files); err != nil {
				t.Fatal(err)
			}
			if err := mir.WriteAllocations(program, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			module := fmt.Sprintf("module oxide-ownership-chaos\n\ngo 1.27.1\n\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))
			for name, data := range map[string][]byte{"go.mod": []byte(module), "chaos_test.go": harness, "expected.stdout": expected} {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"test", "-mod=mod", "-tags=memory.counters", "-count=1", "-timeout=20m", "-v", "."}
			if arch != runtime.GOARCH {
				args = []string{"test", "-mod=mod", "-tags=memory.counters", "-c", "-o", filepath.Join(dir, "chaos.test"), "."}
			}
			cmd = exec.Command("go", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			testResult := run(t, cmd)
			if err := os.WriteFile(filepath.Join(cache, arch+"-test.log"), testResult, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}
