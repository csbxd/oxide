//go:build memory.counters

package rendererfixture

import (
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"modernc.org/libc"
)

// Fixed native references give the randomized ownership test a result oracle
// as well as a memory oracle. No renderer behavior is implemented here.
func TestRendererOwnershipChaos(t *testing.T) {
	checkFileMappingOracle(t)
	seed := int64(0x72656e646572)
	if s := os.Getenv("OXIDE_CHAOS_SEED"); s != "" {
		v, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		seed = v
	}
	epochs := 4
	if s := os.Getenv("OXIDE_CHAOS_EPOCHS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 1000 {
			t.Fatalf("invalid OXIDE_CHAOS_EPOCHS %q", s)
		}
		epochs = v
	}
	type sample struct {
		name             string
		source, svg, png []byte
	}
	var samples []sample
	for _, name := range []string{"flowchart", "sequence", "class", "upstream/architecture/basic", "upstream/pie/basic", "upstream/mindmap/basic"} {
		read := func(path string) []byte {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		samples = append(samples, sample{name, read(filepath.Join("cases", name+".mmd")), read(filepath.Join("reference", name+".svg")), read(filepath.Join("reference", name+".png"))})
	}
	outputPath := filepath.Join(t.TempDir(), "chaos.png")
	invoke := func(c *oxide.Context, s sample, png bool) {
		mark := c.Mark()
		defer c.Restore(mark)
		input := putBytes(c, s.source)
		if png {
			if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			path := putBytes(c, []byte(outputPath))
			frame := c.Mark()
			WritePng(c, input, uintptr(len(s.source)), path, uintptr(len(outputPath)))
			checkFrame(t, c, frame)
			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			compareBytes(t, s.name+" PNG", got, s.png)
		} else {
			output := c.Alloc(uintptr(len(s.svg)), 1)
			frame := c.Mark()
			n := RenderSvg(c, input, uintptr(len(s.source)), output, uintptr(len(s.svg)))
			checkFrame(t, c, frame)
			if n != uintptr(len(s.svg)) {
				t.Fatalf("%s SVG size %d, want %d", s.name, n, len(s.svg))
			}
			compareBytes(t, s.name+" SVG", unsafe.Slice((*byte)(unsafe.Pointer(output)), len(s.svg)), s.svg)
		}
	}
	// regex-automata 0.4.15's Pool has an owner cache plus eight stacks selected
	// by logical thread ID modulo 8. A new Context represents a new Rust thread.
	// Touch every tested path from nine consecutive Contexts, covering the owner
	// and every stack, before fixing the baseline. Do not adapt the baseline to
	// growth during the measured randomized epochs.
	for warm := 0; warm < 9; warm++ {
		c := oxide.NewContext()
		for _, s := range samples {
			invoke(c, s, false)
			invoke(c, s, true)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("warm-context=%d heap=%+v", warm, oxide.HeapStats())
	}
	baseline := oxide.HeapStats()
	libcBaseline := libc.MemStat()
	fileBaseline := fileMappings(t)
	t.Logf("baseline heap=%+v libc=%+v file-mappings=%v", baseline, libcBaseline, fileBaseline)
	r := rand.New(rand.NewSource(seed))
	for epoch := 0; epoch < epochs; epoch++ {
		shared := oxide.NewContext()
		for operation := 0; operation < 12; operation++ {
			c := shared
			fresh := r.Intn(3) == 0
			if fresh {
				c = oxide.NewContext()
			}
			invoke(c, samples[r.Intn(len(samples))], r.Intn(3) == 0)
			if fresh {
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := shared.Close(); err != nil {
			t.Fatal(err)
		}
		got := oxide.HeapStats()
		if got.LiveAllocations != baseline.LiveAllocations {
			t.Fatalf("seed=%#x epoch=%d leaked Rust owners: %+v -> %+v", seed, epoch, baseline, got)
		}
		if got.MappedBytes > baseline.MappedBytes+(4<<20) {
			t.Fatalf("seed=%#x epoch=%d exceeds live baseline plus bounded cache: %+v -> %+v", seed, epoch, baseline, got)
		}
		libcGot := libc.MemStat()
		if libcGot != libcBaseline {
			t.Fatalf("seed=%#x epoch=%d libc ownership changed: %+v -> %+v", seed, epoch, libcBaseline, libcGot)
		}
		if files := fileMappings(t); !maps.Equal(files, fileBaseline) {
			t.Fatalf("seed=%#x epoch=%d file mappings changed: %v -> %v", seed, epoch, fileBaseline, files)
		}
		t.Logf("seed=%#x epoch=%d heap=%+v libc=%+v", seed, epoch, got, libcGot)
	}
}

// fontdb uses memmap2 for font files. Those mappings bypass both allocators.
// Sum bytes per device/inode; VMA counts are unstable because Linux can merge
// adjacent mappings. Anonymous Go heap and Context mappings are separate.
func fileMappings(t *testing.T) map[string]uint64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]uint64)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			t.Fatalf("invalid maps line %q", line)
		}
		if fields[4] == "0" {
			continue
		}
		start, end, ok := strings.Cut(fields[0], "-")
		lo, e1 := strconv.ParseUint(start, 16, 64)
		hi, e2 := strconv.ParseUint(end, 16, 64)
		if !ok || e1 != nil || e2 != nil || hi <= lo {
			t.Fatalf("invalid maps range %q", fields[0])
		}
		result[fields[3]+":"+fields[4]] += hi - lo
	}
	return result
}

func checkFileMappingOracle(t *testing.T) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mapping-oracle-")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	size := syscall.Getpagesize()
	if err := f.Truncate(int64(size)); err != nil {
		t.Fatal(err)
	}
	before := fileMappings(t)
	b, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if b != nil {
			_ = syscall.Munmap(b)
		}
	}()
	if maps.Equal(before, fileMappings(t)) {
		t.Fatal("file mapping oracle missed intentionally retained mmap")
	}
	if err := syscall.Munmap(b); err != nil {
		t.Fatal(err)
	}
	b = nil
	if got := fileMappings(t); !maps.Equal(before, got) {
		t.Fatalf("file mapping cleanup did not restore baseline: %v -> %v", before, got)
	}
	t.Log("negative control: retained file mapping detected; munmap restored baseline")
}
