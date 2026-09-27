package rendererfixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

type apiCase struct {
	ID     uintptr `json:"id"`
	Name   string  `json:"name"`
	Source string  `json:"source"`
	File   string  `json:"file"`
}

func TestPublicAPIMatchesNativeRust(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("reference", "api", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []apiCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no public API reference cases")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	c := oxide.NewContext()
	defer c.Close()
	for _, item := range cases {
		t.Run(item.Name, func(t *testing.T) {
			mark := c.Mark()
			defer c.Restore(mark)
			want, err := os.ReadFile(filepath.Join("reference", "api", item.File))
			if err != nil {
				t.Fatal(err)
			}
			input := putBytes(c, []byte(item.Source))
			path := filepath.Join(t.TempDir(), "scratch")
			pathPointer := putBytes(c, []byte(path))
			frame := c.Mark()
			invoke := func(output, capacity uintptr) uintptr {
				return Api_Run(c, item.ID, input, uintptr(len(item.Source)), pathPointer, uintptr(len(path)), output, capacity)
			}
			if n := invoke(0, 0); n != uintptr(len(want)) {
				t.Fatalf("size query: %d, native %d", n, len(want))
			}
			checkFrame(t, c, frame)
			output := c.Alloc(uintptr(len(want)+16), 16)
			storage := unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want)+16)
			frame = c.Mark()
			for i := range storage {
				storage[i] = 0xa5
			}
			check := func(n uintptr) {
				if n != uintptr(len(want)) {
					t.Fatalf("length: %d, native %d", n, len(want))
				}
				checkFrame(t, c, frame)
				compareBytes(t, item.Name, storage[:len(want)], want)
				for i, b := range storage[len(want):] {
					if b != 0xa5 {
						t.Fatalf("output guard byte %d overwritten", i)
					}
				}
			}
			check(invoke(output, uintptr(len(want))))
			var before, after runtime.MemStats
			for call := 0; call < 3; call++ {
				runtime.ReadMemStats(&before)
				n := invoke(output, uintptr(len(want)))
				runtime.ReadMemStats(&after)
				check(n)
				if objects, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc; objects != 0 || bytes != 0 {
					t.Errorf("call %d: translated API allocated %d Go objects (%d bytes)", call, objects, bytes)
				}
			}
		})
	}
}
