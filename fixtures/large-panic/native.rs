fn main() {
    for case in 0..2 {
        for seed in [0, 1, 65535, u64::MAX] {
            let result = oxide_large_panic_fixture::conformance(case, seed);
            assert_eq!(result, 15, "large value panic case {case}, seed {seed}");
            println!("{case} {seed} {result}");
        }
    }
}
