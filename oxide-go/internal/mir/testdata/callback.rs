#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(oxide_export, feature(unboxed_closures))]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

pub type Args = (u64, u64, u64);
pub type RustCall = extern "rust-call" fn(Args) -> u64;
pub type Ordinary = fn(Args) -> u64;

pub fn conformance(case: u64, seed: u64, spread: RustCall, ordinary: Ordinary) -> u64 {
    let args = (seed.wrapping_add(11), seed.rotate_left(7) ^ 22, !seed ^ 33);
    if case == 0 {
        spread(args)
    } else {
        ordinary(args)
    }
}
