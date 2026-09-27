//go:build memory.counters

// Direct upstream return ownership. No Rust test observer is translated.
package rendererfixture_test

import (
	. "oxide-renderer-conformance"
	"runtime"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"modernc.org/libc"
)

type ownedInputs struct{ valid, invalid oxide.Span }
type ownedObservation struct {
	before, retained, after            oxide.HeapSnapshot
	frame, returnedFrame, droppedFrame oxide.Mark
	failed                             bool
}
type ownedCase struct {
	name, drop string
	invoke     func(*oxide.Context, ownedInputs) ownedObservation
}

func ownedInput(ctx *oxide.Context) ownedInputs {
	return ownedInputs{ctx.CopyString("flowchart LR\n A[Alpha] -->|go| B{Beta}\n"), ctx.CopyString("not a diagram")}
}

func ownedFrame(t *testing.T, item ownedCase, ctx *oxide.Context, mark oxide.Mark, result ownedObservation) {
	t.Helper()
	if result.failed || result.droppedFrame != result.returnedFrame || ctx.Mark() != mark {
		t.Fatalf("%s: return/drop did not restore the Context frame or panic state", item.name)
	}
}

func TestPublicOwnedReturns(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	// Fixed warm-up, never an adaptive baseline: regex's owner plus eight
	// thread-ID shards. Every path runs from each of nine fresh Contexts.
	for warm := 0; warm < 9; warm++ {
		func() {
			ctx := oxide.NewContext()
			defer func() {
				if err := ctx.Close(); err != nil {
					t.Error(err)
				}
			}()
			input := ownedInput(ctx)
			mark := ctx.Mark()
			for _, item := range ownedCases {
				result := item.invoke(ctx, input)
				ownedFrame(t, item, ctx, mark, result)
			}
		}()
	}
	baseline := oxide.HeapStats()
	libcBaseline := libc.MemStat()
	t.Logf("fixed baseline: heap=%+v libc=%+v", baseline, libcBaseline)
	for _, item := range ownedCases {
		t.Run(item.name, func(t *testing.T) {
			ctx := oxide.NewContext()
			defer func() {
				if err := ctx.Close(); err != nil {
					t.Error(err)
				}
				if got := oxide.HeapStats(); got.LiveAllocations != baseline.LiveAllocations {
					t.Errorf("%s: after Context.Close live Rust owners %+v, baseline %+v", item.name, got, baseline)
				}
				if got := libc.MemStat(); got != libcBaseline {
					t.Errorf("%s: after Context.Close libc %+v, baseline %+v", item.name, got, libcBaseline)
				}
			}()
			input := ownedInput(ctx)
			mark := ctx.Mark()
			// Warm this Context's frame/TLS setup outside the Go allocation
			// window. Its closure must still match the fixed process baseline.
			warm := item.invoke(ctx, input)
			ownedFrame(t, item, ctx, mark, warm)
			localBaseline := oxide.HeapStats().LiveAllocations
			for call := 0; call < 3; call++ {
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				result := item.invoke(ctx, input)
				runtime.ReadMemStats(&after)
				ownedFrame(t, item, ctx, mark, result)
				if result.before.LiveAllocations != localBaseline || result.after.LiveAllocations != localBaseline {
					t.Fatalf("call %d: %s failed to release returned owner: before=%+v retained=%+v after=%+v baseline=%d", call, item.drop, result.before, result.retained, result.after, localBaseline)
				}
				if result.retained.LiveAllocations <= result.before.LiveAllocations {
					t.Fatalf("call %d: ownership oracle observed no live returned allocation: %+v -> %+v", call, result.before, result.retained)
				}
				allocations, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc
				if allocations != 0 || bytes != 0 {
					t.Fatalf("call %d: %d Go allocations, %d Go bytes", call, allocations, bytes)
				}
				t.Logf("call=%d type=%s retained=%d released=%d raw-Go-objects=%d raw-Go-bytes=%d", call, item.drop, result.retained.LiveAllocations-result.before.LiveAllocations, result.retained.LiveAllocations-result.after.LiveAllocations, allocations, bytes)
			}
		})
	}
}

var ownedCases = []ownedCase{
	{"theme", "theme", owned_theme},
	{"config", "config", owned_config},
	{"options", "options", owned_options},
	{"parse_ok", "parse_ok", owned_parse_ok},
	{"parse_error", "parse_error", owned_parse_error},
	{"render_ok", "render_ok", owned_render_ok},
	{"render_error", "render_error", owned_render_error},
	{"scene", "scene", owned_scene},
}

func owned_theme(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := Theme_Dark(ctx)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_config(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := TypeConfig.Default(ctx)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_options(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := RenderOptions_Modern(ctx)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_parse_ok(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := ParseMermaid(ctx, input.valid)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_parse_error(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := ParseMermaid(ctx, input.invalid)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_render_ok(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := Render(ctx, input.valid)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_render_error(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := Render(ctx, input.invalid)
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}

func owned_scene(ctx *oxide.Context, input ownedInputs) ownedObservation {
	mark := ctx.Mark()
	before := oxide.HeapStats()
	value := RenderScene(ctx, input.valid, RenderOptions_Modern(ctx))
	retained := oxide.HeapStats()
	valueFrame := ctx.Mark()
	value.Drop(ctx)
	droppedFrame := ctx.Mark()
	after := oxide.HeapStats()
	ctx.Restore(mark)
	return ownedObservation{before, retained, after, mark, valueFrame, droppedFrame, ctx.Failed()}
}
