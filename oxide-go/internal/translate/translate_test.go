package translate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

// Exercise the public driver without requiring a Rust installation. The helper
// models Cargo's freshness rule: a repeated final rustc argument skips export.
func TestMIRDriver(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	for _, name := range []string{"cargo", "frontend"} {
		script := "#!/bin/sh\nexec " + quote(exe) + " -test.run=^TestDriverHelper$ -- " + name + " \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("OXIDE_DRIVER_TEST", root)
	t.Setenv("CARGO_ENCODED_RUSTFLAGS", "invalid ambient compiler flags")
	t.Setenv("OXIDE_ROOTS", "ambient root")
	t.Setenv("OXIDE_EXPORT", "ambient output")
	cfg := Config{Manifest: filepath.Join(root, "Cargo.toml"), Package: "fixture", Output: filepath.Join(root, "out"), Frontend: filepath.Join(bin, "frontend"), Target: "linux/arm64", Roots: "fixture::first", OverflowChecks: true}
	for _, roots := range []string{"fixture::first", "fixture::second", "fixture::second", ""} {
		cfg.Roots = roots
		result, err := Run(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Files) != 2 {
			t.Fatalf("output files: %v", result.Files)
		}
		data, err := os.ReadFile(filepath.Join(cfg.Output, "oxide_gen_00000.go"))
		if err != nil {
			t.Fatal(err)
		}
		want := "First"
		if strings.HasSuffix(roots, "second") {
			want = "Second"
		}
		if !strings.Contains(string(data), "func "+want+"(") {
			t.Fatalf("stale export for roots %q", roots)
		}
	}
	cfg.OverflowChecks = false
	if _, err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	// Generation failure must not publish a new MIR alongside stale Go code.
	before, err := os.ReadFile(filepath.Join(cfg.Output, "oxide.mir.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OXIDE_DRIVER_BAD_MIR", "1")
	if _, err := Run(cfg); err == nil {
		t.Fatal("expected invalid MIR target to fail")
	}
	after, err := os.ReadFile(filepath.Join(cfg.Output, "oxide.mir.json"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("failed export changed published MIR: %v", err)
	}
	entries, err := os.ReadDir(cfg.Output)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".oxide-") {
			t.Fatalf("temporary output leaked: %s", entry.Name())
		}
	}
}

func TestDriverHelper(t *testing.T) {
	root := os.Getenv("OXIDE_DRIVER_TEST")
	if root == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	if args[0] == "frontend" {
		fmt.Println(root)
		os.Exit(0)
	}
	if args[1] == "metadata" {
		fmt.Printf(`{"packages":[{"name":"fixture","manifest_path":%q,"targets":[{"name":"fixture","kind":["lib"],"src_path":"lib.rs"}]}]}`, filepath.Join(root, "Cargo.toml"))
		os.Exit(0)
	}
	if args[1] != "rustc" || !strings.Contains(strings.Join(args, " "), "--package fixture") {
		t.Fatal("expected cargo rustc with selected package", args)
	}
	flags := os.Getenv("CARGO_ENCODED_RUSTFLAGS")
	checks := "yes"
	// The parent first makes four checked exports, then an unchecked export.
	countPath := filepath.Join(root, "count")
	count, _ := os.ReadFile(countPath)
	if len(count) >= 4 {
		checks = "no"
	}
	if flags != "-Zalways-encode-mir\x1f-Zmir-opt-level=0\x1f-Coverflow-checks="+checks || os.Getenv("RUSTFLAGS") != "" || os.Getenv("OXIDE_EXPORT") != "" {
		t.Fatalf("compiler environment not isolated: %q", flags)
	}
	if err := os.WriteFile(countPath, append(count, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	last := args[len(args)-1]
	if !strings.HasPrefix(last, "--oxide-export=") {
		t.Fatal("missing final export argument", args)
	}
	previousPath := filepath.Join(root, "previous")
	previous, _ := os.ReadFile(previousPath)
	if string(previous) == last {
		os.Exit(0)
	}
	if err := os.WriteFile(previousPath, []byte(last), 0o644); err != nil {
		t.Fatal(err)
	}
	name := os.Getenv("OXIDE_ROOTS")
	if name == "" {
		name = "fixture::first"
	}
	var scalar mir.Primitive
	if err := json.Unmarshal([]byte(`{"Int":{"length":"I64","signed":false}}`), &scalar); err != nil {
		t.Fatal(err)
	}
	p := mir.Program{Compiler: "nightly-2026-09-15 (574ff7d98)", Target: "aarch64-unknown-linux-gnu", Roots: []mir.Root{{Name: name, Symbol: "root"}}, Types: []mir.Type{{ID: 1, Name: "u64", Kind: "u64", Size: 8, Align: 8, Sized: true, ValueABI: "Scalar", ABIScalar: &scalar}}, Functions: []mir.Function{{Name: name, Symbol: "root", Kind: "Item", Body: &mir.Body{Locals: []mir.Local{{Type: 1}}, Blocks: []mir.Block{{Terminator: mir.Statement{Kind: json.RawMessage(`"Return"`)}}}}}}}
	if os.Getenv("OXIDE_DRIVER_BAD_MIR") != "" {
		p.Target = "unsupported-target"
	}
	data, err := json.Marshal(&p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strings.TrimPrefix(last, "--oxide-export="), data, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestTargetTriple(t *testing.T) {
	for _, target := range []string{"linux/amd64", "linux/arm64"} {
		if _, err := TargetTriple(target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := TargetTriple("darwin/arm64"); err == nil {
		t.Fatal("accepted unsupported target")
	}
}
