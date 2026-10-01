fn main() {
    for case in 0..9 {
        for seed in [0, 1, 255] {
            let value = oxide_libc_fixture::conformance(case, seed);
            assert_ne!(value, 0, "native C ABI fixture failed: case {case}");
            println!("{case} {seed} {value}");
        }
    }
}
