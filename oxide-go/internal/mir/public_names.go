package mir

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Public names identify Rust types, not their interchangeable internal Go ABI
// representations. The generated namespaces (Rust__, Ref__, ...) are disjoint
// from ordinary exported root paths, which contain only single underscores.
func (g *generator) apiName(id int) string {
	id = g.apiIdentity(id)
	if name := g.apiNames[id]; name != "" {
		return name
	}
	g.fail("missing static Rust type name for %s", g.apiType(id).Name)
	return ""
}

func (g *generator) initAPINames(ids []int) {
	g.apiNames = make(map[int]string)
	used := make(map[string]int)
	aliases := append([]PublicType(nil), g.p.PublicTypes...)
	sort.Slice(aliases, func(i, j int) bool {
		a, b := aliases[i].Name, aliases[j].Name
		if x, y := strings.Count(a, "::"), strings.Count(b, "::"); x != y {
			return x < y
		}
		return a < b
	})
	for _, alias := range aliases {
		id := g.apiIdentity(alias.Type)
		name, err := ExportName(alias.Name)
		if err != nil {
			g.fail("public Rust type %s: %s", alias.Name, err)
		}
		if previous, ok := used[name]; ok && previous != id {
			g.fail("static Go type alias %s conflicts between %q and %q", name, g.apiType(previous).Name, g.apiType(id).Name)
		}
		used[name] = id
		if g.apiNames[id] == "" {
			g.apiNames[id] = name
		}
	}
	// Reuse public names inside compound names. This is only presentation: all
	// semantic operations still follow canonical IDs and compiler metadata.
	known := make(map[string]string)
	for id, name := range g.apiNames {
		rust := g.apiType(id).Name
		if previous := known[rust]; previous != "" && previous != name {
			g.fail("ambiguous canonical Rust type name %q", rust)
		}
		known[rust] = name
	}
	for _, id := range ids {
		id = g.apiIdentity(id)
		if g.apiNames[id] != "" {
			continue
		}
		t := g.apiType(id)
		rust := t.Name
		if rust == "" || strings.HasPrefix(rust, "type#") {
			// Primitive names do not need an ADT definition. Missing names for
			// nominal types cannot safely be replaced with traversal-dependent IDs.
			switch t.Kind {
			case "bool", "char", "usize", "isize", "i8", "i16", "i32", "i64", "i128", "u8", "u16", "u32", "u64", "u128", "f16", "f32", "f64", "f128", "str", "never":
				rust = t.Kind
			default:
				g.fail("missing canonical Rust name for public %s type", t.Kind)
			}
		}
		name, ok := staticTypeName(rust, known, 0)
		if !ok {
			// Function types, opaque compiler types and other anonymous syntax
			// still receive a deterministic name without parsing Rust semantics.
			sum := sha256.Sum256([]byte(rust))
			name = fmt.Sprintf("Anonymous__%x", sum[:8])
		}
		g.apiNames[id] = name
	}
	for _, id := range ids {
		id = g.apiIdentity(id)
		name := g.apiNames[id]
		if previous, ok := used[name]; ok && previous != id {
			g.fail("static Go type name %s conflicts between %q and %q", name, g.apiType(previous).Name, g.apiType(id).Name)
		}
		used[name] = id
	}
}

// This recognizes display-name structure solely to produce readable Go names.
// Unsupported syntax is hashed as a whole by the caller; it never determines
// layouts, trait capabilities, ownership or type equivalence.
func staticTypeName(name string, known map[string]string, depth int) (string, bool) {
	if depth > 64 {
		return "", false
	}
	name = strings.TrimSpace(name)
	if value := known[name]; value != "" {
		return value, true
	}
	parse := func(value string) (string, bool) { return staticTypeName(value, known, depth+1) }
	switch name {
	case "()":
		return "Unit", true
	case "!":
		return "Never", true
	case "true", "false":
		return "Const__" + strings.ToUpper(name[:1]) + name[1:], true
	}
	if strings.HasPrefix(name, "'") && !strings.HasSuffix(name, "'") && strings.IndexFunc(name, unicode.IsSpace) < 0 {
		return "Region", true
	}
	if strings.HasPrefix(name, "&") {
		tail := strings.TrimSpace(name[1:])
		if strings.HasPrefix(tail, "'") {
			end := strings.IndexFunc(tail, unicode.IsSpace)
			if end < 0 {
				return "", false
			}
			tail = strings.TrimSpace(tail[end:])
		}
		prefix := "Ref"
		if strings.HasPrefix(tail, "mut ") {
			prefix = "MutRef"
			tail = strings.TrimSpace(strings.TrimPrefix(tail, "mut "))
		}
		value, ok := parse(tail)
		return prefix + "__Of__" + value + "__End", ok
	}
	for _, prefix := range []struct{ rust, goName string }{{"*mut ", "MutPtr"}, {"*const ", "ConstPtr"}} {
		if strings.HasPrefix(name, prefix.rust) {
			value, ok := parse(strings.TrimPrefix(name, prefix.rust))
			return prefix.goName + "__Of__" + value + "__End", ok
		}
	}
	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		parts, ok := splitStaticType(name[1:len(name)-1], ';')
		if !ok || len(parts) > 2 {
			return "", false
		}
		element, ok := parse(parts[0])
		if !ok {
			return "", false
		}
		if len(parts) == 1 {
			return "Slice__Of__" + element + "__End", true
		}
		length := strings.TrimSpace(parts[1])
		if !staticDigits(length) {
			return "", false
		}
		return "Array__Of__" + element + "__Len__" + length + "__End", true
	}
	if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
		parts, ok := splitStaticType(name[1:len(name)-1], ',')
		if !ok {
			return "", false
		}
		if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
			parts = parts[:len(parts)-1]
		}
		values := make([]string, len(parts))
		for i, part := range parts {
			if values[i], ok = parse(part); !ok {
				return "", false
			}
		}
		return "Tuple__Of__" + strings.Join(values, "__And__") + "__End", true
	}
	if staticDigits(name) {
		return "Const__" + name, true
	}
	base := name
	var arguments []string
	if index := strings.IndexByte(name, '<'); index >= 0 {
		if !strings.HasSuffix(name, ">") {
			return "", false
		}
		base = name[:index]
		parts, ok := splitStaticType(name[index+1:len(name)-1], ',')
		if !ok {
			return "", false
		}
		for _, part := range parts {
			value, ok := parse(part)
			if !ok {
				return "", false
			}
			arguments = append(arguments, value)
		}
	}
	parts := strings.Split(base, "::")
	for i, part := range parts {
		// Whitespace in a display path denotes syntax such as dyn/unsafe/fn,
		// not an identifier. Keep it out of the readable-path subset.
		if strings.IndexFunc(part, unicode.IsSpace) >= 0 {
			return "", false
		}
		var err error
		if parts[i], err = exportComponent(part); err != nil {
			return "", false
		}
	}
	result := strings.Join(parts, "_")
	if arguments != nil {
		result += "__Of__" + strings.Join(arguments, "__And__") + "__End"
	}
	return result, true
}

func staticDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func splitStaticType(value string, separator rune) ([]string, bool) {
	var stack []rune
	start := 0
	var parts []string
	for index, char := range value {
		switch char {
		case '(', '[', '<':
			stack = append(stack, char)
		case ')', ']', '>':
			if len(stack) == 0 {
				return nil, false
			}
			open := stack[len(stack)-1]
			if (char == ')' && open != '(') || (char == ']' && open != '[') || (char == '>' && open != '<') {
				return nil, false
			}
			stack = stack[:len(stack)-1]
		default:
			if char == separator && len(stack) == 0 {
				parts = append(parts, value[start:index])
				start = index + len(string(char))
			}
		}
	}
	if len(stack) != 0 {
		return nil, false
	}
	return append(parts, value[start:]), true
}
