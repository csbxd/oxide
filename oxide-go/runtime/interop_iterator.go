//go:build linux && (amd64 || arm64)

package oxide

// Iterator owns a compiler-described Rust iterator state. Next returns Rust's
// actual Option<Item> in a new Context slot. Consume/drop each result before
// restoring that slot, and drop the iterator before restoring its state.
// Borrowing an iterator does not extend the collection's lifetime.
type Iterator struct {
	State    Value
	NextFunc func(*Context, Value) Value
}

func (v Value) Iterator(ctx *Context) Iterator {
	v.check()
	if v.Type.Iterate == nil {
		panic("oxide: Rust iterator is not exported for this type")
	}
	return v.Type.Iterate(ctx, v)
}

// IteratorMut requires an exclusive Rust borrow for the iterator's lifetime.
func (v Value) IteratorMut(ctx *Context) Iterator {
	v.check()
	if v.Type.IterateMut == nil {
		panic("oxide: Rust mutable iterator is not exported for this type")
	}
	return v.Type.IterateMut(ctx, v)
}

func (it Iterator) Next(ctx *Context) Value {
	it.State.check()
	if it.NextFunc == nil {
		panic("oxide: missing Rust Iterator::next")
	}
	return it.NextFunc(ctx, it.State)
}

func (it Iterator) Drop(ctx *Context) { it.State.Drop(ctx) }

// JSON invokes the concrete Rust serializer and returns its Result<String,E>.
// The returned Result owns its payload, including an error, and must be dropped.
func (v Value) JSON(ctx *Context) Value {
	v.check()
	if v.Type.JSON == nil {
		panic("oxide: Rust JSON serialization is not exported for this type")
	}
	return v.Type.JSON(ctx, v)
}

// JSONValue borrows v and calls serde_json::to_value(&v). Unlike JSON followed
// by parsing, this preserves the serializer's conversion of types such as f32.
// The returned Result owns its serde_json::Value or error payload.
func (v Value) JSONValue(ctx *Context) Value {
	v.check()
	if v.Type.JSONValue == nil {
		panic("oxide: Rust JSON value conversion is not exported for this type")
	}
	return v.Type.JSONValue(ctx, v)
}
