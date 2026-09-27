#![cfg_attr(oxide_export, no_std)]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

use core::sync::atomic::{AtomicU64, Ordering};

const SIZE: usize = 256 * 1024;
const MASK: usize = SIZE - 1;
static DROPS: AtomicU64 = AtomicU64::new(0);
const CONSTANT: [u8; SIZE] = [0xa5; SIZE];

#[inline(never)]
fn take(a: [u8; SIZE], seed: u64) -> u64 {
    a[(seed as usize) & MASK] as u64
}

#[inline(never)]
fn make(seed: u64) -> [u8; SIZE] {
    [seed as u8; SIZE]
}

#[inline(never)]
fn change(mut a: [u8; SIZE], seed: u64) -> [u8; SIZE] {
    let i = (seed as usize) & MASK;
    a[i] = a[i].wrapping_add(0x51);
    a
}

#[inline(never)]
fn change_other(mut a: [u8; SIZE], seed: u64) -> [u8; SIZE] {
    let i = (seed as usize) & MASK;
    a[i] = a[i].wrapping_sub(0x37);
    a
}

#[inline(never)]
fn modify_copy(mut a: [u8; SIZE], seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    a[i] ^= 0xa5;
    a[i] as u64
}

#[repr(transparent)]
struct Wrapped([u8; SIZE]);

trait Transform {
    fn run(&self, a: [u8; SIZE], seed: u64) -> [u8; SIZE];
}

struct Transformer(u8);

impl Transform for Transformer {
    fn run(&self, mut a: [u8; SIZE], seed: u64) -> [u8; SIZE] {
        let i = (seed as usize) & MASK;
        a[i] = a[i].wrapping_add(self.0);
        a
    }
}

#[inline(never)]
fn dynamic(value: &dyn Transform, a: [u8; SIZE], seed: u64) -> [u8; SIZE] {
    value.run(a, seed)
}

struct Large {
    first: u64,
    bytes: [u8; SIZE],
    last: u64,
}

impl Drop for Large {
    fn drop(&mut self) {
        DROPS.fetch_add(1, Ordering::Relaxed);
    }
}

#[inline(never)]
fn make_struct(seed: u64) -> Large {
    Large {
        first: seed.rotate_left(7),
        bytes: [seed as u8; SIZE],
        last: !seed,
    }
}

#[inline(never)]
fn direct(seed: u64) -> u64 {
    let a = [seed as u8; SIZE];
    a[(seed as usize) & MASK] as u64
}

#[inline(never)]
fn moved(seed: u64) -> u64 {
    let a = [seed as u8; SIZE];
    take(a, seed)
}

#[inline(never)]
fn returned(seed: u64) -> u64 {
    let a = make(seed);
    a[(seed as usize) & MASK] as u64
}

#[inline(never)]
fn copied(seed: u64) -> u64 {
    let a = [seed as u8; SIZE];
    let changed = modify_copy(a, seed);
    (a[(seed as usize) & MASK] as u64) | (changed << 8)
}

#[inline(never)]
fn reassigned(seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    let mut a = make(seed);
    a = change(a, seed);
    (a[i] as u64) | ((a[i ^ MASK] as u64) << 8)
}

#[inline(never)]
fn function_pointer(seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    let transform: fn([u8; SIZE], u64) -> [u8; SIZE] =
        if seed & 1 == 0 { change } else { change_other };
    let a = transform(make(seed), seed);
    (a[i] as u64) | ((a[i ^ MASK] as u64) << 8)
}

#[inline(never)]
fn transparent_pointer(seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    // repr(transparent) preserves the Rust ABI of the array field.
    let transform: fn([u8; SIZE], u64) -> [u8; SIZE] = change;
    let transform: fn(Wrapped, u64) -> Wrapped = unsafe { core::mem::transmute(transform) };
    let a = transform(Wrapped(make(seed)), seed);
    (a.0[i] as u64) | ((a.0[i ^ MASK] as u64) << 8)
}

#[inline(never)]
fn virtual_call(seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    let value = Transformer(((seed >> 8) as u8) | 1);
    let a = dynamic(&value, make(seed), seed);
    (a[i] as u64) | ((a[i ^ MASK] as u64) << 8)
}

#[inline(never)]
fn returned_drop(seed: u64) -> u64 {
    DROPS.store(0, Ordering::Relaxed);
    let value = make_struct(seed);
    let result = value.first ^ value.last ^ (value.bytes[(seed as usize) & MASK] as u64);
    drop(value);
    result ^ DROPS.load(Ordering::Relaxed).rotate_left(31)
}

#[inline(never)]
fn constant() -> [u8; SIZE] {
    CONSTANT
}

#[inline(never)]
fn returned_constant(seed: u64) -> u64 {
    let a = constant();
    a[(seed as usize) & MASK] as u64
}

#[inline(never)]
fn repeated(seed: u64) -> u64 {
    let a = [[seed as u8; 32 * 1024]; 2];
    a[(seed as usize) & 1][(seed as usize) & (32 * 1024 - 1)] as u64
}

#[inline(never)]
fn callback<F: FnOnce([u8; SIZE]) -> [u8; SIZE]>(f: F, value: [u8; SIZE]) -> [u8; SIZE] {
    f(value)
}

#[inline(never)]
fn closure_call(seed: u64) -> u64 {
    let i = (seed as usize) & MASK;
    let a = callback(|value| change(value, seed), make(seed));
    (a[i] as u64) | ((a[i ^ MASK] as u64) << 8)
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    match case {
        0 => direct(seed),
        1 => moved(seed),
        2 => returned(seed),
        3 => copied(seed),
        4 => reassigned(seed),
        5 => function_pointer(seed),
        6 => transparent_pointer(seed),
        7 => virtual_call(seed),
        8 => returned_drop(seed),
        9 => returned_constant(seed),
        10 => repeated(seed),
        11 => closure_call(seed),
        _ => unreachable!(),
    }
}
