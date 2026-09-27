#[path = "core.rs"]
mod fixture;

fn main() {
    for case in 0..17 {
        for seed in [
            0,
            1,
            2,
            17,
            127,
            128,
            249,
            255,
            65_537,
            1 << 63,
            u64::MAX - 1,
            u64::MAX,
        ] {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
