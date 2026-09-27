package fixture_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	. "oxide-type-api-conformance"
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
		debug := func(text Value__Alloc_String_String) {
			if text.Ref().String() != want[index] {
				t.Fatalf("case %d: got %q want %q", index, text.Ref().String(), want[index])
			}
			index++
			text.Drop(ctx)
		}
		result := Make(ctx)
		if result.Ref().Variant() != Variant__Outcome__Ok {
			t.Fatal("make failed")
		}
		debug(result.Ref().Field__Ok__0().Debug(ctx))
		result.Drop(ctx)

		doc := Default__Document(ctx)
		doc.Mut().Field__Title().Replace(ctx, String__Alloc_String_String(ctx, "直接调用 雪"))
		vec := Vec__Alloc_Vec_Vec__Of__Node__End(ctx, 2)
		node := Default__Node(ctx)
		node.Mut().Field__Id().Set(7)
		node.Mut().Field__Label().Replace(ctx, New__Core_Option_Option__Of__Alloc_String_String__End__Some(ctx, String__Alloc_String_String(ctx, "甲")))
		vec.Mut().InitAt(0, node)
		node = Default__Node(ctx)
		node.Mut().Field__Id().Set(9)
		vec.Mut().InitAt(1, node)
		vec.Mut().SetLen(2)
		doc.Mut().Field__Nodes().Replace(ctx, vec)
		if Nodes(ctx, doc.Ref().Field__Nodes().Slice()) != 2 {
			t.Fatal("slice argument")
		}
		bytes := Bytes__Alloc_Vec_Vec__Of__U8__End(ctx, []byte{0, 127, 255})
		if Mutable(ctx, bytes.Mut().Slice()) != 3 {
			t.Fatal("mutable slice argument")
		}
		doc.Mut().Field__Outcome().Replace(ctx, New__Core_Result_Result__Of__Alloc_String_String__And__Alloc_Vec_Vec__Of__U8__End__End__Err(ctx, bytes))
		debug(doc.Ref().Debug(ctx))
		// Heap storage preserves both the Rust header and its owned contents
		// after the Context frame used to construct it has been restored.
		heap := Alloc__Document()
		heap.Init(doc)
		ctx.Restore(mark)
		if heap.Ref().Field__Title().String() != "直接调用 雪" {
			t.Fatal("manual storage lifetime")
		}
		heap.Close(ctx)
		for kind := 0; kind < 3; kind++ {
			var event Value__Event
			switch kind {
			case 0:
				event = New__Event__Text(ctx, String__Alloc_String_String(ctx, "事件"))
			case 1:
				event = New__Event__Record(ctx, String__Alloc_String_String(ctx, "点"), Vec__Alloc_Vec_Vec__Of__Node__End(ctx, 0))
			case 2:
				event = New__Event__Empty(ctx)
			}
			debug(event.Ref().Debug(ctx))
			text := DebugEvent(ctx, event.Ref())
			if text.Ref().String() != want[index-1] {
				t.Fatal("generic Debug differs from native Rust expression")
			}
			text.Drop(ctx)
			event.Drop(ctx)
		}
		path := Bytes__Std_Path_PathBuf(ctx, []byte{'a', 255, 'z'})
		osString := OwnedPath(ctx, path)
		debug(osString.Ref().Debug(ctx))
		borrow := Path(ctx, Borrow__Std_Path_Path(osString.Ref().Span()))
		if string(borrow.Bytes()) != "a\xffz" {
			t.Fatal("OS bytes lost")
		}
		osString.Drop(ctx)
		aligned := Default__Aligned(ctx)
		zst := Vec__Alloc_Vec_Vec__Of__Unit__End(ctx, 17)
		if zst.Ref().Cap() != ^uintptr(0) {
			t.Fatal("ZST capacity")
		}
		for i := uintptr(0); i < 17; i++ {
			zst.Mut().InitAt(i, New__Unit(ctx))
		}
		zst.Mut().SetLen(17)
		aligned.Mut().Field__Values().Replace(ctx, zst)
		if aligned.Addr()%64 != 0 {
			t.Fatal("aligned return")
		}
		debug(aligned.Ref().Debug(ctx))
		aligned.Drop(ctx)
		private := Default__PrivateOwner(ctx)
		debug(private.Ref().Debug(ctx))
		private.Drop(ctx)

		maps := MakeMaps(ctx)
		// These iterators have different concrete Rust types; each path stays typed.
		{
			iter := maps.Mut().Field__Hash().Iter(ctx)
			temporary := ctx.Mark()
			next := iter.Mut().Next(ctx)
			if next.Ref().Variant().String() != "Some" {
				t.Fatal("empty HashMap iterator")
			}
			pair := next.Mut().Field__Some__0()
			if pair.Field__0().Deref().String() != "hash" {
				t.Fatal("HashMap key")
			}
			id := pair.Field__1().Deref().Field__Id()
			id.Set(id.Get() + 10)
			next.Drop(ctx)
			ctx.Restore(temporary)
			next = iter.Mut().Next(ctx)
			if next.Ref().Variant().String() != "None" {
				t.Fatal("HashMap iterator termination")
			}
			next.Drop(ctx)
			ctx.Restore(temporary)
			iter.Drop(ctx)
			read := maps.Ref().Field__Hash().Iter(ctx)
			temporary = ctx.Mark()
			item := read.Mut().Next(ctx)
			debug(item.Ref().Field__Some__0().Field__1().Deref().Debug(ctx))
			item.Drop(ctx)
			ctx.Restore(temporary)
			read.Drop(ctx)
		}
		{
			iter := maps.Mut().Field__Tree().Iter(ctx)
			temporary := ctx.Mark()
			next := iter.Mut().Next(ctx)
			if next.Ref().Variant().String() != "Some" {
				t.Fatal("empty BTreeMap iterator")
			}
			pair := next.Mut().Field__Some__0()
			if pair.Field__0().Deref().String() != "tree" {
				t.Fatal("BTreeMap key")
			}
			id := pair.Field__1().Deref().Field__Id()
			id.Set(id.Get() + 10)
			next.Drop(ctx)
			ctx.Restore(temporary)
			next = iter.Mut().Next(ctx)
			if next.Ref().Variant().String() != "None" {
				t.Fatal("BTreeMap iterator termination")
			}
			next.Drop(ctx)
			ctx.Restore(temporary)
			iter.Drop(ctx)
			read := maps.Ref().Field__Tree().Iter(ctx)
			temporary = ctx.Mark()
			item := read.Mut().Next(ctx)
			debug(item.Ref().Field__Some__0().Field__1().Deref().Debug(ctx))
			item.Drop(ctx)
			ctx.Restore(temporary)
			read.Drop(ctx)
		}
		maps.Drop(ctx)
		for _, layout := range dstLayout {
			// Allocate from the native compiler's layout, then independently check the
			// layout calculated by the generated static view of the private-tail DST.
			address := ctx.Alloc(layout[1], layout[2])
			clear(unsafe.Slice((*byte)(unsafe.Pointer(address)), layout[1]))
			value := UnsafeMut__PrivateTail(address, layout[0])
			if value.Size() != layout[1] || value.Align() != layout[2] {
				t.Fatal("private DST compiler/native layout mismatch")
			}
			value.Field__Head().Set(uint8(layout[3]))
			header := New__Ref__Of__PrivateTail__End(ctx)
			header.Mut().SetRef(value.Ref())
			if header.Ref().Deref() != value.Ref() {
				t.Fatal("private DST SetRef/Deref lost metadata")
			}
			frame := ctx.Mark()
			borrowed := DstRef(ctx, value.Ref())
			if borrowed != value.Ref() || borrowed.Size() != layout[1] || borrowed.Align() != layout[2] || borrowed.Field__Head().Get() != uint8(layout[3]) || ctx.Mark() != frame {
				t.Fatal("borrowed DST public root changed layout/address/frame")
			}
			raw := New__MutPtr__Of__PrivateTail__End(ctx)
			raw.Mut().SetRef(value)
			frame = ctx.Mark()
			ctx.Alloc(raw.Size(), raw.Align())
			retained := ctx.Mark()
			ctx.Restore(frame)
			returned := DstRaw(ctx, raw)
			if returned.Mut().Deref() != value || ctx.Mark() != retained {
				t.Fatal("raw DST public root changed metadata/storage")
			}
			returned.Drop(ctx)
			ctx.Restore(frame)
			index++
		}
		for _, number := range [...]uint64{0, 37} {
			frame := ctx.Mark()
			var signature Value__Anonymous__d4760e918d76b845
			ctx.Alloc(signature.Size(), signature.Align())
			retained := ctx.Mark()
			ctx.Restore(frame)
			owner := MakeDebug(ctx, number)
			if ctx.Mark() != retained {
				t.Fatal("dynamic factory retained temporary storage")
			}
			view := owner.Ref().Deref()
			var concrete Value__DebugValue
			if view.Size() != concrete.Size() || view.Align() != concrete.Align() || view.Addr()%view.Align() != 0 || view.Meta() == 0 {
				t.Fatal("dynamic Box concrete layout")
			}
			frame = ctx.Mark()
			if borrowed := DynRef(ctx, view); borrowed != view || ctx.Mark() != frame {
				t.Fatal("shared dyn root changed data/vtable/frame")
			}
			mutable := owner.Mut().Deref()
			if borrowed := DynMut(ctx, mutable); borrowed != mutable || ctx.Mark() != frame {
				t.Fatal("mutable dyn root changed data/vtable/frame")
			}
			header := New__Anonymous__cdc61395cbfd3306(ctx)
			header.Mut().SetRef(view)
			if header.Ref().Deref() != view {
				t.Fatal("dyn SetRef/Deref lost vtable")
			}
			debug(view.Debug(ctx))
			frame = ctx.Mark()
			owner.Drop(ctx)
			if ctx.Mark() != frame {
				t.Fatal("dynamic Box Drop leaked frame")
			}
		}
		if index != len(want) {
			t.Fatal("not every native TypeAPI case executed")
		}
		packed := New__Packed(ctx)
		packed.Mut().Field__Byte().Set(31)
		packed.Mut().Field__Wide().Set(0x1122334455667788)
		if packed.Ref().Read__Wide() != 0x1122334455667788 || *(*byte)(unsafe.Pointer(packed.Addr() + 1)) != 0x88 {
			t.Fatal("packed field offset")
		}
		if Text(ctx, Borrow__Str(ctx.CopyString("雪"))) != 3 {
			t.Fatal("str argument")
		}
		n := New__Word(ctx)
		n.Mut().Set(18446744073709551615)
		text := n.Ref().Display(ctx)
		if text.Ref().String() != "18446744073709551615" {
			t.Fatal("Rust Display")
		}
		text.Drop(ctx)
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
