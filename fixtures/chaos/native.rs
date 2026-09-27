fn main() {
    for seed in [
        1u64,
        2,
        3,
        7,
        42,
        255,
        65535,
        0x5eed,
        0x12345678,
        0x87654321,
        u64::MAX,
        0xdeadbeef,
    ] {
        for steps in [128usize, 257] {
            println!("{seed} {steps} {}", oxide_chaos_fixture::cycle(seed, steps));
        }
    }
}
