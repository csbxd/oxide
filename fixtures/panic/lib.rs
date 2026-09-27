use std::panic::{catch_unwind, panic_any, set_hook};
use std::sync::atomic::{AtomicUsize, Ordering::Relaxed};

static HOOKS: AtomicUsize = AtomicUsize::new(0);
static PAYLOADS: AtomicUsize = AtomicUsize::new(0);
static LOCATIONS: AtomicUsize = AtomicUsize::new(0);
static DROPS: AtomicUsize = AtomicUsize::new(0);
static ORDER: AtomicUsize = AtomicUsize::new(0);

struct Probe {
    id: usize,
    value: u64,
}

impl Drop for Probe {
    fn drop(&mut self) {
        DROPS.fetch_add(1, Relaxed);
        ORDER.store(ORDER.load(Relaxed).wrapping_mul(17).wrapping_add(self.id), Relaxed);
    }
}

trait Value {
    fn value(&self) -> u64;
}

impl Value for Probe {
    fn value(&self) -> u64 {
        self.value
    }
}

impl Value for u64 {
    fn value(&self) -> u64 {
        *self
    }
}

// All panic payloads, hooks, downcasts and vtable drops run in Rust. The Go
// test ABI only receives this scalar summary and compares it with native Rust.
pub fn conformance(case: u64, seed: u64) -> u64 {
    for counter in [&HOOKS, &PAYLOADS, &LOCATIONS, &DROPS, &ORDER] {
        counter.store(0, Relaxed);
    }
    set_hook(Box::new(|info| {
        HOOKS.fetch_add(1, Relaxed);
        let payload = info.payload();
        let kind = if payload.is::<u64>() {
            1
        } else if payload.is::<&str>() {
            2
        } else if payload.is::<Probe>() {
            4
        } else {
            8
        };
        PAYLOADS.fetch_or(kind, Relaxed);
        if info.location().is_some_and(|location| location.line() != 0) {
            LOCATIONS.fetch_add(1, Relaxed);
        }
    }));
    let result = match case {
        0 => match catch_unwind(|| panic_any(42_u64)) {
            Err(payload) => match payload.downcast::<u64>() {
                Ok(value) => u64::from(*value == 42),
                Err(_) => 0,
            },
            Ok(()) => 0,
        },
        1 => {
            let value = if seed & 1 != 0 { u64::MAX } else { 41 };
            match catch_unwind(|| value + 1) {
                Ok(value) => u64::from(value == 42),
                Err(payload) => {
                    drop(payload);
                    2
                }
            }
        }
        2 => {
            let caught = catch_unwind(|| panic_any(Probe { id: 7, value: seed }));
            let result = u64::from(caught.is_err());
            drop(caught);
            result
        }
        3 => {
            let value: Box<dyn Value> = Box::new(Probe { id: 5, value: seed });
            let result = u64::from(value.value() == seed);
            drop(value);
            result
        }
        4 => {
            let values: Vec<Box<dyn Value>> = vec![
                Box::new(Probe { id: 1, value: 1 }),
                Box::new(8_u64),
                Box::new(Probe { id: 2, value: 2 }),
                Box::new(Probe { id: 3, value: 3 }),
            ];
            let sum: u64 = values.iter().map(|value| value.value()).sum();
            drop(values);
            u64::from(sum == 14)
        }
        5 => {
            // Both payload types have empty drop glue (a null vtable slot).
            let number = catch_unwind(|| panic_any(13_u64));
            let text = catch_unwind(|| panic_any("panic payload"));
            let result = u64::from(number.is_err()) + u64::from(text.is_err());
            drop(number);
            drop(text);
            result
        }
        6 => format!("tag {} {}", "name", seed)
            .bytes()
            .fold(0xcbf29ce484222325_u64, |hash, byte| {
                hash.wrapping_mul(0x100000001b3) ^ u64::from(byte)
            }),
        _ => 0,
    };
    result
        | (HOOKS.load(Relaxed) as u64) << 8
        | (PAYLOADS.load(Relaxed) as u64) << 16
        | (LOCATIONS.load(Relaxed) as u64) << 24
        | (DROPS.load(Relaxed) as u64) << 32
        | (ORDER.load(Relaxed) as u64) << 40
}
