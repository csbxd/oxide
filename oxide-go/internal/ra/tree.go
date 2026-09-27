package ra

// Node is the lossless syntax tree emitted by rust-analyzer parse --json.
// Positions are UTF-8 byte offsets, line and column triples.  The byte offset
// is the only position used by the translator; keeping the complete position
// makes diagnostics useful to callers and leaves room for source mapping.
type Node struct {
	Kind     string  `json:"kind"`
	Type     string  `json:"type"`
	Start    []int   `json:"start"`
	End      []int   `json:"end"`
	Text     string  `json:"text"`
	Children []*Node `json:"children"`
}

func (n *Node) Offset() int {
	if n == nil || len(n.Start) == 0 {
		return 0
	}
	return n.Start[0]
}

func (n *Node) EndOffset() int {
	if n == nil || len(n.End) == 0 {
		return 0
	}
	return n.End[0]
}

func (n *Node) Span(src string) string {
	if n == nil {
		return ""
	}
	a, b := n.Offset(), n.EndOffset()
	if a < 0 || b < a || b > len(src) {
		return ""
	}
	return src[a:b]
}

func (n *Node) ChildrenOf(kind string) []*Node {
	var out []*Node
	if n == nil {
		return out
	}
	for _, child := range n.Children {
		if child.Kind == kind {
			out = append(out, child)
		}
	}
	return out
}

func (n *Node) First(kind string) *Node {
	if n == nil {
		return nil
	}
	if n.Kind == kind {
		return n
	}
	for _, child := range n.Children {
		if found := child.First(kind); found != nil {
			return found
		}
	}
	return nil
}

func (n *Node) Walk(fn func(*Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, child := range n.Children {
		child.Walk(fn)
	}
}
