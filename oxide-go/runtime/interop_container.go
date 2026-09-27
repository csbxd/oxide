//go:build linux && (amd64 || arm64)

package oxide

import "strings"

// SliceParts is a logical typed-slice range. Len counts elements, not bytes;
// generated call adapters combine it with the compiler's Rust fat-pointer ABI.
type SliceParts struct {
	Data, Len uintptr
}

func (v Value) Elements(element *Type) SliceParts {
	data, actual := v.elements()
	if element == nil || actual != element {
		panic("oxide: Rust slice element type mismatch")
	}
	return SliceParts{Data: data, Len: v.Len()}
}

func (v Value) Len() uintptr {
	v.check()
	if v.byteDST() {
		return v.Meta
	}
	switch v.Type.Kind {
	case "array":
		return v.Type.Length
	case "slice", "str":
		return v.Meta
	}
	if c := v.Type.Container; c != nil {
		switch c.Kind {
		case "string", "vec", "str_ref", "slice_ref", "os_string", "path_buf", "os_str_ref", "path_ref":
			return v.word(c.LenOffset)
		}
	}
	panic("oxide: Rust value has no length")
}

func (v Value) Cap() uintptr {
	v.check()
	c := v.Type.Container
	if c == nil || (c.Kind != "string" && c.Kind != "vec" && c.Kind != "os_string" && c.Kind != "path_buf") || c.Element == nil {
		panic("oxide: Rust value has no capacity")
	}
	if c.Element.Size == 0 {
		return ^uintptr(0)
	}
	return v.word(c.CapacityOffset)
}

func (v Value) elements() (uintptr, *Type) {
	v.check()
	switch v.Type.Kind {
	case "array", "slice":
		if v.Type.Elem != nil && v.Type.Elem.Sized {
			return v.Addr, v.Type.Elem
		}
	}
	if c := v.Type.Container; c != nil && (c.Kind == "vec" || c.Kind == "slice_ref") && c.Element != nil && c.Element.Sized {
		return v.word(c.DataOffset), c.Element
	}
	panic("oxide: Rust value has no indexable elements")
}

func (v Value) Index(index uintptr) Value {
	data, element := v.elements()
	if index >= v.Len() {
		panic("oxide: Rust index out of bounds")
	}
	return Value{Addr: addAddress(data, arrayBytes(index, element.Size)), Type: element}
}

// InitAt consumes an element into reserved Vec storage outside its current
// length. It does not publish the element: call SetLen after initialization.
func (v Value) InitAt(index uintptr, value Value) {
	v.check()
	if c := v.Type.Container; c == nil || c.Kind != "vec" || c.Element != value.Type {
		panic("oxide: Rust Vec element initialization type mismatch")
	}
	if index < v.Len() || index >= v.Cap() {
		panic("oxide: Rust Vec initialization is outside spare capacity")
	}
	data, element := v.elements()
	Value{Addr: addAddress(data, arrayBytes(index, element.Size)), Type: element}.Init(value)
}

// SetLen changes a Vec's initialized element count. The caller must have
// initialized new elements or consumed/dropped removed ones beforehand.
func (v Value) SetLen(length uintptr) {
	v.check()
	c := v.Type.Container
	if c == nil || c.Kind != "vec" || length > v.Cap() {
		panic("oxide: invalid Rust Vec length")
	}
	v.putWord(c.LenOffset, length)
}

func (v Value) byteSpanCapacity() Span {
	c := v.Type.Container
	if c == nil || c.Element == nil || c.Element.Kind != "u8" || c.Element.Size != 1 {
		panic("oxide: Rust storage is not a byte buffer")
	}
	return Span{Data: v.word(c.DataOffset), Len: v.Cap()}
}

func (v Value) byteSpan() Span {
	v.check()
	if v.Type.Kind == "str" || v.byteDST() {
		return Span{v.Addr, v.Meta}
	}
	if c := v.Type.Container; c != nil && (c.Kind == "string" || c.Kind == "str_ref" || c.Kind == "os_string" || c.Kind == "path_buf" || c.Kind == "os_str_ref" || c.Kind == "path_ref") {
		return Span{v.word(c.DataOffset), v.Len()}
	}
	data, element := v.elements()
	if element.Kind != "u8" || element.Size != 1 {
		panic("oxide: Rust sequence does not contain u8")
	}
	return Span{data, v.Len()}
}

func (v Value) byteDST() bool {
	c := v.Type.Container
	return c != nil && (c.Kind == "path" || c.Kind == "os_str")
}

// Bytes borrows str/String or u8 sequence storage. Respect the Rust borrow and
// mutability rules; modifying string bytes must preserve UTF-8 validity.
func (v Value) Bytes() []byte { return v.byteSpan().Bytes() }

// Span borrows the same byte range as Bytes, preserving Rust's non-null empty
// pointer for generated Rust call adapters without a Go pointer conversion.
func (v Value) Span() Span { return v.byteSpan() }

// String borrows valid str/String bytes. Do not mutate or drop their Rust
// owner while the Go string is in use. StringCopy creates independent Go data.
func (v Value) String() string {
	v.check()
	c := v.Type.Container
	if v.Type.Kind != "str" && (c == nil || (c.Kind != "string" && c.Kind != "str_ref")) {
		panic("oxide: Rust value is not str or String")
	}
	return v.byteSpan().String()
}

func (v Value) StringCopy() string { return strings.Clone(v.String()) }
