pub fn catch_payload(value: u64) -> u64 {
    std::panic::catch_unwind(|| {
        if value == 1 {
            std::panic::panic_any(42u64)
        }
        value + 1
    })
    .unwrap_or_else(|payload| *payload.downcast::<u64>().unwrap())
}

pub fn catch_overflow(value: u64) -> bool {
    std::panic::catch_unwind(|| value + 1).is_err()
}
