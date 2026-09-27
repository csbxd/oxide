use std::os::unix::ffi::OsStrExt;

pub fn args_digest() -> u64 {
    let mut hash = 0xcbf29ce484222325u64;
    for argument in std::env::args_os() {
        hash = hash.wrapping_mul(1099511628211) ^ argument.as_bytes().len() as u64;
        for &byte in argument.as_bytes() {
            hash = hash.wrapping_mul(1099511628211) ^ byte as u64;
        }
        hash ^= if argument.into_string().is_ok() { 0x100 } else { 0x200 };
    }
    let mut reverse = std::env::args_os();
    hash ^= reverse.len() as u64;
    while let Some(argument) = reverse.next_back() {
        for &byte in argument.as_bytes() {
            hash = hash.wrapping_mul(31) ^ byte as u64;
        }
    }
    hash
}

struct StackDrop;
impl Drop for StackDrop {
    fn drop(&mut self) { eprintln!("stack-drop"); }
}

struct ThreadDrop;
impl Drop for ThreadDrop {
    fn drop(&mut self) { eprintln!("tls-drop"); }
}

thread_local! { static TLS: ThreadDrop = const { ThreadDrop }; }

unsafe extern "C" fn pthread_drop(_: *mut std::ffi::c_void) {
    eprintln!("pthread-drop");
}

unsafe extern "C" {
    fn pthread_key_create(key: *mut u32, destructor: Option<unsafe extern "C" fn(*mut std::ffi::c_void)>) -> i32;
    fn pthread_setspecific(key: u32, value: *const std::ffi::c_void) -> i32;
}

pub fn exit_probe(code: i32) -> ! {
    let _stack = StackDrop;
    TLS.with(|_| {});
    let mut key = 0;
    unsafe {
        assert_eq!(pthread_key_create(&mut key, Some(pthread_drop)), 0);
        assert_eq!(pthread_setspecific(key, 1usize as *const std::ffi::c_void), 0);
    }
    print!("buffered-stdout");
    std::process::exit(code)
}
