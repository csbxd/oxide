#![allow(dead_code)]

use std::panic::{catch_unwind, panic_any, set_hook};
use std::sync::atomic::{AtomicU64, Ordering::Relaxed};

static DROPS: AtomicU64 = AtomicU64::new(0);
static DROP_VALUE: AtomicU64 = AtomicU64::new(0);
static HOOKS: AtomicU64 = AtomicU64::new(0);
const LARGE_SIZE: usize = 256 * 1024;

pub fn reset() {
    DROPS.store(0, Relaxed);
    DROP_VALUE.store(0, Relaxed);
    HOOKS.store(0, Relaxed);
}

pub fn init() {
    set_hook(Box::new(|_| { HOOKS.fetch_add(1, Relaxed); }));
}

pub fn drop_state() -> u64 {
    DROPS.load(Relaxed) | (HOOKS.load(Relaxed) << 8) | (DROP_VALUE.load(Relaxed) << 16)
}

fn record(value: u64) {
    DROPS.fetch_add(1, Relaxed);
    DROP_VALUE.fetch_add(value, Relaxed);
}

fn bytes_checksum(value: &[u8]) -> u64 {
    value.iter().fold(0, |sum, byte| sum.wrapping_mul(31).wrapping_add(*byte as u64))
}

pub fn make_string(seed: u64) -> String { "oxide雪".repeat((seed % 5 + 1) as usize) }
pub fn inspect_string(value: &String) -> u64 { bytes_checksum(value.as_bytes()) }
pub fn make_vector(seed: u64) -> Vec<String> { (0..3).map(|i| make_string(seed + i)).collect() }
pub fn inspect_vector(value: &Vec<String>) -> u64 { value.iter().fold(0, |a, b| a ^ inspect_string(b)) }
// Same three-word Go layout as Vec<String>; only the vector buffer is owned.
pub fn make_borrowed_vector<'a>(seed: u64) -> Vec<&'a str> {
    vec!["oxide", "雪", if seed % 2 == 0 { "even" } else { "odd" }]
}
pub fn inspect_borrowed_vector(value: &Vec<&str>) -> u64 {
    value.iter().fold(0, |a, b| a ^ bytes_checksum(b.as_bytes()))
}
pub fn make_box(seed: u64) -> Box<u64> { Box::new(seed ^ 0x5a5a) }
pub fn inspect_box(value: &Box<u64>) -> u64 { **value }
// These distinct owned and borrowed types share the thin-pointer Go ABI.
pub fn make_boxed_string(seed: u64) -> Box<String> { Box::new(make_string(seed)) }
pub fn inspect_boxed_string(value: &Box<String>) -> u64 { inspect_string(value) }
pub fn borrow_box(value: &Box<u64>) -> *const u64 { &**value }

pub trait Check { fn checksum(&self) -> u64; }
pub struct Probe { value: u64, owner: Box<u64> }
impl Check for Probe { fn checksum(&self) -> u64 { self.value ^ *self.owner } }
impl Drop for Probe { fn drop(&mut self) { record(self.value); } }
pub fn make_dynamic(seed: u64) -> Box<dyn Check> {
    Box::new(Probe { value: seed, owner: Box::new(seed.wrapping_mul(3)) })
}
pub fn inspect_dynamic(value: &Box<dyn Check>) -> u64 { value.checksum() }
// &dyn Check and Box<dyn Check> share their two-word representation, not ownership.
pub fn borrow_dynamic(value: &Box<dyn Check>) -> &dyn Check { &**value }

#[repr(align(64))]
pub struct Aligned { owner: Box<u64>, bytes: [u8; 64] }
impl Drop for Aligned {
    fn drop(&mut self) {
        assert_eq!(self as *const Self as usize % 64, 0);
        assert!(self.bytes.iter().all(|&b| b == *self.owner as u8));
        record(*self.owner);
    }
}
pub fn make_aligned(seed: u64) -> Aligned { Aligned { owner: Box::new(seed), bytes: [seed as u8; 64] } }
pub fn inspect_aligned(value: &Aligned) -> u64 { *value.owner ^ bytes_checksum(&value.bytes) }

#[repr(align(64))]
pub struct Large { owner: Box<u64>, bytes: [u8; LARGE_SIZE] }
impl Drop for Large {
    fn drop(&mut self) {
        assert_eq!(self as *const Self as usize % 64, 0);
        assert!(self.bytes.iter().all(|&b| b == *self.owner as u8));
        record(*self.owner);
    }
}
pub fn make_large(seed: u64) -> Large { Large { owner: Box::new(seed), bytes: [seed as u8; LARGE_SIZE] } }
pub fn inspect_large(value: &Large) -> u64 { *value.owner ^ bytes_checksum(&value.bytes) }

pub fn make_result(seed: u64) -> Result<Vec<String>, Box<dyn Check>> {
    if seed % 2 == 0 { Ok(make_vector(seed)) } else { Err(make_dynamic(seed)) }
}
pub fn inspect_result(value: &Result<Vec<String>, Box<dyn Check>>) -> u64 {
    match value { Ok(v) => inspect_vector(v), Err(v) => inspect_dynamic(v) }
}

pub struct Zero;
impl Drop for Zero { fn drop(&mut self) { record(0); } }
pub fn make_zero(_: u64) -> Zero { Zero }
pub fn inspect_zero(_: &Zero) -> u64 { 0 }

pub struct PanicDrop { seed: u64, field: Probe }
impl Drop for PanicDrop { fn drop(&mut self) { record(self.seed); panic_any(self.seed); } }
pub fn make_panic(seed: u64) -> PanicDrop {
    PanicDrop { seed, field: Probe { value: seed + 1, owner: Box::new(seed) } }
}
// The callback runs the public Go destructor in translated execution. Rust's
// real catch_unwind owns and drops the exception payload after that boundary.
pub fn catch_callback(callback: fn(), seed: u64) -> bool {
    match catch_unwind(callback) {
        Err(payload) => payload.downcast::<u64>().is_ok_and(|value| *value == seed),
        Ok(_) => false,
    }
}
