#![allow(dead_code)]
#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(
    oxide_export,
    feature(f16, f128, core_intrinsics, float_minimum_maximum)
)]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

fn bits(seed: u64) -> u128 {
    const BOUNDS: [u128; 18] = [
        0,
        1 << 127,
        1,
        (1 << 127) | 1,
        (1 << 112) - 1,
        1 << 112,
        0x3fff_u128 << 112,
        (0x3fff_u128 << 112) | 1,
        0xbfff_u128 << 112,
        (0x7fff_u128 << 112) - 1,
        (0xffff_u128 << 112) - 1,
        0x7fff_u128 << 112,
        0xffff_u128 << 112,
        (0x7fff_u128 << 112) | 1,
        (0x7fff_u128 << 112) | (1 << 111),
        0x3ffe_u128 << 112,
        0x407e_u128 << 112,
        0x407f_u128 << 112,
    ];
    if seed < BOUNDS.len() as u64 {
        return BOUNDS[seed as usize];
    }
    ((seed as u128) << 64) | seed.rotate_left(29) as u128
}
fn half_bits(seed: u64) -> u16 {
    const BOUNDS: [u16; 18] = [
        0, 0x8000, 1, 0x8001, 0x3ff, 0x400, 0x3c00, 0x3c01, 0xbc00, 0x7bff, 0xfbff, 0x7c00, 0xfc00,
        0x7c01, 0x7e00, 0x3800, 0x7800, 0x7bfe,
    ];
    if seed < 18 {
        BOUNDS[seed as usize]
    } else {
        seed as u16
    }
}
fn canonical16(x: f16) -> u128 {
    if x.is_nan() {
        0x7e00
    } else {
        x.to_bits() as u128
    }
}
fn canonical128(x: f128) -> u128 {
    if x.is_nan() {
        0x7fff8000000000000000000000000000
    } else {
        x.to_bits()
    }
}
fn canonical32(x: f32) -> u128 {
    if x.is_nan() {
        0x7fc00000
    } else {
        x.to_bits() as u128
    }
}
fn canonical64(x: f64) -> u128 {
    if x.is_nan() {
        0x7ff8000000000000
    } else {
        x.to_bits() as u128
    }
}

macro_rules! float_cases {
    ($name:ident, $ty:ty, $bits:ident, $canonical:ident, $other:ty) => {
        fn $name(op: u64, seed: u64) -> u128 {
            let a = <$ty>::from_bits($bits(seed));
            let b = <$ty>::from_bits($bits(seed.wrapping_add(1)));
            let c = <$ty>::from_bits($bits(seed.wrapping_add(2)));
            match op {
                0 => a.to_bits() as u128,
                1 => (-a).to_bits() as u128,
                2 => $canonical(a + b),
                3 => $canonical(a - b),
                4 => $canonical(a * b),
                5 => $canonical(a / b),
                6 => $canonical(a % b),
                7 => $canonical(a.mul_add(b, c)),
                8 => $canonical(a.sqrt()),
                9 => $canonical(a.floor()),
                10 => $canonical(a.ceil()),
                11 => $canonical(a.trunc()),
                12 => $canonical(a.round()),
                13 => $canonical(a.round_ties_even()),
                // The nsz min/max contract permits either sign of zero.
                14 => {
                    let r = a.min(b);
                    if r == 0.0 {
                        0
                    } else {
                        $canonical(r)
                    }
                }
                15 => {
                    let r = a.max(b);
                    if r == 0.0 {
                        0
                    } else {
                        $canonical(r)
                    }
                }
                16 => $canonical(a.minimum(b)),
                17 => $canonical(a.maximum(b)),
                18 => a.copysign(b).to_bits() as u128,
                19 => a.abs().to_bits() as u128,
                20 => a.next_up().to_bits() as u128,
                21 => a.next_down().to_bits() as u128,
                22 => {
                    (a == b) as u128
                        | ((a != b) as u128) << 1
                        | ((a < b) as u128) << 2
                        | ((a <= b) as u128) << 3
                        | ((a > b) as u128) << 4
                        | ((a >= b) as u128) << 5
                }
                23 => (a.total_cmp(&b) as i8) as u128,
                24 => (a as i128) as u128,
                25 => a as u128,
                26 => (a as i8) as u128,
                27 => (a as u8) as u128,
                28 => (a as i64) as u128,
                29 => (a as u64) as u128,
                30 => canonical32(a as f32),
                31 => canonical64(a as f64),
                32 => $canonical(bits(seed) as i128 as $ty),
                33 => $canonical(bits(seed) as $ty),
                34 => $canonical(f32::from_bits(seed as u32) as $ty),
                35 => $canonical(f64::from_bits(seed) as $ty),
                36 => $canonical((a as $other) as $ty),
                37 => unsafe { ((seed as i8 as $ty).to_int_unchecked::<i32>() as i128) as u128 },
                38 => $canonical(a.midpoint(b)),
                39 => $canonical(a.algebraic_add(b)),
                40 => $canonical((seed as i8 as $ty).powi((seed % 7) as i32 - 3)),
                _ => unreachable!(),
            }
        }
    };
}
float_cases!(half, f16, half_bits, canonical16, f128);
float_cases!(quad, f128, bits, canonical128, f16);

#[repr(C)]
struct Layout {
    tag: u8,
    half: f16,
    quad: f128,
    tail: u8,
}
#[repr(C, packed)]
struct Packed {
    tag: u8,
    value: f128,
}
#[repr(transparent)]
struct Wrapped(f128);
fn wrapper(x: Wrapped) -> Wrapped {
    Wrapped(-x.0)
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    let op = case / 2;
    let result = if op < 41 {
        half(op, seed)
    } else if op < 82 {
        quad(op - 41, seed)
    } else {
        match op {
            82 => {
                let mut x = Layout {
                    tag: 1,
                    half: f16::from_bits(half_bits(seed)),
                    quad: f128::from_bits(bits(seed)),
                    tail: 2,
                };
                let p = &mut x.quad as *mut f128;
                assert_eq!((p as usize) % core::mem::align_of::<f128>(), 0);
                unsafe {
                    *p = -*p;
                }
                (core::mem::size_of::<Layout>() as u128) << 64
                    | ((core::mem::offset_of!(Layout, quad) as u128) << 32)
                    | core::mem::align_of::<Layout>() as u128
            }
            83 => {
                let p = Packed {
                    tag: 1,
                    value: f128::from_bits(bits(seed)),
                };
                unsafe { core::ptr::addr_of!(p.value).read_unaligned().to_bits() }
            }
            84 => {
                // Rust permits an ABI-compatible transparent function cast.
                let f: fn(f128) -> f128 =
                    unsafe { core::mem::transmute(wrapper as fn(Wrapped) -> Wrapped) };
                f(f128::from_bits(bits(seed))).to_bits()
            }
            85 => {
                let a = core::hint::black_box(f16::from_bits(0x520b));
                canonical16(a.mul_add(f16::from_bits(0x00e9), f16::from_bits(0x2ff6)))
            }
            _ => unreachable!(),
        }
    };
    (result >> ((case % 2) * 64)) as u64
}
