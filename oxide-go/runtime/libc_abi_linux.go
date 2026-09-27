//go:build linux && (amd64 || arm64)

package oxide

import (
	"unsafe"

	"modernc.org/libc"
)

// These are the GNU Linux layouts exported by Rust libc on both targets.
// Keep the checks in the build so a libc update cannot silently change the ABI.
var (
	_ [16]byte = [unsafe.Sizeof(libc.Ttimespec{})]byte{}
	_ [8]byte  = [unsafe.Offsetof(libc.Ttimespec{}.Ftv_nsec)]byte{}
	_ [16]byte = [unsafe.Sizeof(libc.Tiovec{})]byte{}
	_ [8]byte  = [unsafe.Offsetof(libc.Tiovec{}.Fiov_len)]byte{}
	_ [8]byte  = [unsafe.Sizeof(libc.Tpollfd{})]byte{}
	_ [4]byte  = [unsafe.Offsetof(libc.Tpollfd{}.Fevents)]byte{}
	_ [6]byte  = [unsafe.Offsetof(libc.Tpollfd{}.Frevents)]byte{}

	_ [280]byte = [unsafe.Sizeof(libc.Tdirent{})]byte{}
	_ [8]byte   = [unsafe.Offsetof(libc.Tdirent{}.Fd_off)]byte{}
	_ [16]byte  = [unsafe.Offsetof(libc.Tdirent{}.Fd_reclen)]byte{}
	_ [18]byte  = [unsafe.Offsetof(libc.Tdirent{}.Fd_type)]byte{}
	_ [19]byte  = [unsafe.Offsetof(libc.Tdirent{}.Fd_name)]byte{}

	_ [256]byte = [unsafe.Sizeof(libc.Tstatx{})]byte{}
	_ [8]byte   = [unsafe.Offsetof(libc.Tstatx{}.Fstx_attributes)]byte{}
	_ [28]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_mode)]byte{}
	_ [32]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_ino)]byte{}
	_ [40]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_size)]byte{}
	_ [64]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_atime)]byte{}
	_ [80]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_btime)]byte{}
	_ [96]byte  = [unsafe.Offsetof(libc.Tstatx{}.Fstx_ctime)]byte{}
	_ [112]byte = [unsafe.Offsetof(libc.Tstatx{}.Fstx_mtime)]byte{}
	_ [136]byte = [unsafe.Offsetof(libc.Tstatx{}.Fstx_dev_major)]byte{}

	_ [8]byte   = [unsafe.Offsetof(libc.Tstat{}.Fst_ino)]byte{}
	_ [48]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_size)]byte{}
	_ [56]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_blksize)]byte{}
	_ [64]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_blocks)]byte{}
	_ [72]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_atim)]byte{}
	_ [88]byte  = [unsafe.Offsetof(libc.Tstat{}.Fst_mtim)]byte{}
	_ [104]byte = [unsafe.Offsetof(libc.Tstat{}.Fst_ctim)]byte{}
)
