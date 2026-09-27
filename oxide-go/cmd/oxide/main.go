package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/csbxd/oxide/oxide-go/internal/mir"
	"github.com/csbxd/oxide/oxide-go/internal/translate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "translate":
		translateCommand(os.Args[2:])
	case "audit":
		auditCommand(os.Args[2:])
	case "emit":
		emitCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func emitCommand(args []string) {
	fs := flag.NewFlagSet("emit", flag.ExitOnError)
	input := fs.String("mir", "", "oxide-rs MIR JSON")
	out := fs.String("out", "", "output directory")
	pkg := fs.String("package", "translated", "Go package name")
	fs.Parse(args)
	p, err := mir.Load(*input)
	if err == nil {
		var src []byte
		src, err = mir.Generate(p, *pkg)
		if err == nil {
			err = os.MkdirAll(*out, 0o755)
			if err == nil && len(p.Allocations) > 0 {
				err = mir.WriteAllocations(p, filepath.Join(*out, "oxide_alloc.bin"))
			}
			if err == nil {
				var files []string
				files, err = mir.WriteGoFiles(*out, src)
				if err == nil {
					err = mir.RemoveStaleGoFiles(*out, files)
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func translateCommand(args []string) {
	fs := flag.NewFlagSet("translate", flag.ExitOnError)
	var cfg translate.Config
	fs.StringVar(&cfg.Manifest, "manifest", "", "Cargo.toml to translate")
	fs.StringVar(&cfg.Package, "package", "", "Cargo package (required for a workspace with multiple packages)")
	fs.StringVar(&cfg.Output, "out", "", "output directory")
	fs.StringVar(&cfg.Frontend, "frontend", "", "oxide-rs rustc_public frontend")
	fs.StringVar(&cfg.Roots, "roots", "", "comma-separated Rust API paths; empty exports public functions, re-exports and monomorphic methods")
	fs.StringVar(&cfg.Target, "target", "", "target triple alias: linux/amd64 or linux/arm64")
	fs.BoolVar(&cfg.OverflowChecks, "overflow-checks", true, "preserve Rust checked integer arithmetic")
	fs.Parse(args)
	result, err := translate.Run(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, path := range result.Files {
		fmt.Println(path)
	}
}

func auditCommand(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	manifest := fs.String("manifest", "", "Cargo.toml to audit")
	packageName := fs.String("package", "", "Cargo package")
	analyzer := fs.String("rust-analyzer", "rust-analyzer", "rust-analyzer executable")
	formatName := fs.String("format", "markdown", "markdown or json")
	fs.Parse(args)
	a, err := translate.AuditPackage(*manifest, *packageName, *analyzer)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *formatName == "json" {
		b, err := a.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}
	if *formatName != "markdown" {
		fmt.Fprintf(os.Stderr, "unsupported audit format %q\n", *formatName)
		os.Exit(2)
	}
	fmt.Print(a.Markdown())
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: oxide translate -manifest Cargo.toml -out DIR [-package NAME]")
	fmt.Fprintln(os.Stderr, "       oxide emit -mir FILE -out DIR [-package NAME]")
	fmt.Fprintln(os.Stderr, "       oxide audit -manifest Cargo.toml [-package NAME] [-format markdown|json]")
}
