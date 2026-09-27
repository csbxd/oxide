//go:build linux && (amd64 || arm64)

package oxide

// AddAddress checks address arithmetic used by generated Rust projections.
func AddAddress(address, offset uintptr) uintptr {
	if offset > ^uintptr(0)-address {
		panic("oxide: Rust address overflow")
	}
	return address + offset
}

// ArrayBytes computes a Rust sequence's size within isize::MAX.
func ArrayBytes(length, size uintptr) uintptr {
	if size != 0 && length > (^uintptr(0)>>1)/size {
		panic("oxide: Rust allocation exceeds isize::MAX")
	}
	return length * size
}

func AlignUp(value, align uintptr) uintptr {
	if align == 0 || align&(align-1) != 0 {
		panic("oxide: invalid Rust alignment")
	}
	return AddAddress(value, align-1) &^ (align - 1)
}
