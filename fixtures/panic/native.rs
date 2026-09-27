fn main() {
    for case in 0..7 {
        for seed in [0, 1, 2, u64::MAX] {
            let result = oxide_panic_fixture::conformance(case, seed);
            let (value, hooks, payloads, locations, drops, order): (u64, u64, u64, u64, u64, u64) = match case {
                0 => (1, 1, 1, 1, 0, 0),
                1 if seed & 1 != 0 => (2, 1, 2, 1, 0, 0),
                1 => (1, 0, 0, 0, 0, 0),
                2 => (1, 1, 4, 1, 1, 7),
                3 => (1, 0, 0, 0, 1, 5),
                4 => (1, 0, 0, 0, 3, 326),
                5 => (2, 2, 3, 2, 0, 0),
                6 => {
                    let hash = format!("tag {} {}", "name", seed).bytes().fold(
                        0xcbf29ce484222325_u64,
                        |hash, byte| hash.wrapping_mul(0x100000001b3) ^ u64::from(byte),
                    );
                    (hash, 0, 0, 0, 0, 0)
                }
                _ => unreachable!(),
            };
            assert_eq!(result, value | hooks << 8 | payloads << 16 | locations << 24 | drops << 32 | order << 40);
            println!("{case} {seed} {result}");
        }
    }
}
