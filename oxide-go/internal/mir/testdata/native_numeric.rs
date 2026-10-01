#![feature(
    signed_bigint_helpers,
    disjoint_bitor,
    core_intrinsics,
    funnel_shifts,
    uint_carryless_mul,
    float_minimum_maximum,
    portable_simd
)]

#[path = "numeric.rs"]
mod fixture;

fn main() {
    let mut seeds: Vec<u64> = (0..32).collect();
    seeds.extend([
        127,
        128,
        255,
        256,
        65_535,
        65_536,
        1 << 31,
        (1 << 32) - 1,
        1 << 32,
        (1 << 63) - 1,
        1 << 63,
        u64::MAX - 1,
        u64::MAX,
    ]);
    let mut state = 0x6a09_e667_f3bc_c909u64;
    for _ in 0..64 {
        state ^= state << 13;
        state ^= state >> 7;
        state ^= state << 17;
        seeds.push(state);
    }
    for case in 0..116 {
        for &seed in &seeds {
            let result = fixture::conformance(case, seed);
            if case == 31 || case == 32 {
                let negative_zero = if case == 31 { 1_u64 << 31 } else { 1_u64 << 63 };
                let expected = if seed & 3 == 2 { negative_zero } else { 0 };
                assert_eq!(
                    result, expected,
                    "ordinary float operations fused or changed zero sign"
                );
            }
            println!("{case} {seed} {result}");
        }
    }
}
