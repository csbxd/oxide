//go:build linux && (amd64 || arm64)

package mir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This gate retains default renderer features and exports both SVG and PNG
// roots. It intentionally fails on unsupported translation or differing output.
// Run: OXIDE_RENDERER_TESTS=1 go test ./internal/mir -run '^TestRendererConformance$' -timeout 30m -v
// Staged retries and retained artifacts: python3 oxide/fixtures/renderer/test.py --stage go
func TestRendererConformance(t *testing.T) {
	if os.Getenv("OXIDE_RENDERER_TESTS") != "1" {
		t.Skip("set OXIDE_RENDERER_TESTS=1 to run complete renderer SVG/PNG differential tests")
	}
	script := filepath.Join("..", "..", "..", "fixtures", "renderer", "test.py")
	run(t, exec.Command("python3", script))
}
