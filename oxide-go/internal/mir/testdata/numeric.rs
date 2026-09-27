#![allow(dead_code)]
#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(oxide_export, feature(signed_bigint_helpers, disjoint_bitor))]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

// The harness enables signed_bigint_helpers and disjoint_bitor on the pinned
// compiler. Each export uses scalars while the arithmetic remains Rust code.
fn fold(value: u128) -> u64 {
    (value as u64) ^ ((value >> 64) as u64).rotate_left(29)
}

fn wide(seed: u64) -> u128 {
    match seed {
        0 => 0,
        1 => u128::MAX,
        2 => 1 << 127,
        3 => i128::MAX as u128,
        4 => 1 << 64,
        5 => u64::MAX as u128,
        _ => ((seed as u128) << 64) | ((!seed).rotate_left(17) as u128),
    }
}

macro_rules! carrying {
    ($seed:expr, $ty:ty) => {{
        let a = $seed as $ty;
        let b = $seed.rotate_left(23) as $ty;
        let c = $seed.rotate_right(5) as $ty;
        let d = $seed.rotate_right(11) as $ty;
        let (lo, hi) = a.carrying_mul_add(b, c, d);
        fold(lo as u128).rotate_left(7) ^ fold(hi as u128)
    }};
}

fn float32_bits(seed: u64) -> u32 {
    const BITS: [u32; 18] = [
        0,
        0x8000_0000,
        1,
        0x8000_0001,
        0x3f00_0000,
        0xbf00_0000,
        0x3fc0_0000,
        0xbfc0_0000,
        0x7eff_ffff,
        0x7f00_0000,
        0x7f7f_ffff,
        0xfeff_ffff,
        0xff00_0000,
        0xff7f_ffff,
        0x7f80_0000,
        0xff80_0000,
        0x7fc0_0001,
        0xffc0_0001,
    ];
    if seed < BITS.len() as u64 {
        BITS[seed as usize]
    } else {
        seed as u32
    }
}

fn float64_bits(seed: u64) -> u64 {
    const BITS: [u64; 18] = [
        0,
        0x8000_0000_0000_0000,
        1,
        0x8000_0000_0000_0001,
        0x3fe0_0000_0000_0000,
        0xbfe0_0000_0000_0000,
        0x3ff8_0000_0000_0000,
        0xbff8_0000_0000_0000,
        0x47df_ffff_ffff_ffff,
        0x47e0_0000_0000_0000,
        0x47ef_ffff_ffff_ffff,
        0xc7df_ffff_ffff_ffff,
        0xc7e0_0000_0000_0000,
        0xc7ef_ffff_ffff_ffff,
        0x7ff0_0000_0000_0000,
        0xfff0_0000_0000_0000,
        0x7ff8_0000_0000_0001,
        0xfff8_0000_0000_0001,
    ];
    if seed < BITS.len() as u64 {
        BITS[seed as usize]
    } else {
        seed
    }
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    let n = wide(seed);
    match case {
        0 => fold(n / (seed as u128 | 1)),
        1 => fold(n % (seed as u128 | 1)),
        2 => fold(n / (wide(seed.rotate_left(19)) | 1)),
        3 => fold(n % (wide(seed.rotate_left(19)) | 1)),
        4 => fold(((n as i128) / ((seed as i64 as i128) | 1)) as u128),
        5 => fold(((n as i128) % ((seed as i64 as i128) | 1)) as u128),
        6 => (n as f32).to_bits() as u64,
        7 => (n as f64).to_bits(),
        8 => ((n as i128) as f32).to_bits() as u64,
        9 => ((n as i128) as f64).to_bits(),
        // These halfway + 1 values expose incorrect u128 -> f64 -> f32
        // double rounding. Native Rust performs a direct integer -> f32 cast.
        10 => (((1u128 << 127) + (1u128 << 103) + (seed & 1) as u128) as f32).to_bits() as u64,
        11 => (((1i128 << 126) + (1i128 << 102) + (seed & 1) as i128) as f32).to_bits() as u64,
        12 => ((-((1i128 << 126) + (1i128 << 102) + (seed & 1) as i128)) as f32).to_bits() as u64,
        13 => (((1u128 << 127) + (1u128 << 74) + (seed & 1) as u128) as f64).to_bits(),
        14 => fold(f32::from_bits(float32_bits(seed)) as u128),
        15 => fold((f32::from_bits(float32_bits(seed)) as i128) as u128),
        16 => fold(f64::from_bits(float64_bits(seed)) as u128),
        17 => fold((f64::from_bits(float64_bits(seed)) as i128) as u128),
        18 => carrying!(seed, u8),
        19 => carrying!(seed, u16),
        20 => carrying!(seed, u32),
        21 => carrying!(seed, u64),
        22 => {
            let (lo, hi) =
                n.carrying_mul_add(wide(seed.rotate_left(23)), !n, wide(seed.rotate_right(11)));
            fold(lo).rotate_left(7) ^ fold(hi)
        }
        23 => carrying!(seed, i8),
        24 => carrying!(seed, i16),
        25 => carrying!(seed, i32),
        26 => carrying!(seed, i64),
        27 => {
            let (lo, hi) = (n as i128).carrying_mul_add(
                wide(seed.rotate_left(23)) as i128,
                (!n) as i128,
                wide(seed.rotate_right(11)) as i128,
            );
            fold(lo).rotate_left(7) ^ fold(hi as u128)
        }
        28 => unsafe {
            (seed & 0xffff_ffff).unchecked_disjoint_bitor(seed & 0xffff_ffff_0000_0000)
        },
        29 => fold(unsafe {
            (n & u64::MAX as u128).unchecked_disjoint_bitor(n & !(u64::MAX as u128))
        }),
        // Ordinary Rust multiplication and addition round separately. Go may
        // fuse across statements unless each MIR operation has a barrier.
        31 => {
            let (a, b, c) = match seed & 3 {
                0 => (0x3f80_0001, 0x3f7f_fffe, 0xbf80_0000),
                1 => (0x3f80_0800, 0x3f80_0800, 0xbf80_1000),
                2 => (0x8000_0000, 0x4040_0000, 0x8000_0000),
                _ => (0, 0x4040_0000, 0),
            };
            let product = f32::from_bits(a) * f32::from_bits(b);
            (product + f32::from_bits(c)).to_bits() as u64
        }
        32 => {
            let (a, b, c) = match seed & 3 {
                0 => (0x3ff0_0000_0000_0001, 0x3fef_ffff_ffff_fffe, 0xbff0_0000_0000_0000),
                1 => (0x3ff0_0000_0200_0000, 0x3ff0_0000_0200_0000, 0xbff0_0000_0400_0000),
                2 => (0x8000_0000_0000_0000, 0x4008_0000_0000_0000, 0x8000_0000_0000_0000),
                _ => (0, 0x4008_0000_0000_0000, 0),
            };
            let product = f64::from_bits(a) * f64::from_bits(b);
            (product + f64::from_bits(c)).to_bits()
        }
        _ => {
            let selected = if seed & 1 == 0 {
                core::any::TypeId::of::<u64>()
            } else {
                core::any::TypeId::of::<i64>()
            };
            u64::from(selected == core::any::TypeId::of::<u64>())
                | (u64::from(selected == core::any::TypeId::of::<i64>()) << 1)
                | (u64::from(selected == core::any::TypeId::of::<[u64; 2]>()) << 2)
        }
    }
}
