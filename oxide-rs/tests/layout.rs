#![no_std]
#![feature(linkage)]

use core::fmt::{self, Write};

pub fn higher_ranked_fn(
    callback: for<'a> fn(&'a mut dyn Write) -> usize,
    output: &mut dyn Write,
) -> usize {
    callback(output)
}

pub fn higher_ranked_trait(output: &mut dyn Write) -> fmt::Result {
    let mut callback = |writer: &mut dyn Write| writer.write_str("oxide");
    let callback: &mut dyn FnMut(&mut dyn Write) -> fmt::Result = &mut callback;
    callback(output)
}

#[repr(C)]
pub struct Tail<T: ?Sized> {
    pub prefix: u64,
    pub value: T,
}

pub fn struct_slice(value: &Tail<[u32; 3]>) -> usize {
    let value: &Tail<[u32]> = value;
    value.value.len()
}

pub trait Value {
    fn value(&self) -> u64;
}

impl Value for u64 {
    fn value(&self) -> u64 {
        *self
    }
}

pub fn struct_trait(value: &Tail<u64>) -> u64 {
    let value: &Tail<dyn Value> = value;
    value.value.value()
}

fn erased_argument<T: ?Sized>(_: &T) -> usize {
    7
}

pub fn reify_dyn(value: &(dyn Value + 'static)) -> usize {
    let callback: fn(&(dyn Value + 'static)) -> usize = erased_argument::<dyn Value>;
    callback(value)
}

#[track_caller]
pub fn tracked() -> u32 {
    9
}

pub fn formatted(output: &mut dyn Write, value: u32) -> fmt::Result {
    write!(output, "{value}")
}

pub unsafe fn volatile(value: *const u8) -> u8 {
    unsafe { core::ptr::read_volatile(value) }
}

pub fn type_id() -> core::any::TypeId {
    core::any::TypeId::of::<u32>()
}

unsafe extern "C" {
    #[link_name = "strlen"]
    fn strlen_first(value: *const core::ffi::c_char) -> usize;
    #[link_name = "strlen"]
    fn strlen_second(value: *const core::ffi::c_char) -> usize;
}

pub unsafe fn duplicate_symbols(value: *const core::ffi::c_char) -> usize {
    unsafe { strlen_first(value) + strlen_second(value) }
}

#[track_caller]
pub fn caller_line() -> u32 {
    core::panic::Location::caller().line()
}

pub fn observed_line() -> u32 {
    caller_line()
}

pub fn reified_caller_line() -> u32 {
    let callback: fn() -> u32 = caller_line;
    callback()
}

unsafe extern "C" {
    #[linkage = "extern_weak"]
    #[link_name = "oxide_missing_weak_probe"]
    static WEAK_PROBE: Option<unsafe extern "C" fn() -> u32>;
    #[link_name = "oxide_required_probe"]
    static REQUIRED_PROBE: u64;
    fn oxide_variadic_probe(first: u32, ...) -> i64;
}

pub fn weak_available() -> bool {
    unsafe { WEAK_PROBE.is_some() }
}

pub unsafe fn required_value() -> u64 {
    unsafe { REQUIRED_PROBE }
}

pub unsafe fn variadic_call() -> i64 {
    unsafe { oxide_variadic_probe(1, 2u64) }
}
