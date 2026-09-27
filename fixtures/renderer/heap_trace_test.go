//go:build oxide.heaptrace && memory.counters

package rendererfixture_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func TestRendererHeapTrace(t *testing.T) {
	name, format := os.Getenv("OXIDE_TRACE_CASE"), os.Getenv("OXIDE_TRACE_FORMAT")
	if name == "" {
		name = "class"
	}
	if format == "" {
		format = "svg"
	}
	if format != "svg" && format != "png" {
		t.Fatal("OXIDE_TRACE_FORMAT must be svg or png")
	}
	source, err := os.ReadFile(filepath.Join("cases", name+".mmd"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("reference", name+"."+format))
	if err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs(filepath.Join(t.TempDir(), "trace.png"))
	if err != nil {
		t.Fatal(err)
	}
	render := func() (oxide.HeapSnapshot, oxide.HeapSnapshot) {
		ctx := oxide.NewContext()
		input := ctx.CopyBytes(source)
		if format == "svg" {
			output := ctx.Alloc(uintptr(len(want)), 1)
			frame := ctx.Mark()
			n := renderSVG(ctx, input, unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want)))
			if n != uintptr(len(want)) {
				t.Fatalf("SVG length %d want %d", n, len(want))
			}
			checkFrame(t, ctx, frame)
			compareBytes(t, "SVG", unsafe.Slice((*byte)(unsafe.Pointer(output)), len(want)), want)
		} else {
			output := ctx.CopyString(path)
			frame := ctx.Mark()
			writePNG(ctx, input, output)
			checkFrame(t, ctx, frame)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			compareBytes(t, "PNG", got, want)
		}
		beforeClose := oxide.HeapStats()
		if err := ctx.Close(); err != nil {
			t.Fatal(err)
		}
		return beforeClose, oxide.HeapStats()
	}
	render()
	baseline := oxide.HeapStats()
	oxide.StartHeapTrace()
	defer oxide.StopHeapTrace()
	beforeClose, afterClose := render()
	oxide.StopHeapTrace()
	records := oxide.HeapTraceSnapshot()
	type group struct {
		Count int
		Bytes uintptr
		Size  uintptr
		Stack []string
	}
	groups := map[string]*group{}
	names := map[string]map[string]string{}
	for _, record := range records {
		var stack []string
		n := 0
		for n < len(record.PCs) && record.PCs[n] != 0 {
			n++
		}
		frames := runtime.CallersFrames(record.PCs[:n])
		for {
			f, more := frames.Next()
			fn := f.Function
			if strings.HasPrefix(filepath.Base(f.File), "oxide_gen_") {
				lookup, ok := names[f.File]
				if !ok {
					lookup = map[string]string{}
					data, err := os.ReadFile(f.File)
					if err != nil {
						t.Fatal(err)
					}
					comment := ""
					for _, line := range strings.Split(string(data), "\n") {
						if strings.HasPrefix(line, "// ") {
							comment = strings.TrimPrefix(line, "// ")
						}
						if strings.HasPrefix(line, "func f") {
							if at := strings.IndexByte(line, '('); at >= 0 {
								lookup[line[5:at]] = comment
							}
						}
					}
					names[f.File] = lookup
				}
				short := fn[strings.LastIndexByte(fn, '.')+1:]
				if rust := lookup[short]; rust != "" {
					fn = rust + " [" + short + "]"
				}
			}
			stack = append(stack, fmt.Sprintf("%s (%s:%d)", fn, filepath.Base(f.File), f.Line))
			if !more {
				break
			}
		}
		key := fmt.Sprint(record.Size) + "\n" + strings.Join(stack, "\n")
		g := groups[key]
		if g == nil {
			g = &group{Size: record.Size, Stack: stack}
			groups[key] = g
		}
		g.Count++
		g.Bytes += record.Size
	}
	var ordered []*group
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Bytes != ordered[j].Bytes {
			return ordered[i].Bytes > ordered[j].Bytes
		}
		return strings.Join(ordered[i].Stack, "\n") < strings.Join(ordered[j].Stack, "\n")
	})
	report := struct {
		Case, Format                      string
		Baseline, BeforeClose, AfterClose oxide.HeapSnapshot
		TrackedLive                       int
		Groups                            []*group
	}{name, format, baseline, beforeClose, afterClose, len(records), ordered}
	output := os.Getenv("OXIDE_HEAP_TRACE_REPORT")
	if output == "" {
		output = "heap-trace.json"
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("case=%s format=%s before=%+v before-close=%+v after-close=%+v tracked-live=%d report=%s", name, format, baseline, beforeClose, afterClose, len(records), output)
}
