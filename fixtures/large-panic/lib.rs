use std::panic::{AssertUnwindSafe, catch_unwind, panic_any, set_hook};
use std::sync::atomic::{AtomicU64, Ordering::Relaxed};

const SIZE: usize = 256 * 1024;
static DROPS: AtomicU64 = AtomicU64::new(0);
static DROP_VALUE: AtomicU64 = AtomicU64::new(0);
static HOOKS: AtomicU64 = AtomicU64::new(0);

struct Large {
    marker: u64,
    bytes: [u8; SIZE],
}

impl Drop for Large {
    fn drop(&mut self) {
        DROPS.fetch_add(1, Relaxed);
        DROP_VALUE.store(
            self.marker ^ ((self.bytes[0] as u64) << 8) ^ ((self.bytes[SIZE - 1] as u64) << 16),
            Relaxed,
        );
    }
}

#[inline(never)]
fn interrupted_return(seed: u64, fail: bool) -> [u8; SIZE] {
    let result = [seed as u8; SIZE];
    if fail {
        panic_any(seed);
    }
    result
}

#[inline(never)]
fn interrupted_argument(value: Large, seed: u64, fail: bool) -> [u8; SIZE] {
    let result = value.bytes;
    if fail {
        panic_any(seed);
    }
    result
}

fn payload_matches<T>(caught: std::thread::Result<T>, seed: u64) -> bool {
    match caught {
        Err(payload) => payload.downcast::<u64>().is_ok_and(|value| *value == seed),
        Ok(_) => false,
    }
}

#[inline(never)]
fn return_case(seed: u64) -> u64 {
    let mut output = [0xd3; SIZE];
    let caught = catch_unwind(AssertUnwindSafe(|| {
        output = interrupted_return(seed, true);
    }));
    u64::from(payload_matches(caught, seed))
        | (u64::from(output.iter().all(|byte| *byte == 0xd3)) << 1)
        | (u64::from(DROPS.load(Relaxed) == 0) << 2)
}

#[inline(never)]
fn argument_case(seed: u64) -> u64 {
    let value = Large {
        marker: seed,
        bytes: [seed as u8; SIZE],
    };
    // The closure environment, parameter and Result return all contain a large
    // value. Cleanup must drop the consumed argument exactly once on unwind.
    let caught = catch_unwind(move || interrupted_argument(value, seed, true));
    let expected = seed ^ (((seed as u8) as u64) << 8) ^ (((seed as u8) as u64) << 16);
    u64::from(payload_matches(caught, seed))
        | (u64::from(DROP_VALUE.load(Relaxed) == expected) << 1)
        | (u64::from(DROPS.load(Relaxed) == 1) << 2)
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    DROPS.store(0, Relaxed);
    DROP_VALUE.store(0, Relaxed);
    HOOKS.store(0, Relaxed);
    set_hook(Box::new(|_| {
        HOOKS.fetch_add(1, Relaxed);
    }));
    let result = match case {
        0 => return_case(seed),
        1 => argument_case(seed),
        _ => unreachable!(),
    };
    result | (u64::from(HOOKS.load(Relaxed) == 1) << 3)
}
