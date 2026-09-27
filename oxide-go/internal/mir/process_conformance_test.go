//go:build linux && (amd64 || arm64)

package mir_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

type processResult struct {
	code           int
	stdout, stderr string
}

func runProcessProbe(t *testing.T, path string, args, environment []string) processResult {
	t.Helper()
	cmd := exec.Command(path, args...)
	cmd.Args[0] = "process-probe"
	cmd.Env = append(os.Environ(), "OXIDE_PROCESS_EXIT=", "OXIDE_MUTATE_GO_ARGS=")
	cmd.Env = append(cmd.Env, environment...)
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return processResult{code, out.String(), errout.String()}
}

func TestRustProcessConformance(t *testing.T) {
	frontend, sysroot := rustFrontend(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, ".cache", "process-conformance")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("testdata", "process.rs"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(cache, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte(fmt.Sprintf("[package]\nname=\"oxide_process_fixture\"\nversion=\"0.0.0\"\nedition=\"2021\"\n[lib]\npath=%q\n", source)), 0644); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(cache, "native")
	run(t, exec.Command(filepath.Join(sysroot, "bin", "rustc"), "--edition=2021", "-Zmir-opt-level=0", "-Coverflow-checks=yes", filepath.Join("testdata", "native_process.rs"), "-o", native))
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			metadata := filepath.Join(cache, arch+".json")
			cmd := exec.Command(filepath.Join(sysroot, "bin", "cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest, "--target", targetTriple(arch), "--lib", "--", "--oxide-export="+metadata)
			cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend, "RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS=oxide_process_fixture::args_digest,oxide_process_fixture::exit_probe", "RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"), "CARGO_TARGET_DIR="+filepath.Join(cache, "cargo-target"))
			run(t, cmd)
			p, err := mir.Load(metadata)
			if err != nil {
				t.Fatal(err)
			}
			markers := 0
			for _, f := range p.Functions {
				if f.RuntimeBoundary == "std_args" {
					markers++
				}
			}
			if markers != 1 {
				t.Fatalf("std argc/argv boundaries=%d", markers)
			}
			generated, err := mir.Generate(p, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(cache, "go-"+arch)
			if err := os.MkdirAll(filepath.Join(dir, "fixture"), 0755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"go.mod":               fmt.Sprintf("module process-probe\n\ngo 1.27.1\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", filepath.Join(root, "oxide-go")),
				"fixture/oxide_gen.go": string(generated),
				"main.go": `package main
import("fmt";"os";"strconv";fixture "process-probe/fixture";oxide "github.com/csbxd/oxide/oxide-go/runtime")
func main(){c:=oxide.NewContext();defer c.Close();if code:=os.Getenv("OXIDE_PROCESS_EXIT");code!=""{n,e:=strconv.Atoi(code);if e!=nil{panic(e)};fixture.ExitProbe(c,int32(n));return};if os.Getenv("OXIDE_MUTATE_GO_ARGS")=="1"{os.Args=[]string{"changed"}};fmt.Println(fixture.ArgsDigest(c))}
`,
			}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := mir.WriteAllocations(p, filepath.Join(dir, "fixture", "oxide_alloc.bin")); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "process-probe")
			build := exec.Command("go", "build", "-mod=mod", "-o", binary, ".")
			build.Dir = dir
			build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			run(t, build)
			if arch != runtime.GOARCH {
				return
			}
			for i, args := range [][]string{nil, {""}, {"plain", "two words"}, {"中文雪", "", "trailing"}, {"bad\xff\xfe", "", "a\xc0\x80b"}} {
				want := runProcessProbe(t, native, args, nil)
				for _, env := range [][]string{nil, {"OXIDE_MUTATE_GO_ARGS=1"}} {
					if got := runProcessProbe(t, binary, args, env); got != want {
						t.Fatalf("args case=%d env=%v native=%+v translated=%+v", i, env, want, got)
					}
				}
			}
			for _, code := range []int{0, 7, -1, 256} {
				env := []string{fmt.Sprintf("OXIDE_PROCESS_EXIT=%d", code)}
				want := runProcessProbe(t, native, nil, env)
				if got := runProcessProbe(t, binary, nil, env); got != want {
					t.Fatalf("exit %d native=%+v translated=%+v", code, want, got)
				}
				if want.stdout != "buffered-stdout" || want.stderr != "tls-drop\n" {
					t.Fatalf("native exit lifecycle changed: %+v", want)
				}
			}
		})
	}
}
