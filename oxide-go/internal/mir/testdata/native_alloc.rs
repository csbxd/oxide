#[path = "alloc.rs"]
mod fixture;

fn main() {
    for case in 0..3 {
        for seed in [0, 1, 255, 65_537, u64::MAX - 1, u64::MAX] {
            println!("{case} {seed} {}", fixture::conformance(case, seed));
        }
    }
}
