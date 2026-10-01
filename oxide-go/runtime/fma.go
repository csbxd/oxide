// SPDX-License-Identifier: MIT
// Specialized from Rust libm's fma_wide_round, originating in musl fmaf.c.
// Source: compiler-builtins/libm in Rust nightly-2026-09-15.
// See LICENSE.libm for the upstream license notices.

package oxide

import "math"

// FMA32 computes x*y+z with one rounding to float32. The float32 product is
// exact in float64; a halfway sum needs its discarded error to avoid double
// rounding. Explicit conversions also prevent Go from fusing these operations.
func FMA32(x, y, z float32) float32 {
	xy := float64(float64(x) * float64(y))
	zb := float64(z)
	result := float64(xy + zb)
	ui := math.Float64bits(result)
	if ui&((1<<29)-1) != 1<<28 || ui>>52&0x7ff == 0x7ff ||
		(float64(result-xy) == zb && float64(result-zb) == xy) {
		return float32(result)
	}
	neg := ui>>63 != 0
	var err float64
	if neg == (zb > xy) {
		err = float64(float64(xy-result) + zb)
	} else {
		err = float64(float64(zb-result) + xy)
	}
	if neg == (err < 0) {
		ui++
	} else {
		ui--
	}
	return float32(math.Float64frombits(ui))
}
