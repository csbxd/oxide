//go:build linux && (amd64 || arm64)

package oxide

import (
	"sync"
	"unsafe"
)

// DlIterateCallback preserves the translated callback's exact Rust ABI.
// Its arguments after Context are descriptor, dl_phdr_info, size and user data.
type DlIterateCallback func(*Context, uintptr, uintptr, uintptr, uintptr) int32

type elfProgramHeader struct {
	kind, flags                     uint32
	offset, address, physical       uint64
	fileSize, memorySize, alignment uint64
}

type elfModule struct {
	base, name, headers uintptr
	count               uint16
}

var processELFModules = sync.OnceValue(func() []elfModule {
	return staticELFModules(uintptr(auxv[3]), auxv[4], auxv[5], uintptr(auxv[33]))
})

// LibcDlIteratePhdr enumerates the modules in a statically linked Go process.
// A dynamic loader requires its own lock and TLS bookkeeping; reading link_map
// or /proc/self/maps without that lock cannot implement this API safely.
func LibcDlIteratePhdr(c *Context, descriptor, data uintptr, invoke DlIterateCallback) int32 {
	if descriptor == 0 || invoke == nil {
		panic("oxide: dl_iterate_phdr requires a translated callback adapter")
	}
	LibcGetauxval(c, 6)
	modules := processELFModules()
	mark := c.Mark()
	defer c.Restore(mark)
	info := c.Alloc(32, 8)
	for _, module := range modules {
		*(*uintptr)(unsafe.Pointer(info)) = module.base
		*(*uintptr)(unsafe.Pointer(info + 8)) = module.name
		*(*uintptr)(unsafe.Pointer(info + 16)) = module.headers
		*(*uint64)(unsafe.Pointer(info + 24)) = uint64(module.count)
		// The original GNU ABI is 32 bytes. size tells callbacks that the
		// optional loader counters and native TLS fields are not present.
		if result := invoke(c, descriptor, info, 32, data); result != 0 {
			return result
		}
	}
	return 0
}

func staticELFModules(headers uintptr, entrySize, count uint64, vdso uintptr) []elfModule {
	if headers == 0 || entrySize != 56 || count == 0 || count > 65535 {
		panic("oxide: invalid ELF auxiliary program headers")
	}
	module := elfModule{headers: headers, count: uint16(count)}
	found := false
	for _, ph := range unsafe.Slice((*elfProgramHeader)(unsafe.Pointer(headers)), int(count)) {
		switch ph.kind {
		case 3: // PT_INTERP
			panic("oxide: dl_iterate_phdr in dynamically linked Go programs is not implemented")
		case 6: // PT_PHDR: runtime address minus ELF virtual address is load bias.
			module.base = headers - uintptr(ph.address)
			found = true
		}
	}
	if !found {
		panic("oxide: main executable has no PT_PHDR")
	}
	module.name = RustAlloc(1, 1) // The main executable's name is an empty string.
	if module.name == 0 {
		panic("oxide: cannot allocate ELF module name")
	}
	modules := []elfModule{module}
	if vdso != 0 {
		modules = append(modules, vdsoELFModule(vdso))
	}
	return modules
}

func vdsoELFModule(header uintptr) elfModule {
	ident := *(*[16]byte)(unsafe.Pointer(header))
	if ident[0] != 0x7f || ident[1] != 'E' || ident[2] != 'L' || ident[3] != 'F' || ident[4] != 2 || ident[5] != 1 {
		panic("oxide: invalid vDSO ELF header")
	}
	offset := *(*uint64)(unsafe.Pointer(header + 32))
	entrySize := *(*uint16)(unsafe.Pointer(header + 54))
	count := *(*uint16)(unsafe.Pointer(header + 56))
	if offset > 1<<20 || entrySize != 56 || count == 0 {
		panic("oxide: invalid vDSO program headers")
	}
	module := elfModule{headers: header + uintptr(offset), count: count}
	programs := unsafe.Slice((*elfProgramHeader)(unsafe.Pointer(module.headers)), int(count))
	found := false
	for _, ph := range programs {
		if ph.kind == 1 && ph.offset == 0 { // PT_LOAD containing the ELF header
			module.base = header - uintptr(ph.address)
			found = true
			break
		}
	}
	if !found {
		panic("oxide: vDSO has no header load segment")
	}
	for _, ph := range programs {
		if ph.kind != 2 { // PT_DYNAMIC
			continue
		}
		var strings, length, name uint64
		for offset := uint64(0); offset+16 <= ph.memorySize; offset += 16 {
			entry := module.base + uintptr(ph.address+offset)
			tag := *(*uint64)(unsafe.Pointer(entry))
			value := *(*uint64)(unsafe.Pointer(entry + 8))
			if tag == 0 {
				break
			}
			switch tag {
			case 5: // DT_STRTAB
				strings = value
			case 10: // DT_STRSZ
				length = value
			case 14: // DT_SONAME
				name = value
			}
		}
		if strings == 0 || name >= length {
			continue
		}
		module.name = module.base + uintptr(strings+name)
		for _, b := range unsafe.Slice((*byte)(unsafe.Pointer(module.name)), int(length-name)) {
			if b == 0 {
				return module
			}
		}
	}
	panic("oxide: vDSO has no valid DT_SONAME")
}
