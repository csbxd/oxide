#[path = "dst.rs"]
mod fixture;

fn main() {
    for case in 0..13 {
        for seed in [0, 1, 255, 65535, u32::MAX as u64, u64::MAX] {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
