package mir

import (
	"fmt"
	"sort"
	"strings"
)

type cFunction struct {
	helper string
	params string
	result string
}

// The Rust declarations retain their own layouts. Conversions at this table
// are limited to scalar C ABI values; aggregate ABIs need explicit lowering.
var cFunctions = map[string]cFunction{
	"__errno_location": {"LibcErrnoLocation", "", "uintptr"},
	"strlen":           {"LibcStrlen", "uintptr", "uintptr"},
	"read":             {"LibcRead", "int32 uintptr uintptr", "int64"},
	"write":            {"LibcWrite", "int32 uintptr uintptr", "int64"},
	"readv":            {"LibcReadv", "int32 uintptr int32", "int64"},
	"writev":           {"LibcWritev", "int32 uintptr int32", "int64"},
	"pread64":          {"LibcPread", "int32 uintptr uintptr int64", "int64"},
	"pwrite64":         {"LibcPwrite", "int32 uintptr uintptr int64", "int64"},
	"close":            {"LibcClose", "int32", "int32"},
	"lseek64":          {"LibcLseek", "int32 int64 int32", "int64"},
	"lseek":            {"LibcLseek", "int32 int64 int32", "int64"},
	"mkstemp":          {"LibcMkstemp", "uintptr", "int32"},
	"poll":             {"LibcPoll", "uintptr uint64 int32", "int32"},
	"isatty":           {"LibcIsatty", "int32", "int32"},
	"fsync":            {"LibcFsync", "int32", "int32"},
	"ftruncate64":      {"LibcFtruncate", "int32 int64", "int32"},
	"mkdir":            {"LibcMkdir", "uintptr uint32", "int32"},
	"unlink":           {"LibcUnlink", "uintptr", "int32"},
	"rename":           {"LibcRename", "uintptr uintptr", "int32"},
	"readlink":         {"LibcReadlink", "uintptr uintptr uintptr", "int64"},
	"realpath":         {"LibcRealpath", "uintptr uintptr", "uintptr"},
	"getcwd":           {"LibcGetcwd", "uintptr uintptr", "uintptr"},
	"getenv":           {"LibcGetenv", "uintptr", "uintptr"},
	"opendir":          {"LibcOpendir", "uintptr", "uintptr"},
	"closedir":         {"LibcClosedir", "uintptr", "int32"},
	"dirfd":            {"LibcDirfd", "uintptr", "int32"},
	"stat64":           {"LibcStat", "uintptr uintptr", "int32"},
	"fstat64":          {"LibcFstat", "int32 uintptr", "int32"},
	"fstatat64":        {"LibcFstatat", "int32 uintptr uintptr int32", "int32"},
	"readdir64":        {"LibcReaddir", "uintptr", "uintptr"},
	"mmap64":           {"LibcMmap", "uintptr uintptr int32 int32 int32 int64", "uintptr"},
	"munmap":           {"LibcMunmap", "uintptr uintptr", "int32"},
	"mprotect":         {"LibcMprotect", "uintptr uintptr int32", "int32"},
	"getrandom":        {"LibcGetrandom", "uintptr uintptr uint32", "int64"},
	"gettid":           {"LibcGettid", "", "int32"},
	"getauxval":        {"LibcGetauxval", "uint64", "uint64"},
	"posix_memalign":   {"LibcPosixMemalign", "uintptr uintptr uintptr", "int32"},
	"sysconf":          {"LibcSysconf", "int32", "int64"},
	"clock_gettime":    {"LibcClockGettime", "int32 uintptr", "int32"},
	"statx":            {"LibcStatx", "int32 uintptr int32 uint32 uintptr", "int32"},
	"malloc":           {"LibcMalloc", "uintptr", "uintptr"},
	"calloc":           {"LibcCalloc", "uintptr uintptr", "uintptr"},
	"realloc":          {"LibcRealloc", "uintptr uintptr", "uintptr"},
	"free":             {"LibcFree", "uintptr", ""},
	"memcpy":           {"LibcMemcpy", "uintptr uintptr uintptr", "uintptr"},
	"memmove":          {"LibcMemmove", "uintptr uintptr uintptr", "uintptr"},
	"memset":           {"LibcMemset", "uintptr int32 uintptr", "uintptr"},
	"memcmp":           {"LibcMemcmp", "uintptr uintptr uintptr", "int32"},
	"__xpg_strerror_r": {"LibcStrerrorR", "int32 uintptr uintptr", "int32"},
	"pause":            {"LibcPause", "", "int32"},
}

func supportedExternal(e *External) bool {
	if !e.Weak || e.Linkage != "ExternalWeak" {
		return false
	}
	if e.Symbol == "__dso_handle" {
		return e.Kind == "static"
	}
	if e.Kind != "function" {
		return false
	}
	if e.Symbol == "__cxa_thread_atexit_impl" {
		return true
	}
	_, ok := cFunctions[e.Symbol]
	return ok
}

func (g *generator) externalFunctions() {
	ids := make([]uint64, 0, len(g.externalAllocs))
	for id := range g.externalAllocs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		e := g.externalAllocs[id]
		if !supportedExternal(e) {
			g.fail("unsupported external allocation %d: %s", id, e.Symbol)
		}
		if e.Symbol == "__dso_handle" {
			if t := g.typ(e.Type); t.Kind != "pointer" || t.Size != 8 || t.Align != 8 {
				g.fail("DSO handle slot layout")
			}
			continue
		}
		t, slot := g.typ(e.FunctionType), g.typ(e.Type)
		if t.Kind != "fnptr" || t.FnABI != "C" || t.FnVariadic || t.Size != 8 || slot.Size != 8 || slot.Align != 8 {
			g.fail("external function slot ABI %s", e.Symbol)
		}
		decls := []string{"ctx *oxide.Context"}
		for i, p := range t.FnInputs {
			decls = append(decls, fmt.Sprintf("v%d %s", i+1, g.goType(p)))
		}
		g.line("func oxideWeak%d(%s) %s {", id, strings.Join(decls, ","), g.goType(t.FnOutput))
		if e.Symbol == "__cxa_thread_atexit_impl" {
			g.cxaThreadAtExit(t.FnInputs, t.FnOutput)
		} else {
			g.cFunctionBody(e.Symbol, cFunctions[e.Symbol], t.FnInputs, t.FnOutput)
		}
		g.line("}")
	}
}

func (g *generator) cScalar(id int, expected string) bool {
	t := g.typ(id)
	if expected == "" {
		return t.Size == 0
	}
	actual := g.scalar(t)
	if actual == expected {
		return true
	}
	// Rust isize and C long are both signed 64-bit integers here. size_t,
	// usize and pointers have the corresponding unsigned representation.
	return t.Size == 8 && (actual == "uintptr" || actual == "uint64") && (expected == "uintptr" || expected == "uint64") ||
		t.Size == 8 && (actual == "int" || actual == "int64") && expected == "int64"
}

func (g *generator) cFunctionBody(symbol string, spec cFunction, params []int, ret int) {
	want := strings.Fields(spec.params)
	if len(params) != len(want) || !g.cScalar(ret, spec.result) {
		g.fail("C function signature %s", symbol)
	}
	args := []string{"ctx"}
	for i, typ := range params {
		if !g.cScalar(typ, want[i]) {
			g.fail("C function %s argument %d ABI", symbol, i)
		}
		args = append(args, fmt.Sprintf("%s(v%d)", want[i], i+1))
	}
	call := "oxide." + spec.helper + "(" + strings.Join(args, ",") + ")"
	if spec.result == "" {
		g.line("%s; return %s", call, g.zero(ret))
	} else {
		g.line("return %s(%s)", g.goType(ret), call)
	}
}

func (g *generator) cRuntimeFunction(f *Function, params []int, ret int) bool {
	if f.Body != nil || f.Signature == nil || f.Signature.ABI != "C" {
		return false
	}
	if g.pthreadFunction(f, params, ret) {
		return true
	}
	if g.unwindFunction(f, params, ret) {
		return true
	}
	if spec, ok := cFunctions[f.Symbol]; ok && !f.Signature.Variadic {
		g.cFunctionBody(f.Symbol, spec, params, ret)
		g.line("}")
		return true
	}
	if f.Symbol == "abort" && len(params) == 0 && !f.Signature.Variadic {
		g.line("oxide.Abort(); return %s }", g.zero(ret))
		return true
	}
	if g.cVariadicFunction(f, params, ret) {
		return true
	}
	mathFuncs := map[string]string{"acos": "Acos", "acosf": "Acos", "atan2": "Atan2", "atan2f": "Atan2", "hypot": "Hypot", "hypotf": "Hypot", "tan": "Tan", "tanf": "Tan"}
	if fn := mathFuncs[f.Symbol]; fn != "" && !f.Signature.Variadic {
		n := 1
		if fn == "Atan2" || fn == "Hypot" {
			n = 2
		}
		kind := "f64"
		if strings.HasSuffix(f.Symbol, "f") {
			kind = "f32"
		}
		if len(params) != n || g.typ(ret).Kind != kind {
			g.fail("C math signature %s", f.Symbol)
		}
		args := []string{}
		for i, typ := range params {
			if g.typ(typ).Kind != kind {
				g.fail("C math argument %s", f.Symbol)
			}
			args = append(args, fmt.Sprintf("float64(v%d)", i+1))
		}
		g.line("return %s(math.%s(%s)) }", g.goType(ret), fn, strings.Join(args, ","))
		return true
	}
	return false
}

func (g *generator) cVariadicFunction(f *Function, params []int, ret int) bool {
	if !f.Signature.Variadic {
		return false
	}
	spec, ok := map[string]cFunction{
		"open":     {"LibcOpen", "uintptr int32", "int32"},
		"open64":   {"LibcOpen", "uintptr int32", "int32"},
		"openat":   {"LibcOpenat", "int32 uintptr int32", "int32"},
		"openat64": {"LibcOpenat", "int32 uintptr int32", "int32"},
		"fcntl":    {"LibcFcntl", "int32 int32", "int32"},
		"syscall":  {"LibcSyscall", "int64", "int64"},
	}[f.Symbol]
	if !ok {
		return false
	}
	want := strings.Fields(spec.params)
	if len(params) != len(want) || !g.cScalar(ret, spec.result) || f.Signature.FixedCount != len(params) {
		g.fail("variadic C signature %s", f.Symbol)
	}
	args := []string{"ctx"}
	for i, typ := range params {
		if !g.cScalar(typ, want[i]) {
			g.fail("variadic C argument %s", f.Symbol)
		}
		args = append(args, fmt.Sprintf("%s(v%d)", want[i], i+1))
	}
	if f.Symbol == "syscall" {
		args = append(args, "variadic...")
	} else {
		g.line("if len(variadic)>1 { panic(\"invalid C variadic argument count\") }; var argument uintptr; if len(variadic)==1 { argument=variadic[0] }")
		arg := "uint32(argument)"
		if f.Symbol == "fcntl" {
			arg = "argument"
		}
		args = append(args, arg)
	}
	g.line("return %s(oxide.%s(%s)) }", g.goType(ret), spec.helper, strings.Join(args, ","))
	return true
}
