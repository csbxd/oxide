#![allow(dead_code, internal_features)]
#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(oxide_export, feature(f128, core_intrinsics, repr_simd))]

use core::intrinsics::simd::*;

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

#[repr(simd)]
#[derive(Clone, Copy)]
struct Q([f128; 2]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct I([i128; 2]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct U([u128; 2]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct N([i32; 2]);

fn rows(seed: u64) -> [u64; 32] {
    let nan = f128::from_bits(0x7fff0000000000000000000000001234);
    let a = Q([
        if seed & 16 != 0 {
            nan
        } else if seed & 1 != 0 {
            -1.0
        } else {
            1.0
        },
        if seed & 2 != 0 { -1.0 } else { 0.0 },
    ]);
    let b = Q([0.0, -0.0]);
    unsafe {
        let mask: I = simd_lt(a, b);
        let unsigned: U = simd_cast(mask);
        let other = I([
            if seed & 4 != 0 { -1 } else { 0 },
            if seed & 8 != 0 { -1 } else { 0 },
        ]);
        let or: I = simd_or(mask, other);
        let and: I = simd_and(mask, other);
        let xor: I = simd_xor(or, and);
        let mut left = Q([nan, -0.0]);
        let right = Q([f128::from_bits(0xffff8000000000000000000000009876), 0.0]);
        let selected_unsigned: Q = simd_select(unsigned, right, left);
        left = simd_select(mask, left, right);
        let narrow: N = simd_select(mask, N([11, 22]), N([33, 44]));
        // Read initialized lanes, without direct SIMD field projections.
        let selected = *(&raw const left).cast::<[f128; 2]>();
        let selected_unsigned = *(&raw const selected_unsigned).cast::<[f128; 2]>();
        let narrow = *(&raw const narrow).cast::<[i32; 2]>();
        let reduced: i128 = simd_reduce_or(mask);
        let reduced_unsigned: u128 = simd_reduce_or(unsigned);
        let min: i128 = simd_reduce_min(mask);
        let max: i128 = simd_reduce_max(mask);
        let min_unsigned: u128 = simd_reduce_min(unsigned);
        let max_unsigned: u128 = simd_reduce_max(unsigned);
        let raw: i128 = simd_reduce_or(I([1, 1 << 100]));
        [
            selected[0].to_bits() as u64,
            (selected[0].to_bits() >> 64) as u64,
            selected[1].to_bits() as u64,
            (selected[1].to_bits() >> 64) as u64,
            selected_unsigned[0].to_bits() as u64,
            (selected_unsigned[0].to_bits() >> 64) as u64,
            selected_unsigned[1].to_bits() as u64,
            (selected_unsigned[1].to_bits() >> 64) as u64,
            narrow[0] as u64,
            narrow[1] as u64,
            simd_reduce_all(mask) as u64,
            simd_reduce_any(mask) as u64,
            simd_reduce_all(unsigned) as u64,
            simd_reduce_any(unsigned) as u64,
            simd_bitmask::<_, u8>(mask) as u64,
            simd_bitmask::<_, u8>(unsigned) as u64,
            simd_bitmask::<_, u8>(or) as u64,
            simd_bitmask::<_, u8>(and) as u64,
            simd_bitmask::<_, u8>(xor) as u64,
            reduced as u64,
            ((reduced as u128) >> 64) as u64,
            reduced_unsigned as u64,
            (reduced_unsigned >> 64) as u64,
            min as u64,
            ((min as u128) >> 64) as u64,
            max as u64,
            ((max as u128) >> 64) as u64,
            min_unsigned as u64,
            max_unsigned as u64,
            raw as u64,
            ((raw as u128) >> 64) as u64,
            simd_bitmask::<_, u8>(simd_and(unsigned, simd_or(unsigned, U([u128::MAX; 2])))) as u64,
        ]
    }
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    rows(seed)[case as usize]
}
