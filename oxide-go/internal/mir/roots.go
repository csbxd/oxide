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

// Trait paths can name finite instantiations such as From<dep::Item>. This is
// deliberately a named-type grammar, not a parser for arbitrary Rust syntax.
type rootTypePath struct {
	source string
	offset int
}

func (p *rootTypePath) parse(depth int) (string, error) {
	if depth > 32 {
		return "", fmt.Errorf("trait type nesting exceeds 32")
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
		used[name], names[i] = r.Name, name
	}
	for i, r := range roots {
		f := g.functions[r.Symbol]
		if f == nil {
			g.fail("missing root %s", r.Symbol)
		}
		g.f = f
		name := names[i]
		params, ret := g.signature(f)
		args := []string{"ctx"}
		declarations := []string{"ctx *oxide.Context"}
		if g.indirectValue(ret) {
			declarations = append(declarations, "result uintptr")
			args = append(args, "result")
		}
		for i, t := range params {
			n := fmt.Sprintf("a%d", i)
			declarations = append(declarations, n+" "+g.argumentType(t))
			args = append(args, n)
		}
		if f.TrackCaller {
			declarations = append(declarations, "caller uintptr")
			args = append(args, "caller")
		}
		if f.Signature != nil && f.Signature.Variadic {
			declarations = append(declarations, "variadic ...uintptr")
			args = append(args, "variadic...")
		}
		g.line("// %s translates %s.", name, r.Name)
		if g.indirectValue(ret) {
			g.line("// result points to %d writable bytes with Rust alignment %d.", g.typ(ret).Size, g.typ(ret).Align)
		}
		for i, t := range params {
			if g.indirectValue(t) {
				g.line("// a%d points to %d readable bytes, copied into the callee's Rust frame.", i, g.typ(t).Size)
			}
		}
		if f.TrackCaller {
			g.line("// caller points to a Rust std::panic::Location with static lifetime.")
		}
		g.line("func %s(%s) %s {", name, strings.Join(declarations, ","), g.returnType(ret))
		if g.indirectValue(ret) {
			g.line("%s(%s)", g.names[r.Symbol], strings.Join(args, ","))
			g.line("if ctx.Failed() { panic(ctx.TakePanic()) }; return }")
		} else {
			g.line("r := %s(%s)", g.names[r.Symbol], strings.Join(args, ","))
			g.line("if ctx.Failed() { panic(ctx.TakePanic()) }; return r }")
		}
	}
}
