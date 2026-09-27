//go:build linux && (amd64 || arm64)

package mir_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

func replacementExit(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	failure, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("%v: %v", cmd.Args, err)
	}
	status := failure.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	if code := failure.ExitCode(); code >= 0 {
		return code
	}
	t.Fatalf("unexpected process status: %v\n%s", err, output)
	return -1
}

func TestRustReplaceConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	t.Setenv("GOMEMLIMIT", "3GiB")
	t.Setenv("GOMAXPROCS", "2")
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "replace-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("testdata", "replace.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(cache, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte(fmt.Sprintf("[package]\nname=\"oxide_replace_fixture\"\nversion=\"0.0.0\"\nedition=\"2024\"\n[lib]\npath=%q\n", source)), 0644); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(cache, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2024", "-Awarnings", "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_replace.rs"), "-o", native))
	expected := run(t, exec.Command(native))
	if err := os.WriteFile(filepath.Join(cache, "expected.stdout"), expected, 0644); err != nil {
		t.Fatal(err)
	}
	nativeAbort := replacementExit(t, exec.Command("sh", "-c", "ulimit -c 0; exec \"$@\"", "limit-core", native, "double"))
	if nativeAbort != 134 {
		t.Fatalf("native Rust double panic status=%d", nativeAbort)
	}
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	roots := []string{"setup", "reset", "state", "make_zero", "make_owner", "make_field_panic", "inspect", "source_slot", "catch_callback"}
	for i := range roots {
		roots[i] = "oxide_replace_fixture::" + roots[i]
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			stage, err := os.MkdirTemp(cache, "export-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(stage)
			pending := filepath.Join(stage, "oxide.mir.json")
			cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+pending)
			cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS="+strings.Join(roots, ","), "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(root, ".cache", "frontend-panic", "target"), "CARGO_BUILD_JOBS=2")
			run(t, cmd)
			metadata := filepath.Join(cache, arch+".json")
			if err := os.Rename(pending, metadata); err != nil {
				t.Fatal(err)
			}
			p, err := mir.Load(metadata)
			if err != nil {
				t.Fatal(err)
			}
			var callbackType int
			for _, f := range p.Functions {
				if f.Name == "oxide_replace_fixture::catch_callback" {
					callbackType = f.Body.Locals[1].Type
				}
			}
			unit := -1
			for _, ty := range p.Types {
				if ty.ID == callbackType {
					unit = ty.FnOutput
				}
			}
			if unit < 0 {
				t.Fatal("missing callback ABI")
			}
			generated, err := mir.Generate(p, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			harness, err := os.ReadFile(filepath.Join("testdata", "replace_generated_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			harness = []byte(strings.ReplaceAll(string(harness), "oxideReplaceUnit", fmt.Sprintf("T%d", unit)))
			allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"go.mod": []byte(fmt.Sprintf("module oxide-replace-conformance\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go"))), "oxide_gen_test.go": harness, "allocations_test.go": allocations, "expected.stdout": expected}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			names, err := mir.WriteGoFiles(dir, generated)
			if err != nil {
				t.Fatal(err)
			}
			if err := mir.RemoveStaleGoFiles(dir, names); err != nil {
				t.Fatal(err)
			}
			if err := mir.WriteAllocations(p, filepath.Join(dir, "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "replace.test")
			cmd = exec.Command("go", "test", "-mod=mod", "-tags=memory.counters", "-gcflags=-smallframes", "-c", "-o", binary, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			run(t, cmd)
			if arch == runtime.GOARCH {
				cmd = exec.Command(binary, "-test.v", "-test.count=1")
				cmd.Dir = dir
				log := run(t, cmd)
				if err := os.WriteFile(filepath.Join(cache, arch+".log"), log, 0644); err != nil {
					t.Fatal(err)
				}
				cmd = exec.Command(binary, "-test.run", "^TestReplaceDoublePanic$", "-test.count=1")
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "OXIDE_REPLACE_DOUBLE=1")
				if code := replacementExit(t, cmd); code != nativeAbort {
					t.Fatalf("double panic Go=%d native=%d", code, nativeAbort)
				}
			}
		})
	}
}
