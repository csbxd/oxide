//go:build linux && (amd64 || arm64)

package mir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRustUpstreamRegression reruns every passing upstream coretests/alloctests
// case recorded for the host target. Missing baselines and regressions fail.
// Run: OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRustUpstreamRegression$' -timeout 2h -v
func TestRustUpstreamRegression(t *testing.T) {
	if os.Getenv("OXIDE_RUST_TESTS") != "1" {
		t.Skip("set OXIDE_RUST_TESTS=1 to run the upstream Rust regression baseline")
	}
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", filepath.Join(repo, "fixtures", "upstream", "run.py"), "check")
	cmd.Dir = repo
	cmd.Env = os.Environ()
	run(t, cmd)
}
