//go:build linux && (amd64 || arm64)

package oxide

import (
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

type Globals struct {
	pointers map[uint64]uintptr
	mapping  []byte
}
type Relocation struct {
	Offset uintptr
	Target uintptr
}
type AddressAllocation struct {
	ID      uint64
	Pointer uintptr
}
type AllocationAlias struct{ ID, Target uint64 }

// LoadAllocations installs a translated crate's Rust static allocation image.
// The addresses are outside the Go heap; pointers stored in the image stay
// valid across garbage collections and goroutine stack growth.
func LoadAllocations(image []byte, addresses []AddressAllocation, aliases []AllocationAlias) *Globals {
	var size uintptr
	align := uintptr(1)
	n := walkAllocationImage(image, func(a imageAllocation) {
		_, size = staticBounds(size, uintptr(len(a.bytes)), a.align)
		align = max(align, a.align)
	})
	g := &Globals{pointers: make(map[uint64]uintptr, n)}
	committed := false
	defer func() {
		if !committed && g.mapping != nil {
			_ = syscall.Munmap(g.mapping)
		}
	}()
	var storage []byte
	if n != 0 {
		g.mapping, storage = staticMapping(size, align)
	}
	type reloc struct {
		offset uintptr
		target uint64
	}
	var relocs []reloc
	var offset uintptr
	walkAllocationImage(image, func(a imageAllocation) {
		start, end := staticBounds(offset, uintptr(len(a.bytes)), a.align)
		block := storage[start:end]
		g.pointers[a.id] = uintptr(unsafe.Pointer(unsafe.SliceData(block)))
		copy(block, a.bytes)
		for i := 0; i < len(a.relocations); i += 16 {
			off := binary.LittleEndian.Uint64(a.relocations[i:])
			target := binary.LittleEndian.Uint64(a.relocations[i+8:])
			relocs = append(relocs, reloc{start + uintptr(off), target})
		}
		offset = end
	})
	for _, f := range addresses {
		g.pointers[f.ID] = f.Pointer
	}
	for len(aliases) > 0 {
		pending := aliases[:0]
		for _, a := range aliases {
			if p, ok := g.pointers[a.Target]; ok {
				g.pointers[a.ID] = p
			} else {
				pending = append(pending, a)
			}
		}
		if len(pending) == len(aliases) {
			panic("cyclic or missing oxide allocation alias")
		}
		aliases = pending
	}
	for _, r := range relocs {
		p := storage[r.offset:]
		v := binary.LittleEndian.Uint64(p)
		binary.LittleEndian.PutUint64(p, v+uint64(g.Get(r.target)))
	}
	committed = true
	return g
}

// FunctionPointer is the generated-code function ABI. Only declarations are
// passed here: Rust closure environments are explicit MIR parameters, never Go
// closure captures. The func value descriptor therefore has static lifetime.
func FunctionPointer[F any](f F) uintptr { return *(*uintptr)(unsafe.Pointer(&f)) }

func (g *Globals) Get(id uint64) uintptr {
	p, ok := g.pointers[id]
	if !ok {
		panic(fmt.Sprintf("missing oxide global %d", id))
	}
	return p
}

// staticBounds validates both the allocation layout and accumulated arena size.
// Zero-sized objects reserve one byte; their addresses may differ in Rust.
func staticBounds(offset, size, align uintptr) (start, end uintptr) {
	if _, ok := storageSize(size, align); !ok {
		panic("invalid Rust static layout")
	}
	if _, ok := storageSize(offset, align); !ok {
		panic("Rust static arena size overflow")
	}
	start = (offset + align - 1) &^ (align - 1)
	size = max(size, 1)
	if size > (^uintptr(0)>>1)-start {
		panic("Rust static arena size overflow")
	}
	return start, start + size
}

type imageAllocation struct {
	id          uint64
	align       uintptr
	bytes       []byte
	relocations []byte
}

// The first walk validates the entire image before creating a mapping. The
// second copies into it; neither walk allocates per-record Go objects.
func walkAllocationImage(image []byte, visit func(imageAllocation)) uint64 {
	if len(image) < 13 || string(image[:5]) != "OXAL1" {
		panic("invalid oxide allocation image")
	}
	i := 5
	read := func() uint64 {
		if i > len(image)-8 {
			panic("truncated oxide allocation image")
		}
		v := binary.LittleEndian.Uint64(image[i : i+8])
		i += 8
		return v
	}
	n := read()
	if n > uint64((len(image)-i)/32) {
		panic("truncated oxide allocation headers")
	}
	for j := uint64(0); j < n; j++ {
		id, size, align, nr := read(), read(), read(), read()
		if size > uint64(len(image)-i) {
			panic("truncated oxide allocation bytes")
		}
		data := image[i : i+int(size)]
		i += int(size)
		if nr > uint64((len(image)-i)/16) {
			panic("truncated oxide relocations")
		}
		relocations := image[i : i+int(nr)*16]
		i += len(relocations)
		for k := 0; k < len(relocations); k += 16 {
			off := binary.LittleEndian.Uint64(relocations[k:])
			if off > size || size-off < 8 {
				panic("oxide relocation out of bounds")
			}
		}
		visit(imageAllocation{id, uintptr(align), data, relocations})
	}
	if i != len(image) {
		panic("trailing oxide allocation bytes")
	}
	return n
}

func mapStatic(size uintptr) []byte {
	b, err := syscall.Mmap(-1, 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		panic(fmt.Errorf("oxide static mmap: %w", err))
	}
	return b
}

func staticMapping(size, align uintptr) (mapping, storage []byte) {
	n, ok := storageSize(size, align)
	if !ok {
		panic("invalid Rust static layout")
	}
	mapping = mapStatic(n)
	base := uintptr(unsafe.Pointer(unsafe.SliceData(mapping)))
	off := -base & (align - 1)
	return mapping, mapping[off : off+max(size, 1)]
}

// Inline constants and linker slots arrive independently during package init.
// Keep their small allocations together too. Mappings have static lifetime.
var staticPool struct {
	sync.Mutex
	mappings [][]byte
	current  []byte
	offset   uintptr
}

func staticStorage(size, align uintptr) (uintptr, []byte) {
	n, ok := storageSize(size, align)
	if !ok {
		panic("invalid Rust static layout")
	}
	const chunkSize = 1 << 20
	staticPool.Lock()
	defer staticPool.Unlock()
	var block []byte
	if n > chunkSize {
		mapping, storage := staticMapping(size, align)
		staticPool.mappings = append(staticPool.mappings, mapping)
		block = storage
	} else {
		size = max(size, 1)
		for {
			b := staticPool.current
			base := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
			off := staticPool.offset + (-(base + staticPool.offset) & (align - 1))
			if off <= uintptr(len(b)) && size <= uintptr(len(b))-off {
				block = b[off : off+size]
				staticPool.offset = off + size
				break
			}
			staticPool.current = mapStatic(chunkSize)
			staticPool.mappings = append(staticPool.mappings, staticPool.current)
			staticPool.offset = 0
		}
	}
	return uintptr(unsafe.Pointer(unsafe.SliceData(block))), block
}

func ConstAlloc(bytes []byte, align uintptr, relocs []Relocation) uintptr {
	for _, r := range relocs {
		if r.Offset > uintptr(len(bytes)) || uintptr(len(bytes))-r.Offset < 8 {
			panic("oxide relocation out of bounds")
		}
	}
	p, b := staticStorage(uintptr(len(bytes)), align)
	copy(b, bytes)
	for _, r := range relocs {
		v := binary.LittleEndian.Uint64(b[r.Offset:])
		binary.LittleEndian.PutUint64(b[r.Offset:], v+uint64(r.Target))
	}
	return p
}
