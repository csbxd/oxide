//go:build linux && (amd64 || arm64)

package oxide

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"modernc.org/libc"
)

// Enabled by context_chaos_counters_test.go. Context mappings are checked
// independently with mincore; cached allocator pages are not live objects.
var contextChaosLiveHeap func() int

type chaosMapping struct{ address, size uintptr }
type chaosBlock struct {
	address, size, align uintptr
	tag                  byte
}

func chaosPageRange(address, size uintptr) chaosMapping {
	page := uintptr(syscall.Getpagesize())
	start := address &^ (page - 1)
	return chaosMapping{start, (address + size - start + page - 1) &^ (page - 1)}
}

// No Go allocation or mapping is made between Close and this check. Run the
// whole lifecycle test in a child so other tests cannot reuse these addresses.
func chaosMappedPage(ranges []chaosMapping) (uintptr, syscall.Errno) {
	page := uintptr(syscall.Getpagesize())
	var resident byte
	for _, r := range ranges {
		for p := r.address; p < r.address+r.size; p += page {
			_, _, errno := syscall.RawSyscall(syscall.SYS_MINCORE, p, page, uintptr(unsafe.Pointer(&resident)))
			if errno != syscall.ENOMEM {
				return p, errno
			}
		}
	}
	return 0, 0
}

func chaosContextMappings(c *Context) []chaosMapping {
	var ranges []chaosMapping
	for _, b := range c.segments {
		ranges = append(ranges, chaosPageRange(uintptr(unsafe.Pointer(unsafe.SliceData(b))), uintptr(len(b))))
	}
	for _, local := range c.threadLocals {
		b := local.mapping
		ranges = append(ranges, chaosPageRange(uintptr(unsafe.Pointer(unsafe.SliceData(b))), uintptr(len(b))))
	}
	return ranges
}

func (b chaosBlock) paint() {
	if b.size == 0 {
		return
	}
	for _, i := range []uintptr{0, b.size / 2, b.size - 1} {
		*(*byte)(unsafe.Pointer(b.address + i)) = b.tag ^ byte(i)
	}
}

func (b chaosBlock) valid() bool {
	if b.address%b.align != 0 {
		return false
	}
	if b.size == 0 {
		return true
	}
	for _, i := range []uintptr{0, b.size / 2, b.size - 1} {
		if *(*byte)(unsafe.Pointer(b.address + i)) != b.tag^byte(i) {
			return false
		}
	}
	return true
}

type contextChaos struct {
	t                *testing.T
	seed             uint64
	epoch, operation int
	rng              *rand.Rand
	owned            map[uintptr]chaosBlock
	callbackError    string
	closing          *[]chaosMapping
}

func (h *contextChaos) fail(format string, args ...any) {
	h.t.Helper()
	h.t.Fatalf("seed=%#x epoch=%d op=%d: %s", h.seed, h.epoch, h.operation, fmt.Sprintf(format, args...))
}

func (h *contextChaos) own(size, align uintptr, tag byte) uintptr {
	p := RustAlloc(size, align)
	if p == 0 {
		h.fail("RustAlloc(%d,%d) failed", size, align)
	}
	b := chaosBlock{p, size, align, tag}
	b.paint()
	if _, exists := h.owned[p]; exists {
		h.fail("allocator reused live pointer %#x", p)
	}
	h.owned[p] = b
	return p
}

func (h *contextChaos) free(p uintptr) {
	b, ok := h.owned[p]
	if !ok {
		h.callbackError = fmt.Sprintf("duplicate/unknown destructor pointer %#x", p)
		return
	}
	if !b.valid() {
		h.callbackError = fmt.Sprintf("owned object %#x corrupted", p)
	}
	RustDealloc(p, b.size, b.align)
	delete(h.owned, p)
}

func (h *contextChaos) close(c *Context, extra []chaosMapping) {
	ranges := append(chaosContextMappings(c), extra...)
	tls := c.libcTLS
	h.closing = &ranges
	if err := c.Close(); err != nil {
		h.fail("Close: %v", err)
	}
	h.closing = nil
	if p, errno := chaosMappedPage(ranges); p != 0 {
		h.fail("closed mapping %#x still exists: %v", p, errno)
	}
	if !reflect.ValueOf(*c).IsZero() {
		h.fail("Context fields survive Close")
	}
	if tls != nil && !reflect.ValueOf(*tls).IsZero() {
		h.fail("libc TLS fields survive Close")
	}
	if err := c.Close(); err != nil {
		h.fail("repeated Close: %v", err)
	}
}

func (h *contextChaos) destructorStorage(c *Context, template uintptr) {
	mark := c.Mark()
	defer c.Restore(mark)
	// The first destructor grows a new segment and initializes a previously
	// untouched TLS template. Include both in the post-Close unmap oracle.
	b := chaosBlock{c.Alloc((3<<20)+1, 65536), (3 << 20) + 1, 65536, 0x37}
	b.paint()
	p := c.ThreadLocal(template, 64, 65536)
	if !b.valid() || p%65536 != 0 {
		h.callbackError = "destructor storage invalid"
	}
	for _, r := range chaosContextMappings(c) {
		seen := false
		for _, old := range *h.closing {
			seen = seen || old.address == r.address
		}
		if !seen {
			*h.closing = append(*h.closing, r)
		}
	}
}

func (h *contextChaos) oracle() {
	c := NewContext()
	ranges := chaosContextMappings(c)
	// Deliberately omit Close/free first: each oracle must detect the live
	// resource, then normal cleanup must make the same oracle pass.
	if p, errno := chaosMappedPage(ranges); p == 0 || errno != 0 {
		h.fail("mapping oracle missed omitted Close")
	}
	h.close(c, nil)
	p := h.own(2<<20, 64, 0x59) // Bypasses the <=1 MiB cache; owns its mapping.
	ranges = []chaosMapping{chaosPageRange(p, 2<<20)}
	if q, errno := chaosMappedPage(ranges); q == 0 || errno != 0 {
		h.fail("mapping oracle missed omitted free")
	}
	h.free(p)
	if q, errno := chaosMappedPage(ranges); q != 0 {
		h.fail("freed large object still mapped: %#x %v", q, errno)
	}
	if contextChaosLiveHeap != nil {
		before := contextChaosLiveHeap()
		p = h.own(64, 16, 0x42)
		if contextChaosLiveHeap() != before+1 {
			h.fail("heap oracle missed omitted small free")
		}
		h.free(p)
		if contextChaosLiveHeap() != before {
			h.fail("heap oracle counts retained cache as live")
		}
	}
}

func (h *contextChaos) unwind(c *Context, depth int, token uint64) {
	mark := c.Mark()
	defer c.Restore(mark)
	b := chaosBlock{c.Alloc(4097, 4096), 4097, 4096, byte(depth)}
	b.paint()
	if depth == 0 {
		c.Fail(token)
		panic(token)
	}
	defer func() {
		outer := c.TakePanic()
		nested := token ^ uint64(depth)
		before := c.Mark()
		func() {
			defer func() {
				if recover() != nested || c.TakePanic() != nested {
					h.fail("nested panic state lost")
				}
			}()
			m := c.Mark()
			defer c.Restore(m)
			c.Alloc(32768, 65536)
			c.Fail(nested)
			panic(nested)
		}()
		if c.Mark() != before || !b.valid() {
			h.fail("nested unwind damaged outer frame")
		}
		c.Fail(outer)
	}()
	h.unwind(c, depth-1, token)
}

func (h *contextChaos) frames(c *Context) {
	base := c.Mark()
	// Force both a new segment and alignment larger than the OS page size.
	p := c.Alloc(2<<20, 1<<20)
	if p%(1<<20) != 0 || c.segment == base.segment {
		h.fail("high-alignment segment not exercised")
	}
	c.Restore(base)
	if q, errno := chaosMappedPage(chaosContextMappings(c)); q == 0 || errno != 0 {
		h.fail("Restore discarded retained frame mappings")
	}
	type frame struct {
		mark   Mark
		blocks int
	}
	stack := []frame{{base, 0}}
	var blocks []chaosBlock
	sizes := []uintptr{0, 1, 31, 4097, 32768, 512 << 10, (1 << 20) + 17}
	aligns := []uintptr{1, 2, 8, 64, 4096, 65536}
	for h.operation = 0; h.operation < 96; h.operation++ {
		switch h.rng.IntN(5) {
		case 0:
			if len(stack) < 6 {
				stack = append(stack, frame{c.Mark(), len(blocks)})
			}
		case 1:
			if len(stack) > 1 {
				f := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				c.Restore(f.mark)
				blocks = blocks[:f.blocks]
			}
		case 2:
			before := c.Mark()
			token := h.seed ^ uint64(h.operation+1)
			func() {
				defer func() {
					if recover() != token || c.TakePanic() != token {
						h.fail("outer panic state lost")
					}
				}()
				h.unwind(c, 1+h.rng.IntN(4), token)
			}()
			if c.Mark() != before || c.Failed() {
				h.fail("panic did not restore frame/state")
			}
		case 3:
			p := h.own(64, 16, byte(h.operation))
			c.RaiseException(p)
			if !c.Failed() || c.TakeException() != p || c.Failed() {
				h.fail("exception handle was lost")
			}
			h.free(p)
		case 4:
			var mapped int
			for _, s := range c.segments {
				mapped += len(s)
			}
			if mapped > 20<<20 || len(blocks) >= 32 {
				c.Restore(base)
				blocks = blocks[:0]
				stack = stack[:1]
			}
			size, align := sizes[h.rng.IntN(len(sizes))], aligns[h.rng.IntN(len(aligns))]
			if mapped > 20<<20 {
				size, align = 31, 8
			}
			b := chaosBlock{c.Alloc(size, align), size, align, byte(h.operation)}
			for _, old := range blocks {
				if b.address < old.address+max(old.size, 1) && old.address < b.address+max(b.size, 1) {
					h.fail("live frames overlap")
				}
			}
			b.paint()
			blocks = append(blocks, b)
		}
		for _, b := range blocks {
			if !b.valid() {
				h.fail("outer frame bytes/alignment changed")
			}
		}
	}
	c.Restore(base)
}

func (h *contextChaos) destructors(c *Context, template uintptr) func() {
	type job struct {
		id, left int
		key      uint32
	}
	jobs := map[uintptr]job{}
	var got, want []int
	newValue := func(j job) uintptr {
		p := h.own(uintptr(64+(j.id%17)*16), 16, byte(j.id))
		jobs[p] = j
		return p
	}
	var cxa DestructorCallback
	cxa = func(ctx *Context, descriptor, p uintptr) {
		j, ok := jobs[p]
		delete(jobs, p)
		if !ok || ctx != c || descriptor != 1 {
			h.callbackError = "invalid Cxa callback"
			return
		}
		got = append(got, j.id)
		h.free(p)
		h.destructorStorage(ctx, template)
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(ctx))) = int32(j.id)
		if j.left > 0 {
			LibcCxaThreadAtExit(ctx, 1, newValue(job{j.id + 1, j.left - 1, 0}), 17, cxa)
		}
	}
	var roots []job
	for i := 0; i < 2+h.rng.IntN(4); i++ {
		j := job{id: (i + 1) * 16, left: h.rng.IntN(3)}
		roots = append(roots, j)
		LibcCxaThreadAtExit(c, 1, newValue(j), 17, cxa)
	}
	for i := len(roots) - 1; i >= 0; i-- {
		for n := 0; n <= roots[i].left; n++ {
			want = append(want, roots[i].id+n)
		}
	}
	var keys []uint32
	counts, rounds := map[uint32]int{}, map[uint32]int{}
	var pthread DestructorCallback
	pthread = func(ctx *Context, descriptor, p uintptr) {
		j, ok := jobs[p]
		delete(jobs, p)
		if !ok || ctx != c || descriptor != 2 {
			h.callbackError = "invalid pthread callback"
			return
		}
		if !reflect.DeepEqual(got, want) || LibcPthreadGetspecific(ctx, j.key) != 0 {
			h.callbackError = "pthread ran before Cxa or before value clear"
		}
		counts[j.key]++
		h.free(p)
		h.destructorStorage(ctx, template)
		if j.left > 1 {
			LibcPthreadSetspecific(ctx, j.key, newValue(job{j.id + 1, j.left - 1, j.key}))
		}
	}
	for i := 0; i < 3; i++ {
		storage := c.Alloc(4, 4)
		if LibcPthreadKeyCreate(c, storage, 2, pthread) != 0 {
			h.fail("key create")
		}
		key := *(*uint32)(unsafe.Pointer(storage))
		keys = append(keys, key)
		n := 1 + h.rng.IntN(4)
		if i == 0 {
			n = 4
		}
		rounds[key] = n
		LibcPthreadSetspecific(c, key, newValue(job{1024 + i*16, n, key}))
	}
	return func() {
		if !reflect.DeepEqual(got, want) {
			h.fail("Cxa order got %v, want %v", got, want)
		}
		for _, key := range keys {
			if counts[key] != rounds[key] {
				h.fail("key %d destructor count %d, want %d", key, counts[key], rounds[key])
			}
			if LibcPthreadKeyDelete(c, key) != 0 {
				h.fail("key delete")
			}
		}
		if len(jobs) != 0 || h.callbackError != "" {
			h.fail("destructor ledger: %d jobs, %s", len(jobs), h.callbackError)
		}
	}
}

func (h *contextChaos) keyGenerations(c, other *Context) func() {
	storage := c.Alloc(4, 4)
	oldCalls, newCalls := 0, 0
	old := func(_ *Context, _, p uintptr) { oldCalls++; h.free(p) }
	var key uint32
	for round, n := 0, 1+h.rng.IntN(4); round < n; round++ {
		if LibcPthreadKeyCreate(c, storage, 3, old) != 0 {
			h.fail("generation key create")
		}
		next := *(*uint32)(unsafe.Pointer(storage))
		if round != 0 && next != key {
			h.fail("key slot was not reused")
		}
		key = next
		if LibcPthreadGetspecific(c, key) != 0 || LibcPthreadGetspecific(other, key) != 0 {
			h.fail("stale generation became visible")
		}
		a, b := h.own(64, 16, 0x19), h.own(64, 16, 0x73)
		LibcPthreadSetspecific(c, key, a)
		LibcPthreadSetspecific(other, key, b)
		if LibcPthreadKeyDelete(c, key) != 0 || oldCalls != 0 {
			h.fail("key deletion invoked a destructor")
		}
		// POSIX key deletion does not destroy caller-owned values. Free them
		// explicitly while stale generation entries remain in both Contexts.
		h.free(a)
		h.free(b)
	}
	callback := func(ctx *Context, descriptor, p uintptr) {
		newCalls++
		if ctx != other || descriptor != 4 {
			h.callbackError = "stale generation invoked replacement destructor"
		}
		h.free(p)
	}
	if LibcPthreadKeyCreate(c, storage, 4, callback) != 0 || *(*uint32)(unsafe.Pointer(storage)) != key {
		h.fail("replacement key create")
	}
	if LibcPthreadGetspecific(c, key) != 0 || LibcPthreadGetspecific(other, key) != 0 {
		h.fail("replacement inherited stale values")
	}
	LibcPthreadSetspecific(other, key, h.own(64, 16, 0x52))
	return func() {
		if oldCalls != 0 || newCalls != 1 {
			h.fail("generation destructors old=%d new=%d", oldCalls, newCalls)
		}
		if LibcPthreadKeyDelete(c, key) != 0 {
			h.fail("replacement key delete")
		}
	}
}

func (h *contextChaos) epochRun() {
	beforeLibc := libc.MemStat()
	beforeHeap := 0
	if contextChaosLiveHeap != nil {
		beforeHeap = contextChaosLiveHeap()
	}
	c, other := NewContext(), NewContext()
	defer c.Close()
	defer other.Close()
	var templates []uintptr
	for i := 0; i < 3; i++ {
		size, align := uintptr(32+i*1024), uintptr(16)<<uint(4*i)
		p := h.own(size, align, byte(i+31))
		templates = append(templates, p)
		a, b := c.ThreadLocal(p, size, align), other.ThreadLocal(p, size, align)
		if a == b || a%align != 0 || b%align != 0 {
			h.fail("TLS isolation/alignment")
		}
		if !reflect.DeepEqual(unsafe.Slice((*byte)(unsafe.Pointer(a)), size), unsafe.Slice((*byte)(unsafe.Pointer(p)), size)) {
			h.fail("TLS template not copied")
		}
		*(*byte)(unsafe.Pointer(a)) ^= 0xff
		if *(*byte)(unsafe.Pointer(b)) != byte(i+31) || !h.owned[p].valid() {
			h.fail("TLS instance changed template/peer")
		}
		if c.ThreadLocal(p, size, align) != a {
			h.fail("TLS instance address changed")
		}
	}
	checkGenerations := h.keyGenerations(c, other)
	h.frames(c)
	// Include libc's own large cached stack slot, which Close must release.
	tls := c.libc()
	p := tls.Alloc(2 << 20)
	*(*byte)(unsafe.Pointer(p)) = 7
	*(*byte)(unsafe.Pointer(p + (2 << 20) - 1)) = 9
	tls.Free(2 << 20)
	lateTemplate := h.own(64, 16, 0x85)
	templates = append(templates, lateTemplate)
	checkDestructors := h.destructors(c, lateTemplate)
	var mapped uintptr
	for _, ctx := range []*Context{c, other} {
		for _, r := range chaosContextMappings(ctx) {
			mapped += r.size
		}
	}
	if mapped > 32<<20 {
		h.fail("Context/TLS mapping budget exceeded: %d", mapped)
	}
	c.Fail(h.seed) // Close must clear a pending diagnostic without retaining it.
	h.close(c, []chaosMapping{chaosPageRange(p, 2<<20)})
	checkDestructors()
	h.close(other, nil)
	checkGenerations()
	for _, p := range templates {
		h.free(p)
	}
	if len(h.owned) != 0 || h.callbackError != "" {
		h.fail("owned allocation leak: %d, %s", len(h.owned), h.callbackError)
	}
	if contextChaosLiveHeap != nil && contextChaosLiveHeap() != beforeHeap {
		h.fail("Rust live heap did not return to baseline")
	}
	if got := libc.MemStat(); got != beforeLibc {
		h.fail("libc owned storage leaked: got %+v, baseline %+v", got, beforeLibc)
	}
}

func TestContextLifecycleChaos(t *testing.T) {
	if os.Getenv("OXIDE_CONTEXT_CHAOS_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestContextLifecycleChaos$", "-test.v")
		cmd.Env = append(os.Environ(), "OXIDE_CONTEXT_CHAOS_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated context chaos: %v\n%s", err, output)
		} else {
			t.Logf("%s", output)
		}
		return
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	// libc keeps its environment for process lifetime; initialize it before
	// measuring TLS ownership so that permanent storage is not called a leak.
	warm := NewContext()
	LibcErrnoLocation(warm)
	warm.Close()
	seeds := []uint64{1, 0x5eedc0de, 0xdeadbeef12345678, ^uint64(0)}
	if raw := os.Getenv("OXIDE_CHAOS_SEED"); raw != "" {
		seed, err := strconv.ParseUint(raw, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		seeds = []uint64{seed}
	}
	epochs := 8
	if os.Getenv("OXIDE_CHAOS_LONG") == "1" {
		epochs = 128
	}
	t.Logf("seeds=%x epochs=%d heap-counters=%v", seeds, epochs, contextChaosLiveHeap != nil)
	for _, seed := range seeds {
		h := contextChaos{t: t, seed: seed, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), owned: map[uintptr]chaosBlock{}}
		h.oracle()
		for h.epoch = 0; h.epoch < epochs; h.epoch++ {
			h.epochRun()
		}
	}
}
