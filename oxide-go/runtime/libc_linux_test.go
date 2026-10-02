//go:build linux && (amd64 || arm64)

package oxide

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

func libcString(c *Context, s string) uintptr {
	p := c.Alloc(uintptr(len(s)+1), 1)
	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return p
}
func libcErrno(c *Context) int32 { return *(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) }

func TestLibcSchedGetaffinity(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c := NewContext()
	defer c.Close()
	const size = 4096
	mask := c.Alloc(size+8, 8)
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(mask)), size+8)
	for i := range bytes {
		bytes[i] = 0xa5
	}
	var expected [size]byte
	n, _, errno := syscall.RawSyscall(syscall.SYS_SCHED_GETAFFINITY, 0, size, uintptr(unsafe.Pointer(&expected[0])))
	if errno != 0 {
		t.Fatalf("native affinity: %v", errno)
	}
	*(*int32)(unsafe.Pointer(LibcErrnoLocation(c))) = int32(syscall.EDOM)
	if got := LibcSchedGetaffinity(c, 0, size, mask); got != 0 {
		t.Fatalf("affinity = %d, errno = %d", got, libcErrno(c))
	}
	for i, want := range expected {
		if bytes[i] != want {
			t.Fatalf("affinity byte %d = %#x, want %#x (kernel size %d)", i, bytes[i], want, n)
		}
	}
	for _, got := range bytes[size:] {
		if got != 0xa5 {
			t.Fatal("affinity wrote past the supplied CPU set")
		}
	}
	if libcErrno(c) != int32(syscall.EDOM) {
		t.Fatal("successful affinity query changed errno")
	}
	if got := LibcSchedGetaffinity(c, 0, 0, mask); got != -1 || libcErrno(c) != int32(syscall.EINVAL) {
		t.Fatalf("empty CPU set = %d, errno = %d", got, libcErrno(c))
	}
	if got := LibcSchedGetaffinity(c, 0, size, 0); got != -1 || libcErrno(c) != int32(syscall.EFAULT) {
		t.Fatalf("null CPU set = %d, errno = %d", got, libcErrno(c))
	}
}

func TestLibcGammaBoundaries(t *testing.T) {
	c := NewContext()
	defer c.Close()
	if LibcCosh(c, 0) != 1 || LibcCoshf(c, 0) != 1 || math.Abs(LibcAcosh(c, LibcCosh(c, 1.25))-1.25) > 1e-13 {
		t.Fatal("C hyperbolic function composition")
	}
	for _, tc := range []struct{ x, want float64 }{
		{0, math.Inf(1)}, {math.Copysign(0, -1), math.Inf(-1)},
		{1, 1}, {2, 1}, {3, 2}, {4, 6}, {5, 24},
		{0.5, math.Sqrt(math.Pi)}, {-0.5, -2 * math.Sqrt(math.Pi)},
		{-1, math.NaN()}, {-2, math.NaN()}, {math.NaN(), math.NaN()},
		{math.Inf(-1), math.NaN()}, {math.Inf(1), math.Inf(1)},
	} {
		for _, got := range []float64{LibcTgamma(c, tc.x), float64(LibcTgammaf(c, float32(tc.x)))} {
			if math.IsNaN(tc.want) {
				if !math.IsNaN(got) {
					t.Fatalf("gamma(%v) = %v", tc.x, got)
				}
			} else if math.IsInf(tc.want, 0) {
				if got != tc.want {
					t.Fatalf("gamma(%v) = %v, want %v", tc.x, got, tc.want)
				}
			} else if math.Abs(got-tc.want) > 1e-6*math.Max(1, math.Abs(tc.want)) {
				t.Fatalf("gamma(%v) = %v, want %v", tc.x, got, tc.want)
			}
		}
	}
	storage := c.Alloc(16, 4)
	words := unsafe.Slice((*uint32)(unsafe.Pointer(storage)), 4)
	for _, tc := range []struct {
		x, want float64
		sign    int32
	}{
		{0, math.Inf(1), 1}, {math.Copysign(0, -1), math.Inf(1), -1},
		{1, 0, 1}, {2, 0, 1}, {-0.5, math.Log(2 * math.Sqrt(math.Pi)), -1},
	} {
		for _, width := range []int{32, 64} {
			for i := range words {
				words[i] = 0x89abcdef
			}
			var got float64
			if width == 32 {
				got = float64(LibcLgammafR(c, float32(tc.x), storage+4))
			} else {
				got = LibcLgammaR(c, tc.x, storage+4)
			}
			if int32(words[1]) != tc.sign || words[0] != 0x89abcdef || words[2] != 0x89abcdef || words[3] != 0x89abcdef {
				t.Fatalf("lgamma%d(%v) sign/adjacent storage: %x", width, tc.x, words)
			}
			if math.IsInf(tc.want, 0) {
				if got != tc.want {
					t.Fatalf("lgamma%d(%v) = %v", width, tc.x, got)
				}
			} else if math.Abs(got-tc.want) > 1e-6 {
				t.Fatalf("lgamma%d(%v) = %v, want %v", width, tc.x, got, tc.want)
			}
		}
	}
	mark := c.Mark()
	requireNoGoAllocations(t, 100, func() {
		LibcAcosh(c, 2)
		LibcAcoshf(c, 2)
		LibcAsinh(c, 0.5)
		LibcAsinhf(c, 0.5)
		LibcCosh(c, 1.25)
		LibcCoshf(c, 1.25)
		LibcTgamma(c, 0.5)
		LibcTgammaf(c, 0.5)
		LibcLgammaR(c, -0.5, storage+4)
		LibcLgammafR(c, -0.5, storage+4)
		if c.Mark() != mark {
			panic("C math leaked automatic storage")
		}
	})
}

func TestLibcFDDirectoryOwnership(t *testing.T) {
	// libc's environment lives for the process. Its TLS call-stack slots live
	// until Close, including extra nested slots first used by a valid fdopendir.
	warm := libc.NewTLS()
	warm.Close()
	baseline := libc.MemStat()
	c := NewContext()
	defer c.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "entry"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := LibcFdopendir(c, -1); p != 0 || libcErrno(c) != int32(syscall.EBADF) {
		t.Fatalf("fdopendir invalid descriptor: %#x, errno %d", p, libcErrno(c))
	}
	fd := LibcOpen(c, libcString(c, dir), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if fd < 0 {
		t.Fatalf("open directory: errno %d", libcErrno(c))
	}
	p := LibcFdopendir(c, fd)
	if p == 0 {
		LibcClose(c, fd)
		t.Fatalf("fdopendir: errno %d", libcErrno(c))
	}
	defer func() {
		if p != 0 {
			LibcClosedir(c, p)
		}
	}()
	if got := LibcDirfd(c, p); got != fd {
		t.Fatalf("directory descriptor %d, want %d", got, fd)
	}
	found := false
	for entry := LibcReaddir(c, p); entry != 0; entry = LibcReaddir(c, p) {
		name := entry + 19
		found = found || string(unsafe.Slice((*byte)(unsafe.Pointer(name)), LibcStrlen(c, name))) == "entry"
	}
	if !found {
		t.Fatal("fdopendir directory missed the file")
	}
	status := LibcClosedir(c, p)
	p = 0
	if status != 0 || LibcFcntl(c, fd, syscall.F_GETFD, 0) != -1 || libcErrno(c) != int32(syscall.EBADF) {
		t.Fatal("closedir did not consume its descriptor")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if got := libc.MemStat(); got != baseline {
		t.Fatalf("directory storage retained: %+v -> %+v", baseline, got)
	}
}

func TestLibcRelativeRemovalAndSymlinks(t *testing.T) {
	c := NewContext()
	defer c.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	fd := LibcOpen(c, libcString(c, dir), syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if fd < 0 {
		t.Fatalf("directory open: errno %d", libcErrno(c))
	}
	defer LibcClose(c, fd)
	stat := c.Alloc(unsafe.Sizeof(libc.Tstat{}), 8)
	link := libcString(c, filepath.Join(dir, "link"))
	if LibcLstat(c, link, stat) != 0 || (*libc.Tstat)(unsafe.Pointer(stat)).Fst_mode&syscall.S_IFMT != syscall.S_IFLNK {
		t.Fatal("lstat followed or lost the symlink")
	}
	if LibcStat(c, link, stat) != 0 || (*libc.Tstat)(unsafe.Pointer(stat)).Fst_mode&syscall.S_IFMT != syscall.S_IFREG {
		t.Fatal("stat did not follow the symlink")
	}
	if LibcUnlinkat(c, fd, libcString(c, "link"), 0) != 0 {
		t.Fatalf("unlinkat symlink: errno %d", libcErrno(c))
	}
	if data, err := os.ReadFile(filepath.Join(dir, "target")); err != nil || string(data) != "data" {
		t.Fatal("unlinkat removed the symlink target", err)
	}
	empty := libcString(c, "empty")
	if LibcUnlinkat(c, fd, empty, 0) != -1 || libcErrno(c) != int32(syscall.EISDIR) {
		t.Fatal("unlinkat without AT_REMOVEDIR accepted a directory")
	}
	if LibcUnlinkat(c, fd, empty, 0x200) != 0 {
		t.Fatalf("unlinkat AT_REMOVEDIR: errno %d", libcErrno(c))
	}
	if LibcUnlinkat(c, fd, empty, 0x200) != -1 || libcErrno(c) != int32(syscall.ENOENT) {
		t.Fatal("unlinkat did not remove the directory")
	}
}

func TestLibcFileAndDirectory(t *testing.T) {
	c := NewContext()
	defer c.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample")
	cp := libcString(c, path)
	fd := LibcOpen(c, cp, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC, 0o600)
	if fd < 0 {
		t.Fatalf("open: errno %d", libcErrno(c))
	}
	defer LibcClose(c, fd)
	if flags := LibcFcntl(c, fd, syscall.F_GETFD, 0); flags&syscall.FD_CLOEXEC == 0 {
		t.Fatalf("fcntl(F_GETFD): %d", flags)
	}
	payload := libcString(c, "abcDEF")
	iov := c.Alloc(32, 8)
	*(*libc.Tiovec)(unsafe.Pointer(iov)) = libc.Tiovec{Fiov_base: payload, Fiov_len: 3}
	*(*libc.Tiovec)(unsafe.Pointer(iov + 16)) = libc.Tiovec{Fiov_base: payload + 3, Fiov_len: 3}
	if got := LibcWritev(c, fd, iov, 2); got != 6 {
		t.Fatalf("writev: %d, errno %d", got, libcErrno(c))
	}
	buf := c.Alloc(16, 8)
	if LibcLseek(c, fd, 0, 0) != 0 || LibcRead(c, fd, buf, 6) != 6 {
		t.Fatalf("seek/read: errno %d", libcErrno(c))
	}
	if got := string(unsafe.Slice((*byte)(unsafe.Pointer(buf)), 6)); got != "abcDEF" {
		t.Fatalf("read: %q", got)
	}
	if LibcPwrite(c, fd, payload, 3, 3) != 3 || LibcPread(c, fd, buf, 6, 0) != 6 {
		t.Fatalf("pread/pwrite: errno %d", libcErrno(c))
	}
	if got := string(unsafe.Slice((*byte)(unsafe.Pointer(buf)), 6)); got != "abcabc" {
		t.Fatalf("pread: %q", got)
	}
	stat := c.Alloc(unsafe.Sizeof(libc.Tstat{}), 8)
	if LibcFstat(c, fd, stat) != 0 || *(*int64)(unsafe.Pointer(stat + 48)) != 6 {
		t.Fatalf("GNU fstat layout: errno %d", libcErrno(c))
	}
	if LibcStat(c, cp, stat) != 0 || LibcFstatat(c, -100, cp, stat, 0) != 0 {
		t.Fatalf("stat/fstatat: errno %d", libcErrno(c))
	}
	stx := c.Alloc(256, 8)
	if LibcStatx(c, -100, cp, 0, 0x7ff, stx) != 0 {
		t.Fatalf("statx: errno %d", libcErrno(c))
	}
	if size := *(*uint64)(unsafe.Pointer(stx + 40)); size != 6 {
		t.Fatalf("GNU statx size: %d", size)
	}
	d := LibcOpendir(c, libcString(c, dir))
	if d == 0 {
		t.Fatalf("opendir: errno %d", libcErrno(c))
	}
	defer LibcClosedir(c, d)
	if LibcDirfd(c, d) < 0 {
		t.Fatal("dirfd failed")
	}
	found := false
	for entry := LibcReaddir(c, d); entry != 0; entry = LibcReaddir(c, d) {
		name := entry + 19
		if string(unsafe.Slice((*byte)(unsafe.Pointer(name)), LibcStrlen(c, name))) == "sample" {
			found = true
		}
	}
	if !found {
		t.Fatal("readdir did not return the created file")
	}
	resolved := LibcRealpath(c, cp, 0)
	if resolved == 0 {
		t.Fatalf("realpath: errno %d", libcErrno(c))
	}
	defer LibcFree(c, resolved)
	if got := string(unsafe.Slice((*byte)(unsafe.Pointer(resolved)), LibcStrlen(c, resolved))); got != path {
		t.Fatalf("realpath: %q != %q", got, path)
	}
}

func TestLibcErrnoPerContext(t *testing.T) {
	a, b := NewContext(), NewContext()
	defer a.Close()
	defer b.Close()
	pa, pb := LibcErrnoLocation(a), LibcErrnoLocation(b)
	if pa == pb {
		t.Fatal("Rust threads share errno storage")
	}
	*(*int32)(unsafe.Pointer(pb)) = int32(syscall.ENOENT)
	if LibcRead(a, -1, 0, 0) != -1 || libcErrno(a) != int32(syscall.EBADF) {
		t.Fatalf("read(-1): errno %d", libcErrno(a))
	}
	runtime.GC()
	if LibcErrnoLocation(a) != pa || LibcErrnoLocation(b) != pb || libcErrno(b) != int32(syscall.ENOENT) {
		t.Fatal("errno location moved or another Context changed it")
	}
	if pid := LibcSyscall(a, syscall.SYS_GETPID); pid != int64(os.Getpid()) || libcErrno(a) != int32(syscall.EBADF) {
		t.Fatalf("getpid: %d, errno %d", pid, libcErrno(a))
	}
	if LibcSyscall(a, syscall.SYS_CLOSE, ^uintptr(0)) != -1 || libcErrno(a) != int32(syscall.EBADF) {
		t.Fatal("raw syscall did not set Context errno")
	}
	buf := a.Alloc(128, 1)
	if LibcStrerrorR(a, int32(syscall.ENOENT), buf, 128) != 0 || LibcStrlen(a, buf) == 0 {
		t.Fatal("strerror_r did not return an error message")
	}
	if LibcStrerrorR(a, int32(syscall.ENOENT), buf, 1) != int32(syscall.ERANGE) {
		t.Fatal("strerror_r did not report a short buffer")
	}
}

func TestLibcMemoryAndMapping(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := LibcCalloc(c, 16, 2)
	if p == 0 {
		t.Fatalf("calloc: errno %d", libcErrno(c))
	}
	for _, value := range unsafe.Slice((*byte)(unsafe.Pointer(p)), 32) {
		if value != 0 {
			t.Fatal("calloc returned nonzero memory")
		}
	}
	LibcMemset(c, p, 0x7f, 32)
	q := LibcRealloc(c, p, 128)
	if q == 0 {
		LibcFree(c, p)
		t.Fatalf("realloc: errno %d", libcErrno(c))
	}
	defer LibcFree(c, q)
	for _, value := range unsafe.Slice((*byte)(unsafe.Pointer(q)), 32) {
		if value != 0x7f {
			t.Fatal("realloc lost existing bytes")
		}
	}
	if LibcRealloc(c, q, ^uintptr(0)) != 0 || libcErrno(c) != int32(syscall.ENOMEM) {
		t.Fatal("oversized realloc did not preserve the old allocation")
	}
	if LibcCalloc(c, ^uintptr(0), 2) != 0 || libcErrno(c) != int32(syscall.ENOMEM) {
		t.Fatal("calloc multiplication overflow was not rejected")
	}
	mapping := LibcMmap(c, 0, 4096, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON, -1, 0)
	if mapping == ^uintptr(0) {
		t.Fatalf("mmap: errno %d", libcErrno(c))
	}
	LibcMemcpy(c, mapping, q, 32)
	LibcMemmove(c, mapping+1, mapping, 31)
	if LibcMemcmp(c, mapping, q, 32) != 0 {
		t.Fatal("memory copy/move changed bytes")
	}
	if LibcMprotect(c, mapping, 4096, syscall.PROT_READ) != 0 || LibcMunmap(c, mapping, 4096) != 0 {
		t.Fatalf("mprotect/munmap: errno %d", libcErrno(c))
	}
}

func TestLibcRandomTimeAuxv(t *testing.T) {
	c := NewContext()
	defer c.Close()
	p := c.Alloc(32, 8)
	if n := LibcGetrandom(c, p, 32, 0); n != 32 {
		t.Fatalf("getrandom: %d, errno %d", n, libcErrno(c))
	}
	if LibcClockGettime(c, 1, p) != 0 {
		t.Fatalf("clock_gettime: errno %d", libcErrno(c))
	}
	if nsec := *(*int64)(unsafe.Pointer(p + 8)); nsec < 0 || nsec >= 1_000_000_000 {
		t.Fatalf("GNU timespec nanoseconds: %d", nsec)
	}
	if page := LibcGetauxval(c, 6); page != uint64(os.Getpagesize()) {
		t.Fatalf("AT_PAGESZ: %d", page)
	}
	if LibcGetauxval(c, ^uint64(0)) != 0 || libcErrno(c) != int32(syscall.ENOENT) {
		t.Fatal("missing auxiliary vector key did not set ENOENT")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if tid := LibcGettid(c); tid != int32(syscall.Gettid()) {
		t.Fatalf("gettid: %d", tid)
	}
	requireNoGoAllocations(t, 100, func() {
		LibcRead(c, -1, 0, 0)
		LibcClockGettime(c, 1, p)
		LibcGetrandom(c, p, 16, 0)
		LibcGetauxval(c, 6)
	})
}

func TestLibcAlignedAllocation(t *testing.T) {
	c := NewContext()
	defer c.Close()
	out := c.Alloc(8, 8)
	errno := LibcErrnoLocation(c)
	*(*int32)(unsafe.Pointer(errno)) = int32(syscall.EBADF)
	for _, align := range []uintptr{0, 1, 4, 12} {
		*(*uintptr)(unsafe.Pointer(out)) = 123
		if code := LibcPosixMemalign(c, out, align, 100); code != int32(syscall.EINVAL) {
			t.Fatalf("posix_memalign(%d): %d", align, code)
		}
		if *(*uintptr)(unsafe.Pointer(out)) != 123 || libcErrno(c) != int32(syscall.EBADF) {
			t.Fatal("invalid alignment changed output or errno")
		}
	}
	if code := LibcPosixMemalign(c, out, 64, ^uintptr(0)); code != int32(syscall.ENOMEM) {
		t.Fatalf("oversized posix_memalign: %d", code)
	}
	if *(*uintptr)(unsafe.Pointer(out)) != 123 || libcErrno(c) != int32(syscall.EBADF) {
		t.Fatal("failed posix_memalign changed output or errno")
	}
	for _, align := range []uintptr{8, 16, 64, 4096, 8192} {
		if code := LibcPosixMemalign(c, out, align, 100); code != 0 {
			t.Fatalf("posix_memalign(%d): %d", align, code)
		}
		p := *(*uintptr)(unsafe.Pointer(out))
		if p == 0 || p%align != 0 {
			t.Fatalf("posix_memalign(%d): address %#x", align, p)
		}
		LibcMemset(c, p, 0x39, 100)
		q := LibcRealloc(c, p, 150)
		if q == 0 {
			LibcFree(c, p)
			t.Fatal("realloc of aligned C allocation failed")
		}
		for _, b := range unsafe.Slice((*byte)(unsafe.Pointer(q)), 100) {
			if b != 0x39 {
				t.Fatal("realloc of aligned allocation lost content")
			}
		}
		LibcFree(c, q)
	}
	if libcErrno(c) != int32(syscall.EBADF) {
		t.Fatal("successful allocation changed errno")
	}
}

func TestLibcOwnedStringsAndSysconf(t *testing.T) {
	c := NewContext()
	defer c.Close()
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []uintptr{0, 4096} {
		p := LibcGetcwd(c, 0, size)
		if p == 0 {
			t.Fatalf("getcwd(NULL, %d): errno %d", size, libcErrno(c))
		}
		if got := string(unsafe.Slice((*byte)(unsafe.Pointer(p)), LibcStrlen(c, p))); got != want {
			t.Fatalf("getcwd: %q, want %q", got, want)
		}
		LibcFree(c, p)
	}
	buf := c.Alloc(4096, 8)
	if LibcGetcwd(c, buf, 4096) != buf || LibcRealpath(c, libcString(c, "."), buf) != buf {
		t.Fatal("getcwd/realpath did not retain the supplied buffer")
	}
	if pagesize := LibcSysconf(c, 30); pagesize != int64(os.Getpagesize()) {
		t.Fatalf("sysconf(_SC_PAGESIZE): %d", pagesize)
	}
	if cpus := LibcSysconf(c, 84); cpus < 1 {
		t.Fatalf("sysconf(_SC_NPROCESSORS_ONLN): %d", cpus)
	}
	if LibcSysconf(c, 73) != pthreadDestructorIterations || LibcSysconf(c, 74) != pthreadKeysMax {
		t.Fatal("sysconf does not describe the translated pthread runtime limits")
	}
	if result := LibcSysconf(c, -1); result != -1 || libcErrno(c) != int32(syscall.EINVAL) {
		t.Fatalf("sysconf(-1): %d, errno %d", result, libcErrno(c))
	}
	template := libcString(c, filepath.Join(t.TempDir(), "temp-XXXXXX"))
	fd := LibcMkstemp(c, template)
	if fd < 0 {
		t.Fatalf("mkstemp: errno %d", libcErrno(c))
	}
	defer LibcClose(c, fd)
	if LibcUnlink(c, template) != 0 {
		t.Fatalf("unlink mkstemp: errno %d", libcErrno(c))
	}
}
