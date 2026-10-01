#![feature(f16, f128)]
#[path = "float_math.rs"]
mod fixture;

fn main() {
    let mut inputs = vec![
        0.0_f128, -0.0, 0.25, -0.25, 0.5, -0.5, 0.75, -0.75, 1.0, -1.0, 1.5, -1.5, 2.0, 3.0, 4.25,
        8.0, 12.0, 150.5, -150.5, 1755.5, 1756.5, 65472.0,
    ];
    inputs.extend([
        f128::from_bits(1),
        f128::MIN_POSITIVE,
        f128::MAX,
        f128::INFINITY,
        f128::NEG_INFINITY,
        f128::NAN,
    ]);
    inputs.extend([
        f128::from_bits(0x3fff0000000000000000000000000001),
        f128::from_bits(0x43e70000000000000000000000000001),
    ]);
    for half in [false, true] {
        for op in 0..30 {
            for (i, &input) in inputs.iter().enumerate() {
                let other = [0.75_f128, 1.5, -3.2][i % 3];
                let (x, y) = if half {
                    (
                        (input as f16).to_bits() as u128,
                        (other as f16).to_bits() as u128,
                    )
                } else {
                    (input.to_bits(), other.to_bits())
                };
                let result = fixture::evaluate(op, half, x, y);
                let sign = if op == 24 {
                    fixture::gamma_sign(half, x)
                } else {
                    0
                };
                println!(
                    "{} {op} {:016x} {:016x} {:016x} {:016x} {:016x} {:016x} {sign}",
                    half as u8,
                    x >> 64,
                    x as u64,
                    y >> 64,
                    y as u64,
                    result >> 64,
                    result as u64
                );
            }
        }
    }
}
