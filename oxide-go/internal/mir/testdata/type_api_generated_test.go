package fixture

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func TestDirectTypeAPI(t *testing.T) {
	b, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(want) != 14 {
		t.Fatalf("native TypeAPI cases=%d, want 14", len(want))
	}
	var dstLayout [2][4]uintptr
	for i := range dstLayout {
		if _, err := fmt.Sscanf(want[10+i], "dst %d %d %d %d", &dstLayout[i][0], &dstLayout[i][1], &dstLayout[i][2], &dstLayout[i][3]); err != nil {
			t.Fatal(err)
		}
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	mark := ctx.Mark()
	check := func() {
		index := 0
		debug := func(v oxide.Value) {
			s := v.Debug(ctx)
			if s.String() != want[index] {
				t.Fatalf("case %d: got %q want %q", index, s.String(), want[index])
			}
			index++
			s.Drop(ctx)
		}
		result := Make(ctx)
		if result.Variant() != "Ok" {
			t.Fatal("make failed")
		}
		debug(result.Field("0"))
		result.Drop(ctx)
		doc := TypeDocument.Default(ctx)
		str := doc.Field("title").Type
		doc.Field("title").Replace(ctx, str.String(ctx, "直接调用 雪"))
		vec := doc.Field("nodes").Type.Vec(ctx, 2)
		node := TypeNode.Default(ctx)
		node.Field("id").SetUint(7)
		label := node.Field("label")
		label.Replace(ctx, label.Type.Enum(ctx, "Some", str.String(ctx, "甲")))
		vec.InitAt(0, node)
		node = TypeNode.Default(ctx)
		node.Field("id").SetUint(9)
		vec.InitAt(1, node)
		vec.SetLen(2)
		doc.Field("nodes").Replace(ctx, vec)
		if Nodes(ctx, doc.Field("nodes")) != 2 {
			t.Fatal("slice argument")
		}
		outcome := doc.Field("outcome")
		bytesType := outcome.Type.Variants[1].Fields[0].Type
		bytes := bytesType.Bytes(ctx, []byte{0, 127, 255})
		if Mutable(ctx, bytes) != 3 {
			t.Fatal("mutable slice argument")
		}
		outcome.Replace(ctx, outcome.Type.Enum(ctx, "Err", bytes))
		debug(doc)
		// Heap-owned header survives restoring the Context which constructed it.
		heap := TypeDocument.HeapAlloc()
		heap.Value.Init(doc)
		ctx.Restore(mark)
		if heap.Field("title").String() != "直接调用 雪" {
			t.Fatal("manual storage lifetime")
		}
		heap.Close(ctx)
		for _, name := range [...]string{"Text", "Record", "Empty"} {
			var event oxide.Value
			switch name {
			case "Text":
				event = TypeEvent.Enum(ctx, name, str.String(ctx, "事件"))
			case "Record":
				event = TypeEvent.Enum(ctx, name, str.String(ctx, "点"), vec.Type.Vec(ctx, 0))
			case "Empty":
				event = TypeEvent.Enum(ctx, name)
			}
			debug(event)
			s := DebugEvent(ctx, event)
			if s.String() != want[index-1] {
				t.Fatal("generic Debug differs from native Rust expression")
			}
			s.Drop(ctx)
			event.Drop(ctx)
		}
		path := OwnedPathTypes.Params[0].Bytes(ctx, []byte{'a', 255, 'z'})
		osString := OwnedPath(ctx, path)
		debug(osString)
		span := osString.Span()
		borrow := Path(ctx, span)
		if string(borrow.Bytes()) != "a\xffz" {
			t.Fatal("OS bytes lost")
		}
		osString.Drop(ctx)
		aligned := TypeAligned.Default(ctx)
		zst := aligned.Field("values").Type.Vec(ctx, 17)
		if zst.Cap() != ^uintptr(0) {
			t.Fatal("ZST capacity")
		}
		for i := uintptr(0); i < 17; i++ {
			zst.InitAt(i, zst.Type.Container.Element.Uninit(ctx))
		}
		zst.SetLen(17)
		aligned.Field("values").Replace(ctx, zst)
		if aligned.Addr%64 != 0 {
			t.Fatal("aligned return")
		}
		debug(aligned)
		aligned.Drop(ctx)
		private := TypePrivateOwner.Default(ctx)
		debug(private)
		private.Drop(ctx)
		maps := MakeMaps(ctx)
		for _, name := range [...]string{"hash", "tree"} {
			collection := maps.Field(name)
			iter := collection.IteratorMut(ctx)
			temporary := ctx.Mark()
			next := iter.Next(ctx)
			if next.Variant() != "Some" {
				t.Fatal("empty Rust map iterator")
			}
			pair := next.Field("0")
			if pair.Field("0").Deref().String() != name {
				t.Fatal("map key")
			}
			id := pair.Field("1").Deref().Field("id")
			id.SetUint(id.Uint() + 10)
			next.Drop(ctx)
			ctx.Restore(temporary)
			next = iter.Next(ctx)
			if next.Variant() != "None" {
				t.Fatal("map iterator termination")
			}
			next.Drop(ctx)
			ctx.Restore(temporary)
			iter.Drop(ctx)
			read := collection.Iterator(ctx)
			temporary = ctx.Mark()
			next = read.Next(ctx)
			debug(next.Field("0").Field("1").Deref())
			next.Drop(ctx)
			ctx.Restore(temporary)
			read.Drop(ctx)
		}
		maps.Drop(ctx)
		for _, layout := range dstLayout {
			prototype := oxide.Value{Addr: TypePrivateTail.Align, Type: TypePrivateTail, Meta: layout[0]}
			size, align := prototype.Size(), prototype.Align()
			if size != layout[1] || align != layout[2] {
				t.Fatal("private DST compiler/native layout mismatch")
			}
			address := ctx.Alloc(size, align)
			clear(unsafe.Slice((*byte)(unsafe.Pointer(address)), size))
			value := oxide.Value{Addr: address, Type: TypePrivateTail, Meta: layout[0]}
			value.Field("head").SetUint(uint64(layout[3]))
			if !privateTailFieldRejected(value) {
				t.Fatal("private DST field became accessible")
			}
			header := DstRefTypes.Params[0].Uninit(ctx)
			header.SetRef(value)
			if header.Deref() != value {
				t.Fatal("private DST SetRef/Deref lost metadata")
			}
			frame := ctx.Mark()
			borrowed := DstRef(ctx, value)
			if borrowed != value || borrowed.Size() != size || borrowed.Align() != align || borrowed.Field("head").Uint() != uint64(layout[3]) || ctx.Mark() != frame {
				t.Fatal("borrowed DST public root changed layout/address/frame")
			}
			raw := DstRawTypes.Params[0].Uninit(ctx)
			raw.SetRef(value)
			frame = ctx.Mark()
			resultType := DstRawTypes.Result
			ctx.Alloc(resultType.Size, resultType.Align)
			retained := ctx.Mark()
			ctx.Restore(frame)
			returned := DstRaw(ctx, raw)
			if returned.Deref() != value || ctx.Mark() != retained {
				t.Fatal("raw DST public root changed metadata/storage")
			}
			returned.Drop(ctx)
			ctx.Restore(frame)
			index++
		}
		for _, number := range [...]uint64{0, 37} {
			frame := ctx.Mark()
			resultType := MakeDebugTypes.Result
			ctx.Alloc(resultType.Size, resultType.Align)
			retained := ctx.Mark()
			ctx.Restore(frame)
			owner := MakeDebug(ctx, number)
			if ctx.Mark() != retained {
				t.Fatal("dynamic factory retained temporary storage")
			}
			view := owner.Deref()
			if view.Size() != TypeDebugValue.Size || view.Align() != TypeDebugValue.Align || view.Addr%view.Align() != 0 || view.Meta == 0 {
				t.Fatal("dynamic Box concrete layout")
			}
			frame = ctx.Mark()
			if borrowed := DynRef(ctx, view); borrowed != view || ctx.Mark() != frame {
				t.Fatal("shared dyn root changed data/vtable/frame")
			}
			if borrowed := DynMut(ctx, view); borrowed != view || ctx.Mark() != frame {
				t.Fatal("mutable dyn root changed data/vtable/frame")
			}
			header := DynRefTypes.Params[0].Uninit(ctx)
			header.SetRef(view)
			if header.Deref() != view {
				t.Fatal("dyn SetRef/Deref lost vtable")
			}
			debug(view)
			frame = ctx.Mark()
			owner.Drop(ctx)
			if ctx.Mark() != frame {
				t.Fatal("dynamic Box Drop leaked frame")
			}
		}
		if index != len(want) {
			t.Fatal("not every native TypeAPI case executed")
		}
		packed := TypePacked.Uninit(ctx)
		packed.Field("byte").SetUint(31)
		packed.Field("wide").SetUint(0x1122334455667788)
		if packed.Field("wide").Uint() != 0x1122334455667788 || *(*byte)(unsafe.Pointer(packed.Addr + 1)) != 0x88 {
			t.Fatal("packed field offset")
		}
		if Text(ctx, ctx.CopyString("雪")) != 3 {
			t.Fatal("str argument")
		}
		n := TypeWord.Uninit(ctx)
		n.SetUint(18446744073709551615)
		s := n.Display(ctx)
		if s.String() != "18446744073709551615" {
			t.Fatal("Rust Display")
		}
		s.Drop(ctx)
		ctx.Restore(mark)
	}
	check()
	baseline := oxide.HeapStats().LiveAllocations
	requireNoGoAllocations(t, 100, func() {
		check()
		if ctx.Mark() != mark || ctx.Failed() {
			t.Fatal("frame/panic escaped")
		}
		if oxide.HeapStats().LiveAllocations != baseline {
			t.Fatal("Rust owner leaked")
		}
	})
}

// One recovering defer avoids Go's savedOpenDeferState allocation, keeping the
// access-control negative check inside the exact raw allocation measurement.
func privateTailFieldRejected(value oxide.Value) (rejected bool) {
	defer func() { rejected = recover() != nil }()
	value.Field("tail")
	return false
}
