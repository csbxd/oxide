use std::mem::{ManuallyDrop, align_of_val, size_of_val};
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

static DROPS: AtomicU64 = AtomicU64::new(0);

trait Pattern {
    fn read(&self) -> u64;
}

#[repr(align(32))]
struct Aligned32([u64; 4]);
#[repr(align(64))]
struct Aligned64([u64; 8]);

impl Pattern for Aligned32 {
    fn read(&self) -> u64 {
        assert_eq!((self as *const Self as usize) % 32, 0);
        self.0[0] ^ self.0[1].rotate_left(7) ^ self.0[3].rotate_right(9)
    }
}
impl Pattern for Aligned64 {
    fn read(&self) -> u64 {
        assert_eq!((self as *const Self as usize) % 64, 0);
        self.0[0] ^ self.0[2].rotate_left(13) ^ self.0[7].rotate_right(17)
    }
}
impl Drop for Aligned32 {
    fn drop(&mut self) {
        DROPS.fetch_add(1, Ordering::Relaxed);
    }
}
impl Drop for Aligned64 {
    fn drop(&mut self) {
        DROPS.fetch_add(1, Ordering::Relaxed);
    }
}

#[repr(C)]
struct Tail<T: ?Sized> {
    prefix: u8,
    value: T,
}
#[repr(C)]
struct Nested<T: ?Sized> {
    prefix: u16,
    value: Tail<T>,
}
#[repr(C, packed(2))]
struct Packed<T: ?Sized> {
    prefix: u8,
    value: ManuallyDrop<T>,
}

fn arc_pattern(value: Arc<dyn Pattern>) -> u64 {
    let cloned = value.clone();
    let pattern = value.read();
    let result = pattern
        ^ (align_of_val(&*value) as u64).rotate_left(3)
        ^ (size_of_val(&*value) as u64).rotate_left(19);
    drop(value);
    assert_eq!(DROPS.load(Ordering::Relaxed), 0);
    assert_eq!(cloned.read(), pattern);
    drop(cloned);
    assert_eq!(DROPS.load(Ordering::Relaxed), 1);
    result
}

fn nested_pattern(value: Arc<Nested<dyn Pattern>>) -> u64 {
    let cloned = value.clone();
    let pattern = value.value.value.read();
    let result = pattern
        ^ value.prefix as u64
        ^ value.value.prefix as u64
        ^ (align_of_val(&*value) as u64).rotate_left(3)
        ^ (size_of_val(&*value) as u64).rotate_left(19);
    drop(value);
    assert_eq!(DROPS.load(Ordering::Relaxed), 0);
    assert_eq!(cloned.value.value.read(), pattern);
    drop(cloned);
    assert_eq!(DROPS.load(Ordering::Relaxed), 1);
    result
}

#[no_mangle]
pub fn conformance(case: u64, seed: u64) -> u64 {
    DROPS.store(0, Ordering::Relaxed);
    let a = [seed, seed.wrapping_add(0x1234), seed ^ 0xa5a5, !seed];
    let b = [seed, 1, seed.wrapping_add(0x1234), 3, 4, 5, 6, !seed];
    let result = match case {
        0 => arc_pattern(Arc::new(Aligned32(a))),
        1 => arc_pattern(Arc::new(Aligned64(b))),
        2 => nested_pattern(Arc::new(Nested {
            prefix: 0x789a,
            value: Tail {
                prefix: 0x37,
                value: Aligned32(a),
            },
        })),
        3 => nested_pattern(Arc::new(Nested {
            prefix: 0x789a,
            value: Tail {
                prefix: 0x37,
                value: Aligned64(b),
            },
        })),
        4 => {
            let value: Box<Tail<[u64]>> = Box::new(Tail {
                prefix: 0x37,
                value: a,
            });
            let result =
                value.value[0] ^ value.value[3] ^ (size_of_val(&*value) as u64).rotate_left(9);
            assert_eq!(value.prefix, 0x37);
            result
        }
        5 => {
            let value: Box<Packed<[u64]>> = Box::new(Packed {
                prefix: 0x37,
                value: ManuallyDrop::new(a),
            });
            // A packed DST field must use its capped alignment. Form only a
            // raw pointer and read unaligned; borrowing the field is invalid.
            let data = std::ptr::addr_of!(value.value) as *const u64;
            let result = unsafe { data.read_unaligned() ^ data.add(3).read_unaligned() }
                ^ (size_of_val(&*value) as u64).rotate_left(9);
            assert_eq!(value.prefix, 0x37);
            assert_eq!(align_of_val(&*value), 2);
            result
        }
        6 => {
            let value: Box<Packed<dyn Pattern>> = Box::new(Packed {
                prefix: 0x37,
                value: ManuallyDrop::new(Aligned64(b)),
            });
            let data = std::ptr::addr_of!(value.value) as *const u64;
            let result = unsafe { data.read_unaligned() ^ data.add(7).read_unaligned() }
                ^ (size_of_val(&*value) as u64).rotate_left(9);
            assert_eq!(value.prefix, 0x37);
            assert_eq!(align_of_val(&*value), 2);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 0);
            result
        }
        7 => {
            let value = Aligned32(a);
            let marker: &dyn Send = &value;
            let result = size_of_val(marker) as u64 ^ (align_of_val(marker) as u64).rotate_left(9);
            assert_eq!(size_of_val(marker), 32);
            assert_eq!(align_of_val(marker), 32);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 1);
            result
        }
        8 => {
            let value: Box<dyn Send> = Box::new(Aligned64(b));
            let result =
                size_of_val(&*value) as u64 ^ (align_of_val(&*value) as u64).rotate_left(9);
            assert_eq!(((&*value as *const dyn Send) as *const () as usize) % 64, 0);
            assert_eq!(size_of_val(&*value), 64);
            assert_eq!(align_of_val(&*value), 64);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 1);
            result
        }
        9 => {
            let value: Box<dyn Send + Sync> = Box::new(Aligned64(b));
            let result =
                size_of_val(&*value) as u64 ^ (align_of_val(&*value) as u64).rotate_left(9);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 1);
            result
        }
        10 => {
            let value: Box<Tail<dyn Send>> = Box::new(Tail {
                prefix: 0x37,
                value: Aligned64(b),
            });
            assert_eq!(size_of_val(&*value), 128);
            assert_eq!(align_of_val(&*value), 64);
            assert_eq!(size_of_val(&value.value), 64);
            assert_eq!(align_of_val(&value.value), 64);
            let result = value.prefix as u64 ^ (size_of_val(&*value) as u64).rotate_left(9);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 1);
            result
        }
        11 => {
            let value = Aligned64(b);
            let source: &(dyn Pattern + Send) = &value;
            let marker: &dyn Send = source;
            assert_eq!(size_of_val(marker), 64);
            assert_eq!(align_of_val(marker), 64);
            let result = source.read() ^ (size_of_val(marker) as u64).rotate_left(9);
            drop(value);
            assert_eq!(DROPS.load(Ordering::Relaxed), 1);
            result
        }
        12 => {
            let value = [core::marker::PhantomPinned; 2];
            let marker: &dyn Send = &value;
            assert_eq!(size_of_val(marker), 0);
            assert_eq!(align_of_val(marker), 1);
            let marker: &dyn Sync = &value;
            assert_eq!(size_of_val(marker), 0);
            assert_eq!(align_of_val(marker), 1);
            0
        }
        _ => unreachable!(),
    };
    result ^ DROPS.load(Ordering::Relaxed).rotate_left(31)
}
