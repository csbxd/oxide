package translate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/csbxd/oxide/oxide-go/internal/cargo"
	"github.com/csbxd/oxide/oxide-go/internal/ra"
)

type Audit struct {
	Package      string         `json:"package"`
	Files        int            `json:"files"`
	Lines        int            `json:"lines"`
	NodeKinds    map[string]int `json:"node_kinds"`
	Items        map[string]int `json:"items"`
	Attributes   map[string]int `json:"attributes"`
	Uses         []string       `json:"uses"`
	Dependencies []string       `json:"dependencies"`
}

func AuditPackage(manifest, packageName, analyzer string) (*Audit, error) {
	m, err := cargo.Load(manifest)
	if err != nil {
		return nil, err
	}
	pkg, err := m.Package(packageName)
	if err != nil {
		return nil, err
	}
	target, err := libraryTarget(pkg)
	if err != nil {
		return nil, err
	}
	a := &Audit{Package: pkg.Name, NodeKinds: map[string]int{}, Items: map[string]int{}, Attributes: map[string]int{}}
	for _, d := range pkg.Dependencies {
		a.Dependencies = append(a.Dependencies, d.Name)
	}
	sort.Strings(a.Dependencies)
	root := filepath.Dir(target.CrateRoot)
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			if info.Name() == "target" || info.Name() == "tests" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".rs" {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	parser := ra.Parser{Binary: analyzer}
	for _, path := range files {
		src, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		a.Files++
		a.Lines += strings.Count(string(src), "\n") + 1
		tree, e := parser.Parse(src)
		if e != nil {
			return nil, fmt.Errorf("%s: %w", path, e)
		}
		tree.Walk(func(n *ra.Node) bool {
			a.NodeKinds[n.Kind]++
			if n.Kind == "ATTR" {
				x := strings.TrimSpace(n.Span(string(src)))
				a.Attributes[x]++
			}
			if n.Kind == "USE" {
				x := strings.TrimSpace(n.Span(string(src)))
				a.Uses = append(a.Uses, x)
			}
			return true
		})
		for _, i := range tree.Children {
			if i.Type == "Node" {
				a.Items[i.Kind]++
			}
		}
	}
	return a, nil
}

func (a *Audit) JSON() ([]byte, error) { return json.MarshalIndent(a, "", "  ") }
func (a *Audit) Markdown() string {
	var b strings.Builder
	b.WriteString("# Oxide audit: " + a.Package + "\n\n")
	fmt.Fprintf(&b, "- Rust files: %d\n- Lines: %d\n\n", a.Files, a.Lines)
	b.WriteString("Syntax inventory only. Translation support is measured by MIR generation and Rust/Go differential tests; see MILESTONES.md.\n\n")
	b.WriteString("## Items\n\n| Kind | Count |\n| --- | ---: |\n")
	for _, k := range sortedKeys(a.Items) {
		fmt.Fprintf(&b, "| `%s` | %d |\n", k, a.Items[k])
	}
	b.WriteString("\n## External dependencies\n\n")
	for _, d := range a.Dependencies {
		fmt.Fprintf(&b, "- `%s`\n", d)
	}
	return b.String()
}
func sortedKeys(m map[string]int) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
