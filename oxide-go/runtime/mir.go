package oxide

import "os"

// Scalar keeps an MIR constant a runtime value in Go: Go constant arithmetic
// must not reject Rust's explicit casts, wrapping arithmetic or IEEE division.
func Scalar[T any](v T) T { return v }

func AddWithOverflow[T Integer](a, b T) (T, bool) {
	r := a + b
	if ^T(0) > 0 {
		return r, r < a
	}
	return r, (b > 0 && r < a) || (b < 0 && r > a)
}
func SubWithOverflow[T Integer](a, b T) (T, bool) {
	r := a - b
	if ^T(0) > 0 {
		return r, a < b
	}
	return r, (b < 0 && r < a) || (b > 0 && r > a)
}
func MulWithOverflow[T Integer](a, b T) (T, bool) {
	r := a * b
	return r, b != 0 && (r/b != a || ^T(0) < 0 && b == ^T(0) && a == minValue[T]())
}
func Abort() { os.Exit(134) }
