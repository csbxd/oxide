package fixture

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func TestRustMath(t *testing.T) {
	data, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var half, op uint8
		var x, y, want oxide.U128
		var sign int32
		if _, err := fmt.Sscanf(line, "%d %d %x %x %x %x %x %x %d", &half, &op, &x.Hi, &x.Lo, &y.Hi, &y.Lo, &want.Hi, &want.Lo, &sign); err != nil {
			t.Fatal(err)
		}
		mark := ctx.Mark()
		requireNoGoAllocations(t, 100, func() {
			got := Evaluate(ctx, op, half != 0, x, y)
			if !mathClose(got, want, half != 0) {
				t.Fatalf("half=%d op=%d x=%x:%x y=%x:%x: Go %x:%x native %x:%x", half, op, x.Hi, x.Lo, y.Hi, y.Lo, got.Hi, got.Lo, want.Hi, want.Lo)
			}
			if op == 24 && gammaSignDefined(x, half != 0) {
				if v := GammaSign(ctx, half != 0, x); v != sign {
					t.Fatalf("gamma sign half=%d x=%x:%x: %v, want %v", half, x.Hi, x.Lo, v, sign)
				}
			}
			if ctx.Failed() || ctx.Mark() != mark {
				t.Fatal("math panic or leaked frame")
			}
		})
	}
}

// Gamma is undefined at negative integers and -Inf; libc implementations
// disagree on the sign returned by lgamma there. Signed zero is defined.
func gammaSignDefined(x oxide.U128, half bool) bool {
	if half {
		v := oxide.F16ToF64(oxide.F16(x.Lo))
		return !math.IsNaN(v) && !math.IsInf(v, 0) && (v >= 0 || v != math.Trunc(v))
	}
	v := oxide.F128(x)
	if oxide.F128IsNaN(v) || x.Hi&0x7fffffffffffffff == 0x7fff000000000000 && x.Lo == 0 {
		return false
	}
	return x.Hi>>63 == 0 || oxide.F128Eq(v, oxide.F128{}) || !oxide.F128Eq(oxide.F128Trunc(v), v)
}

func mathNaN(x oxide.U128, half bool) bool {
	if half {
		return x.Lo&0x7fff > 0x7c00
	}
	return oxide.F128IsNaN(oxide.F128(x))
}
func mathClose(a, b oxide.U128, half bool) bool {
	if mathNaN(b, half) {
		return mathNaN(a, half)
	}
	if mathNaN(a, half) {
		return false
	}
	if half {
		if b.Lo&0x7fff == 0 || b.Lo&0x7fff == 0x7c00 {
			return a == b
		}
		if a.Lo>>15 != b.Lo>>15 {
			return false
		}
	} else {
		if b.Lo == 0 && (b.Hi&0x7fffffffffffffff == 0 || b.Hi&0x7fffffffffffffff == 0x7fff000000000000) {
			return a == b
		}
		if a.Hi>>63 != b.Hi>>63 {
			return false
		}
	}
	if oxide.U128Lt(a, b) {
		a, b = b, a
	}
	d := oxide.U128SubValue(a, b)
	return d.Hi == 0 && d.Lo <= 4
}
