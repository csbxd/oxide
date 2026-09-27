#![allow(dead_code, internal_features)]
#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(oxide_export, feature(core_intrinsics, repr_simd))]

use core::intrinsics::simd::simd_insert;

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

#[repr(simd)]
#[derive(Clone, Copy)]
struct U4([u64; 4]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct U3([u64; 3]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct F4([f32; 4]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct F3([f32; 3]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct D2([f64; 2]);
#[repr(simd)]
#[derive(Clone, Copy)]
struct D3([f64; 3]);

// Pinned rustc forbids direct SIMD field projections (MCP#838). The sole
// repr(simd) array field starts at offset zero. Read exactly its initialized
// lanes through an array pointer; a three-lane vector has extra tail padding.
macro_rules! lanes {
    ($vector:ty, $element:ty, $count:expr) => {
        impl $vector {
            fn lanes(self) -> [$element; $count] {
                unsafe { *(&raw const self).cast::<[$element; $count]>() }
            }
        }
    };
}
lanes!(U4, u64, 4);
lanes!(U3, u64, 3);
lanes!(F4, f32, 4);
lanes!(F3, f32, 3);
lanes!(D2, f64, 2);
lanes!(D3, f64, 3);

const F32: [u32; 8] = [
    0,
    0x8000_0000,
    0x7fc1_2345,
    0xffc5_4321,
    0x7fa0_0001,
    1,
    0x7f80_0000,
    0x3d_cc_cc_cd,
];
const F64: [u64; 8] = [
    0,
    0x8000_0000_0000_0000,
    0x7ff8_1234_5678_9abc,
    0xfff8_cafe_9876_5432,
    0x7ff0_0000_0000_0001,
    1,
    0x7ff0_0000_0000_0000,
    0x3fb9_9999_9999_999a,
];

fn u4_rows(before: U4, after: U4) -> [u64; 8] {
    // Read through the repr(simd) field array, never through padding bytes.
    let a = before.lanes();
    let b = after.lanes();
    [a[0], a[1], a[2], a[3], b[0], b[1], b[2], b[3]]
}

fn f4_rows(before: F4, after: F4) -> [u64; 8] {
    let a = before.lanes();
    let b = after.lanes();
    [
        a[0].to_bits() as u64,
        a[1].to_bits() as u64,
        a[2].to_bits() as u64,
        a[3].to_bits() as u64,
        b[0].to_bits() as u64,
        b[1].to_bits() as u64,
        b[2].to_bits() as u64,
        b[3].to_bits() as u64,
    ]
}

fn rows(case: u64, seed: u64) -> [u64; 8] {
    let n = (seed & 7) as usize;
    match case {
        0..=3 => {
            let original = U4([seed, !seed, seed.rotate_left(19), 0x8000_0000_0000_0001]);
            let mut value = original;
            value = unsafe {
                match case {
                    0 => simd_insert(value, 0, seed ^ 0xffff_ffff_0000_0000),
                    1 => simd_insert(value, 3, seed ^ 0x8000_0000_0000_0000),
                    2 => simd_insert(value, 0, value.lanes()[3]),
                    _ => simd_insert(value, 3, value.lanes()[0]),
                }
            };
            u4_rows(original, value)
        }
        4 | 5 => {
            let original = F4([
                f32::from_bits(F32[(n + 1) & 7]),
                f32::from_bits(F32[(n + 2) & 7]),
                f32::from_bits(F32[(n + 3) & 7]),
                f32::from_bits(F32[(n + 4) & 7]),
            ]);
            let inserted = f32::from_bits(F32[n]);
            let value = unsafe {
                if case == 4 {
                    simd_insert(original, 0, inserted)
                } else {
                    simd_insert(original, 3, inserted)
                }
            };
            f4_rows(original, value)
        }
        6 | 7 => {
            let original = D2([
                f64::from_bits(F64[(n + 1) & 7]),
                f64::from_bits(F64[(n + 2) & 7]),
            ]);
            let inserted = f64::from_bits(F64[n]);
            let value = unsafe {
                if case == 6 {
                    simd_insert(original, 0, inserted)
                } else {
                    simd_insert(original, 1, inserted)
                }
            };
            let a = original.lanes();
            let b = value.lanes();
            [
                a[0].to_bits(),
                a[1].to_bits(),
                2,
                16,
                b[0].to_bits(),
                b[1].to_bits(),
                2,
                16,
            ]
        }
        8 => {
            let original = U3([seed, !seed, 0x8000_0000_0000_0001]);
            let value = unsafe { simd_insert(original, 2, seed.rotate_right(7)) };
            let a = original.lanes();
            let b = value.lanes();
            [
                a[0],
                a[1],
                a[2],
                core::mem::size_of::<U3>() as u64,
                b[0],
                b[1],
                b[2],
                core::mem::align_of::<U3>() as u64,
            ]
        }
        9 | 11 => {
            let original = F3([
                f32::from_bits(F32[(n + 1) & 7]),
                f32::from_bits(F32[(n + 2) & 7]),
                f32::from_bits(F32[(n + 3) & 7]),
            ]);
            let mut value = original;
            value = unsafe {
                if case == 9 {
                    simd_insert(value, 1, f32::from_bits(F32[n]))
                } else {
                    simd_insert(value, 2, value.lanes()[0])
                }
            };
            let a = original.lanes();
            let b = value.lanes();
            [
                a[0].to_bits() as u64,
                a[1].to_bits() as u64,
                a[2].to_bits() as u64,
                core::mem::size_of::<F3>() as u64,
                b[0].to_bits() as u64,
                b[1].to_bits() as u64,
                b[2].to_bits() as u64,
                core::mem::align_of::<F3>() as u64,
            ]
        }
        10 => {
            let original = D3([
                f64::from_bits(F64[(n + 1) & 7]),
                f64::from_bits(F64[(n + 2) & 7]),
                f64::from_bits(F64[(n + 3) & 7]),
            ]);
            let value = unsafe { simd_insert(original, 0, f64::from_bits(F64[n])) };
            let a = original.lanes();
            let b = value.lanes();
            [
                a[0].to_bits(),
                a[1].to_bits(),
                a[2].to_bits(),
                core::mem::size_of::<D3>() as u64,
                b[0].to_bits(),
                b[1].to_bits(),
                b[2].to_bits(),
                core::mem::align_of::<D3>() as u64,
            ]
        }
        _ => unreachable!(),
    }
}

// One scalar result per observed lane: no hash can mask a changed payload.
pub fn conformance(case: u64, seed: u64) -> u64 {
    rows(case / 8, seed)[(case % 8) as usize]
}
