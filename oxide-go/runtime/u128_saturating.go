package oxide

func U128SaturatingAdd(a, b U128) U128 {
	r, overflow := U128Add(a, b)
	if overflow {
		return U128{Lo: ^uint64(0), Hi: ^uint64(0)}
	}
	return r
}

func U128SaturatingSub(a, b U128) U128 {
	r, overflow := U128Sub(a, b)
	if overflow {
		return U128{}
	}
	return r
}

func I128SaturatingAdd(a, b I128) I128 {
	r, overflow := I128Add(a, b)
	if overflow {
		if a.Hi>>63 != 0 {
			return I128{Hi: 1 << 63}
		}
		return I128{Lo: ^uint64(0), Hi: (1 << 63) - 1}
	}
	return r
}

func I128SaturatingSub(a, b I128) I128 {
	r, overflow := I128Sub(a, b)
	if overflow {
		if a.Hi>>63 != 0 {
			return I128{Hi: 1 << 63}
		}
		return I128{Lo: ^uint64(0), Hi: (1 << 63) - 1}
	}
	return r
}
