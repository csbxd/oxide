//go:build memory.counters

package chaosfixture

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

type input struct{ seed, steps, want uint64 }

func inputs(t *testing.T) []input {
	t.Helper()
	f, err := os.Open("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var result []input
	s := bufio.NewScanner(f)
	for s.Scan() {
		var v input
		if _, err := fmt.Sscan(s.Text(), &v.seed, &v.steps, &v.want); err != nil {
			t.Fatal(err)
		}
		result = append(result, v)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result) != 24 {
		t.Fatalf("native input count %d, want 24", len(result))
	}
	return result
}

func checkCall(t *testing.T, c *oxide.Context, v input) {
	t.Helper()
	mark := c.Mark()
	got := Cycle(c, v.seed, uintptr(v.steps))
	if got != v.want || c.Failed() || c.Mark() != mark {
		t.Fatalf("seed=%#x steps=%d result=%#x want=%#x failed=%v frame=%v/%v", v.seed, v.steps, got, v.want, c.Failed(), c.Mark(), mark)
	}
}

func TestTranslatedOwnershipChaos(t *testing.T) {
	seed := int64(0x5eedcafe)
	if s := os.Getenv("OXIDE_CHAOS_SEED"); s != "" {
		v, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		seed = v
	}
	epochs := 8
	if s := os.Getenv("OXIDE_CHAOS_EPOCHS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 1000 {
			t.Fatalf("invalid OXIDE_CHAOS_EPOCHS %q", s)
		}
		epochs = v
	}
	cases := inputs(t)
	var coverage uint64
	for _, v := range cases {
		coverage |= v.want >> 54
	}
	if coverage != 1023 {
		t.Fatalf("native random corpus misses operations: %#x", coverage)
	}
	// Establish the legitimate process-lifetime state before measuring epochs.
	c := oxide.NewContext()
	for _, v := range cases {
		checkCall(t, c, v)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	baseline := oxide.HeapStats()
	if baseline.LiveAllocations != 0 {
		t.Fatalf("fixture retained Rust owners after warm-up: %+v", baseline)
	}
	r := rand.New(rand.NewSource(seed))
	for epoch := 0; epoch < epochs; epoch++ {
		r.Shuffle(len(cases), func(i, j int) { cases[i], cases[j] = cases[j], cases[i] })
		reused := oxide.NewContext()
		for op, v := range cases {
			current := reused
			fresh := r.Intn(3) == 0
			if fresh {
				current = oxide.NewContext()
			}
			checkCall(t, current, v)
			if fresh {
				if err := current.Close(); err != nil {
					t.Fatalf("seed=%#x epoch=%d op=%d: %v", seed, epoch, op, err)
				}
			}
		}
		if err := reused.Close(); err != nil {
			t.Fatal(err)
		}
		got := oxide.HeapStats()
		if got.LiveAllocations != baseline.LiveAllocations {
			t.Fatalf("seed=%#x epoch=%d: Rust heap leak: baseline=%+v actual=%+v", seed, epoch, baseline, got)
		}
		if got.Mappings != got.CachedMappings || got.MappedBytes != got.CachedMappedBytes || got.MappedBytes >= 4<<20 {
			t.Fatalf("seed=%#x epoch=%d: unowned mapping outside bounded cache: %+v", seed, epoch, got)
		}
	}
	t.Logf("seed=%#x epochs=%d calls=%d Rust operations=%d baseline=%+v final=%+v", seed, epochs, epochs*len(cases), epochs*12*(128+257), baseline, oxide.HeapStats())
}

func TestTranslatedLeakOracle(t *testing.T) {
	c := oxide.NewContext()
	defer c.Close()
	before := oxide.HeapStats()
	p := Retain(c, 0x5eed)
	defer func() {
		if p != 0 {
			Reclaim(c, p)
		}
	}()
	leaked := oxide.HeapStats()
	if p == 0 || leaked.LiveAllocations != before.LiveAllocations+1 {
		t.Fatalf("oracle missed intentionally retained Rust Box: %+v -> %+v", before, leaked)
	}
	// Consume the Rust Box through translated Drop; do not free it using Go.
	Reclaim(c, p)
	p = 0
	after := oxide.HeapStats()
	if after.LiveAllocations != before.LiveAllocations {
		t.Fatalf("reclaim did not restore baseline: %+v -> %+v", before, after)
	}
}
