//go:build linux && (amd64 || arm64)

package oxide

import (
	"bytes"
	"runtime"
	"testing"
)

func TestInteropSpanCopiesStableStorage(t *testing.T) {
	c := NewContext()
	defer c.Close()
	input := []byte{0xff, 0, 1, 2}
	span := c.CopyBytes(input)
	input[2] = 9
	if !bytes.Equal(span.Bytes(), []byte{0xff, 0, 1, 2}) {
		t.Fatal("borrowed Go data instead of copying it")
	}
	span.Bytes()[3] = 7
	if input[3] != 2 {
		t.Fatal("Rust view aliases Go input")
	}
	text := c.CopyString("oxide 雪\x00")
	interopMoveStack(100)
	runtime.GC()
	if text.String() != "oxide 雪\x00" || text.Len != uintptr(len("oxide 雪\x00")) {
		t.Fatal("copied str changed after Go stack growth/GC")
	}
	for _, empty := range []Span{c.CopyBytes(nil), c.CopyString("")} {
		if empty.Data == 0 || empty.Len != 0 || len(empty.Bytes()) != 0 || empty.String() != "" {
			t.Fatal("empty Rust borrow must have a non-null pointer")
		}
	}
	// Non-null dangling pointers are valid for empty Rust containers, but
	// must not become low Go pointer values scanned during stack growth.
	empty := Span{Data: 1}
	if empty.Bytes() != nil || empty.String() != "" {
		t.Fatal("empty Rust dangling address escaped into a Go pointer")
	}
}

//go:noinline
func interopMoveStack(depth int) byte {
	var data [2048]byte
	data[depth%len(data)] = byte(depth)
	if depth > 0 {
		data[0] = interopMoveStack(depth - 1)
	}
	return data[depth%len(data)]
}

func TestInteropSpanRejectsInvalidInput(t *testing.T) {
	c := NewContext()
	defer c.Close()
	mark := c.Mark()
	for _, call := range []func(){
		func() { c.CopyString("bad\xff") },
		func() { _ = (Span{Len: 1}).Bytes() },
		func() { _ = (Span{Data: 1, Len: ^uintptr(0)}).String() },
		func() { _ = (Span{Data: ^uintptr(0) - 1, Len: 3}).Bytes() },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid span/string accepted")
				}
			}()
			call()
		}()
	}
	if c.Mark() != mark {
		t.Fatal("invalid UTF-8 changed the Context frame")
	}
}

func TestInteropSpanNoGoAllocations(t *testing.T) {
	c := NewContext()
	defer c.Close()
	input := []byte{0xff, 0, 1, 2}
	mark := c.Mark()
	requireNoGoAllocations(t, 100, func() {
		defer c.Restore(mark)
		span := c.CopyBytes(input)
		if !bytes.Equal(span.Bytes(), input) {
			t.Fatal("copied bytes changed")
		}
		text := c.CopyString("oxide 雪")
		if text.String() != "oxide 雪" {
			t.Fatal("copied str changed")
		}
	})
	if c.Mark() != mark {
		t.Fatal("borrowed input frame leaked")
	}
}
