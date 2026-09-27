package mir

// These markers are attached by oxide-rs to verified sysroot definitions.
// User functions with similar names retain their ordinary MIR bodies.
func (g *generator) processStartupFunction(f *Function, params []int, ret int) bool {
	if f.RuntimeBoundary == "" {
		return false
	}
	if f.RuntimeBoundary != "std_args" {
		g.fail("unknown runtime boundary %q", f.RuntimeBoundary)
	}
	r := g.typ(ret)
	if f.Body == nil || len(params) != 0 || r.Kind != "aggregate" || r.Size != 16 || len(r.Fields) != 2 || len(r.FieldTypes) != 2 {
		g.fail("std argc/argv return layout")
	}
	argc, argv := g.typ(r.FieldTypes[0]), g.typ(r.FieldTypes[1])
	if argc.Kind != "isize" || argc.Size != 8 || argv.Kind != "pointer" || argv.Size != 8 || g.typ(argv.Pointee).Kind != "pointer" {
		g.fail("std argc/argv field types")
	}
	g.line("{ argc,argv:=oxide.ProcessArguments(); var result %s; *(*int64)(unsafe.Add(unsafe.Pointer(&result),%d))=argc; *(*uintptr)(unsafe.Add(unsafe.Pointer(&result),%d))=argv; return result }", g.goType(ret), r.Fields[0], r.Fields[1])
	g.line("}")
	return true
}
