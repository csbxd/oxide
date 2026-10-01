package oxide

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"sort"
	"testing"
)

func TestF128MathMPFR(t *testing.T) {
	f, err := os.Open("testdata/f128_math.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	s := bufio.NewScanner(z)
	failures := map[string]int{}
	counts := map[string]int{}
	for s.Scan() {
		var op string
		var x, y, want F128
		var sign int32
		if _, err := fmt.Sscanf(s.Text(), "%s %x %x %x %x %x %x %d", &op, &x.Hi, &x.Lo, &y.Hi, &y.Lo, &want.Hi, &want.Lo, &sign); err != nil {
			t.Fatal(err)
		}
		fn := op
		if fn == "gamma" {
			fn = "tgamma"
		}
		got, gotSign := f128Math(fn, x, y)
		counts[op]++
		ok := false
		if F128IsNaN(want) {
			ok = F128IsNaN(got)
		} else if f128IsZero(want) || f128IsInf(want) {
			ok = got == want
		} else if !F128IsNaN(got) && got.Hi>>63 == want.Hi>>63 {
			a, b := U128(got), U128(want)
			if U128Lt(a, b) {
				a, b = b, a
			}
			difference := U128SubValue(a, b)
			ok = difference.Hi == 0 && difference.Lo <= 1
		}
		// At -Inf the sign of Gamma is undefined; libm and MPFR use
		// different conventions. Finite arguments, including -0, are checked.
		if op == "lgamma" && !F128IsNaN(want) && !f128IsInf(x) && sign != gotSign {
			ok = false
		}
		if !ok {
			failures[op]++
			if failures[op] <= 5 {
				t.Errorf("%s(%016x:%016x,%016x:%016x)=%016x:%016x sign=%d; MPFR %016x:%016x sign=%d", op, x.Hi, x.Lo, y.Hi, y.Lo, got.Hi, got.Lo, gotSign, want.Hi, want.Lo, sign)
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	ops := make([]string, 0, len(counts))
	for op := range counts {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	for _, op := range ops {
		count := counts[op]
		t.Logf("%s: %d cases, %d outside 1 ULP", op, count, failures[op])
	}
}

func TestF128MathAllocations(t *testing.T) {
	ctx := NewContext()
	defer ctx.Close()
	for _, op := range []string{"exp", "exp2", "expm1", "log", "log2", "log10", "log1p", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "cbrt", "hypot", "pow", "tgamma", "lgamma", "erf", "erfc"} {
		t.Run(op, func(t *testing.T) {
			for _, x := range []F128{F64ToF128(0.75), F64ToF128(4.25), {Lo: 1, Hi: 0x7ffeffffffffffff}} {
				requireNoGoAllocations(t, 100, func() { softFloatSink = F128Math(ctx, op, x, F64ToF128(1.5)) })
			}
		})
	}
}

func BenchmarkF128Math(b *testing.B) {
	for _, op := range []string{"exp", "log", "sin", "sin_large", "pow", "tgamma", "erfc"} {
		b.Run(op, func(b *testing.B) {
			x := F64ToF128(1.5)
			name := op
			if op == "sin_large" {
				name = "sin"
				x = F128{Lo: 1, Hi: 0x7ffeffffffffffff}
			}
			b.ReportAllocs()
			for b.Loop() {
				softFloatSink, _ = f128Math(name, x, F64ToF128(2.5))
			}
		})
	}
}
