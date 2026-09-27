package ra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Parser invokes rust-analyzer's stable command-line parser.  Keeping parsing
// in rust-analyzer gives oxide the same tokenization and recovery behaviour as
// the Rust ecosystem without embedding a second Rust parser in Go.
type Parser struct {
	Binary string
}

func (p Parser) Parse(src []byte) (*Node, error) {
	bin := p.Binary
	if bin == "" {
		bin = "rust-analyzer"
	}
	cmd := exec.Command(bin, "parse", "--json")
	cmd.Stdin = bytes.NewReader(src)
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rust-analyzer parse: %w: %s", err, bytes.TrimSpace(exit.Stderr))
		}
		return nil, fmt.Errorf("rust-analyzer parse: %w", err)
	}
	var root Node
	if err := json.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("decode rust-analyzer parse output: %w", err)
	}
	return &root, nil
}
