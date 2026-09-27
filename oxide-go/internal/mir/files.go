package mir

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const generatedFileLines = 30_000
const generatedFileBytes = 2 << 20

type generatedFile struct {
	name string
	data []byte
}

// Keep declaration order, including init functions, in lexical filename order.
// A single large declaration stays intact even if it exceeds the target size.
func splitGeneratedFiles(src []byte, maxLines, maxBytes int) ([]generatedFile, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "oxide_gen.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("split generated Go: %w", err)
	}
	file := fset.File(f.Pos())
	header := src[:file.Offset(f.Name.End())]
	type imported struct {
		name, path string
		source     []byte
	}
	imports := make([]imported, 0, len(f.Imports))
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		name := path.Base(p)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." {
			return nil, fmt.Errorf("split generated Go: dot import %q", p)
		}
		imports = append(imports, imported{name, p, src[file.Offset(spec.Pos()):file.Offset(spec.End())]})
	}
	var files []generatedFile
	var body bytes.Buffer
	used := map[string]bool{}
	lines := 0
	embed := false
	flush := func() {
		var out bytes.Buffer
		out.Write(header)
		out.WriteString("\n\n")
		var specs [][]byte
		for _, spec := range imports {
			include := used[spec.name]
			if spec.name == "_" {
				include = len(files) == 0
			}
			if spec.path == "embed" {
				include = embed || used[spec.name]
			}
			if include {
				specs = append(specs, spec.source)
			}
		}
		if len(specs) != 0 {
			out.WriteString("import (\n")
			for _, spec := range specs {
				out.WriteByte('\t')
				out.Write(spec)
				out.WriteByte('\n')
			}
			out.WriteString(")\n\n")
		}
		out.Write(body.Bytes())
		files = append(files, generatedFile{fmt.Sprintf("oxide_gen_%05d.go", len(files)), out.Bytes()})
		body.Reset()
		clear(used)
		lines, embed = 0, false
	}
	// Reserve room for the repeated header and the complete import block.
	overheadBytes, overheadLines := len(header)+16, bytes.Count(header, []byte{'\n'})+8
	for _, spec := range imports {
		overheadBytes += len(spec.source) + 2
		overheadLines++
	}
	for _, decl := range f.Decls {
		start := decl.Pos()
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		}
		text := src[file.Offset(start):file.Offset(decl.End())]
		declLines := bytes.Count(text, []byte{'\n'}) + 2
		if declLines+overheadLines >= 1<<20-2 {
			return nil, fmt.Errorf("split generated Go: declaration at line %d exceeds compiler source line limit", file.Line(start))
		}
		if body.Len() != 0 && (lines+declLines+overheadLines > maxLines || body.Len()+len(text)+2+overheadBytes > maxBytes) {
			flush()
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := s.X.(*ast.Ident); ok {
					used[id.Name] = true
				}
			}
			return true
		})
		if bytes.Contains(text, []byte("//go:embed ")) || bytes.Contains(text, []byte("//go:embed\t")) {
			embed = true
		}
		body.Write(text)
		body.WriteString("\n\n")
		lines += declLines
	}
	if body.Len() != 0 || len(files) == 0 {
		flush()
	}
	return files, nil
}

func generatedFilename(name string) bool {
	if name == "oxide_gen.go" {
		return true
	}
	if !strings.HasPrefix(name, "oxide_gen_") || !strings.HasSuffix(name, ".go") {
		return false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(name, "oxide_gen_"), ".go")
	if len(digits) != 5 {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// WriteGoFiles splits and publishes generated declarations in source order.
func WriteGoFiles(dir string, src []byte) ([]string, error) {
	files, err := splitGeneratedFiles(src, generatedFileLines, generatedFileBytes)
	if err != nil {
		return nil, err
	}
	return publishGeneratedFiles(dir, files)
}

func publishGeneratedFiles(dir string, files []generatedFile) ([]string, error) {
	stage, err := os.MkdirTemp(dir, ".oxide-go-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(stage, f.name), f.data, 0o644); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		dst := filepath.Join(dir, f.name)
		if err := os.Rename(filepath.Join(stage, f.name), dst); err != nil {
			return nil, err
		}
		paths = append(paths, dst)
	}
	if err := RemoveStaleGoFiles(dir, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// RemoveStaleGoFiles removes only source files owned by the generator.
func RemoveStaleGoFiles(dir string, paths []string) error {
	keep := make(map[string]bool, len(paths))
	for _, path := range paths {
		keep[filepath.Base(path)] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && generatedFilename(entry.Name()) && !keep[entry.Name()] {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
