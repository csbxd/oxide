package mir

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateReferences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alloc string
		want  string
	}{
		{"unknown function", `{"id":0,"function":"absent"}`, "missing function"},
		{"external static", `{"id":0,"external":{"name":"foreign_static","symbol":"foreign_static"}}`, "external static"},
		{"unknown weak symbol", `{"id":0,"external":{"name":"weak","symbol":"unknown_weak","kind":"function","weak":true,"linkage":"ExternalWeak"}}`, "external static"},
		{"unsupported", `{"id":0,"unsupported":"TypeId"}`, "unsupported allocation"},
		{"missing value", `{"id":0}`, "no translated value"},
		{"unknown relocation", `{"id":0,"memory":{"provenance":{"ptrs":[[0,1]]}}}`, "missing allocation"},
		{"unknown alias", `{"id":0,"alias":1}`, "aliases missing allocation"},
		{"cyclic alias", `{"id":0,"alias":0}`, "alias cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Program{Allocations: []json.RawMessage{json.RawMessage(tc.alloc)}}
			if err := p.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v; want %q", err, tc.want)
			}
		})
	}
	p := &Program{
		Functions: []Function{{Symbol: "present", Name: "present", Calls: map[int]string{0: "<indirect>", 1: "<virtual:3>", 2: "<intrinsic:assume>"}}},
		Roots:     []Root{{Symbol: "present"}},
		Types:     []Type{{ID: 1, Function: "present"}},
		Allocations: []json.RawMessage{
			json.RawMessage(`{"id":0,"function":"present"}`),
			json.RawMessage(`{"id":1,"alias":0}`),
			json.RawMessage(`{"id":2,"memory":{"provenance":{"ptrs":[[0,1]]}}}`),
			json.RawMessage(`{"id":3,"address":0}`),
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Functions[0].Calls[3] = "absent"
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "missing function") {
		t.Fatalf("missing direct call: %v", err)
	}
}
