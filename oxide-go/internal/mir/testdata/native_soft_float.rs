#![feature(f16, f128, core_intrinsics, float_minimum_maximum)]
#[path = "soft_float.rs"]
mod fixture;
fn main() {
    let mut seeds: Vec<u64> = (0..32).collect();
    seeds.extend([
        u64::MAX,
        1 << 63,
        (1 << 63) - 1,
        65520,
        65519,
        65521,
        0x3ff0020000000001,
        0x3ff001ffffffffff,
    ]);
    let mut state = 0x6a09e667f3bcc909u64;
    for _ in 0..48 {
        state ^= state << 13;
        state ^= state >> 7;
        state ^= state << 17;
        seeds.push(state);
    }
    if std::env::args().any(|s| s == "--probe") {
        println!(
            "fma16={:04x} signed-cast-high={:016x}",
            fixture::conformance(170, 0),
            fixture::conformance(49, 8)
        );
        return;
    }
    assert_eq!(fixture::conformance(170, 0), 0x3001, "LLVM #98389");
    assert_eq!(
        fixture::conformance(49, 8),
        u64::MAX,
        "f16 -> i128 must sign-extend"
    );
    for case in 0..172 {
        for &seed in &seeds {
            // Non-arithmetic copysign must preserve the NaN payload and
            // signaling bit. LLVM's software f16 lowering can quiet sNaNs,
            // even with SelectionDAG, so derive this reference from raw bits.
            let result = if case == 36 {
                (fixture::conformance(0, seed) & 0x7fff)
                    | (fixture::conformance(0, seed.wrapping_add(1)) & 0x8000)
            } else {
                fixture::conformance(case, seed)
            };
            println!("{case} {seed} {result}");
        }
    }
}
