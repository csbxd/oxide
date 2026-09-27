package rendererfixture_test

import (
	"cmp"
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	. "oxide-renderer-conformance"
	"slices"
	"strings"
	"unsafe"
)

type observedEntry[K, V apiPlace] struct {
	key   K
	value V
}

// appendMapNodeOrder iterates the actual Rust map and only orders its observed output.
func appendMapNodeOrder(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Usize__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Alloc_String_String, Ref__Usize]{}), unsafe.Alignof(observedEntry[Ref__Alloc_String_String, Ref__Usize]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Alloc_String_String, Ref__Usize])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Alloc_String_String, Ref__Usize]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Alloc_String_String, Ref__Usize]) int {
		return strings.Compare(a.key.String(), b.key.String())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

// appendMapClassDefs iterates the actual Rust map and only orders its observed output.
func appendMapClassDefs(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Ir_NodeStyle__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Alloc_String_String, Ref__Ir_NodeStyle]{}), unsafe.Alignof(observedEntry[Ref__Alloc_String_String, Ref__Ir_NodeStyle]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Alloc_String_String, Ref__Ir_NodeStyle])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Alloc_String_String, Ref__Ir_NodeStyle]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Alloc_String_String, Ref__Ir_NodeStyle]) int {
		return strings.Compare(a.key.String(), b.key.String())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

// appendMapNodeClasses iterates the actual Rust map and only orders its observed output.
func appendMapNodeClasses(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Alloc_Vec_Vec__Of__Alloc_String_String__End__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Alloc_String_String, Ref__Alloc_Vec_Vec__Of__Alloc_String_String__End]{}), unsafe.Alignof(observedEntry[Ref__Alloc_String_String, Ref__Alloc_Vec_Vec__Of__Alloc_String_String__End]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Alloc_String_String, Ref__Alloc_Vec_Vec__Of__Alloc_String_String__End])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Alloc_String_String, Ref__Alloc_Vec_Vec__Of__Alloc_String_String__End]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Alloc_String_String, Ref__Alloc_Vec_Vec__Of__Alloc_String_String__End]) int {
		return strings.Compare(a.key.String(), b.key.String())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

// appendMapNodeLinks iterates the actual Rust map and only orders its observed output.
func appendMapNodeLinks(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__NodeLink__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Alloc_String_String, Ref__NodeLink]{}), unsafe.Alignof(observedEntry[Ref__Alloc_String_String, Ref__NodeLink]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Alloc_String_String, Ref__NodeLink])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Alloc_String_String, Ref__NodeLink]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Alloc_String_String, Ref__NodeLink]) int {
		return strings.Compare(a.key.String(), b.key.String())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

// appendMapEdgeStyles iterates the actual Rust map and only orders its observed output.
func appendMapEdgeStyles(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Usize__And__Ir_EdgeStyleOverride__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Usize, Ref__Ir_EdgeStyleOverride]{}), unsafe.Alignof(observedEntry[Ref__Usize, Ref__Ir_EdgeStyleOverride]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Usize, Ref__Ir_EdgeStyleOverride])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Usize, Ref__Ir_EdgeStyleOverride]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Usize, Ref__Ir_EdgeStyleOverride]) int {
		return cmp.Compare(a.key.Get(), b.key.Get())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

// appendMapArchEdgePorts iterates the actual Rust map and only orders its observed output.
func appendMapArchEdgePorts(ctx *oxide.Context, dst []byte, value Ref__Std_Collections_Hash_Map_HashMap__Of__Usize__And__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End__End) []byte {
	mark := ctx.Mark()
	it := value.Iter(ctx)
	count := 0
	for {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		done := next.Ref().Variant().String() == "None"
		next.Drop(ctx)
		ctx.Restore(temporary)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	address := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry[Ref__Usize, Ref__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End]{}), unsafe.Alignof(observedEntry[Ref__Usize, Ref__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End]{}))
	entries := unsafe.Slice((*observedEntry[Ref__Usize, Ref__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End])(unsafe.Pointer(address)), count)
	mark = ctx.Mark()
	it = value.Iter(ctx)
	for j := 0; j < count; j++ {
		temporary := ctx.Mark()
		next := it.Mut().Next(ctx)
		pair := next.Ref().Field__Some__0()
		entries[j] = observedEntry[Ref__Usize, Ref__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End]{pair.Field__0().Deref(), pair.Field__1().Deref()}
		next.Drop(ctx)
		ctx.Restore(temporary)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry[Ref__Usize, Ref__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End]) int {
		return cmp.Compare(a.key.Get(), b.key.Get())
	})
	for j, e := range entries {
		if j > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = appendDebug(ctx, dst, e.key)
		dst = append(dst, ':', ' ')
		dst = appendDebug(ctx, dst, e.value)
	}
	return dst
}

func appendGraph(ctx *oxide.Context, dst []byte, source Ref__Graph) []byte {
	graph := Graph_As_Core_Clone_Clone_Clone(ctx, source)
	defer graph.Drop(ctx)

	dst = append(dst, "node_order={"...)
	dst = appendMapNodeOrder(ctx, dst, graph.Ref().Field__NodeOrder())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__NodeOrder().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Usize__End(ctx))

	dst = append(dst, "class_defs={"...)
	dst = appendMapClassDefs(ctx, dst, graph.Ref().Field__ClassDefs())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__ClassDefs().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Ir_NodeStyle__End(ctx))

	dst = append(dst, "node_classes={"...)
	dst = appendMapNodeClasses(ctx, dst, graph.Ref().Field__NodeClasses())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__NodeClasses().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Alloc_Vec_Vec__Of__Alloc_String_String__End__End(ctx))

	dst = append(dst, "node_styles={"...)
	dst = appendMapClassDefs(ctx, dst, graph.Ref().Field__NodeStyles())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__NodeStyles().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Ir_NodeStyle__End(ctx))

	dst = append(dst, "subgraph_styles={"...)
	dst = appendMapClassDefs(ctx, dst, graph.Ref().Field__SubgraphStyles())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__SubgraphStyles().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Ir_NodeStyle__End(ctx))

	dst = append(dst, "subgraph_classes={"...)
	dst = appendMapNodeClasses(ctx, dst, graph.Ref().Field__SubgraphClasses())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__SubgraphClasses().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__Alloc_Vec_Vec__Of__Alloc_String_String__End__End(ctx))

	dst = append(dst, "node_links={"...)
	dst = appendMapNodeLinks(ctx, dst, graph.Ref().Field__NodeLinks())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__NodeLinks().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Alloc_String_String__And__NodeLink__End(ctx))

	dst = append(dst, "edge_styles={"...)
	dst = appendMapEdgeStyles(ctx, dst, graph.Ref().Field__EdgeStyles())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__EdgeStyles().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Usize__And__Ir_EdgeStyleOverride__End(ctx))

	dst = append(dst, "arch_edge_ports={"...)
	dst = appendMapArchEdgePorts(ctx, dst, graph.Ref().Field__ArchEdgePorts())
	dst = append(dst, '}', '\n')
	graph.Mut().Field__ArchEdgePorts().Replace(ctx, Default__Std_Collections_Hash_Map_HashMap__Of__Usize__And__Tuple__Of__Core_Option_Option__Of__Ir_ArchDir__End__And__Core_Option_Option__Of__Ir_ArchDir__End__End__End(ctx))

	dst = appendDebug(ctx, dst, graph.Ref())
	return append(dst, '\n')
}
