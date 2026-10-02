package mir

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"unicode"
)

// ExportName removes the crate qualifier and preserves module/type boundaries.
// Re-export aliases use their public path, independently of the callee symbol.
func ExportName(path string) (string, error) {
	if strings.HasPrefix(path, "<") {
		end := strings.LastIndex(path, ">::")
		if end < 0 {
			return "", fmt.Errorf("invalid qualified public root %q", path)
		}
		self, trait, ok := strings.Cut(path[1:end], " as ")
		method := path[end+3:]
		if !ok || strings.Contains(method, "::") {
			return "", fmt.Errorf("invalid qualified public root %q", path)
		}
		s, err := exportPath(self, true)
		if err != nil {
			return "", fmt.Errorf("public root %q: %w", path, err)
		}
		p := rootTypePath{source: trait}
		tr, err := p.parse(0)
		if err != nil || p.offset != len(trait) {
			return "", fmt.Errorf("public root %q has an unsupported trait path %q", path, trait)
		}
		m, err := exportComponent(method)
		if err != nil {
			return "", fmt.Errorf("public root %q: %w", path, err)
		}
		return s + "_As_" + tr + "_" + m, nil
	}
	return exportPath(path, true)
}

func exportPath(path string, skipCrate bool) (string, error) {
	parts := strings.Split(path, "::")
	if skipCrate && len(parts) > 1 {
		if _, err := exportComponent(parts[0]); err != nil {
			return "", err
		}
		parts = parts[1:]
	}
	for i, part := range parts {
		name, err := exportComponent(part)
		if err != nil {
			return "", fmt.Errorf("public root %q: %w", path, err)
		}
		parts[i] = name
	}
	return strings.Join(parts, "_"), nil
}

func exportComponent(part string) (string, error) {
	part = strings.TrimPrefix(part, "r#")
	var name strings.Builder
	upper := true
	for _, r := range part {
		if r == '_' {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
		}
		name.WriteRune(r)
		upper = false
	}
	result := name.String()
	if !token.IsIdentifier(result) || !ast.IsExported(result) {
		return "", fmt.Errorf("path component %q has no Go export name", part)
	}
	return result, nil
}

// Trait paths can name finite instantiations such as From<dep::Item> and
// Index<&'a str>. Reference lifetimes do not change the generated ABI name.
type rootTypePath struct {
	source string
	offset int
}

func (p *rootTypePath) parse(depth int) (string, error) {
	if depth > 32 {
		return "", fmt.Errorf("trait type nesting exceeds 32")
	}
	for p.offset < len(p.source) && unicode.IsSpace(rune(p.source[p.offset])) {
		p.offset++
	}
	if p.offset < len(p.source) && p.source[p.offset] == '&' {
		p.offset++
		for p.offset < len(p.source) && unicode.IsSpace(rune(p.source[p.offset])) {
			p.offset++
		}
		if p.offset < len(p.source) && p.source[p.offset] == '\'' {
			p.offset++
			// rustc validates the spelling; consume only the display token.
			end := strings.IndexAny(p.source[p.offset:], " \t\r\n<>,&")
			if end <= 0 {
				return "", fmt.Errorf("malformed reference lifetime")
			}
			p.offset += end
			for p.offset < len(p.source) && unicode.IsSpace(rune(p.source[p.offset])) {
				p.offset++
			}
		}
		prefix := "Ref"
		if strings.HasPrefix(p.source[p.offset:], "mut ") {
			p.offset += len("mut ")
			prefix = "MutRef"
		}
		name, err := p.parse(depth + 1)
		return prefix + "_Of_" + name + "_End", err
	}
	var parts []string
	for {
		start := p.offset
		for p.offset < len(p.source) && !strings.ContainsRune(":<>,", rune(p.source[p.offset])) {
			p.offset++
		}
		name, err := exportComponent(strings.TrimSpace(p.source[start:p.offset]))
		if err != nil {
			return "", err
		}
		if p.offset < len(p.source) && p.source[p.offset] == '<' {
			p.offset++
			var arguments []string
			for {
				arg, err := p.parse(depth + 1)
				if err != nil {
					return "", err
				}
				arguments = append(arguments, arg)
				if p.offset >= len(p.source) {
					return "", fmt.Errorf("unclosed trait arguments")
				}
				next := p.source[p.offset]
				p.offset++
				if next == '>' {
					break
				}
				if next != ',' {
					return "", fmt.Errorf("invalid trait argument separator")
				}
			}
			name += "_Of_" + strings.Join(arguments, "_And_") + "_End"
		}
		parts = append(parts, name)
		if !strings.HasPrefix(p.source[p.offset:], "::") {
			break
		}
		p.offset += 2
	}
	return strings.Join(parts, "_"), nil
}

func (g *generator) emitRoots() {
	g.f = nil
	roots := append([]Root(nil), g.p.Roots...)
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	names := make([]string, len(roots))
	used := map[string]string{}
	typeNames := map[string]bool{}
	reserved := map[string]bool{"RustExit": true}
	for _, t := range g.types {
		if t.Sized && g.scalar(t) == "" {
			typeNames[fmt.Sprintf("T%d", t.ID)] = true
		}
	}
	for i, r := range roots {
		name, err := ExportName(r.Name)
		if err != nil {
			g.fail("%s", err)
		}
		if previous, ok := used[name]; ok {
			g.fail("public roots %q and %q both export Go name %q", previous, r.Name, name)
		}
		if typeNames[name] {
			g.fail("public root %q exports %q, which is a generated Rust layout type", r.Name, name)
		}
		if reserved[name] {
			g.fail("public root %q collides with generated type API %s", r.Name, name)
		}
		used[name], names[i] = r.Name, name
	}
	for i, r := range roots {
		f := g.functions[r.Symbol]
		if f == nil {
			g.fail("missing root %s", r.Symbol)
		}
		g.f = f
		name := names[i]
		params, ret := g.publicSignature(r, f)
		declarations := []string{"ctx *oxide.Context"}
		for i, t := range params {
			n := fmt.Sprintf("a%d", i)
			declarations = append(declarations, n+" "+g.publicGoType(t, false))
		}
		if f.TrackCaller {
			declarations = append(declarations, "caller uintptr")
		}
		if f.Signature != nil && f.Signature.Variadic {
			declarations = append(declarations, "variadic ...uintptr")
		}
		g.line("// %s translates %s.", name, r.Name)
		g.line("// By-value Rust owners are consumed. Drop owned results before restoring ctx.")
		if f.TrackCaller {
			g.line("// caller points to a Rust std::panic::Location with static lifetime.")
		}
		g.line("func %s(%s) %s {", name, strings.Join(declarations, ","), g.publicGoType(ret, true))
		g.beginPublicFrame(ret)
		args := []string{"ctx"}
		for i, id := range params {
			args = append(args, g.publicArgument(fmt.Sprintf("a%d", i), id))
		}
		if f.TrackCaller {
			args = append(args, "caller")
		}
		if f.Signature != nil && f.Signature.Variadic {
			args = append(args, "variadic...")
		}
		g.publicCall(r.Symbol, ret, args)
		g.line("}")
	}
}
