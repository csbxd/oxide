#![allow(dead_code)]

use std::panic::{catch_unwind, panic_any, set_hook};
use std::sync::atomic::{AtomicPtr, AtomicU64, Ordering::Relaxed};

static COUNT: AtomicU64 = AtomicU64::new(0);
static ORDER: AtomicU64 = AtomicU64::new(0);
static SOURCE: AtomicPtr<Owner> = AtomicPtr::new(core::ptr::null_mut());

fn record(id: u64) {
    COUNT.fetch_add(1, Relaxed);
    ORDER.store(ORDER.load(Relaxed) * 100 + id, Relaxed);
}

pub fn setup() { set_hook(Box::new(|_| {})); }
pub fn reset() { COUNT.store(0, Relaxed); ORDER.store(0, Relaxed); SOURCE.store(core::ptr::null_mut(), Relaxed); }
pub fn state() -> u64 { COUNT.load(Relaxed) * 10000 + ORDER.load(Relaxed) }

pub struct Zero;
impl Drop for Zero { fn drop(&mut self) { record(0); } }
pub fn make_zero() -> Zero { Zero }

pub struct Owner {
    pub id: u64,
    pub panic: bool,
    payload: Box<Payload>,
}
struct Payload { value: u64, panic: bool }
impl Drop for Payload { fn drop(&mut self) { if self.panic { panic_any(self.value); } } }
impl Drop for Owner {
    fn drop(&mut self) {
        assert_eq!(self.payload.value, 100 + self.id);
        record(self.id);
        if self.id == 1 {
            let source = SOURCE.load(Relaxed);
            if !source.is_null() {
                // The source is a ManuallyDrop slot in native Rust. The value
                // has already been moved out; mutate the remaining raw storage.
                unsafe { core::ptr::addr_of_mut!((*source).id).write(99); }
            }
        }
        if self.panic { panic_any(self.id); }
    }
}
pub fn make_owner(id: u64, panic: bool) -> Owner { Owner { id, panic, payload: Box::new(Payload { value: 100 + id, panic: false }) } }
pub fn make_field_panic() -> Owner { Owner { id: 1, panic: true, payload: Box::new(Payload { value: 101, panic: true }) } }
pub fn inspect(value: &Owner) -> u64 { value.id * 1000 + value.payload.value }
pub fn source_slot(value: *mut Owner) { SOURCE.store(value, Relaxed); }
pub fn catch_callback(callback: fn()) -> bool {
    match catch_unwind(callback) {
        Err(payload) => payload.downcast::<u64>().is_ok_and(|id| *id == 1),
        Ok(_) => false,
    }
}
