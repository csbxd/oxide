//go:build linux && (amd64 || arm64)

package oxide

import (
	"math"
	"unicode/utf8"
)

// Value borrows Rust storage. Addr points to the value itself, not a Go object.
// For unsized values Meta is the slice/str length or the trait-object vtable.
// Go cannot enforce Rust moves or borrows: consuming a value invalidates every
// copy of its ownership bits; a borrowed view never extends its owner's life.
type Value struct {
	Addr uintptr
	Type *Type
	Meta uintptr
}

func checkRange(size, offset, length uintptr) {
	if offset > size || length > size-offset {
		panic("oxide: field exceeds its compiler layout")
	}
}

func addAddress(address, offset uintptr) uintptr {
	if offset > ^uintptr(0)-address {
		panic("oxide: Rust address overflow")
	}
	return address + offset
}

func arrayBytes(length, size uintptr) uintptr {
	if size != 0 && length > (^uintptr(0)>>1)/size {
		panic("oxide: Rust allocation exceeds isize::MAX")
	}
	return length * size
}

func (v Value) check() {
	if v.Type == nil || v.Addr == 0 {
		panic("oxide: null Rust value or missing type")
	}
}

func (v Value) Size() uintptr {
	v.check()
	if v.Type.Sized {
		return v.Type.Size
	}
	if v.byteDST() {
		return arrayBytes(v.Meta, 1)
	}
	switch v.Type.Kind {
	case "str":
		return arrayBytes(v.Meta, 1)
	case "slice":
		return arrayBytes(v.Meta, v.Type.Elem.Size)
	case "dynamic":
		return readWord(v.Meta, 8)
	case "aggregate":
		if len(v.Type.Fields) != 0 {
			field := v.Type.Fields[len(v.Type.Fields)-1]
			tail := v.field(field)
			return alignAddress(addAddress(tail.Addr-v.Addr, tail.Size()), v.Align())
		}
	}
	panic("oxide: unsupported unsized Rust layout")
}

func (v Value) Align() uintptr {
	v.check()
	if v.Type.Sized {
		return v.Type.Align
	}
	if v.byteDST() {
		return v.Type.Align
	}
	align := v.Type.Align
	switch v.Type.Kind {
	case "str":
		align = 1
	case "slice":
		align = v.Type.Elem.Align
	case "dynamic":
		align = readWord(v.Meta, 16)
	case "aggregate":
		if len(v.Type.Fields) == 0 {
			panic("oxide: missing unsized tail")
		}
		tail := v.Type.Fields[len(v.Type.Fields)-1]
		align = max(align, (Value{Addr: v.Addr, Type: tail.Type, Meta: v.Meta}).Align())
	default:
		panic("oxide: unsupported unsized Rust alignment")
	}
	if v.Type.Pack != 0 {
		align = min(align, v.Type.Pack)
	}
	if align == 0 || align&(align-1) != 0 {
		panic("oxide: invalid Rust dynamic alignment")
	}
	return align
}

func alignAddress(value, align uintptr) uintptr {
	if align == 0 || align&(align-1) != 0 {
		panic("oxide: invalid Rust alignment")
	}
	return addAddress(value, align-1) &^ (align - 1)
}

func (v Value) field(f Field) Value {
	v.check()
	if f.Type == nil {
		panic("oxide: missing Rust field type")
	}
	offset := f.Offset
	if f.Type.Sized {
		if v.Type.Sized {
			checkRange(v.Type.Size, offset, f.Type.Size)
		}
	} else {
		align := (Value{Addr: v.Addr, Type: f.Type, Meta: v.Meta}).Align()
		if v.Type.Pack != 0 {
			align = min(align, v.Type.Pack)
		}
		offset = alignAddress(offset, align)
	}
	return Value{Addr: addAddress(v.Addr, offset), Type: f.Type, Meta: v.Meta}
}

func (v Value) Field(name string) Value {
	v.check()
	fields := v.Type.Fields
	if len(v.Type.Variants) != 0 {
		fields = v.Type.Variants[v.variantIndex()].Fields
	}
	for _, f := range fields {
		if f.Name == name {
			if !f.Public {
				panic("oxide: Rust field is private")
			}
			return v.field(f)
		}
	}
	panic("oxide: unknown Rust field")
}

func (v Value) Variant() string {
	index := v.variantIndex()
	return v.Type.Variants[index].Name
}

func truncate128(value U128, size uintptr) U128 {
	if size == 0 || size > 16 {
		panic("oxide: invalid Rust integer width")
	}
	if size < 8 {
		value.Lo &= uint64(1)<<(size*8) - 1
	}
	if size <= 8 {
		value.Hi = 0
	} else if size < 16 {
		value.Hi &= uint64(1)<<((size-8)*8) - 1
	}
	return value
}

func (v Value) variantIndex() int {
	v.check()
	t := v.Type
	if len(t.Variants) == 0 {
		panic("oxide: value is not a Rust enum")
	}
	if t.Tag == nil {
		if t.SingleVariant < 0 || t.SingleVariant >= len(t.Variants) {
			panic("oxide: invalid single Rust variant")
		}
		return v.checkedVariant(t.SingleVariant)
	}
	tag := t.Tag
	checkRange(t.Size, tag.Offset, tag.Size)
	raw := readInteger(addAddress(v.Addr, tag.Offset), tag.Size)
	if tag.Encoding == "direct" {
		for i, variant := range t.Variants {
			if truncate128(variant.Discriminant, tag.Size) == raw {
				return v.checkedVariant(i)
			}
		}
	} else if tag.Encoding == "niche" {
		delta := truncate128(U128SubValue(raw, tag.Start), tag.Size)
		if tag.First < 0 || tag.Last < tag.First || tag.Last >= len(t.Variants) || tag.Untagged < 0 || tag.Untagged >= len(t.Variants) {
			panic("oxide: invalid Rust niche range")
		}
		if delta.Hi == 0 && delta.Lo <= uint64(tag.Last-tag.First) {
			return v.checkedVariant(tag.First + int(delta.Lo))
		}
		return v.checkedVariant(tag.Untagged)
	}
	panic("oxide: invalid Rust enum discriminant")
}

func (v Value) checkedVariant(index int) int {
	if index < 0 || index >= len(v.Type.Variants) || v.Type.Variants[index].Uninhabited {
		panic("oxide: uninhabited Rust enum variant")
	}
	return index
}

func (v Value) setVariant(index int) {
	t := v.Type
	if t.Tag == nil {
		if index != t.SingleVariant {
			panic("oxide: uninhabited Rust enum variant")
		}
		return
	}
	tag := t.Tag
	checkRange(t.Size, tag.Offset, tag.Size)
	var bits U128
	switch tag.Encoding {
	case "direct":
		bits = t.Variants[index].Discriminant
	case "niche":
		if index == tag.Untagged {
			return // The initialized payload already carries this variant's niche.
		}
		if index < tag.First || index > tag.Last {
			panic("oxide: variant is outside Rust niche range")
		}
		bits = U128AddValue(tag.Start, U128{Lo: uint64(index - tag.First)})
	default:
		panic("oxide: unsupported Rust enum tag")
	}
	writeInteger(addAddress(v.Addr, tag.Offset), tag.Size, truncate128(bits, tag.Size))
}

// Init copies one same-type Rust value into uninitialized storage, consuming
// the source ownership. It does not run a destructor for previous destination bits.
func (v Value) Init(other Value) {
	v.check()
	other.check()
	if v.Type != other.Type || !v.Type.Sized {
		panic("oxide: Rust initialization type mismatch")
	}
	copy((Span{v.Addr, v.Type.Size}).Bytes(), (Span{other.Addr, other.Type.Size}).Bytes())
}

// Replace consumes other with Rust assignment semantics. Snapshot the right
// hand side before the old destructor can affect its storage. Even if that
// destructor panics, the destination contains the new value afterwards.
func (v Value) Replace(ctx *Context, other Value) {
	v.check()
	other.check()
	if v.Type != other.Type || !v.Type.Sized {
		panic("oxide: Rust replacement type mismatch")
	}
	if v.Type.Size != 0 && v.Addr == other.Addr {
		return
	}
	mark := ctx.Mark()
	value := v.Type.Uninit(ctx)
	value.Init(other)
	defer func() {
		v.Init(value)
		ctx.Restore(mark)
	}()
	v.Drop(ctx)
}

func (v Value) Drop(ctx *Context) {
	v.check()
	if !v.Type.Sized {
		panic("oxide: drop the sized owner of an unsized Rust borrow")
	}
	if v.Type.NeedsDrop && v.Type.Drop == nil {
		panic("oxide: Rust destructor is not exported for this type")
	}
	if v.Type.Drop != nil {
		v.Type.Drop(ctx, v.Addr)
		if ctx.Failed() {
			panic(ctx.TakePanic())
		}
	}
}

func (v Value) Display(ctx *Context) Value {
	v.check()
	if v.Type.Display == nil {
		panic("oxide: Rust Display is not exported for this type")
	}
	result := v.Type.Display(ctx, v)
	if ctx.Failed() {
		panic(ctx.TakePanic())
	}
	return result
}

func (v Value) Debug(ctx *Context) Value {
	v.check()
	if v.Type.Debug == nil {
		panic("oxide: Rust Debug is not exported for this type")
	}
	result := v.Type.Debug(ctx, v)
	if ctx.Failed() {
		panic(ctx.TakePanic())
	}
	return result
}

func readInteger(address, size uintptr) U128 {
	if address == 0 || size == 0 || size > 16 {
		panic("oxide: invalid Rust integer storage")
	}
	b := (Span{address, size}).Bytes()
	var value U128
	for i, x := range b {
		if i < 8 {
			value.Lo |= uint64(x) << (i * 8)
		} else {
			value.Hi |= uint64(x) << ((i - 8) * 8)
		}
	}
	return value
}

func writeInteger(address, size uintptr, value U128) {
	if address == 0 || size == 0 || size > 16 {
		panic("oxide: invalid Rust integer storage")
	}
	b := (Span{address, size}).Bytes()
	for i := range b {
		if i < 8 {
			b[i] = byte(value.Lo >> (i * 8))
		} else {
			b[i] = byte(value.Hi >> ((i - 8) * 8))
		}
	}
}

func readWord(address, offset uintptr) uintptr {
	if address == 0 {
		panic("oxide: null Rust storage")
	}
	return uintptr(readInteger(addAddress(address, offset), 8).Lo)
}
func (v Value) word(offset uintptr) uintptr {
	v.check()
	checkRange(v.Type.Size, offset, 8)
	return readWord(v.Addr, offset)
}
func (v Value) putWord(offset, value uintptr) {
	v.check()
	checkRange(v.Type.Size, offset, 8)
	writeInteger(addAddress(v.Addr, offset), 8, U128{Lo: uint64(value)})
}

func (v Value) integer(signed bool) uintptr {
	v.check()
	kind := v.Type.Kind
	valid := false
	switch kind {
	case "u8", "u16", "u32", "u64", "u128", "usize":
		valid = !signed
	case "i8", "i16", "i32", "i64", "i128", "isize":
		valid = signed
	}
	if !valid || !v.Type.Sized || v.Type.Size == 0 || v.Type.Size > 16 {
		panic("oxide: Rust value is not the requested integer kind")
	}
	return v.Type.Size
}

func (v Value) Uint128() U128 { return readInteger(v.Addr, v.integer(false)) }
func (v Value) SetUint128(value U128) {
	size := v.integer(false)
	if truncate128(value, size) != value {
		panic("oxide: Rust unsigned integer overflow")
	}
	writeInteger(v.Addr, size, value)
}
func (v Value) Uint() uint64 {
	if v.integer(false) > 8 {
		panic("oxide: use Uint128 for a Rust u128")
	}
	return v.Uint128().Lo
}
func (v Value) SetUint(value uint64) { v.SetUint128(U128{Lo: value}) }
func (v Value) Int128() I128 {
	size := v.integer(true)
	bits := readInteger(v.Addr, size)
	if size < 8 && bits.Lo>>(size*8-1) != 0 {
		bits.Lo |= ^uint64(0) << (size * 8)
	}
	if size <= 8 && bits.Lo>>63 != 0 {
		bits.Hi = ^uint64(0)
	}
	if size > 8 && size < 16 && bits.Hi>>((size-8)*8-1) != 0 {
		bits.Hi |= ^uint64(0) << ((size - 8) * 8)
	}
	return I128(bits)
}
func (v Value) SetInt128(value I128) {
	size := v.integer(true)
	bits := U128(value)
	if size < 16 {
		shift := uint64(128 - size*8)
		if I128Shr(I128Shl(value, shift), shift) != value {
			panic("oxide: Rust signed integer overflow")
		}
	}
	writeInteger(v.Addr, size, bits)
}
func (v Value) Int() int64 {
	if v.integer(true) > 8 {
		panic("oxide: use Int128 for a Rust i128")
	}
	return int64(v.Int128().Lo)
}
func (v Value) SetInt(value int64) { v.SetInt128(I128From64(value)) }
func (v Value) Bool() bool {
	v.check()
	if v.Type.Kind != "bool" || v.Type.Size != 1 {
		panic("oxide: Rust value is not bool")
	}
	n := readInteger(v.Addr, 1).Lo
	if n > 1 {
		panic("oxide: invalid Rust bool")
	}
	return n != 0
}
func (v Value) SetBool(value bool) {
	v.check()
	if v.Type.Kind != "bool" || v.Type.Size != 1 {
		panic("oxide: Rust value is not bool")
	}
	n := uint64(0)
	if value {
		n = 1
	}
	writeInteger(v.Addr, 1, U128{Lo: n})
}
func (v Value) Float() float64 {
	v.check()
	switch v.Type.Kind {
	case "f32":
		return float64(math.Float32frombits(uint32(readInteger(v.Addr, 4).Lo)))
	case "f64":
		return math.Float64frombits(readInteger(v.Addr, 8).Lo)
	}
	panic("oxide: Rust value is not a float")
}
func (v Value) SetFloat(value float64) {
	v.check()
	switch v.Type.Kind {
	case "f32":
		writeInteger(v.Addr, 4, U128{Lo: uint64(math.Float32bits(float32(value)))})
	case "f64":
		writeInteger(v.Addr, 8, U128{Lo: math.Float64bits(value)})
	default:
		panic("oxide: Rust value is not a float")
	}
}

func (v Value) Char() rune {
	v.check()
	if v.Type.Kind != "char" || v.Type.Size != 4 {
		panic("oxide: Rust value is not char")
	}
	r := rune(readInteger(v.Addr, 4).Lo)
	if !utf8.ValidRune(r) {
		panic("oxide: invalid Rust char")
	}
	return r
}

func (v Value) SetChar(value rune) {
	v.check()
	if v.Type.Kind != "char" || v.Type.Size != 4 || !utf8.ValidRune(value) {
		panic("oxide: invalid Rust char")
	}
	writeInteger(v.Addr, 4, U128{Lo: uint64(value)})
}

// rawPointer accesses only compiler-identified Rust pointer storage.
func (v Value) rawPointer() (uintptr, uintptr, *Type) {
	v.check()
	if c := v.Type.Container; c != nil && c.Kind == "box" {
		if c.Element == nil {
			panic("oxide: missing Rust Box element type")
		}
		data, meta := v.word(c.DataOffset), uintptr(0)
		if !c.Element.Sized {
			if c.Metadata != "length" && c.Metadata != "vtable" {
				panic("oxide: missing Rust Box metadata")
			}
			meta = v.word(c.MetaOffset)
		}
		return data, meta, c.Element
	}
	if v.Type.Kind != "pointer" || v.Type.Elem == nil {
		panic("oxide: Rust value is not a pointer")
	}
	data, meta := v.word(0), uintptr(0)
	if !v.Type.Elem.Sized {
		c := v.Type.Container
		if c == nil {
			panic("oxide: missing Rust fat-pointer metadata")
		}
		data = v.word(c.DataOffset)
		if c.Metadata == "vtable" {
			meta = v.word(c.MetaOffset)
		} else {
			meta = v.word(c.LenOffset)
		}
	}
	return data, meta, v.Type.Elem
}
func (v Value) Deref() Value {
	data, meta, element := v.rawPointer()
	if data == 0 {
		panic("oxide: null Rust pointer dereference")
	}
	return Value{Addr: data, Type: element, Meta: meta}
}

// SetRef initializes a Rust reference/raw-pointer value from a borrowed view.
// The caller is responsible for the Rust lifetime, mutability and alias rules.
func (v Value) SetRef(other Value) {
	v.check()
	other.check()
	if v.Type.Kind != "pointer" || v.Type.Elem != other.Type {
		panic("oxide: Rust pointer pointee type mismatch")
	}
	if other.Type.Sized {
		v.putWord(0, other.Addr)
		return
	}
	c := v.Type.Container
	if c == nil {
		panic("oxide: missing Rust fat-pointer layout")
	}
	v.putWord(c.DataOffset, other.Addr)
	if c.Metadata == "vtable" {
		v.putWord(c.MetaOffset, other.Meta)
	} else {
		v.putWord(c.LenOffset, other.Meta)
	}
}

func (v Value) Pointer() uintptr {
	v.check()
	if v.Type.Kind != "pointer" || v.Type.Elem == nil || !v.Type.Elem.Sized {
		panic("oxide: Rust value is not a thin pointer")
	}
	return v.word(0)
}
func (v Value) SetPointer(address uintptr) {
	v.check()
	if v.Type.Kind != "pointer" || v.Type.Elem == nil || !v.Type.Elem.Sized {
		panic("oxide: Rust value is not a thin pointer")
	}
	v.putWord(0, address)
}
