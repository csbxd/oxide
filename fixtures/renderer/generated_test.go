package rendererfixture

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

// This file only invokes the translated scalar test ABI and compares bytes.
// All rendering and PNG encoding are performed by generated Rust code.
func TestRendererExactGoHeap(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	for _, format := range []string{"svg", "png"} {
		for _, name := range []string{"flowchart", "sequence", "class"} {
			t.Run(format+"/"+name, func(t *testing.T) {
				source, err := os.ReadFile(filepath.Join("cases", name+".mmd"))
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join("reference", name+"."+format))
				if err != nil {
					t.Fatal(err)
				}
				ctx := oxide.NewContext()
				defer ctx.Close()
				input := putBytes(ctx, source)
				var output, pathPointer uintptr
				var storage []byte
				var path string
				if format == "svg" {
					output = ctx.Alloc(uintptr(len(want)), 1)
					storage = unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want))
				} else {
					path = filepath.Join(t.TempDir(), name+".png")
					pathPointer = putBytes(ctx, []byte(path))
				}
				frame := ctx.Mark()
				invoke := func() uintptr {
					if format == "svg" {
						return Fixture_RenderSvg(ctx, input, uintptr(len(source)), output, uintptr(len(storage)))
					}
					Fixture_WritePng(ctx, input, uintptr(len(source)), pathPointer, uintptr(len(path)))
					return uintptr(len(want))
				}
				check := func(size uintptr) {
					if size != uintptr(len(want)) {
						t.Fatalf("length: translated %d, native Rust %d", size, len(want))
					}
					checkFrame(t, ctx, frame)
					got := storage
					if format == "png" {
						got, err = os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
					}
					compareBytes(t, format, got, want)
				}
				check(invoke())
				var before, after runtime.MemStats
				for i := 0; i < 3; i++ {
					runtime.ReadMemStats(&before)
					size := invoke()
					runtime.ReadMemStats(&after)
					allocations, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc
					check(size)
					t.Logf("call %d: exact Go allocations=%d bytes=%d", i, allocations, bytes)
					if allocations != 0 || bytes != 0 {
						t.Errorf("warmed rendering allocated %d Go objects (%d bytes)", allocations, bytes)
					}
				}
			})
		}
	}
}

func TestRendererMatchesNativeRust(t *testing.T) {
	var cases []string
	err := filepath.WalkDir("cases", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".mmd" {
			name, err := filepath.Rel("cases", path)
			if err != nil {
				return err
			}
			cases = append(cases, strings.TrimSuffix(name, ".mmd"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no renderer cases")
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("cases", name+".mmd"))
			if err != nil {
				t.Fatal(err)
			}
			mark := ctx.Mark()
			defer ctx.Restore(mark)
			input := putBytes(ctx, source)
			inputFrame := ctx.Mark()
			want, err := os.ReadFile(filepath.Join("reference", name+".svg"))
			if err != nil {
				t.Fatal(err)
			}
			output := ctx.Alloc(uintptr(len(want)), 1)
			frame := ctx.Mark()
			storage := unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want))
			for i := range storage {
				storage[i] = 0xa5
			}
			size := Fixture_RenderSvg(ctx, input, uintptr(len(source)), output, uintptr(len(storage)))
			checkFrame(t, ctx, frame)
			if size != uintptr(len(want)) {
				t.Fatalf("SVG length: translated %d, native Rust %d", size, len(want))
			}
			actualSVG := filepath.Join("actual", name+".svg")
			if err := os.MkdirAll(filepath.Dir(actualSVG), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(actualSVG, storage, 0o644); err != nil {
				t.Fatal(err)
			}
			compareBytes(t, "SVG", storage, want)
			ctx.Restore(inputFrame)

			outputPNG, err := filepath.Abs(filepath.Join("actual", name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(outputPNG); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			path := putBytes(ctx, []byte(outputPNG))
			frame = ctx.Mark()
			Fixture_WritePng(ctx, input, uintptr(len(source)), path, uintptr(len(outputPNG)))
			checkFrame(t, ctx, frame)
			ctx.Restore(inputFrame)
			got, err := os.ReadFile(outputPNG)
			if err != nil {
				t.Fatal(err)
			}
			want, err = os.ReadFile(filepath.Join("reference", name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			compareBytes(t, "PNG", got, want)
		})
	}
}

func BenchmarkRendererSVG(b *testing.B) {
	for _, name := range []string{"flowchart", "sequence", "class"} {
		b.Run(name, func(b *testing.B) {
			b.StopTimer()
			source, err := os.ReadFile(filepath.Join("cases", name+".mmd"))
			if err != nil {
				b.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("reference", name+".svg"))
			if err != nil {
				b.Fatal(err)
			}
			ctx := oxide.NewContext()
			defer ctx.Close()
			input := putBytes(ctx, source)
			output := ctx.Alloc(uintptr(len(want)), 1)
			storage := unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want))
			frame := ctx.Mark()
			if size := Fixture_RenderSvg(ctx, input, uintptr(len(source)), output, uintptr(len(storage))); size != uintptr(len(want)) {
				b.Fatalf("SVG length: translated %d, native Rust %d", size, len(want))
			}
			checkFrame(b, ctx, frame)
			compareBytes(b, "SVG", storage, want)
			b.ReportAllocs()
			b.SetBytes(int64(len(want)))
			var before, after runtime.MemStats
			b.ResetTimer()
			runtime.ReadMemStats(&before)
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				if size := Fixture_RenderSvg(ctx, input, uintptr(len(source)), output, uintptr(len(storage))); size != uintptr(len(want)) {
					b.Fatalf("SVG length: translated %d, native Rust %d", size, len(want))
				}
				checkFrame(b, ctx, frame)
			}
			b.StopTimer()
			runtime.ReadMemStats(&after)
			reportExactAllocations(b, before, after)
			compareBytes(b, "SVG", storage, want)
		})
	}
}

// PNG measurements include the translated SVG renderer, PNG encoder and file write.
func BenchmarkRendererPNG(b *testing.B) {
	for _, name := range []string{"flowchart", "sequence", "class"} {
		b.Run(name, func(b *testing.B) {
			b.StopTimer()
			source, err := os.ReadFile(filepath.Join("cases", name+".mmd"))
			if err != nil {
				b.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("reference", name+".png"))
			if err != nil {
				b.Fatal(err)
			}
			output := filepath.Join(b.TempDir(), name+".png")
			ctx := oxide.NewContext()
			defer ctx.Close()
			input := putBytes(ctx, source)
			path := putBytes(ctx, []byte(output))
			frame := ctx.Mark()
			Fixture_WritePng(ctx, input, uintptr(len(source)), path, uintptr(len(output)))
			checkFrame(b, ctx, frame)
			got, err := os.ReadFile(output)
			if err != nil {
				b.Fatal(err)
			}
			compareBytes(b, "PNG", got, want)
			b.ReportAllocs()
			b.SetBytes(int64(len(want)))
			var before, after runtime.MemStats
			b.ResetTimer()
			runtime.ReadMemStats(&before)
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				Fixture_WritePng(ctx, input, uintptr(len(source)), path, uintptr(len(output)))
				checkFrame(b, ctx, frame)
			}
			b.StopTimer()
			runtime.ReadMemStats(&after)
			reportExactAllocations(b, before, after)
			got, err = os.ReadFile(output)
			if err != nil {
				b.Fatal(err)
			}
			compareBytes(b, "PNG", got, want)
		})
	}
}

func reportExactAllocations(b *testing.B, before, after runtime.MemStats) {
	// testing's B/op and allocs/op, including AllocsPerRun, use integer division.
	// Totals retain even a single allocation across a long benchmark run.
	b.ReportMetric(float64(after.Mallocs-before.Mallocs), "Go-allocs-total")
	b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc), "Go-bytes-total")
}

func putBytes(ctx *oxide.Context, data []byte) uintptr {
	pointer := ctx.Alloc(uintptr(len(data)), 1)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(pointer)), len(data)), data)
	return pointer
}

func checkFrame(t testing.TB, ctx *oxide.Context, mark oxide.Mark) {
	if ctx.Failed() {
		t.Helper()
		t.Fatal("translated renderer left an uncaught Rust panic")
	}
	if ctx.Mark() != mark {
		t.Helper()
		t.Fatal("rendering leaked Rust automatic storage")
	}
}

func compareBytes(t testing.TB, format string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	first := 0
	for first < len(got) && first < len(want) && got[first] == want[first] {
		first++
	}
	t.Fatal(fmt.Sprintf("%s differs at byte %d; translated length %d, native Rust length %d (see actual/ and reference/)", format, first, len(got), len(want)))
}
