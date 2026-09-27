//go:build linux && (amd64 || arm64)

package oxide

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestLibcDlIteratePhdr(t *testing.T) {
	c := NewContext()
	defer c.Close()
	headers := uintptr(LibcGetauxval(c, 3))
	count := LibcGetauxval(c, 5)
	dynamic := false
	for _, ph := range unsafe.Slice((*elfProgramHeader)(unsafe.Pointer(headers)), int(count)) {
		dynamic = dynamic || ph.kind == 3
	}
	if dynamic {
		// Race/cgo builds can use the native loader. Never silently omit its
		// libraries, even on a repeated call after initialization panicked.
		for range 2 {
			func() {
				defer func() {
					failure := recover()
					if failure == nil || !strings.Contains(failure.(string), "dynamically linked") {
						t.Fatalf("dynamic loader rejection: %v", failure)
					}
				}()
				LibcDlIteratePhdr(c, 1, 0, func(*Context, uintptr, uintptr, uintptr, uintptr) int32 {
					t.Fatal("dynamic loader invoked callback with incomplete module list")
					return 0
				})
			}()
		}
		return
	}
	pc := reflect.ValueOf(LibcDlIteratePhdr).Pointer()
	visited := 0
	foundCode := false
	result := LibcDlIteratePhdr(c, 123, 456, func(current *Context, descriptor, info, size, data uintptr) int32 {
		if current != c || descriptor != 123 || data != 456 || size != 32 {
			t.Fatal("dl_iterate_phdr changed callback arguments")
		}
		base := *(*uintptr)(unsafe.Pointer(info))
		name := *(*uintptr)(unsafe.Pointer(info + 8))
		phdr := *(*uintptr)(unsafe.Pointer(info + 16))
		phnum := *(*uint16)(unsafe.Pointer(info + 24))
		nameText := string(unsafe.Slice((*byte)(unsafe.Pointer(name)), LibcStrlen(c, name)))
		if visited == 0 {
			if nameText != "" || phdr != headers || uint64(phnum) != count {
				t.Fatal("main executable program headers differ from kernel auxiliary vector")
			}
			for _, ph := range unsafe.Slice((*elfProgramHeader)(unsafe.Pointer(phdr)), int(phnum)) {
				if ph.kind == 1 && pc >= base+uintptr(ph.address) && pc < base+uintptr(ph.address+ph.memorySize) {
					foundCode = true
				}
			}
		} else if nameText != "linux-vdso.so.1" || base != uintptr(auxv[33]) {
			t.Fatalf("vDSO: name %q, base %#x", nameText, base)
		}
		visited++
		return 0
	})
	want := 1
	if auxv[33] != 0 {
		want++
	}
	if result != 0 || visited != want || !foundCode {
		t.Fatalf("module iteration result %d, visited %d, code segment found %v", result, visited, foundCode)
	}
	visited = 0
	if result := LibcDlIteratePhdr(c, 123, 0, func(*Context, uintptr, uintptr, uintptr, uintptr) int32 {
		visited++
		return 17
	}); result != 17 || visited != 1 {
		t.Fatal("module iteration did not stop and propagate callback result")
	}
}

func TestStaticELFLoadBias(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := c.Alloc(56, 8)
	*(*elfProgramHeader)(unsafe.Pointer(p)) = elfProgramHeader{kind: 6, address: 64}
	modules := staticELFModules(p, 56, 1, 0)
	defer RustDealloc(modules[0].name, 1, 1)
	if len(modules) != 1 || modules[0].base != p-64 || modules[0].headers != p || modules[0].count != 1 {
		t.Fatal("static PIE relocation load bias is incorrect")
	}
}
