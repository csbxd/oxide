#![feature(unboxed_closures)]

#[path = "callback.rs"]
mod fixture;

fn encode(value: fixture::Args) -> u64 {
    value
        .0
        .wrapping_mul(10_000)
        .wrapping_add(value.1.wrapping_mul(100))
        .wrapping_add(value.2)
}
extern "rust-call" fn spread(value: fixture::Args) -> u64 {
    encode(value)
}
fn ordinary(value: fixture::Args) -> u64 {
    encode(value)
}

fn main() {
    for case in 0..2 {
        for seed in [0, 1, 2, 17, 1 << 31, 1 << 63, u64::MAX - 1, u64::MAX] {
            println!(
                "{case} {seed} {}",
                fixture::conformance(case, seed, spread, ordinary)
            );
        }
    }
}
