//go:build linux && (amd64 || arm64)

package oxide

import "testing"

func TestStaticMemoryArithmeticBounds(t *testing.T) {
	max := ^uintptr(0)
	if AddAddress(max-7, 7) != max || ArrayBytes(max, 0) != 0 || AlignUp(65, 64) != 128 {
		t.Fatal("valid arithmetic result")
	}
	for _, run := range []func(){
		func() { AddAddress(max, 1) },
		func() { ArrayBytes(max>>1, 2) },
		func() { AlignUp(1, 0) },
		func() { AlignUp(1, 3) },
		func() { AlignUp(max, 64) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid arithmetic or storage accepted")
				}
			}()
			run()
		}()
	}
}
