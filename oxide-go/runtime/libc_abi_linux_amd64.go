//go:build linux && amd64

package oxide

import (
	"unsafe"

	"modernc.org/libc"
)

var (
	_ [144]byte = [unsafe.Sizeof(libc.Tstat{})]byte{}
	_ [16]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_nlink)]byte{}
	_ [24]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_mode)]byte{}
	_ [28]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_uid)]byte{}
	_ [32]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_gid)]byte{}
	_ [40]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_rdev)]byte{}
)
