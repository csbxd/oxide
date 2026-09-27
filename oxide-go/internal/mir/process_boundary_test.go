package mir

import (
	"strings"
	"testing"
)

func TestProcessBoundaryRequiresCompilerMarker(t *testing.T) {
	p := rootTestProgram()
	p.Functions = p.Functions[:1]
	p.Functions[0].Name = "std::sys::args::unix::imp::argc_argv"
	p.Roots = []Root{{Name: "user::fake", Symbol: p.Functions[0].Symbol}}
	source, err := Generate(p, "fixture")
	if err != nil || strings.Contains(string(source), "oxide.ProcessArguments()") {
		t.Fatalf("user function mistaken for startup boundary: %v", err)
	}
	p.Functions[0].RuntimeBoundary = "unknown"
	if _, err := Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), "unknown runtime boundary") {
		t.Fatalf("unknown boundary accepted: %v", err)
	}
	p.Functions[0].RuntimeBoundary = "std_args"
	if _, err := Generate(p, "fixture"); err == nil || !strings.Contains(err.Error(), "argc/argv return layout") {
		t.Fatalf("invalid boundary signature accepted: %v", err)
	}
}
