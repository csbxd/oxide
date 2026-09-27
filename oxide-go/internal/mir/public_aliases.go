package mir

import (
	"fmt"
	"sort"
	"strings"
)

// Only compiler-confirmed Rust aliases share Go identity. Equal layout alone
// does not create a Go alias or share an owning handle.
func (g *generator) emitStaticAliases() {
	aliases := append([]PublicType(nil), g.p.PublicTypes...)
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].Name < aliases[j].Name })
	seen := make(map[string]int)
	for _, p := range aliases {
		name, err := ExportName(p.Name)
		if err != nil {
			g.fail("public type alias: %s", err)
		}
		id := g.apiIdentity(p.Type)
		canonical := g.apiName(id)
		if name == canonical {
			continue
		}
		if previous, ok := seen[name]; ok {
			if previous != id {
				g.fail("ambiguous public alias %s", name)
			}
			continue
		}
		seen[name] = id
		t := g.apiType(id)
		prefixes := []string{"Ref", "Mut"}
		if t.Sized {
			prefixes = append(prefixes, "Rust", "Value", "Storage")
		}
		if t.AdtKind == "Enum" {
			prefixes = append(prefixes, "Tag")
			for _, variant := range t.VariantNames {
				v := apiMemberName(variant)
				g.line("const Variant__%s__%s=Variant__%s__%s", name, v, canonical, v)
			}
		}
		for _, prefix := range prefixes {
			g.line("type %s__%s = %s__%s", prefix, name, prefix, canonical)
		}
		for _, prefix := range []string{"RustName", "RustAlign"} {
			g.line("const %s__%s = %s__%s", prefix, name, prefix, canonical)
		}
		if !t.Sized {
			continue
		}
		g.line("const RustSize__%s = RustSize__%s", name, canonical)
		g.line("func New__%s(ctx *oxide.Context) Value__%s {return New__%s(ctx)}", name, name, canonical)
		g.line("func Alloc__%s() Storage__%s {return Alloc__%s()}", name, name, canonical)
		if t.DefaultSymbol != "" {
			_, ret := g.signature(g.functions[t.DefaultSymbol])
			result := g.publicGoType(ret, true)
			call := "Default__" + canonical + "(ctx)"
			if result != "" {
				call = "return " + call
			}
			g.line("func Default__%s(ctx *oxide.Context) %s {%s}", name, result, call)
		}
		if t.AdtKind != "Enum" {
			continue
		}
		for i, variant := range t.VariantNames {
			if !g.apiVariantInhabited(t, i) || i < len(t.VariantsInfo) && t.VariantsInfo[i].NonExhaustive {
				continue
			}
			declarations := []string{"ctx *oxide.Context"}
			args := []string{"ctx"}
			accessible := true
			if i < len(t.Members) {
				for n, f := range t.Members[i] {
					if !f.Public || !g.apiType(f.Type).Sized {
						accessible = false
						break
					}
					arg := fmt.Sprintf("a%d", n)
					declarations = append(declarations, arg+" "+g.publicGoType(f.Type, false))
					args = append(args, arg)
				}
			}
			if accessible {
				v := apiMemberName(variant)
				g.line("func New__%s__%s(%s) Value__%s {return New__%s__%s(%s)}", name, v, strings.Join(declarations, ","), name, canonical, v, strings.Join(args, ","))
			}
		}
	}
}
