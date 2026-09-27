#[path = "large.rs"]
mod fixture;

fn main() {
    for case in 0..12 {
        for seed in [0, 1, 255, 65535, 262143, 262144, u32::MAX as u64, u64::MAX] {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
