#![feature(linkage)]

use libc::{c_char, c_int, c_uint, c_void, pid_t, size_t, ssize_t};

type GetRandom = unsafe extern "C" fn(*mut c_void, size_t, c_uint) -> ssize_t;
type GetTid = unsafe extern "C" fn() -> pid_t;
type Statx = unsafe extern "C" fn(c_int, *const c_char, c_int, c_uint, *mut libc::statx) -> c_int;

unsafe extern "C" {
    #[linkage = "extern_weak"]
    #[link_name = "getrandom"]
    static WEAK_GETRANDOM: Option<GetRandom>;
    #[linkage = "extern_weak"]
    #[link_name = "gettid"]
    static WEAK_GETTID: Option<GetTid>;
    #[linkage = "extern_weak"]
    #[link_name = "statx"]
    static WEAK_STATX: Option<Statx>;
}

fn random() -> u64 {
    match unsafe { WEAK_GETRANDOM } {
        Some(getrandom) => {
            let mut bytes = [0u8; 32];
            u64::from(unsafe { getrandom(bytes.as_mut_ptr().cast(), bytes.len(), 0) } == 32)
        }
        None => 0,
    }
}

fn tid() -> u64 {
    match unsafe { WEAK_GETTID } {
        Some(gettid) => u64::from(unsafe { gettid() } > 0),
        None => 0,
    }
}

fn stat() -> u64 {
    match unsafe { WEAK_STATX } {
        Some(statx) => {
            let mut stat = unsafe { core::mem::zeroed::<libc::statx>() };
            let result = unsafe {
                statx(
                    libc::AT_FDCWD,
                    c"/dev/null".as_ptr(),
                    0,
                    libc::STATX_TYPE,
                    &mut stat,
                )
            };
            u64::from(result == 0 && stat.stx_mode as u32 & libc::S_IFMT == libc::S_IFCHR)
        }
        None => 0,
    }
}

fn file() -> u64 {
    const CONTENT: &[u8] = b"oxide Rust extern C fixture\n";
    // The harness supplies a private writable working directory. A fixed /tmp
    // would make this C ABI test depend on an unrelated filesystem's capacity.
    let mut path = *b"oxide-libc-XXXXXX\0";
    let initial = unsafe { libc::mkstemp(path.as_mut_ptr().cast()) };
    if initial < 0 {
        return 0;
    }
    let first_close = unsafe { libc::close(initial) };
    // O_CREAT makes the variadic mode argument part of the real C call.
    let fd = unsafe {
        libc::open(
            path.as_ptr().cast(),
            libc::O_RDWR | libc::O_CREAT | libc::O_TRUNC,
            0o600 as libc::mode_t,
        )
    };
    if fd < 0 {
        unsafe { libc::unlink(path.as_ptr().cast()) };
        return 0;
    }
    let written = unsafe { libc::write(fd, CONTENT.as_ptr().cast(), CONTENT.len()) };
    let position = unsafe { libc::lseek(fd, 0, libc::SEEK_SET) };
    let mut bytes = [0u8; 64];
    let read = unsafe { libc::read(fd, bytes.as_mut_ptr().cast(), CONTENT.len()) };
    let closed = unsafe { libc::close(fd) };
    let unlinked = unsafe { libc::unlink(path.as_ptr().cast()) };
    if first_close != 0
        || written != CONTENT.len() as isize
        || position != 0
        || read != CONTENT.len() as isize
        || closed != 0
        || unlinked != 0
        || &bytes[..CONTENT.len()] != CONTENT
    {
        return 0;
    }
    bytes[..CONTENT.len()].iter().fold(1u64, |hash, &byte| {
        hash.wrapping_mul(131).wrapping_add(byte as u64)
    })
}

fn errno() -> u64 {
    unsafe { *libc::__errno_location() = 0 };
    let fd = unsafe { libc::open(c"/dev/null/oxide-fixture".as_ptr(), libc::O_RDONLY) };
    let error = unsafe { *libc::__errno_location() };
    if fd >= 0 {
        unsafe { libc::close(fd) };
    }
    u64::from(fd == -1 && error == libc::ENOTDIR)
}

fn aligned(seed: u64) -> u64 {
    let mut pointer: *mut c_void = core::ptr::null_mut();
    if unsafe { libc::posix_memalign(&mut pointer, 64, 96) } != 0 {
        return 0;
    }
    let valid = !pointer.is_null() && pointer as usize % 64 == 0;
    let bytes = unsafe { core::slice::from_raw_parts_mut(pointer.cast::<u8>(), 96) };
    for (i, byte) in bytes.iter_mut().enumerate() {
        *byte = seed.wrapping_add(i as u64) as u8;
    }
    let hash = bytes.iter().fold(u64::from(valid), |hash, &byte| {
        hash.wrapping_mul(131).wrapping_add(byte as u64)
    });
    // The pointer belongs to the C allocator, not the Rust allocator.
    unsafe { libc::free(pointer) };
    hash
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    match case {
        0 => random(),
        1 => tid(),
        2 => stat(),
        3 => file(),
        4 => errno(),
        5 => aligned(seed),
        6 => {
            let mut buffer = [0u8; 128];
            let mut count = -1;
            let value = 0xfedc_ba98_7654_3210u64 | seed;
            let output = unsafe {
                libc::snprintf(
                    buffer.as_mut_ptr().cast(),
                    buffer.len(),
                    c"%d|%llu|%.2f|%s%n".as_ptr(),
                    -(seed as i32),
                    value,
                    1.25f64,
                    c"oxide".as_ptr(),
                    &mut count as *mut i32,
                )
            };
            let expected = format!("{}|{}|1.25|oxide", -(seed as i32), value);
            assert_eq!(output as usize, expected.len());
            assert_eq!(count, output);
            assert_eq!(&buffer[..expected.len()], expected.as_bytes());
            assert_eq!(buffer[expected.len()], 0);
            1
        }
        7 => {
            let mut buffer = [0xaa_u8; 4];
            let f: unsafe extern "C" fn(*mut c_char, size_t, *const c_char, ...) -> c_int =
                libc::snprintf;
            let output = unsafe {
                f(
                    buffer.as_mut_ptr().cast(),
                    buffer.len(),
                    c"%s-%d".as_ptr(),
                    c"oxide".as_ptr(),
                    seed as i32,
                )
            };
            let expected = format!("oxide-{seed}");
            assert_eq!(output as usize, expected.len());
            assert_eq!(buffer, [b'o', b'x', b'i', 0]);
            1
        }
        8 => {
            let f: unsafe extern "C" fn(*const c_char, ...) -> c_int = libc::printf;
            let mut count = -1;
            assert_eq!(unsafe { f(c"%n".as_ptr(), &mut count as *mut i32) }, 0);
            assert_eq!(count, 0);
            1
        }
        9 => {
            let mut mask = [0xa5u8; 4096];
            unsafe { *libc::__errno_location() = libc::EDOM };
            assert_eq!(unsafe { libc::sched_getaffinity(0, mask.len(), mask.as_mut_ptr().cast()) }, 0);
            assert!(mask.iter().any(|&byte| byte != 0));
            assert_eq!(mask[4095], 0);
            assert_eq!(unsafe { *libc::__errno_location() }, libc::EDOM);
            assert_eq!(unsafe { libc::sched_getaffinity(0, 0, mask.as_mut_ptr().cast()) }, -1);
            assert_eq!(unsafe { *libc::__errno_location() }, libc::EINVAL);
            assert_eq!(unsafe { libc::sched_getaffinity(0, mask.len(), core::ptr::null_mut()) }, -1);
            assert_eq!(unsafe { *libc::__errno_location() }, libc::EFAULT);
            1
        }
        _ => panic!("unknown C ABI fixture"),
    }
}
