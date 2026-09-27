#![feature(core_intrinsics, repr_simd)]
#![allow(internal_features)]

#[path = "simd_insert.rs"]
mod fixture;

fn main() {
    let seeds = [
        0,
        1,
        2,
        3,
        0x8000_0000_0000_0004,
        0xffff_ffff_0000_0005,
        u64::MAX - 1,
        u64::MAX,
    ];
    for case in 0..96 {
        for seed in seeds {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
