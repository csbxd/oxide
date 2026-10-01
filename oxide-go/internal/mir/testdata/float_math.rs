#![allow(dead_code)]
#![cfg_attr(oxide_export, feature(f16, f128))]

pub fn evaluate(op: u8, half: bool, x: u128, y: u128) -> u128 {
    macro_rules! eval {
        ($float:ty, $bits:ty) => {{
            let a = <$float>::from_bits(x as $bits);
            let b = <$float>::from_bits(y as $bits);
            let result = match op {
                0 => a.exp(),
                1 => a.exp2(),
                2 => a.exp_m1(),
                3 => a.ln(),
                4 => a.log2(),
                5 => a.log10(),
                6 => a.ln_1p(),
                7 => a.sin(),
                8 => a.cos(),
                9 => a.tan(),
                10 => a.asin(),
                11 => a.acos(),
                12 => a.atan(),
                13 => a.atan2(b),
                14 => a.sinh(),
                15 => a.cosh(),
                16 => a.tanh(),
                17 => a.asinh(),
                18 => a.acosh(),
                19 => a.atanh(),
                20 => a.cbrt(),
                21 => a.hypot(b),
                22 => a.powf(b),
                23 => a.gamma(),
                24 => a.ln_gamma().0,
                25 => a.erf(),
                26 => a.erfc(),
                27 => a.log(b),
                28 => a.sin_cos().0,
                29 => a.sin_cos().1,
                _ => unreachable!(),
            };
            result.to_bits() as u128
        }};
    }
    if half {
        eval!(f16, u16)
    } else {
        eval!(f128, u128)
    }
}

pub fn gamma_sign(half: bool, x: u128) -> i32 {
    if half {
        f16::from_bits(x as u16).ln_gamma().1
    } else {
        f128::from_bits(x).ln_gamma().1
    }
}
