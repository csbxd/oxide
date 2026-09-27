// Package oxide implements the semantic operations shared by generated code.
// The compiler and this package are released together.
package oxide

import "unsafe"

type Integer interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~int | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

func Add[T Integer](a, b T) T {
	r := a + b
	if ^T(0) > 0 {
		if r < a {
			panic("Rust integer addition overflow")
		}
	} else if b > 0 && r < a || b < 0 && r > a {
		panic("Rust integer addition overflow")
	}
	return r
}

func Sub[T Integer](a, b T) T {
	r := a - b
	if ^T(0) > 0 {
		if a < b {
			panic("Rust integer subtraction overflow")
		}
	} else if b < 0 && r < a || b > 0 && r > a {
		panic("Rust integer subtraction overflow")
	}
	return r
}

func Mul[T Integer](a, b T) T {
	r := a * b
	if b != 0 && (r/b != a || ^T(0) < 0 && b == ^T(0) && a == minValue[T]()) {
		panic("Rust integer multiplication overflow")
	}
	return r
}

func Div[T Integer](a, b T) T {
	if b == 0 {
		panic("Rust integer division by zero")
	}
	if ^T(0) < 0 && a == minValue[T]() && b == ^T(0) {
		panic("Rust integer division overflow")
	}
	return a / b
}

func Rem[T Integer](a, b T) T {
	if b == 0 {
		panic("Rust integer remainder by zero")
	}
	if ^T(0) < 0 && a == minValue[T]() && b == ^T(0) {
		panic("Rust integer remainder overflow")
	}
	return a % b
}

func Neg[T Integer](a T) T {
	if a == minValue[T]() {
		panic("Rust integer negation overflow")
	}
	return -a
}

func minValue[T Integer]() T { return T(1) << (unsafe.Sizeof(T(0))*8 - 1) }

func Cast[T, U Integer](v U) T { return T(v) }

func Shl[T, U Integer](a T, b U) T {
	if b < 0 || uint64(b) >= uint64(unsafe.Sizeof(a)*8) {
		panic("Rust shift overflow")
	}
	return a << b
}

func Shr[T, U Integer](a T, b U) T {
	if b < 0 || uint64(b) >= uint64(unsafe.Sizeof(a)*8) {
		panic("Rust shift overflow")
	}
	return a >> b
}
