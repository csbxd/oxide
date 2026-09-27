package mir

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// This diagnostic does not claim that partial output is runnable. It collects
// backend errors across a large exported dependency graph so missing runtime
// symbols do not conceal independent lowering work. Generate still validates
// the complete program and stops before publishing any unsupported output.
func TestLoweringDiagnostics(t *testing.T) {
	path := os.Getenv("OXIDE_MIR_DIAGNOSTICS")
	if path == "" {
		t.Skip("set OXIDE_MIR_DIAGNOSTICS to an exported MIR file")
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g, err := newGenerator(p)
	if err != nil {
		t.Fatal(err)
	}
	type failure struct {
		Name   string `json:"function"`
		Symbol string `json:"symbol"`
		Reason string `json:"reason"`
	}
	failures := []failure{}
	counts := map[string]int{}
	for i := range p.Functions {
		f := &p.Functions[i]
		func() {
			defer func() {
				if e := recover(); e != nil {
					reason := strings.TrimPrefix(fmt.Sprint(e), f.Name+": ")
					failures = append(failures, failure{f.Name, f.Symbol, reason})
					counts[reason]++
				}
			}()
			g.function(f)
		}()
		g.b.Reset()
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	t.Logf("%d functions, %d lowering failures, %d distinct reasons", len(p.Functions), len(failures), len(keys))
	for _, key := range keys[:min(len(keys), 80)] {
		t.Logf("%d: %s", counts[key], key)
	}
	if path := os.Getenv("OXIDE_DIAGNOSTICS_OUT"); path != "" {
		data, err := json.MarshalIndent(failures, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
