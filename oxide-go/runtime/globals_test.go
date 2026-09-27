//go:build linux && (amd64 || arm64)

package oxide

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"unsafe"
)

func TestTypeIDRelocations(t *testing.T) {
	// rustc keeps the TypeId hash in the allocation bytes; both provenance
	// entries relocate against address zero. Zero must remain a present value.
	var image bytes.Buffer
	image.WriteString("OXAL1")
	put := func(v uint64) {
		if err := binary.Write(&image, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	put(1) // one memory allocation
	put(0) // id
	put(16)
	put(8)
	put(2) // two relocations
	put(0x123456789abcdef0)
	put(0xfedcba9876543210)
	put(0)
	put(1)
	put(8)
	put(2)
	globals := LoadAllocations(image.Bytes(), []AddressAllocation{{ID: 1, Pointer: 0}}, []AllocationAlias{{ID: 2, Target: 1}})
	t.Cleanup(func() { _ = syscall.Munmap(globals.mapping) })
	if globals.Get(1) != 0 || globals.Get(2) != 0 {
		t.Fatal("TypeId provenance must have base address zero")
	}
	value := *(*U128)(unsafe.Pointer(globals.Get(0)))
	if value.Lo != 0x123456789abcdef0 || value.Hi != 0xfedcba9876543210 {
		t.Fatalf("TypeId hash changed during relocation: %#v", value)
	}
}

type testAllocation struct {
	id, align uint64
	bytes     []byte
	relocs    [][2]uint64
}

func allocationImage(allocations ...testAllocation) []byte {
	b := append([]byte("OXAL1"), make([]byte, 8)...)
	binary.LittleEndian.PutUint64(b[5:], uint64(len(allocations)))
	for _, a := range allocations {
		for _, v := range []uint64{a.id, uint64(len(a.bytes)), a.align, uint64(len(a.relocs))} {
			b = binary.LittleEndian.AppendUint64(b, v)
		}
		b = append(b, a.bytes...)
		for _, r := range a.relocs {
			b = binary.LittleEndian.AppendUint64(b, r[0])
			b = binary.LittleEndian.AppendUint64(b, r[1])
		}
	}
	return b
}

func TestGlobalArenaAlignmentAndRelocations(t *testing.T) {
	unaligned := make([]byte, 17)
	unaligned[0] = 0xa5
	binary.LittleEndian.PutUint64(unaligned[1:], 3)
	binary.LittleEndian.PutUint64(unaligned[9:], 0x123456789abcdef0)
	largeAlign := bytes.Repeat([]byte{0x7b}, 65)
	g := LoadAllocations(allocationImage(
		testAllocation{10, 1, []byte{0xab}, nil},
		testAllocation{11, 4096, nil, nil},
		testAllocation{12, 64, unaligned, [][2]uint64{{1, 30}, {9, 20}}},
		testAllocation{13, 1 << 16, largeAlign, nil},
		testAllocation{14, 1, nil, nil},
	), []AddressAllocation{{ID: 20, Pointer: 0}}, []AllocationAlias{{30, 31}, {31, 13}})
	t.Cleanup(func() { _ = syscall.Munmap(g.mapping) })
	base := uintptr(unsafe.Pointer(unsafe.SliceData(g.mapping)))
	for _, x := range [][3]uintptr{{10, 1, 1}, {11, 4096, 0}, {12, 64, 17}, {13, 1 << 16, 65}, {14, 1, 0}} {
		p := g.Get(uint64(x[0]))
		if p%x[1] != 0 || p < base || p+max(x[2], 1) > base+uintptr(len(g.mapping)) {
			t.Fatalf("allocation %d outside its aligned arena: %#x", x[0], p)
		}
	}
	if g.Get(11) == g.Get(14) {
		t.Fatal("zero-sized storage unexpectedly overlaps")
	}
	if g.Get(30) != g.Get(13) || g.Get(31) != g.Get(13) || g.Get(20) != 0 {
		t.Fatal("forward alias chain or address-zero allocation changed")
	}
	got := unsafe.Slice((*byte)(unsafe.Pointer(g.Get(12))), 17)
	if got[0] != 0xa5 || binary.LittleEndian.Uint64(got[1:]) != uint64(g.Get(13))+3 ||
		binary.LittleEndian.Uint64(got[9:]) != 0x123456789abcdef0 {
		t.Fatalf("unaligned relocation changed allocation bytes: %x", got)
	}
	runtime.GC()
	if *(*byte)(unsafe.Pointer(g.Get(10))) != 0xab || !bytes.Equal(unsafe.Slice((*byte)(unsafe.Pointer(g.Get(13))), 65), largeAlign) {
		t.Fatal("static bytes changed after GC")
	}
}

func TestGlobalArenaManySmallObjects(t *testing.T) {
	const count = 77056
	allocations := make([]testAllocation, count)
	for i := range allocations {
		allocations[i] = testAllocation{uint64(i), 8, []byte{byte(i)}, nil}
	}
	g := LoadAllocations(allocationImage(allocations...), nil, nil)
	t.Cleanup(func() { _ = syscall.Munmap(g.mapping) })
	if len(g.mapping) > count*8 {
		t.Fatalf("small statics waste mapping bytes: %d", len(g.mapping))
	}
	base := uintptr(unsafe.Pointer(unsafe.SliceData(g.mapping)))
	for i := range allocations {
		p := g.Get(uint64(i))
		if p%8 != 0 || p < base || p >= base+uintptr(len(g.mapping)) || *(*byte)(unsafe.Pointer(p)) != byte(i) {
			t.Fatalf("allocation %d is not intact in the single mapping", i)
		}
		if i > 0 && p <= g.Get(uint64(i-1)) {
			t.Fatal("nonzero statics overlap")
		}
	}
	t.Logf("%d statics occupy one %d-byte mapping; per-object mmap required at least %d page bytes", count, len(g.mapping), count*syscall.Getpagesize())
}

func TestGlobalArenaRejectsMalformedImage(t *testing.T) {
	for name, image := range map[string][]byte{
		"zero alignment":      allocationImage(testAllocation{1, 0, nil, nil}),
		"non-power alignment": allocationImage(testAllocation{1, 3, nil, nil}),
		"alignment overflow":  allocationImage(testAllocation{1, 1 << 63, nil, nil}),
		"arena overflow":      allocationImage(testAllocation{1, 1 << 62, nil, nil}, testAllocation{2, 1 << 62, nil, nil}, testAllocation{3, 1 << 62, nil, nil}),
		"relocation overflow": allocationImage(testAllocation{1, 8, make([]byte, 8), [][2]uint64{{^uint64(0), 1}}}),
		"relocation outside":  allocationImage(testAllocation{1, 8, make([]byte, 8), [][2]uint64{{1, 1}}}),
		"trailing bytes":      append(allocationImage(), 0),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("accepted malformed static image")
				}
			}()
			LoadAllocations(image, nil, nil)
		})
	}
	g := LoadAllocations(allocationImage(), []AddressAllocation{{ID: 1, Pointer: 0}}, []AllocationAlias{{2, 1}})
	if g.mapping != nil || g.Get(2) != 0 {
		t.Fatal("address-only image should not allocate memory")
	}
}

func TestConstAllocPool(t *testing.T) {
	staticPool.Lock()
	before := len(staticPool.mappings)
	staticPool.Unlock()
	const count = 32768
	var first uintptr
	for i := range count {
		b := binary.LittleEndian.AppendUint64(nil, uint64(i))
		p := ConstAlloc(b, 16, nil)
		if p%16 != 0 {
			t.Fatal("pooled constant is unaligned")
		}
		if i == 0 {
			first = p
		}
		if *(*uint64)(unsafe.Pointer(p)) != uint64(i) {
			t.Fatal("pooled constant has wrong bytes")
		}
	}
	staticPool.Lock()
	added := len(staticPool.mappings) - before
	staticPool.Unlock()
	if added > 1 || *(*uint64)(unsafe.Pointer(first)) != 0 {
		t.Fatalf("%d constants added %d mappings or changed prior bytes", count, added)
	}
	p := ConstAlloc(make([]byte, 9), 64, []Relocation{{Offset: 1, Target: first}})
	if binary.LittleEndian.Uint64(unsafe.Slice((*byte)(unsafe.Pointer(p)), 9)[1:]) != uint64(first) {
		t.Fatal("inline constant relocation lost its target")
	}
}

func TestConstAllocConcurrent(t *testing.T) {
	const count = 512
	addresses := make(chan uintptr, count)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range count / 4 {
				b := binary.LittleEndian.AppendUint64(nil, uint64(i*count+j))
				addresses <- ConstAlloc(b, 8, nil)
			}
		})
	}
	wg.Wait()
	close(addresses)
	seen := make(map[uintptr]bool, count)
	for p := range addresses {
		if seen[p] || p%8 != 0 {
			t.Fatal("concurrent constants overlap or lose alignment")
		}
		seen[p] = true
	}
}

func TestConstAllocLargeAlignment(t *testing.T) {
	const align = 2 << 20
	p := ConstAlloc([]byte{0x5a}, align, nil)
	if p%align != 0 || *(*byte)(unsafe.Pointer(p)) != 0x5a {
		t.Fatal("constant larger than a pool chunk lost its alignment or bytes")
	}
	q := ConstAlloc([]byte{0x7b}, 8, nil)
	runtime.GC()
	if p == q || *(*byte)(unsafe.Pointer(p)) != 0x5a || *(*byte)(unsafe.Pointer(q)) != 0x7b {
		t.Fatal("large constant mapping changed when returning to the small pool")
	}
}
