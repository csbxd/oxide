//go:build memory.counters

package rendererfixture_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"modernc.org/libc"
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
		t.Fatal("no native API references")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	ctx := oxide.NewContext()
	defer ctx.Close()
	for _, item := range cases {
		t.Run(item.Name, func(t *testing.T) {
			mark := ctx.Mark()
			defer ctx.Restore(mark)
			want, err := os.ReadFile(filepath.Join("reference", "api", item.File))
			if err != nil {
				t.Fatal(err)
			}
			source := ctx.CopyString(item.Source)
			path := ctx.CopyString(filepath.Join(t.TempDir(), "scratch"))
			prepareAPIPath(t, item.Name, item.Source, path.String())
			frame := ctx.Mark()
			buffer := make([]byte, 0, len(want))
			invoke := func() []byte {
				defer ctx.Restore(frame)
				return directAPI(ctx, item.Name, source, path, buffer[:0])
			}
			check := func(got []byte) {
				checkFrame(t, ctx, frame)
				if apiFileOutput(item.Name) {
					if len(got) != 0 {
						t.Fatalf("writer returned an error: %s", got)
					}
					var err error
					got, err = os.ReadFile(path.String())
					if err != nil {
						t.Fatal(err)
					}
				}
				compareBytes(t, item.Name, got, want)
				// Native observers reset their path before every call. Do this
				// outside the allocation window and reject stale-file success.
				if apiFileOutput(item.Name) {
					if err := os.Remove(path.String()); err != nil {
						t.Fatal(err)
					}
				}
			}
			check(invoke())
			heapBaseline := oxide.HeapStats()
			libcBaseline := libc.MemStat()
			filesBaseline := fileMappings(t)
			var before, after runtime.MemStats
			for call := 0; call < 3; call++ {
				runtime.ReadMemStats(&before)
				got := invoke()
				runtime.ReadMemStats(&after)
				check(got)
				if heap := oxide.HeapStats(); heap.LiveAllocations != heapBaseline.LiveAllocations {
					t.Fatalf("call %d: leaked Rust owners: %+v -> %+v", call, heapBaseline, heap)
				}
				if state := libc.MemStat(); state != libcBaseline {
					t.Fatalf("call %d: libc ownership changed: %+v -> %+v", call, libcBaseline, state)
				}
				if files := fileMappings(t); !maps.Equal(files, filesBaseline) {
					t.Fatalf("call %d: file mappings changed: %v -> %v", call, filesBaseline, files)
				}
				if objects, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc; objects != 0 || bytes != 0 {
					t.Errorf("call %d: %d Go objects, %d Go bytes", call, objects, bytes)
				}
			}
		})
	}
}

func apiFileOutput(name string) bool {
	return name == "write_svg" || name == "write_png" || name == "layout_dump" || name == "layered_dump"
}

func prepareAPIPath(t *testing.T, name, source, path string) {
	t.Helper()
	switch name {
	case "config_file", "config_json_error", "config_theme":
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	case "write_svg_error", "write_png_error", "layout_dump_error", "layered_dump_error":
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
}
