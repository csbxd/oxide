//go:build memory.counters && linux && (amd64 || arm64)

package oxide

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"testing"
	"unsafe"
)

func failNextHeapMapping() {
	rustHeap.mu.Lock()
	rustHeap.pages.FailNextMapping()
	rustHeap.mu.Unlock()
}

func TestHeapMappingFailurePreservesOwnership(t *testing.T) {
	p := RustAlloc(257, 65536)
	if p == 0 {
		t.Fatal("initial allocation failed")
	}
	defer func() { RustDealloc(p, 257, 65536) }()
	for i := uintptr(0); i < 257; i++ {
		*(*byte)(unsafe.Pointer(p + i)) = byte(i ^ 0xd3)
	}
	before := HeapStats()
	failNextHeapMapping()
	if q := RustRealloc(p, 257, 65536, 2<<20); q != 0 {
		RustDealloc(q, 2<<20, 65536)
		p = 0
		t.Fatal("injected realloc failure was ignored")
	}
	for i := uintptr(0); i < 257; i++ {
		if *(*byte)(unsafe.Pointer(p + i)) != byte(i^0xd3) {
			t.Fatalf("failed realloc changed byte %d", i)
		}
	}
	if allocationSize(p) != 257 {
		t.Fatal("failed realloc changed the allocation header")
	}
	if after := HeapStats(); after != before {
		t.Fatalf("failed realloc changed ownership: before=%+v after=%+v", before, after)
	}
	failNextHeapMapping()
	if q := RustAlloc(2<<20, 1<<20); q != 0 {
		RustDealloc(q, 2<<20, 1<<20)
		t.Fatal("injected allocation failure was ignored")
	}
	if after := HeapStats(); after != before {
		t.Fatalf("failed allocation changed ownership: before=%+v after=%+v", before, after)
	}
}

func heapChaosSeed(defaultSeed uint64) uint64 {
	if value := os.Getenv("OXIDE_CHAOS_SEED"); value != "" {
		seed, err := strconv.ParseUint(value, 0, 64)
		if err != nil {
			panic(err)
		}
		return seed
	}
	return defaultSeed
}

type heapChaosBlock struct {
	pointer, size, align uintptr
	pattern              uint64
}

func heapChaosByte(key uint64, i int) byte { return byte(key) ^ byte(i*131) ^ byte(i>>8) }

func heapChaosCheck(b heapChaosBlock, preserved uintptr, zero bool) error {
	for i, value := range unsafe.Slice((*byte)(unsafe.Pointer(b.pointer)), max(b.size, 1)) {
		want := byte(0)
		if !zero && uintptr(i) < preserved {
			want = heapChaosByte(b.pattern, i)
		}
		if value != want {
			return fmt.Errorf("pointer=%#x size=%d byte=%d got=%d want=%d", b.pointer, b.size, i, value, want)
		}
	}
	return nil
}

func heapChaosFill(b heapChaosBlock) {
	for i := range unsafe.Slice((*byte)(unsafe.Pointer(b.pointer)), max(b.size, 1)) {
		*(*byte)(unsafe.Pointer(b.pointer + uintptr(i))) = heapChaosByte(b.pattern, i)
	}
}

func heapChaosSize(r *rand.Rand) uintptr {
	if r.Uint64N(16) == 0 {
		return []uintptr{(1 << 20) - 33, (1 << 20) - 1, 1 << 20, (1 << 20) + 1, (2 << 20) + 1}[r.IntN(5)]
	}
	size := int64(1) << r.Uint64N(17)
	size += []int64{-33, -32, -31, -1, 0, 1}[r.IntN(6)]
	return uintptr(max(size, 0))
}

func heapChaosStep(r *rand.Rand, slots []heapChaosBlock) error {
	i := r.IntN(len(slots))
	old := slots[i]
	if old.pointer == 0 {
		size, align := heapChaosSize(r), uintptr(1)<<r.Uint64N(21)
		p := RustAlloc(size, align)
		if p == 0 || p%align != 0 || p%16 != 0 {
			return fmt.Errorf("alloc size=%d align=%d returned %#x", size, align, p)
		}
		b := heapChaosBlock{p, size, align, r.Uint64()}
		slots[i] = b
		if err := heapChaosCheck(b, 0, true); err != nil {
			return err
		}
		heapChaosFill(b)
		return nil
	}
	if err := heapChaosCheck(old, max(old.size, 1), false); err != nil {
		return err
	}
	switch r.IntN(4) {
	case 0:
		RustDealloc(old.pointer, old.size, old.align)
		slots[i] = heapChaosBlock{}
	case 1:
		before := allocationSize(old.pointer)
		if p := RustRealloc(old.pointer, old.size, old.align, ^uintptr(0)); p != 0 {
			return fmt.Errorf("invalid realloc unexpectedly succeeded")
		}
		if allocationSize(old.pointer) != before {
			return fmt.Errorf("failed realloc changed size")
		}
		return heapChaosCheck(old, max(old.size, 1), false)
	default:
		size := heapChaosSize(r)
		p := RustRealloc(old.pointer, old.size, old.align, size)
		if p == 0 {
			return fmt.Errorf("realloc size=%d align=%d failed", size, old.align)
		}
		b := heapChaosBlock{p, size, old.align, old.pattern}
		slots[i] = b
		if p%old.align != 0 || p%16 != 0 {
			return fmt.Errorf("realloc returned unaligned pointer %#x", p)
		}
		// Realloc(size=0) retains physical storage, but has no logical bytes.
		if size != 0 {
			if err := heapChaosCheck(b, min(old.size, size), false); err != nil {
				return err
			}
		}
		b.pattern = r.Uint64()
		slots[i] = b
		heapChaosFill(b)
	}
	return nil
}

func heapChaosRanges(slots [][]heapChaosBlock) (int, error) {
	var live []heapChaosBlock
	for _, worker := range slots {
		for _, b := range worker {
			if b.pointer != 0 {
				live = append(live, b)
			}
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].pointer < live[j].pointer })
	for i, b := range live {
		if i > 0 && live[i-1].pointer+max(live[i-1].size, 1) > b.pointer {
			return 0, fmt.Errorf("overlapping live allocations %#x and %#x", live[i-1].pointer, b.pointer)
		}
	}
	return len(live), nil
}

func heapChaosDrain(t *testing.T, r *rand.Rand, workers [][]heapChaosBlock) {
	t.Helper()
	for _, slots := range workers {
		for _, i := range r.Perm(len(slots)) {
			b := slots[i]
			if b.pointer == 0 {
				continue
			}
			if err := heapChaosCheck(b, max(b.size, 1), false); err != nil {
				t.Fatal(err)
			}
			RustDealloc(b.pointer, b.size, b.align)
			slots[i] = heapChaosBlock{}
		}
	}
}

func heapChaosQuiescent(t *testing.T) HeapSnapshot {
	t.Helper()
	s := HeapStats()
	if s.LiveAllocations != 0 || s.BackingAllocations != s.CachedAllocations || s.Mappings != s.CachedMappings || s.MappedBytes != s.CachedMappedBytes || s.MappedBytes >= 4<<20 {
		t.Fatalf("drained heap contains unexplained ownership: %+v", s)
	}
	return s
}

func heapChaosBoundaries(t *testing.T, baseline HeapSnapshot) {
	t.Helper()
	for log := 6; log <= heapCacheMaxLog+1; log++ {
		for _, align := range []uintptr{1, 16, 1 << 16, 1 << 20} {
			for _, delta := range []int64{-1, 0, 1} {
				// Cross the allocator's actual storage class, including its
				// header and the caller's alignment padding.
				n := int64(1<<log) - int64(align-1) - int64(allocationHeaderSize) + delta
				if n < 0 {
					continue
				}
				size := uintptr(n)
				p := RustAlloc(size, align)
				if p == 0 {
					t.Fatalf("boundary allocation: log=%d align=%d delta=%d", log, align, delta)
				}
				b := heapChaosBlock{p, size, align, uint64(log)}
				if err := heapChaosCheck(b, 0, true); err != nil {
					t.Fatal(err)
				}
				heapChaosFill(b)
				q := RustRealloc(p, size, align, size+2)
				if q == 0 {
					RustDealloc(p, size, align)
					t.Fatal("boundary realloc failed")
				}
				b.pointer, b.size = q, size+2
				if err := heapChaosCheck(b, size, false); err != nil {
					RustDealloc(q, size+2, align)
					t.Fatal(err)
				}
				RustDealloc(q, size+2, align)
				if s := heapChaosQuiescent(t); s != baseline {
					t.Fatalf("boundary drain: log=%d align=%d delta=%d stats=%+v", log, align, delta, s)
				}
			}
		}
	}
}

func TestHeapChaos(t *testing.T) {
	// Isolate the allocator oracle from legitimate process-lifetime objects
	// created by other runtime tests, including randomized test execution order.
	if os.Getenv("OXIDE_ALLOCATOR_CHAOS_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHeapChaos$", "-test.v", "-test.timeout=10m")
		cmd.Env = append(os.Environ(), "OXIDE_ALLOCATOR_CHAOS_CHILD=1")
		output, err := cmd.CombinedOutput()
		t.Logf("%s", output)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	for log := 6; log <= heapCacheMaxLog; log++ {
		size := uintptr(1<<log) - allocationHeaderSize
		p := RustAlloc(size, 1)
		if p == 0 {
			t.Fatal("warm allocation failed")
		}
		RustDealloc(p, size, 1)
	}
	baseline := heapChaosQuiescent(t)
	heapChaosBoundaries(t, baseline)
	for _, defaultSeed := range []uint64{0x6f78696465, 0xc0ffee, 0x9e3779b97f4a7c15} {
		seed := heapChaosSeed(defaultSeed)
		t.Run(fmt.Sprintf("serial_%x", seed), func(t *testing.T) {
			r := rand.New(rand.NewPCG(seed, 1))
			workers := [][]heapChaosBlock{make([]heapChaosBlock, 16)}
			defer heapChaosDrain(t, r, workers)
			for epoch := 0; epoch < 4; epoch++ {
				for step := 0; step < 512; step++ {
					if err := heapChaosStep(r, workers[0]); err != nil {
						t.Fatalf("seed=%#x epoch=%d step=%d: %v", seed, epoch, step, err)
					}
					n, err := heapChaosRanges(workers)
					if err != nil {
						t.Fatalf("seed=%#x epoch=%d step=%d: %v", seed, epoch, step, err)
					}
					if s := HeapStats(); s.LiveAllocations != n {
						t.Fatalf("seed=%#x epoch=%d step=%d model=%d stats=%+v", seed, epoch, step, n, s)
					}
				}
				heapChaosDrain(t, r, workers)
				if s := heapChaosQuiescent(t); s != baseline {
					t.Fatalf("seed=%#x epoch=%d retained storage changed: baseline=%+v got=%+v", seed, epoch, baseline, s)
				}
				t.Logf("seed=%#x epoch=%d drained %+v", seed, epoch, HeapStats())
			}
		})
	}
	for _, defaultSeed := range []uint64{0x31415926, 0x27182818} {
		seed := heapChaosSeed(defaultSeed)
		t.Run(fmt.Sprintf("concurrent_%x", seed), func(t *testing.T) {
			var slots [4][4]heapChaosBlock
			var random [4]*rand.Rand
			workers := make([][]heapChaosBlock, len(slots))
			for i := range slots {
				workers[i] = slots[i][:]
				random[i] = rand.New(rand.NewPCG(seed, uint64(i+1)))
			}
			drainRandom := rand.New(rand.NewPCG(seed, 99))
			defer heapChaosDrain(t, drainRandom, workers)
			for epoch := 0; epoch < 3; epoch++ {
				for wave := 0; wave < 3; wave++ {
					var wg sync.WaitGroup
					errors := make([]error, len(slots))
					for worker := range slots {
						wg.Go(func() {
							for step := 0; step < 96; step++ {
								if err := heapChaosStep(random[worker], workers[worker]); err != nil {
									errors[worker] = fmt.Errorf("worker=%d step=%d: %w", worker, step, err)
									return
								}
							}
						})
					}
					wg.Wait()
					for _, err := range errors {
						if err != nil {
							t.Fatalf("seed=%#x epoch=%d wave=%d: %v", seed, epoch, wave, err)
						}
					}
					n, err := heapChaosRanges(workers)
					if err != nil {
						t.Fatal(err)
					}
					if s := HeapStats(); s.LiveAllocations != n {
						t.Fatalf("seed=%#x epoch=%d wave=%d model=%d stats=%+v", seed, epoch, wave, n, s)
					}
				}
				heapChaosDrain(t, drainRandom, workers)
				if s := heapChaosQuiescent(t); s != baseline {
					t.Fatalf("seed=%#x epoch=%d retained storage changed: %+v", seed, epoch, s)
				}
				t.Logf("seed=%#x epoch=%d drained %+v", seed, epoch, HeapStats())
			}
		})
	}
}

func TestHeapLeakOracleDetectsHeldAllocation(t *testing.T) {
	warm := RustAlloc(4096, 16)
	if warm == 0 {
		t.Fatal("warm allocation failed")
	}
	RustDealloc(warm, 4096, 16)
	before := HeapStats()
	func() {
		p := RustAlloc(4096, 16)
		if p == 0 {
			t.Fatal("allocation failed")
		}
		defer RustDealloc(p, 4096, 16)
		held := HeapStats()
		if held.LiveAllocations != before.LiveAllocations+1 || held.MappedBytes != before.MappedBytes || held.Mappings != before.Mappings {
			t.Fatalf("oracle missed held allocation inside existing storage: before=%+v held=%+v", before, held)
		}
		t.Logf("intentional held allocation detected despite unchanged mappings: before=%+v held=%+v", before, held)
	}()
	if after := HeapStats(); after != before {
		t.Fatalf("negative control cleanup did not restore baseline: before=%+v after=%+v", before, after)
	}
}
