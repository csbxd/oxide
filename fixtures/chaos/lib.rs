//! Reproducible ownership churn. All allocation and unwinding runs as Rust.
use std::panic::{catch_unwind, panic_any, set_hook};
use std::sync::Arc;
use std::sync::atomic::{AtomicIsize, Ordering::Relaxed};

trait Read {
    fn check(&self) -> u64;
}

#[repr(align(64))]
struct Blob {
    bytes: Vec<u8>,
    tag: u8,
    live: Arc<AtomicIsize>,
}

impl Blob {
    fn new(tag: u8, len: usize, live: &Arc<AtomicIsize>) -> Self {
        let mut bytes = Vec::with_capacity(len);
        for i in 0..len {
            bytes.push(tag.wrapping_add(i as u8));
        }
        live.fetch_add(1, Relaxed);
        Self {
            bytes,
            tag,
            live: live.clone(),
        }
    }
}

impl Read for Blob {
    fn check(&self) -> u64 {
        for i in [0, self.bytes.len() / 2, self.bytes.len() - 1] {
            assert_eq!(self.bytes[i], self.tag.wrapping_add(i as u8));
        }
        self.bytes.len() as u64 ^ ((self.tag as u64) << 32)
    }
}

impl Drop for Blob {
    fn drop(&mut self) {
        self.check();
        assert!(self.live.fetch_sub(1, Relaxed) > 0);
    }
}

enum Item {
    Empty,
    Owned(Box<Blob>),
    Shared(Arc<dyn Read>),
    Text(String),
    Bytes(Vec<u8>),
}

fn next(state: &mut u64) -> u64 {
    *state ^= *state << 13;
    *state ^= *state >> 7;
    *state ^= *state << 17;
    *state
}

pub fn cycle(seed: u64, steps: usize) -> u64 {
    set_hook(Box::new(|_| {}));
    let live = Arc::new(AtomicIsize::new(0));
    let mut slots = Vec::new();
    for _ in 0..32 {
        slots.push(Item::Empty);
    }
    let mut state = seed | 1;
    let mut digest = 0u64;
    let mut operations = 0u64;
    for _ in 0..steps {
        let r = next(&mut state);
        let index = (r as usize >> 8) % slots.len();
        let other = (r as usize >> 16) % slots.len();
        let len = (1usize << ((r >> 24) % 16)) + ((r as usize >> 40) & 31);
        let tag = r as u8;
        let operation = r % 10;
        operations |= 1 << operation;
        match operation {
            0 => slots[index] = Item::Owned(Box::new(Blob::new(tag, len, &live))),
            1 => slots[index] = Item::Shared(Arc::new(Blob::new(tag, len, &live))),
            2 => {
                if let Item::Shared(value) = &slots[index] {
                    let copy = value.clone();
                    slots[other] = Item::Shared(copy);
                }
            }
            3 => {
                let mut text = String::new();
                for i in 0..(len / 8 + 1) {
                    text.push(char::from(b'a' + (i % 26) as u8));
                }
                slots[index] = Item::Text(text);
            }
            4 => {
                if let Item::Bytes(bytes) = &mut slots[index] {
                    bytes.resize(len, tag);
                    bytes.fill(tag);
                } else {
                    slots[index] = Item::Bytes(vec![tag; len]);
                }
            }
            5 => slots.swap(index, other),
            6 => slots[index] = Item::Empty,
            7 => {
                let caught = catch_unwind(|| {
                    let _first = Box::new(Blob::new(tag, len, &live));
                    let _second: Arc<dyn Read> = Arc::new(Blob::new(tag, len + 1, &live));
                    panic_any(Box::new(Blob::new(tag, len + 2, &live)));
                });
                let payload = caught.expect_err("chaos panic must be caught");
                let payload = payload.downcast::<Box<Blob>>().expect("owned Blob payload");
                digest = digest.wrapping_add(payload.check());
                drop(payload);
            }
            8 => {
                if let Item::Bytes(bytes) = &mut slots[index] {
                    bytes.truncate(len / 2);
                    bytes.shrink_to_fit();
                }
            }
            9 => {
                let old = std::mem::replace(&mut slots[index], Item::Empty);
                slots[other] = old;
            }
            _ => unreachable!(),
        }
        let value = match &slots[index] {
            Item::Empty => 0,
            Item::Owned(value) => value.check(),
            Item::Shared(value) => value.check(),
            Item::Text(value) => value.len() as u64,
            Item::Bytes(value) => value.len() as u64 ^ value.first().copied().unwrap_or(0) as u64,
        };
        digest = digest.rotate_left(3) ^ value;
    }
    drop(slots);
    assert_eq!(live.load(Relaxed), 0, "all owning paths must run Drop");
    (digest & ((1 << 54) - 1)) | (operations << 54)
}

// Negative control: the Go observer must detect this intentionally outstanding
// Rust allocation; reclaim is called immediately after that assertion.
pub fn retain(seed: u64) -> usize {
    Box::into_raw(Box::new(seed)) as usize
}

/// # Safety
/// pointer must be the still-owned result of retain, consumed exactly once.
pub unsafe fn reclaim(pointer: usize) {
    unsafe { drop(Box::from_raw(pointer as *mut u64)) };
}
