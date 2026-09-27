//go:build linux && (amd64 || arm64)

package oxide

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// Run separately from other mmap tests. Successful Globals and ConstAlloc have
// static lifetime; this test only requires failed construction to release its
// mapping. The deliberately retained successful image is the leak oracle's
// negative control, and is explicitly unmapped by the test afterwards.
func TestGlobalsFailureChaos(t *testing.T) {
	if os.Getenv("OXIDE_GLOBALS_CHAOS_CHILD") == "1" {
		globalsFailureChaos(t)
		return
	}
	seeds := []string{"1", "0x9e3779b97f4a7c15", "0xffffffffffffffff"}
	if seed := os.Getenv("OXIDE_CHAOS_SEED"); seed != "" {
		seeds = []string{seed}
	}
	for _, seed := range seeds {
		t.Run(seed, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestGlobalsFailureChaos$", "-test.v")
			cmd.Env = append(os.Environ(), "OXIDE_GLOBALS_CHAOS_CHILD=1", "OXIDE_CHAOS_SEED="+seed)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("seed %s: %v\n%s", seed, err, out)
			}
			t.Logf("%s", out)
		})
	}
}

func globalsFailureChaos(t *testing.T) {
	seed, err := strconv.ParseUint(os.Getenv("OXIDE_CHAOS_SEED"), 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	steps := 48
	if s := os.Getenv("OXIDE_CHAOS_STEPS"); s != "" {
		steps, err = strconv.Atoi(s)
		if err != nil || steps <= 0 {
			t.Fatalf("invalid OXIDE_CHAOS_STEPS %q", s)
		}
	}
	rng := rand.New(rand.NewPCG(seed, seed^0xd1b54a32d192ed03))
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	page := syscall.Getpagesize()
	marker := make([]byte, 32)
	copy(marker, "oxide-global-chaos-marker")
	binary.LittleEndian.PutUint64(marker[24:], seed)
	data := make([]byte, page*3)
	copy(data, marker)
	image := allocationImage(testAllocation{1, 1, data, nil})
	before := globalsMapRanges(t)
	g := LoadAllocations(image, nil, nil)
	control := globalsNewMarkerPages(t, mem, before, marker)
	p := uintptr(unsafe.Pointer(unsafe.SliceData(g.mapping)))
	if len(control) != 1 || control[0] != p || !globalsPageMapped(t, p) {
		t.Fatalf("negative control missed retained static mapping %#x: %x", p, control)
	}
	if err := syscall.Munmap(g.mapping); err != nil {
		t.Fatal(err)
	}
	if globalsPageMapped(t, p) || len(globalsNewMarkerPages(t, mem, before, marker)) != 0 {
		t.Fatal("negative control did not return to its mapping baseline")
	}
	t.Log("negative control: retained mapping detected; explicit cleanup verified by mincore")

	for step := range steps {
		binary.LittleEndian.PutUint64(marker[24:], rng.Uint64())
		data = make([]byte, page*(1+rng.IntN(13)))
		copy(data, marker)
		missing := uint64(10 + rng.IntN(1000000))
		aliases := []AllocationAlias{{3, 2}} // A defined address of zero is valid.
		var relocs [][2]uint64
		kind := step % 6
		switch kind {
		case 0:
			aliases = append(aliases, AllocationAlias{4, missing})
		case 1:
			aliases = append(aliases, AllocationAlias{4, 5}, AllocationAlias{5, 4})
		case 2:
			aliases = append(aliases, AllocationAlias{4, 4})
		case 3:
			relocs = [][2]uint64{{32, missing}}
		case 4:
			relocs = [][2]uint64{{32, 1}, {40, missing}}
		case 5:
			aliases = append(aliases, AllocationAlias{4, 5}, AllocationAlias{5, 6}, AllocationAlias{6, missing})
		}
		image = allocationImage(testAllocation{1, 1, data, relocs})
		before = globalsMapRanges(t)
		var failure any
		func() {
			defer func() { failure = recover() }()
			LoadAllocations(image, []AddressAllocation{{ID: 2}}, aliases)
		}()
		if failure == nil {
			t.Fatalf("seed %#x step %d kind %d accepted malformed globals", seed, step, kind)
		}
		message := "cyclic or missing oxide allocation alias"
		if len(relocs) != 0 {
			message = "missing oxide global"
		}
		if !strings.Contains(fmt.Sprint(failure), message) {
			t.Fatalf("seed %#x step %d kind %d unexpected panic: %v", seed, step, kind, failure)
		}
		if leaked := globalsNewMarkerPages(t, mem, before, marker); len(leaked) != 0 {
			for _, address := range leaked {
				if !globalsPageMapped(t, address) {
					t.Fatal("mapping oracle and mincore disagree")
				}
			}
			t.Fatalf("seed %#x step %d kind %d leaked %d static mapping(s): %x", seed, step, kind, len(leaked), leaked)
		}
	}
	t.Logf("seed %#x: %d malformed alias/relocation loads returned to mapping baseline", seed, steps)
}

type globalsMapRange struct {
	start, end uintptr
	anonymous  bool
}

func globalsMapRanges(t *testing.T) []globalsMapRange {
	t.Helper()
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatal(err)
	}
	var ranges []globalsMapRange
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			t.Fatalf("invalid maps line %q", line)
		}
		pair := strings.SplitN(fields[0], "-", 2)
		if len(pair) != 2 {
			t.Fatalf("invalid maps address %q", fields[0])
		}
		start, e1 := strconv.ParseUint(pair[0], 16, 64)
		end, e2 := strconv.ParseUint(pair[1], 16, 64)
		if e1 != nil || e2 != nil || end <= start {
			t.Fatalf("invalid maps range %q", fields[0])
		}
		ranges = append(ranges, globalsMapRange{uintptr(start), uintptr(end), len(fields) == 5 && fields[1] == "rw-p" && fields[4] == "0"})
	}
	return ranges
}

// Subtract address intervals, not VMA counts: adjacent anonymous mappings can
// merge. Read only a marker-sized prefix at each newly mapped page through
// /proc/self/mem; no arbitrary Go pointer dereference or RSS inference is used.
func globalsNewMarkerPages(t *testing.T, mem *os.File, before []globalsMapRange, marker []byte) []uintptr {
	t.Helper()
	var found []uintptr
	var prefix [32]byte
	page := uintptr(syscall.Getpagesize())
	visited := 0
	inspect := func(start, end uintptr) {
		for p := start; p < end; p += page {
			visited++
			if visited > 1<<20 {
				t.Fatal("unexpectedly large new mapping range in isolated child")
			}
			if n, err := mem.ReadAt(prefix[:], int64(p)); err == nil && n == len(prefix) && bytes.Equal(prefix[:], marker) {
				found = append(found, p)
			}
		}
	}
	for _, current := range globalsMapRanges(t) {
		if !current.anonymous {
			continue
		}
		cursor := current.start
		for _, old := range before {
			if old.end <= cursor {
				continue
			}
			if old.start >= current.end {
				break
			}
			if old.start > cursor {
				inspect(cursor, min(old.start, current.end))
			}
			cursor = max(cursor, old.end)
			if cursor >= current.end {
				break
			}
		}
		if cursor < current.end {
			inspect(cursor, current.end)
		}
	}
	return found
}

func globalsPageMapped(t *testing.T, address uintptr) bool {
	t.Helper()
	var resident byte
	_, _, err := syscall.RawSyscall(syscall.SYS_MINCORE, address, uintptr(syscall.Getpagesize()), uintptr(unsafe.Pointer(&resident)))
	switch err {
	case 0:
		return true
	case syscall.ENOMEM:
		return false
	default:
		t.Fatal(fmt.Errorf("mincore(%#x): %w", address, err))
		return false
	}
}
