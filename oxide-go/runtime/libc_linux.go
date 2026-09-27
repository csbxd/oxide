//go:build linux && (amd64 || arm64)

package oxide

import (
	"encoding/binary"
	"math/bits"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"modernc.org/libc"
)

// C errno belongs to the Rust thread represented by Context, not whichever
// operating-system thread happens to run the Go goroutine. libc owns its TLS
// storage until Context.Close; its address stays outside Go's moving stack.
func (c *Context) libc() *libc.TLS {
	if c.libcTLS == nil {
		c.libcTLS = libc.NewTLS()
	}
	return c.libcTLS
}

func LibcErrnoLocation(c *Context) uintptr { return libc.X__errno_location(c.libc()) }

func LibcRead(c *Context, fd int32, buf, count uintptr) int64 {
	return libc.Xread(c.libc(), fd, buf, uint64(count))
}
func LibcWrite(c *Context, fd int32, buf, count uintptr) int64 {
	return libc.Xwrite(c.libc(), fd, buf, uint64(count))
}
func LibcReadv(c *Context, fd int32, iov uintptr, count int32) int64 {
	return libc.Xreadv(c.libc(), fd, iov, count)
}
func LibcWritev(c *Context, fd int32, iov uintptr, count int32) int64 {
	return libc.Xwritev(c.libc(), fd, iov, count)
}
func LibcPread(c *Context, fd int32, buf, count uintptr, offset int64) int64 {
	return libc.Xpread(c.libc(), fd, buf, uint64(count), offset)
}
func LibcPwrite(c *Context, fd int32, buf, count uintptr, offset int64) int64 {
	return libc.Xpwrite(c.libc(), fd, buf, uint64(count), offset)
}
func LibcClose(c *Context, fd int32) int32 { return libc.Xclose(c.libc(), fd) }
func LibcLseek(c *Context, fd int32, offset int64, whence int32) int64 {
	return libc.Xlseek(c.libc(), fd, offset, whence)
}
func LibcOpen(c *Context, path uintptr, flags int32, mode uint32) int32 {
	return LibcOpenat(c, -100, path, flags, mode)
}
func LibcMkstemp(c *Context, template uintptr) int32 { return libc.Xmkstemp(c.libc(), template) }
func LibcOpenat(c *Context, fd int32, path uintptr, flags int32, mode uint32) int32 {
	mark := c.Mark()
	defer c.Restore(mark)
	args := c.Alloc(8, 8)
	*(*uint64)(unsafe.Pointer(args)) = uint64(mode)
	return libc.Xopenat(c.libc(), fd, path, flags, args)
}
func LibcFcntl(c *Context, fd, command int32, argument uintptr) int32 {
	mark := c.Mark()
	defer c.Restore(mark)
	args := c.Alloc(8, 8)
	*(*uintptr)(unsafe.Pointer(args)) = argument
	return libc.Xfcntl(c.libc(), fd, command, args)
}
func LibcPoll(c *Context, fds uintptr, count uint64, timeout int32) int32 {
	return libc.Xpoll(c.libc(), fds, count, timeout)
}
func LibcIsatty(c *Context, fd int32) int32 { return libc.Xisatty(c.libc(), fd) }
func LibcFsync(c *Context, fd int32) int32  { return libc.Xfsync(c.libc(), fd) }
func LibcFtruncate(c *Context, fd int32, length int64) int32 {
	return libc.Xftruncate(c.libc(), fd, length)
}

func LibcMkdir(c *Context, path uintptr, mode uint32) int32 {
	return libc.Xmkdir(c.libc(), path, mode)
}
func LibcUnlink(c *Context, path uintptr) int32 { return libc.Xunlink(c.libc(), path) }
func LibcRename(c *Context, old, new uintptr) int32 {
	return libc.Xrename(c.libc(), old, new)
}
func LibcReadlink(c *Context, path, buf, size uintptr) int64 {
	return libc.Xreadlink(c.libc(), path, buf, uint64(size))
}
func LibcRealpath(c *Context, path, resolved uintptr) uintptr {
	p := libc.Xrealpath(c.libc(), path, resolved)
	if p != 0 && resolved == 0 {
		return libcOwnedString(c, p, 0)
	}
	return p
}
func LibcGetcwd(c *Context, buf, size uintptr) uintptr {
	p := libc.Xgetcwd(c.libc(), buf, uint64(size))
	if p != 0 && buf == 0 {
		return libcOwnedString(c, p, size)
	}
	return p
}

// Transfer malloc-owned results from libc's internal allocator to the common
// C/Rust allocation ABI. Opaque DIR/TLS and borrowed getenv strings stay in libc.
func libcOwnedString(c *Context, p, size uintptr) uintptr {
	n := uintptr(libc.Xstrlen(c.libc(), p)) + 1
	q := LibcMalloc(c, max(size, n))
	if q != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(q)), n), unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	}
	libc.Xfree(c.libc(), p)
	return q
}
func LibcGetenv(c *Context, name uintptr) uintptr { return libc.Xgetenv(c.libc(), name) }
func LibcOpendir(c *Context, name uintptr) uintptr {
	return libc.Xopendir(c.libc(), name)
}
func LibcClosedir(c *Context, dir uintptr) int32  { return libc.Xclosedir(c.libc(), dir) }
func LibcDirfd(c *Context, dir uintptr) int32     { return libc.Xdirfd(c.libc(), dir) }
func LibcReaddir(c *Context, dir uintptr) uintptr { return libc.Xreaddir(c.libc(), dir) }
func LibcStat(c *Context, path, buf uintptr) int32 {
	return libc.Xstat(c.libc(), path, buf)
}
func LibcFstat(c *Context, fd int32, buf uintptr) int32 {
	return libc.Xfstat(c.libc(), fd, buf)
}
func LibcFstatat(c *Context, dirfd int32, path, buf uintptr, flags int32) int32 {
	return libc.Xfstatat(c.libc(), dirfd, path, buf, flags)
}

func LibcMmap(c *Context, address, length uintptr, prot, flags, fd int32, offset int64) uintptr {
	return libc.Xmmap(c.libc(), address, uint64(length), prot, flags, fd, offset)
}
func LibcMunmap(c *Context, address, length uintptr) int32 {
	return libc.Xmunmap(c.libc(), address, uint64(length))
}
func LibcMprotect(c *Context, address, length uintptr, prot int32) int32 {
	return libc.Xmprotect(c.libc(), address, uint64(length), prot)
}
func LibcGetrandom(c *Context, buf, size uintptr, flags uint32) int64 {
	return libc.Xgetrandom(c.libc(), buf, uint64(size), flags)
}
func LibcGettid(c *Context) int32 {
	id, _, err := syscall.RawSyscall(syscall.SYS_GETTID, 0, 0, 0)
	if err != 0 {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(err)
		return -1
	}
	return int32(id)
}

// Callers that rely on native thread identity must bind the entire Rust entry
// to an OS thread. Locking only around this syscall would not preserve it.
func LibcSyscall(c *Context, number int64, arguments ...uintptr) int64 {
	if len(arguments) > 6 {
		panic("oxide: Linux syscalls accept at most six arguments")
	}
	var args [6]uintptr
	copy(args[:], arguments)
	r, _, err := syscall.Syscall6(uintptr(number), args[0], args[1], args[2], args[3], args[4], args[5])
	if err != 0 {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(err)
		return -1
	}
	return int64(r)
}
func LibcPause(c *Context) int32 { return libc.Xpause(c.libc()) }

var auxvOnce sync.Once
var auxv map[uint64]uint64

func LibcGetauxval(c *Context, key uint64) uint64 {
	auxvOnce.Do(func() {
		data, err := os.ReadFile("/proc/self/auxv")
		if err != nil {
			panic(err)
		}
		if len(data)%16 != 0 {
			panic("oxide: invalid Linux auxiliary vector")
		}
		// Some libc routines (including sysconf) read the startup vector
		// internally. ccgo does not install it when embedded in a Go program.
		p := RustAlloc(uintptr(len(data)), 8)
		if p == 0 {
			panic("oxide: cannot allocate Linux auxiliary vector")
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
		libc.X__libc.Fauxv = p
		auxv = make(map[uint64]uint64, len(data)/16)
		for len(data) >= 16 {
			key := binary.LittleEndian.Uint64(data)
			if key == 0 {
				break
			}
			auxv[key] = binary.LittleEndian.Uint64(data[8:])
			data = data[16:]
		}
	})
	if auxv == nil {
		panic("oxide: Linux auxiliary vector initialization failed")
	}
	v, ok := auxv[key]
	if !ok {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(syscall.ENOENT)
	}
	return v
}

func LibcSysconf(c *Context, name int32) int64 {
	switch name {
	case 73: // _SC_THREAD_DESTRUCTOR_ITERATIONS
		return pthreadDestructorIterations
	case 74: // _SC_THREAD_KEYS_MAX
		return pthreadKeysMax
	}
	// The pinned Linux musl constants share GNU's numeric _SC namespace.
	// Initialize auxv before selectors that inspect AT_MINSIGSTKSZ.
	LibcGetauxval(c, 6)
	return libc.Xsysconf(c.libc(), name)
}

// Linux's 64-bit GNU and musl ABIs use the kernel timespec, iovec, pollfd and
// statx layouts. Their sizes and field offsets are checked in libc_linux_test.
func LibcClockGettime(c *Context, clock int32, ts uintptr) int32 {
	return libc.Xclock_gettime(c.libc(), clock, ts)
}
func LibcStatx(c *Context, dirfd int32, path uintptr, flags int32, mask uint32, buf uintptr) int32 {
	return libc.Xstatx(c.libc(), dirfd, path, flags, mask, buf)
}

func LibcMalloc(c *Context, size uintptr) uintptr {
	p := RustAlloc(size, 16)
	if p == 0 {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(syscall.ENOMEM)
	}
	return p
}
func LibcCalloc(c *Context, count, size uintptr) uintptr {
	hi, lo := bits.Mul64(uint64(count), uint64(size))
	if hi != 0 {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(syscall.ENOMEM)
		return 0
	}
	// RustAlloc clears both fresh and reused storage.
	return LibcMalloc(c, uintptr(lo))
}
func LibcRealloc(c *Context, p, size uintptr) uintptr {
	if p == 0 {
		return LibcMalloc(c, size)
	}
	if size == 0 {
		LibcFree(c, p)
		return 0
	}
	q := RustRealloc(p, allocationSize(p), 16, size)
	if q == 0 {
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(syscall.ENOMEM)
	}
	return q
}
func LibcFree(c *Context, p uintptr) { RustDealloc(p, 0, 0) }
func LibcPosixMemalign(c *Context, out, align, size uintptr) int32 {
	if align < 8 || align&(align-1) != 0 {
		return int32(syscall.EINVAL)
	}
	p := RustAlloc(size, align)
	if p == 0 {
		return int32(syscall.ENOMEM)
	}
	*(*uintptr)(unsafe.Pointer(out)) = p
	return 0
}
func LibcStrlen(c *Context, p uintptr) uintptr {
	return uintptr(libc.Xstrlen(c.libc(), p))
}
func LibcMemcpy(c *Context, dst, src, size uintptr) uintptr {
	return libc.Xmemcpy(c.libc(), dst, src, uint64(size))
}
func LibcMemmove(c *Context, dst, src, size uintptr) uintptr {
	return libc.Xmemmove(c.libc(), dst, src, uint64(size))
}
func LibcMemset(c *Context, dst uintptr, value int32, size uintptr) uintptr {
	return libc.Xmemset(c.libc(), dst, value, uint64(size))
}
func LibcMemcmp(c *Context, left, right, size uintptr) int32 {
	return libc.Xmemcmp(c.libc(), left, right, uint64(size))
}
func LibcStrerrorR(c *Context, errno int32, buf, size uintptr) int32 {
	return libc.X__xpg_strerror_r(c.libc(), errno, buf, uint64(size))
}
