package mir

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Validate checks that every emitted address and direct call resolves to a
// translated object. An opaque token is not a substitute for callable code or
// a Rust static allocation.
func (p *Program) Validate() error {
	functions := make(map[string]bool, len(p.Functions))
	for _, f := range p.Functions {
		functions[f.Symbol] = true
	}
	for _, root := range p.Roots {
		if !functions[root.Symbol] {
			return fmt.Errorf("missing root function %q", root.Symbol)
		}
	}
	for _, f := range p.Functions {
		for block, call := range f.AssertCalls {
			if !functions[call.Symbol] {
				return fmt.Errorf("%s: assertion block %d refers to missing function %q", f.Name, block, call.Symbol)
			}
		}
		for block, symbol := range f.Calls {
			if symbol == "<indirect>" || strings.HasPrefix(symbol, "<intrinsic:") || strings.HasPrefix(symbol, "<virtual:") {
				continue
			}
			if !functions[symbol] {
				return fmt.Errorf("%s: block %d refers to missing function %q", f.Name, block, symbol)
			}
		}
	}
	for _, t := range p.Types {
		if t.Function != "" && !functions[t.Function] {
			return fmt.Errorf("type %d refers to missing function %q", t.ID, t.Function)
		}
	}
	type allocation struct {
		ID          uint64    `json:"id"`
		Function    string    `json:"function"`
		Alias       *uint64   `json:"alias"`
		Address     *uint64   `json:"address"`
		External    *External `json:"external"`
		Unsupported string    `json:"unsupported"`
		Memory      *struct {
			Provenance struct {
				Pointers [][]uint64 `json:"ptrs"`
			} `json:"provenance"`
		} `json:"memory"`
	}
	allocations := make(map[uint64]allocation, len(p.Allocations))
	for _, raw := range p.Allocations {
		var a allocation
		if err := json.Unmarshal(raw, &a); err != nil {
			return fmt.Errorf("allocation: %w", err)
		}
		if _, ok := allocations[a.ID]; ok {
			return fmt.Errorf("duplicate allocation %d", a.ID)
		}
		if a.External != nil && !supportedExternal(a.External) {
			return fmt.Errorf("allocation %d requires external static %q (symbol %q, linkage %s)", a.ID, a.External.Name, a.External.Symbol, a.External.Linkage)
		}
		if a.Unsupported != "" {
			return fmt.Errorf("unsupported allocation %d: %s", a.ID, a.Unsupported)
		}
		if a.Function != "" && !functions[a.Function] {
			return fmt.Errorf("allocation %d refers to missing function %q", a.ID, a.Function)
		}
		if a.Function == "" && a.Memory == nil && a.Alias == nil && a.Address == nil && a.External == nil {
			return fmt.Errorf("allocation %d has no translated value", a.ID)
		}
		allocations[a.ID] = a
	}
	for _, a := range allocations {
		if a.Memory != nil {
			for _, pair := range a.Memory.Provenance.Pointers {
				if len(pair) != 2 {
					return fmt.Errorf("allocation %d has invalid relocation", a.ID)
				}
				if _, ok := allocations[pair[1]]; !ok {
					return fmt.Errorf("allocation %d refers to missing allocation %d", a.ID, pair[1])
				}
			}
		}
		seen := map[uint64]bool{}
		for a.Alias != nil {
			if seen[a.ID] {
				return fmt.Errorf("allocation alias cycle at %d", a.ID)
			}
			seen[a.ID] = true
			target, ok := allocations[*a.Alias]
			if !ok {
				return fmt.Errorf("allocation %d aliases missing allocation %d", a.ID, *a.Alias)
			}
			a = target
		}
	}
	return nil
}
