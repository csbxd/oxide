#![feature(f128, core_intrinsics, repr_simd)]
#![allow(internal_features)]
#[path = "simd_masks.rs"]
mod fixture;

fn main() {
    for case in 0..32 {
        for seed in 0..32 {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
