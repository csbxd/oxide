//go:build linux && (amd64 || arm64)

package mir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Each source is an external consumer of the generated package. The positive
// consumer runs first, so an unavailable toolchain or broken generated package
// cannot make a negative consumer pass accidentally.
func testTypeAPICompile(t *testing.T, generated, arch string) {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join("testdata", "type_api_compile", "*.go.txt"))
	if err != nil || len(sources) < 2 {
		t.Fatalf("static API compile cases: %v, count=%d", err, len(sources))
	}
	for index, source := range sources {
		name := strings.TrimSuffix(filepath.Base(source), ".go.txt")
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		var expected []*regexp.Regexp
		for _, line := range strings.Split(string(data), "\n") {
			if pattern, ok := strings.CutPrefix(line, "// want-error: "); ok {
				expected = append(expected, regexp.MustCompile(pattern))
			}
		}
		if (index == 0) != (len(expected) == 0) {
			t.Fatalf("%s: require one positive consumer followed by explicit negative consumers", name)
		}
		t.Run("compile_"+name, func(t *testing.T) {
			relative := filepath.Join("testdata", "compile", name)
			dir := filepath.Join(generated, relative)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "consumer.go"), data, 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("go", "build", "-mod=mod", "-gcflags=oxide-type-api-conformance=-smallframes", "./"+filepath.ToSlash(relative))
			cmd.Dir = generated
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
			output, err := cmd.CombinedOutput()
			if err := os.WriteFile(filepath.Join(dir, "compile.log"), output, 0644); err != nil {
				t.Fatal(err)
			}
			if len(expected) == 0 {
				if err != nil {
					t.Fatalf("positive external consumer: %v\n%s", err, output)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid Rust type operation compiled")
			}
			var diagnostics []string
			for _, line := range strings.Split(string(output), "\n") {
				if strings.Contains(line, "consumer.go:") {
					diagnostics = append(diagnostics, line)
				}
			}
			if len(diagnostics) != len(expected) {
				t.Fatalf("wanted %d local type errors, got %d: %v\n%s", len(expected), len(diagnostics), err, output)
			}
			for i, pattern := range expected {
				if !pattern.MatchString(diagnostics[i]) {
					t.Fatalf("diagnostic %d did not match %q:\n%s", i, pattern, output)
				}
			}
		})
		if index == 0 && t.Failed() {
			t.Fatal("positive consumer failed; refusing to interpret negative builds")
		}
	}
}
