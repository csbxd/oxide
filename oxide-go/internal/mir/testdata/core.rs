#![allow(dead_code)]
#![cfg_attr(oxide_export, no_std)]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

use core::mem::{align_of, offset_of, size_of};
use core::num::{NonZeroU128, NonZeroU64};
use core::ptr::{addr_of, addr_of_mut, read_unaligned, write_unaligned};

struct RustLayout {
    a: u8,
    b: u64,
    c: u16,
}

#[repr(C)]
struct CLayout {
    a: u8,
    b: u64,
    c: u16,
}

#[repr(C, packed)]
struct Packed {
    tag: u8,
    value: u64,
}

#[repr(align(64))]
struct Aligned {
    value: u64,
}

#[repr(i8)]
enum SignedTag {
    Negative = -7,
    Zero = 0,
    Positive = 42,
}

#[repr(i128)]
enum WideTag {
    Negative = -7,
    High = (1i128 << 100) + 3,
}

#[inline(never)]
fn layouts(seed: u64) -> u64 {
    let native = RustLayout {
        a: seed as u8,
        b: seed,
        c: seed as u16,
    };
    let c = CLayout {
        a: seed as u8,
        b: seed,
        c: seed as u16,
    };
    let aligned = Aligned { value: seed };
    // Force addressable locals, including storage whose alignment exceeds Go's.
    let observed = unsafe { addr_of!(native.b).read() ^ addr_of!(c.b).read() };
    let alignment = addr_of!(aligned) as usize % align_of::<Aligned>();
    (size_of::<RustLayout>() as u64)
        | ((align_of::<RustLayout>() as u64) << 8)
        | ((offset_of!(RustLayout, a) as u64) << 16)
        | ((offset_of!(RustLayout, b) as u64) << 24)
        | ((offset_of!(RustLayout, c) as u64) << 32)
        | ((size_of::<CLayout>() as u64) << 40)
        | ((offset_of!(CLayout, b) as u64) << 48)
        | ((alignment as u64) << 56)
        | observed
}

#[inline(never)]
fn niche(seed: u64) -> u64 {
    let value = if seed & 1 == 0 {
        None
    } else {
        Some(unsafe { NonZeroU64::new_unchecked(seed) })
    };
    let bits = unsafe { core::mem::transmute::<Option<NonZeroU64>, u64>(value) };
    match value {
        None => bits,
        Some(value) => bits ^ value.get().rotate_left(13),
    }
}

#[inline(never)]
fn packed(seed: u64) -> u64 {
    let mut value = Packed {
        tag: 7,
        value: seed,
    };
    unsafe {
        let ptr = addr_of_mut!(value.value);
        write_unaligned(ptr, read_unaligned(ptr).wrapping_add(17));
        read_unaligned(addr_of!(value.value)) ^ value.tag as u64
    }
}

#[inline(never)]
unsafe fn mutate(ptr: *mut u64, seed: u64) {
    ptr.add(1).write(ptr.read().wrapping_add(seed));
}

#[inline(never)]
fn pointers(seed: u64) -> u64 {
    let mut values = [seed, 3, 5, 7];
    unsafe {
        let ptr = addr_of_mut!(values).cast::<u64>();
        mutate(ptr, 11);
        ptr.add(2).write(ptr.add(1).read().wrapping_mul(3));
        ptr.add(2).read() ^ ptr.add(3).read()
    }
}

struct RecordDrop {
    value: *mut u64,
    digit: u64,
}

impl Drop for RecordDrop {
    fn drop(&mut self) {
        unsafe { *self.value = (*self.value).wrapping_mul(10).wrapping_add(self.digit) }
    }
}

#[inline(never)]
fn drops(seed: u64) -> u64 {
    let mut value = seed;
    {
        let _first = RecordDrop {
            value: addr_of_mut!(value),
            digit: 1,
        };
        let _second = RecordDrop {
            value: addr_of_mut!(value),
            digit: 2,
        };
    }
    value
}

#[inline(never)]
fn signed_widening(seed: u64) -> u64 {
    let value = (seed as i8) as u128;
    ((value >> 64) as u64) ^ (value as u64).rotate_left(11)
}

#[inline(never)]
fn unsigned_widening(seed: u64) -> u64 {
    let value = seed as i128;
    ((value >> 64) as u64) ^ (value as u64).rotate_left(19)
}

#[inline(never)]
fn wide_casts(seed: u64) -> u64 {
    let unsigned = ((seed as u128) << 64) | ((!seed) as u128);
    let signed = unsigned as i128;
    let round_trip = signed as u128;
    ((round_trip >> 64) as u64).rotate_left(23) ^ round_trip as u64
}

#[inline(never)]
fn subslice(seed: u64) -> u64 {
    let [_, middle @ .., _] = [1, seed, 3, 4, 5, 6];
    // MIR projects a shorter array type, not the original six-element array.
    let bytes = unsafe { core::mem::transmute::<[u64; 4], [u8; 32]>(middle) };
    u64::from_le_bytes([
        bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
    ]) ^ (middle[1] + middle[2] + middle[3])
}

#[inline(never)]
fn signed_tag(seed: u64) -> u64 {
    let value = match seed % 3 {
        0 => SignedTag::Negative,
        1 => SignedTag::Zero,
        _ => SignedTag::Positive,
    };
    let encoded = unsafe { (&value as *const SignedTag).cast::<i8>().read() };
    match value {
        SignedTag::Negative => (encoded as u64).wrapping_add(seed),
        SignedTag::Zero => seed ^ 17,
        SignedTag::Positive => (encoded as u64) ^ seed,
    }
}

#[inline(never)]
fn wide_niche(seed: u64) -> u64 {
    let bits = ((seed as u128) << 64) | 1;
    let value = if seed & 1 == 0 {
        None
    } else {
        Some(unsafe { NonZeroU128::new_unchecked(bits) })
    };
    let encoded = unsafe { core::mem::transmute::<Option<NonZeroU128>, u128>(value) };
    match value {
        None => ((encoded >> 64) as u64) ^ encoded as u64,
        Some(value) => ((value.get() >> 64) as u64) ^ encoded as u64,
    }
}

#[inline(never)]
fn wide_bits(seed: u64) -> u64 {
    let value = ((seed as u128) << 64) | (seed.reverse_bits() as u128);
    let negative = -(seed as i128);
    let mask = (!value & (negative as u128)) | (value ^ (value >> 5));
    let signed = !negative ^ seed as i128;
    ((mask >> 64) as u64) ^ (mask as u64).rotate_left(7) ^ ((signed >> 64) as u64)
}

#[inline(never)]
fn wide_tag(seed: u64) -> u64 {
    let value = if seed & 1 == 0 {
        WideTag::Negative
    } else {
        WideTag::High
    };
    let selected = match &value {
        WideTag::Negative => 1,
        WideTag::High => 2,
    };
    let encoded = value as i128;
    ((encoded >> 64) as u64) ^ encoded as u64 ^ selected
}

#[inline(never)]
fn signed_switch(seed: u64) -> u64 {
    match seed as i8 {
        -7 => seed ^ 1,
        -1 => seed ^ 2,
        0 => seed ^ 3,
        _ => seed ^ 4,
    }
}

#[inline(never)]
fn wide_switch(seed: u64) -> u64 {
    const HIGH: u128 = 1 << 100;
    let value = match seed % 3 {
        0 => u128::MAX,
        1 => HIGH,
        _ => seed as u128,
    };
    match value {
        u128::MAX => seed ^ 1,
        HIGH => seed ^ 2,
        _ => seed ^ 3,
    }
}

#[track_caller]
#[inline(never)]
fn location_inner() -> u64 {
    let location = core::panic::Location::caller();
    ((location.line() as u64) << 32) | location.column() as u64
}

#[track_caller]
#[inline(never)]
fn location_wrapper() -> u64 {
    location_inner()
}

#[inline(never)]
fn caller_locations(seed: u64) -> u64 {
    // Both calls must report their own line here, through both tracked frames.
    let first = location_wrapper();
    let second = location_wrapper();
    first.rotate_left(7) ^ second ^ seed
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    match case {
        0 => layouts(seed),
        1 => niche(seed),
        2 => packed(seed),
        3 => pointers(seed),
        4 => drops(seed),
        6 => signed_widening(seed),
        7 => unsigned_widening(seed),
        8 => wide_casts(seed),
        9 => subslice(seed),
        10 => signed_tag(seed),
        11 => wide_niche(seed),
        12 => wide_bits(seed),
        13 => wide_tag(seed),
        14 => signed_switch(seed),
        15 => wide_switch(seed),
        16 => caller_locations(seed),
        _ => seed.wrapping_mul(17).rotate_right(9),
    }
}
