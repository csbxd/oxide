package oxide

import "testing"

func TestExceptionStateNoGoAllocs(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := c.Alloc(64, 16)
	requireNoGoAllocations(t, 100, func() {
		c.RaiseException(p)
		if !c.Failed() || c.TakeException() != p || c.Failed() {
			panic("exception state")
		}
	})
}
