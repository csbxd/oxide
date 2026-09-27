mod public_ownership;
use public_ownership::*;
use std::sync::atomic::{AtomicU64, Ordering::Relaxed};

static SEED: AtomicU64 = AtomicU64::new(0);
fn panic_callback() { drop(make_panic(SEED.load(Relaxed))); }

fn run(case: u64, seed: u64) -> u64 {
    reset();
    let checksum = match case {
        0 => { let v = make_string(seed); let n = inspect_string(&v); drop(v); n },
        1 => { let v = make_vector(seed); let n = inspect_vector(&v); drop(v); n },
        2 => { let v = make_box(seed); let n = inspect_box(&v); drop(v); n },
        3 => { let v = make_dynamic(seed); let n = inspect_dynamic(&v); drop(v); n },
        4 => { let v = make_aligned(seed); let n = inspect_aligned(&v); drop(v); n },
        5 => { let v = make_large(seed); let n = inspect_large(&v); drop(v); n },
        6 => { let v = make_result(seed); let n = inspect_result(&v); drop(v); n },
        7 => { let v = make_zero(seed); let n = inspect_zero(&v); drop(v); n },
        8 => { let v = make_boxed_string(seed); let n = inspect_boxed_string(&v); drop(v); n },
        9 => { let v = make_borrowed_vector(seed); let n = inspect_borrowed_vector(&v); drop(v); n },
        10 => { SEED.store(seed, Relaxed); u64::from(catch_callback(panic_callback, seed)) },
        _ => unreachable!(),
    };
    checksum.wrapping_add(drop_state())
}

fn main() {
    init();
    for case in 0..11 {
        for seed in [0, 1, 7, 255] { println!("{case} {seed} {}", run(case, seed)); }
    }
}
