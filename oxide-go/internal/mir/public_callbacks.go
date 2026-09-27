package mir

import "strings"

// Callback aliases describe the translated function-pointer calling convention,
// not the owning public API. They permit external Go callbacks to name exact
// ABI types without depending on rustc traversal IDs or copying Rust owners.
func (g *generator) emitStaticCallbacks(ids []int) {
	seen := make(map[int]bool)
	for _, id := range ids {
		t := g.apiType(id)
		if t.Kind != "fnptr" {
			continue
		}
		for _, value := range append(append([]int(nil), g.callbackInputs(t)...), t.FnOutput) {
			value = g.apiIdentity(value)
			if seen[value] {
				continue
			}
			seen[value] = true
			g.line("// ABI__%s is a low-level translated callback ABI alias; it does not grant ownership or extend a borrow.", g.apiName(value))
			g.line("type ABI__%s = %s", g.apiName(value), g.argumentType(value))
		}
		g.emitStaticCallback(id)
	}
}

func (g *generator) emitStaticCallback(id int) {
	t := g.apiType(id)
	if t.Kind != "fnptr" {
		return
	}
	args := []string{"*oxide.Context"}
	ret := "ABI__" + g.apiName(t.FnOutput)
	if g.indirectValue(t.FnOutput) {
		args = append(args, ret) // Caller-provided result storage precedes inputs.
		ret = ""
	}
	for _, param := range g.callbackInputs(t) {
		args = append(args, "ABI__"+g.apiName(param))
	}
	if t.FnVariadic {
		if t.FnFixedCount != len(t.FnInputs) {
			g.fail("invalid callback variadic signature for %s", t.Name)
		}
		args = append(args, "...uintptr")
	}
	g.line("// Callback__%s has the exact translated Go function-pointer ABI for %s.", g.apiName(id), t.Name)
	g.line("// Register with oxide.FunctionPointer; reference/owner lifetimes and unwind rules remain the caller's responsibility.")
	g.line("type Callback__%s = func(%s) %s", g.apiName(id), strings.Join(args, ","), ret)
}

func (g *generator) callbackInputs(t *Type) []int {
	if t.FnSpreadArg == nil {
		g.fail("missing compiler function-pointer spread ABI for %s", t.Name)
	}
	index := *t.FnSpreadArg
	if index == -1 {
		return t.FnInputs
	}
	if index < 0 || index != len(t.FnInputs)-1 || t.FnVariadic {
		g.fail("invalid function-pointer spread ABI for %s", t.Name)
	}
	tuple := g.apiType(t.FnInputs[index])
	if tuple.Kind != "aggregate" || tuple.AdtKind != "" || len(tuple.Fields) != len(tuple.FieldTypes) {
		g.fail("invalid function-pointer tuple layout for %s", t.Name)
	}
	inputs := append([]int(nil), t.FnInputs[:index]...)
	return append(inputs, tuple.FieldTypes...)
}
