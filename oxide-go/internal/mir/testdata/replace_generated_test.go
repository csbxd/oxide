package fixture

import (
	"fmt"
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"os"
	"strings"
	"testing"
)

var replacementTarget, replacementSource oxide.Value

func replacementCallback(ctx *oxide.Context) (unit oxideReplaceUnit) {
	// The actual Rust catch_unwind owns and releases the forwarded exception.
	defer func() {
		if value := recover(); value != nil {
			ctx.Fail(value)
		}
	}()
	replacementTarget.Replace(ctx, replacementSource)
	return
}

func doubleReplacementCallback(ctx *oxide.Context) (unit oxideReplaceUnit) {
	// Both panics occur inside the Rust drop glue, including its real cleanup
	// edge; a Go defer would instead have Go's foreign cleanup semantics.
	target := MakeFieldPanic(ctx)
	source := MakeOwner(ctx, 2, false)
	target.Replace(ctx, source)
	return
}

func TestReplaceAssignmentMatchesNative(t *testing.T) {
	expected, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	Setup(ctx)
	mark := ctx.Mark()
	baseline := oxide.HeapStats().LiveAllocations
	for _, line := range strings.Split(strings.TrimSpace(string(expected)), "\n") {
		var id, after, want, final, panicked uint64
		if _, err := fmt.Sscanf(line, "%d %d %d %d %d", &id, &after, &want, &final, &panicked); err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			requireNoGoAllocations(t, 100, func() {
				Reset(ctx)
				if id == 0 {
					left, right := MakeZero(ctx), MakeZero(ctx)
					if left.Type.Size != 0 || right.Type != left.Type {
						t.Fatal("ZST fixture type")
					}
					right.Addr = left.Addr // Distinct ZST owners legally share storage.
					frame := ctx.Mark()
					left.Replace(ctx, right)
					if State(ctx) != after || ctx.Mark() != frame {
						t.Fatal("ZST replacement skipped old Drop or leaked its snapshot")
					}
					left.Drop(ctx)
				} else {
					replacementTarget = MakeOwner(ctx, 1, panicked != 0)
					replacementSource = MakeOwner(ctx, 2, false)
					sourceID := replacementSource.Field("id")
					if id >= 3 {
						SourceSlot(ctx, replacementSource.Addr)
					}
					frame := ctx.Mark()
					if caught := CatchCallback(ctx, oxide.FunctionPointer(replacementCallback)); caught != (panicked != 0) {
						t.Fatal("replacement panic payload")
					}
					SourceSlot(ctx, 0)
					if ctx.Mark() != frame || ctx.Failed() {
						t.Fatal("replace snapshot/panic state leaked")
					}
					if State(ctx) != after || Inspect(ctx, replacementTarget) != want {
						t.Fatal("assignment did not install the evaluated RHS")
					}
					// Inspect only the raw scalar slot newly written by the
					// destructor, never the already-consumed source Owner.
					if id >= 3 && (oxide.Value{Addr: sourceID.Addr, Type: sourceID.Type}).Uint() != 99 {
						t.Fatal("old destructor did not exercise source-slot mutation")
					}
					replacementTarget.Drop(ctx)
				}
				if State(ctx) != final {
					t.Fatalf("final drop state %d, native %d", State(ctx), final)
				}
				if oxide.HeapStats().LiveAllocations != baseline {
					t.Fatal("assignment leaked a Rust owner or panic payload")
				}
				ctx.Restore(mark)
				if ctx.Failed() {
					t.Fatal("uncaught replacement panic")
				}
			})
		})
	}
}

func TestReplaceDoublePanic(t *testing.T) {
	if os.Getenv("OXIDE_REPLACE_DOUBLE") != "1" {
		t.Skip("isolated abort probe")
	}
	ctx := oxide.NewContext()
	Setup(ctx)
	CatchCallback(ctx, oxide.FunctionPointer(doubleReplacementCallback))
	t.Fatal("double Rust panic unexpectedly returned")
}
