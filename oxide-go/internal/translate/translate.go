package translate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/csbxd/oxide/oxide-go/internal/cargo"
	"github.com/csbxd/oxide/oxide-go/internal/mir"
)

type Config struct {
	Manifest, Package, Output, Target, Frontend, Roots string
	Features                                           string
	NoDefaultFeatures                                  bool
	OverflowChecks                                     bool
}

type Result struct {
	Package string
	Target  string
	Files   []string
}

// Run translates only compiler-validated MIR and target layouts.
func Run(cfg Config) (*Result, error) { return runMIR(cfg) }

func runMIR(cfg Config) (*Result, error) {
	if cfg.Target == "" {
		cfg.Target = "linux/" + runtime.GOARCH
	}
	triple, err := TargetTriple(cfg.Target)
	if err != nil {
		return nil, err
	}
	frontend, err := findFrontend(cfg.Frontend)
	if err != nil {
		return nil, err
	}
	if frontend, err = filepath.Abs(frontend); err != nil {
		return nil, err
	}
	sysrootBytes, err := exec.Command(frontend, "--print-sysroot").Output()
	if err != nil {
		return nil, fmt.Errorf("frontend sysroot: %w", err)
	}
	sysroot := strings.TrimSpace(string(sysrootBytes))
	cargoBin := filepath.Join(sysroot, "bin", "cargo")
	m, err := cargo.LoadWith(cargoBin, cfg.Manifest)
	if err != nil {
		return nil, err
	}
	pkg, err := m.Package(cfg.Package)
	if err != nil {
		return nil, err
	}
	target, err := libraryTarget(pkg)
	if err != nil {
		return nil, err
	}
	if cfg.Output == "" {
		return nil, fmt.Errorf("-out is required")
	}
	if cfg.Output, err = filepath.Abs(cfg.Output); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Output, 0o755); err != nil {
		return nil, err
	}
	// Keep the previous output intact on a compiler/generator failure. The
	// unique final rustc argument also prevents stale Cargo MIR exports when
	// roots, options or the output directory change.
	stage, err := os.MkdirTemp(cfg.Output, ".oxide-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	exportPath := filepath.Join(stage, "oxide.mir.json")
	args := []string{"rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", pkg.ManifestPath, "--package", pkg.Name, "--target", triple}
	if cfg.Features != "" {
		args = append(args, "--features", cfg.Features)
	}
	if cfg.NoDefaultFeatures {
		args = append(args, "--no-default-features")
	}
	if len(target.Kind) > 0 && target.Kind[0] == "bin" {
		args = append(args, "--bin", target.Name)
	} else {
		args = append(args, "--lib")
	}
	args = append(args, "--", "--oxide-export="+exportPath)
	cmd := exec.Command(cargoBin, args...)
	checks := "no"
	if cfg.OverflowChecks {
		checks = "yes"
	}
	// Cargo gives CARGO_ENCODED_RUSTFLAGS precedence over RUSTFLAGS. Set both
	// deliberately so ambient flags cannot change the exported MIR contract.
	flags := []string{"-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=" + checks}
	cmd.Env = append(os.Environ(), "RUSTC="+filepath.Join(sysroot, "bin", "rustc"), "RUSTC_WRAPPER="+frontend,
		"RUSTC_WORKSPACE_WRAPPER=", "RUSTC_BOOTSTRAP=1", "OXIDE_EXPORT=", "OXIDE_ROOTS="+cfg.Roots,
		"RUSTFLAGS=", "CARGO_ENCODED_RUSTFLAGS="+strings.Join(flags, "\x1f"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("Rust MIR build failed: %w\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(stage, "oxide.mir.api.json")); err != nil {
		return nil, fmt.Errorf("Rust public API manifest: %w", err)
	}
	program, err := mir.Load(exportPath)
	if err != nil {
		return nil, err
	}
	source, err := mir.Generate(program, strings.ReplaceAll(pkg.Name, "-", "_"))
	if err != nil {
		return nil, err
	}
	if len(program.Allocations) > 0 {
		if err := mir.WriteAllocations(program, filepath.Join(stage, "oxide_alloc.bin")); err != nil {
			return nil, err
		}
	}
	generated, err := mir.WriteGoFiles(stage, source)
	if err != nil {
		return nil, err
	}
	files := []string{"oxide.mir.json", "oxide.mir.api.json"}
	if len(program.Allocations) > 0 {
		files = append(files, "oxide_alloc.bin")
	}
	for _, path := range generated {
		files = append(files, filepath.Base(path))
	}
	result := &Result{Package: pkg.Name, Target: cfg.Target}
	for _, name := range files {
		dst := filepath.Join(cfg.Output, name)
		if err := os.Rename(filepath.Join(stage, name), dst); err != nil {
			return nil, err
		}
		result.Files = append(result.Files, dst)
	}
	if err := mir.RemoveStaleGoFiles(cfg.Output, result.Files); err != nil {
		return nil, err
	}
	return result, nil
}

func findFrontend(given string) (string, error) {
	if given != "" {
		if _, err := os.Stat(given); err != nil {
			return "", err
		}
		return given, nil
	}
	if s := os.Getenv("OXIDE_FRONTEND"); s != "" {
		return s, nil
	}
	for _, p := range []string{"../../bin/oxide-rs", "../bin/oxide-rs", "bin/oxide-rs"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("oxide-rs frontend not found; run oxide/oxide-rs/build.sh or pass -frontend")
}

func TargetTriple(target string) (string, error) {
	switch target {
	case "linux/amd64":
		return "x86_64-unknown-linux-gnu", nil
	case "linux/arm64":
		return "aarch64-unknown-linux-gnu", nil
	default:
		return "", fmt.Errorf("unsupported target %q; supported: linux/amd64, linux/arm64", target)
	}
}

func libraryTarget(pkg *cargo.Package) (*cargo.Target, error) {
	for _, kind := range []string{"lib", "bin"} {
		for i := range pkg.Targets {
			for _, k := range pkg.Targets[i].Kind {
				if k == kind {
					return &pkg.Targets[i], nil
				}
			}
		}
	}
	return nil, fmt.Errorf("package %q has no library or binary target", pkg.Name)
}
