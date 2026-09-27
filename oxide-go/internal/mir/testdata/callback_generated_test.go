package fixture_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	f "oxide-callback-conformance"
)

func encode(a, b, c uint64) uint64 { return a*10_000 + b*100 + c }

func spread(_ *oxide.Context, a, b, c f.ABI__U64) f.ABI__U64 { return encode(a, b, c) }

func ordinary(ctx *oxide.Context, args f.ABI__Args) f.ABI__U64 {
	mark := ctx.Mark()
	defer ctx.Restore(mark)
	value := f.New__Args(ctx)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(value.Addr())), f.RustSize__Args), unsafe.Slice((*byte)(unsafe.Pointer(&args)), f.RustSize__Args))
	view := value.Ref()
	return encode(view.Field__0().Get(), view.Field__1().Get(), view.Field__2().Get())
}

var _ f.Callback__RustCall = spread
var _ f.Callback__Ordinary = ordinary

func TestCallbacksMatchNativeRust(t *testing.T) {
	data, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 16 {
		t.Fatal("incomplete native callback reference")
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	a := oxide.FunctionPointer(f.Callback__RustCall(spread))
	b := oxide.FunctionPointer(f.Callback__Ordinary(ordinary))
	mark := ctx.Mark()
	for _, line := range lines {
		var kind, seed, want uint64
		if _, err := fmt.Sscanf(line, "%d %d %d", &kind, &seed, &want); err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("case_%d/seed_%d", kind, seed), func(t *testing.T) {
			requireNoGoAllocations(t, 100, func() {
				if got := f.Conformance(ctx, kind, seed, a, b); got != want {
					t.Fatalf("callback result %d, Rust %d", got, want)
				}
				if ctx.Failed() || ctx.Mark() != mark {
					t.Fatal("callback frame/panic state")
				}
			})
		})
	}
}
