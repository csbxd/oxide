#![allow(dead_code)]
#![cfg_attr(oxide_export, no_std)]
#![cfg_attr(
    oxide_export,
    feature(
        signed_bigint_helpers,
        disjoint_bitor,
        core_intrinsics,
        funnel_shifts,
        uint_carryless_mul,
        float_minimum_maximum,
        portable_simd
    )
)]

#[cfg(oxide_export)]
#[panic_handler]
fn panic_handler(_: &core::panic::PanicInfo<'_>) -> ! {
    loop {}
}

// The harness enables the intrinsic and integer features on the pinned
// compiler. Each export uses scalars while the arithmetic remains Rust code.
fn fold(value: u128) -> u64 {
    (value as u64) ^ ((value >> 64) as u64).rotate_left(29)
}

fn wide(seed: u64) -> u128 {
    match seed {
        0 => 0,
        1 => u128::MAX,
        2 => 1 << 127,
        3 => i128::MAX as u128,
        4 => 1 << 64,
        5 => u64::MAX as u128,
        _ => ((seed as u128) << 64) | ((!seed).rotate_left(17) as u128),
    }
}

macro_rules! carrying {
    ($seed:expr, $ty:ty) => {{
        let a = $seed as $ty;
        let b = $seed.rotate_left(23) as $ty;
        let c = $seed.rotate_right(5) as $ty;
        let d = $seed.rotate_right(11) as $ty;
        let (lo, hi) = a.carrying_mul_add(b, c, d);
        fold(lo as u128).rotate_left(7) ^ fold(hi as u128)
    }};
}

fn float32_bits(seed: u64) -> u32 {
    const BITS: [u32; 20] = [
        0,
        0x8000_0000,
        1,
        0x8000_0001,
        0x3f00_0000,
        0xbf00_0000,
        0x3fc0_0000,
        0xbfc0_0000,
        0x7eff_ffff,
        0x7f00_0000,
        0x7f7f_ffff,
        0xfeff_ffff,
        0xff00_0000,
        0xff7f_ffff,
        0x7f80_0000,
        0xff80_0000,
        0x7fc0_0001,
        0xffc0_0001,
        0x7f80_0001,
        0xff80_0001,
    ];
    if seed < BITS.len() as u64 {
        BITS[seed as usize]
    } else {
        seed as u32
    }
}

fn float64_bits(seed: u64) -> u64 {
    const BITS: [u64; 20] = [
        0,
        0x8000_0000_0000_0000,
        1,
        0x8000_0000_0000_0001,
        0x3fe0_0000_0000_0000,
        0xbfe0_0000_0000_0000,
        0x3ff8_0000_0000_0000,
        0xbff8_0000_0000_0000,
        0x47df_ffff_ffff_ffff,
        0x47e0_0000_0000_0000,
        0x47ef_ffff_ffff_ffff,
        0xc7df_ffff_ffff_ffff,
        0xc7e0_0000_0000_0000,
        0xc7ef_ffff_ffff_ffff,
        0x7ff0_0000_0000_0000,
        0xfff0_0000_0000_0000,
        0x7ff8_0000_0000_0001,
        0xfff8_0000_0000_0001,
        0x7ff0_0000_0000_0001,
        0xfff0_0000_0000_0001,
    ];
    if seed < BITS.len() as u64 {
        BITS[seed as usize]
    } else {
        seed
    }
}

fn rounding_value(seed: u64) -> f64 {
    const VALUES: [f64; 24] = [
        0.0,
        -0.0,
        0.5,
        -0.5,
        1.5,
        -1.5,
        2.5,
        -2.5,
        0.49999999999999994,
        0.5000000000000001,
        -0.49999999999999994,
        -0.5000000000000001,
        8388607.5,
        8388608.0,
        4503599627370495.5,
        4503599627370496.0,
        f64::INFINITY,
        f64::NEG_INFINITY,
        f64::from_bits(0x7ff8_0000_0000_0001),
        f64::from_bits(0xfff8_0000_0000_0001),
        f64::from_bits(0x7ff0_0000_0000_0001),
        f64::from_bits(0xfff0_0000_0000_0001),
        f64::from_bits(1),
        f64::from_bits(0x8000_0000_0000_0001),
    ];
    if seed < VALUES.len() as u64 {
        VALUES[seed as usize]
    } else {
        f64::from_bits(seed)
    }
}

// NaN sign and payload are unspecified, but retain the quiet/signaling bit.
fn bits32(value: f32) -> u64 {
    (if value.is_nan() {
        value.to_bits() & 0x7fc0_0000
    } else {
        value.to_bits()
    }) as u64
}

fn bits64(value: f64) -> u64 {
    if value.is_nan() {
        value.to_bits() & 0x7ff8_0000_0000_0000
    } else {
        value.to_bits()
    }
}

#[link(name = "m")]
unsafe extern "C" {
    fn ldexp(value: f64, exponent: i32) -> f64;
    fn ldexpf(value: f32, exponent: i32) -> f32;
    fn __errno_location() -> *mut i32;
}

fn exponent(seed: u64) -> i32 {
    const EXPONENTS: [i32; 12] = [
        i32::MIN,
        -1075,
        -1074,
        -1023,
        -150,
        -149,
        0,
        1,
        127,
        1023,
        1024,
        i32::MAX,
    ];
    EXPONENTS[seed as usize % EXPONENTS.len()]
}

static STATIC_A: [u8; 2] = [104, 105];
static STATIC_B: [u8; 2] = [104, 105];
static mut MUTABLE_A: u32 = 73;
static mut MUTABLE_B: u32 = 73;

macro_rules! funnel {
    ($seed:expr, $ty:ty, $method:ident) => {{
        let a = wide($seed) as $ty;
        let b = wide($seed.rotate_left(23)) as $ty;
        fold(a.$method(b, $seed as u32 & (<$ty>::BITS - 1)) as u128)
    }};
}

macro_rules! carryless {
    ($seed:expr, $ty:ty) => {{
        let a = wide($seed) as $ty;
        let b = wide($seed.rotate_left(23)) as $ty;
        fold(a.carryless_mul(b) as u128)
    }};
}

struct PendingCount {
    remaining: u32,
    value: u64,
}

impl core::future::Future for PendingCount {
    type Output = u64;
    fn poll(
        mut self: core::pin::Pin<&mut Self>,
        cx: &mut core::task::Context<'_>,
    ) -> core::task::Poll<u64> {
        if self.remaining == 0 {
            return core::task::Poll::Ready(self.value);
        }
        self.remaining -= 1;
        cx.waker().wake_by_ref();
        core::task::Poll::Pending
    }
}

struct CoroutineDrop<'a> {
    trace: &'a core::cell::Cell<u64>,
    digit: u64,
}

impl Drop for CoroutineDrop<'_> {
    fn drop(&mut self) {
        self.trace.set(self.trace.get() * 10 + self.digit);
    }
}

fn coroutine_case(case: u64, seed: u64) -> u64 {
    use core::future::Future;
    use core::task::{Context, Poll, Waker};
    let trace = core::cell::Cell::new(0);
    let n = if case == 0 { 0 } else { (seed % 3 + 1) as u32 };
    let m = if case == 3 { (seed % 4 + 1) as u32 } else { 0 };
    let mut polls = 0u64;
    let mut output = 0u64;
    {
        let mut future = core::pin::pin!(async {
            let _first = CoroutineDrop {
                trace: &trace,
                digit: 1,
            };
            let a = PendingCount {
                remaining: n,
                value: seed,
            }
            .await;
            let _second = CoroutineDrop {
                trace: &trace,
                digit: 2,
            };
            let b = PendingCount {
                remaining: m,
                value: !seed,
            }
            .await;
            a.wrapping_mul(7) ^ b.rotate_left(5)
        });
        let mut cx = Context::from_waker(Waker::noop());
        loop {
            polls += 1;
            match future.as_mut().poll(&mut cx) {
                Poll::Ready(value) => {
                    output = value;
                    break;
                }
                Poll::Pending if case == 2 => break,
                Poll::Pending => assert!(polls <= (n + m + 1) as u64),
            }
        }
    }
    assert_eq!(trace.get(), if case == 2 { 1 } else { 21 });
    assert_eq!(polls, if case == 2 { 1 } else { (n + m + 1) as u64 });
    output ^ trace.get().rotate_left(13) ^ polls.rotate_left(29)
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    let n = wide(seed);
    match case {
        0 => fold(n / (seed as u128 | 1)),
        1 => fold(n % (seed as u128 | 1)),
        2 => fold(n / (wide(seed.rotate_left(19)) | 1)),
        3 => fold(n % (wide(seed.rotate_left(19)) | 1)),
        4 => fold(((n as i128) / ((seed as i64 as i128) | 1)) as u128),
        5 => fold(((n as i128) % ((seed as i64 as i128) | 1)) as u128),
        6 => (n as f32).to_bits() as u64,
        7 => (n as f64).to_bits(),
        8 => ((n as i128) as f32).to_bits() as u64,
        9 => ((n as i128) as f64).to_bits(),
        // These halfway + 1 values expose incorrect u128 -> f64 -> f32
        // double rounding. Native Rust performs a direct integer -> f32 cast.
        10 => (((1u128 << 127) + (1u128 << 103) + (seed & 1) as u128) as f32).to_bits() as u64,
        11 => (((1i128 << 126) + (1i128 << 102) + (seed & 1) as i128) as f32).to_bits() as u64,
        12 => ((-((1i128 << 126) + (1i128 << 102) + (seed & 1) as i128)) as f32).to_bits() as u64,
        13 => (((1u128 << 127) + (1u128 << 74) + (seed & 1) as u128) as f64).to_bits(),
        14 => fold(f32::from_bits(float32_bits(seed)) as u128),
        15 => fold((f32::from_bits(float32_bits(seed)) as i128) as u128),
        16 => fold(f64::from_bits(float64_bits(seed)) as u128),
        17 => fold((f64::from_bits(float64_bits(seed)) as i128) as u128),
        18 => carrying!(seed, u8),
        19 => carrying!(seed, u16),
        20 => carrying!(seed, u32),
        21 => carrying!(seed, u64),
        22 => {
            let (lo, hi) =
                n.carrying_mul_add(wide(seed.rotate_left(23)), !n, wide(seed.rotate_right(11)));
            fold(lo).rotate_left(7) ^ fold(hi)
        }
        23 => carrying!(seed, i8),
        24 => carrying!(seed, i16),
        25 => carrying!(seed, i32),
        26 => carrying!(seed, i64),
        27 => {
            let (lo, hi) = (n as i128).carrying_mul_add(
                wide(seed.rotate_left(23)) as i128,
                (!n) as i128,
                wide(seed.rotate_right(11)) as i128,
            );
            fold(lo).rotate_left(7) ^ fold(hi as u128)
        }
        28 => unsafe {
            (seed & 0xffff_ffff).unchecked_disjoint_bitor(seed & 0xffff_ffff_0000_0000)
        },
        29 => fold(unsafe {
            (n & u64::MAX as u128).unchecked_disjoint_bitor(n & !(u64::MAX as u128))
        }),
        // Ordinary Rust multiplication and addition round separately. Go may
        // fuse across statements unless each MIR operation has a barrier.
        31 => {
            let (a, b, c) = match seed & 3 {
                0 => (0x3f80_0001, 0x3f7f_fffe, 0xbf80_0000),
                1 => (0x3f80_0800, 0x3f80_0800, 0xbf80_1000),
                2 => (0x8000_0000, 0x4040_0000, 0x8000_0000),
                _ => (0, 0x4040_0000, 0),
            };
            let product = f32::from_bits(a) * f32::from_bits(b);
            (product + f32::from_bits(c)).to_bits() as u64
        }
        32 => {
            let (a, b, c) = match seed & 3 {
                0 => (
                    0x3ff0_0000_0000_0001,
                    0x3fef_ffff_ffff_fffe,
                    0xbff0_0000_0000_0000,
                ),
                1 => (
                    0x3ff0_0000_0200_0000,
                    0x3ff0_0000_0200_0000,
                    0xbff0_0000_0400_0000,
                ),
                2 => (
                    0x8000_0000_0000_0000,
                    0x4008_0000_0000_0000,
                    0x8000_0000_0000_0000,
                ),
                _ => (0, 0x4008_0000_0000_0000, 0),
            };
            let product = f64::from_bits(a) * f64::from_bits(b);
            (product + f64::from_bits(c)).to_bits()
        }
        33 => {
            let (a, b) = (seed & 1 != 0, seed & 2 != 0);
            u64::from(a < b)
                | (u64::from(a <= b) << 1)
                | (u64::from(a > b) << 2)
                | (u64::from(a >= b) << 3)
        }
        34 => n.leading_zeros() as u64,
        35 => n.trailing_zeros() as u64,
        36 => n.count_ones() as u64,
        37 => fold(n.rotate_left(seed as u32)),
        38 => fold(n.rotate_right(seed as u32)),
        39 => {
            fold((n as i128).rotate_right(seed as u32) as u128)
                ^ ((n as i128).leading_zeros() as u64) << 32
                ^ ((n as i128).trailing_zeros() as u64) << 16
                ^ (n as i128).count_ones() as u64
        }
        40 => seed << ((seed & 63) as u128),
        41 => fold(n >> ((seed & 127) as i128)),
        42 => fold(((n as i128) >> ((seed & 127) as i128)) as u128),
        43 => fold(unsafe { core::intrinsics::exact_div((n as i128) & !3, 4) } as u128),
        44 => fold(unsafe { core::intrinsics::exact_div(n & !3, 4) }),
        45 => {
            let f: fn(u32) -> [u32; 3] = |value| [value, 0, 0];
            let values = f(seed as u32);
            values[0] as u64 ^ (values[1] as u64) << 32 ^ values[2] as u64
        }
        46 => fold(n.min(wide(seed.rotate_left(19)))),
        47 => fold((n as i128).max(wide(seed.rotate_left(19)) as i128) as u128),
        48 => bits32(core::intrinsics::round_ties_even_f32(
            rounding_value(seed) as f32
        )),
        49 => bits64(core::intrinsics::round_ties_even_f64(rounding_value(seed))),
        50 => bits64(core::intrinsics::truncf64(rounding_value(seed))),
        51 => bits32(
            f32::from_bits(float32_bits(seed))
                .minimum(f32::from_bits(float32_bits(seed.wrapping_add(1) % 20))),
        ),
        52 => bits32(
            f32::from_bits(float32_bits(seed))
                .maximum(f32::from_bits(float32_bits(seed.wrapping_add(1) % 20))),
        ),
        53 => bits64(
            f64::from_bits(float64_bits(seed))
                .minimum(f64::from_bits(float64_bits(seed.wrapping_add(1) % 20))),
        ),
        54 => bits64(
            f64::from_bits(float64_bits(seed))
                .maximum(f64::from_bits(float64_bits(seed.wrapping_add(1) % 20))),
        ),
        55 => {
            let x = rounding_value(seed);
            if x >= -128.0 && x < 128.0 {
                unsafe { x.to_int_unchecked::<i8>() as u64 }
            } else {
                0
            }
        }
        56 => {
            let x = rounding_value(seed);
            if x > -1.0 && x < 256.0 {
                unsafe { x.to_int_unchecked::<u8>() as u64 }
            } else {
                0
            }
        }
        57 => {
            let x = rounding_value(seed);
            if x >= -9223372036854775808.0 && x < 9223372036854775808.0 {
                unsafe { x.to_int_unchecked::<i64>() as u64 }
            } else {
                0
            }
        }
        58 => {
            let x = rounding_value(seed);
            if x >= 0.0 && x < 18446744073709551616.0 {
                unsafe { x.to_int_unchecked::<u64>() }
            } else {
                0
            }
        }
        59 => {
            let x = rounding_value(seed);
            if x >= i128::MIN as f64 && x < -(i128::MIN as f64) {
                fold(unsafe { x.to_int_unchecked::<i128>() } as u128)
            } else {
                0
            }
        }
        60 => {
            let x = rounding_value(seed);
            if x >= 0.0 && x < 2.0 * (1u128 << 127) as f64 {
                fold(unsafe { x.to_int_unchecked::<u128>() })
            } else {
                0
            }
        }
        61 => unsafe { core::intrinsics::const_allocate(seed as usize, 4) as u64 },
        62 => {
            let mut values = [seed as u32; 3];
            unsafe { core::intrinsics::const_deallocate(values.as_mut_ptr().cast(), 12, 4) };
            values[0] as u64 ^ ((values[2] as u64) << 32)
        }
        63 => funnel!(seed, u8, funnel_shl),
        64 => funnel!(seed, u8, funnel_shr),
        65 => funnel!(seed, u16, funnel_shl),
        66 => funnel!(seed, u16, funnel_shr),
        67 => funnel!(seed, u32, funnel_shl),
        68 => funnel!(seed, u32, funnel_shr),
        69 => funnel!(seed, u64, funnel_shl),
        70 => funnel!(seed, u64, funnel_shr),
        71 => funnel!(seed, u128, funnel_shl),
        72 => funnel!(seed, u128, funnel_shr),
        73 => carryless!(seed, u8),
        74 => carryless!(seed, u16),
        75 => carryless!(seed, u32),
        76 => carryless!(seed, u64),
        77 => carryless!(seed, u128),
        78 => {
            let x = f32::from_bits(float32_bits(seed));
            if x >= i128::MIN as f32 && x < -(i128::MIN as f32) {
                fold(unsafe { x.to_int_unchecked::<i128>() } as u128)
            } else {
                0
            }
        }
        79 => {
            let x = f32::from_bits(float32_bits(seed));
            if x >= 0.0 && x.is_finite() {
                fold(unsafe { x.to_int_unchecked::<u128>() })
            } else {
                0
            }
        }
        80 => bits64(core::intrinsics::fmaf64(
            f64::from_bits(float64_bits(seed)),
            f64::from_bits(float64_bits(seed.wrapping_add(1) % 20)),
            rounding_value(seed),
        )),
        81 => bits64(unsafe { ldexp(f64::from_bits(float64_bits(seed)), exponent(seed)) }),
        82 => bits32(unsafe { ldexpf(f32::from_bits(float32_bits(seed)), exponent(seed)) }),
        83 => unsafe {
            let errno = __errno_location();
            *errno = 117;
            ldexp(f64::from_bits(float64_bits(seed)), exponent(seed));
            *errno as u64
        },
        84 => unsafe {
            let errno = __errno_location();
            *errno = 117;
            ldexpf(f32::from_bits(float32_bits(seed)), exponent(seed));
            *errno as u64
        },
        85 => {
            const A: [u8; 2] = [104, 105];
            const B: &[u8; 2] = &A;
            const C: *const u8 = B as *const u8;
            u64::from(&A as *const u8 == C)
        }
        86 => u64::from(&STATIC_A as *const [u8; 2] != &STATIC_B as *const [u8; 2]),
        87 => {
            const A: u128 = 0x6968;
            const B: [u8; 16] = [104, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
            let a = &A as *const u128 as *const u8;
            let b = &B as *const u8;
            u64::from(a == b) | (u64::from((a as usize) % 16 == 0) << 1)
        }
        88 => unsafe {
            let a = &raw mut MUTABLE_A;
            let b = &raw mut MUTABLE_B;
            *a = seed as u32;
            u64::from(a != b && *b == 73)
        },
        89..=93 => {
            let a = (seed as i32 as f32) / 13.0;
            let b = (seed.rotate_left(17) as i16 as f32) / 7.0 + 0.25;
            bits32(match case {
                89 => core::intrinsics::fadd_algebraic(a, b),
                90 => core::intrinsics::fsub_algebraic(a, b),
                91 => core::intrinsics::fmul_algebraic(a, b),
                92 => core::intrinsics::fdiv_algebraic(a, b),
                _ => core::intrinsics::frem_algebraic(a, b),
            })
        }
        94..=98 => {
            let a = (seed as i32 as f64) / 13.0;
            let b = (seed.rotate_left(17) as i16 as f64) / 7.0 + 0.25;
            bits64(match case {
                94 => core::intrinsics::fadd_algebraic(a, b),
                95 => core::intrinsics::fsub_algebraic(a, b),
                96 => core::intrinsics::fmul_algebraic(a, b),
                97 => core::intrinsics::fdiv_algebraic(a, b),
                _ => core::intrinsics::frem_algebraic(a, b),
            })
        }
        99..=100 => {
            let x = core::simd::Simd::from_array([
                f32::from_bits(float32_bits(seed)),
                f32::from_bits(float32_bits(seed.wrapping_add(1) % 20)),
                f32::from_bits(float32_bits(seed.wrapping_add(2) % 20)),
                f32::from_bits(float32_bits(seed.wrapping_add(3) % 20)),
            ]);
            use core::simd::num::SimdFloat;
            let y = if case == 99 { -x } else { x.abs() }.to_array();
            y.into_iter().fold(0u64, |acc, lane| {
                acc.rotate_left(13) ^ lane.to_bits() as u64
            })
        }
        101..=102 => {
            let x = core::simd::Simd::from_array([
                f64::from_bits(float64_bits(seed)),
                f64::from_bits(float64_bits(seed.wrapping_add(1) % 20)),
            ]);
            use core::simd::num::SimdFloat;
            let y = if case == 101 { -x } else { x.abs() }.to_array();
            y.into_iter()
                .fold(0u64, |acc, lane| acc.rotate_left(13) ^ lane.to_bits())
        }
        103 => {
            let x = core::simd::Simd::from_array([seed as i8, i8::MIN, i8::MAX, -1]);
            (-x).to_array()
                .into_iter()
                .fold(0u64, |acc, lane| acc.rotate_left(13) ^ lane as u8 as u64)
        }
        104 => {
            let x = core::simd::Simd::from_array([seed as i16, i16::MIN, i16::MAX, -1]);
            (-x).to_array()
                .into_iter()
                .fold(0u64, |acc, lane| acc.rotate_left(13) ^ lane as u16 as u64)
        }
        105 => {
            let x = core::simd::Simd::from_array([seed as i32, i32::MIN, i32::MAX, -1]);
            (-x).to_array()
                .into_iter()
                .fold(0u64, |acc, lane| acc.rotate_left(13) ^ lane as u32 as u64)
        }
        106 => {
            let x = core::simd::Simd::from_array([seed as i64, i64::MIN, i64::MAX, -1]);
            (-x).to_array()
                .into_iter()
                .fold(0u64, |acc, lane| acc.rotate_left(13) ^ lane as u64)
        }
        107..=110 => coroutine_case(case - 107, seed),
        111 => bits32(core::intrinsics::fmaf32(
            f32::from_bits(float32_bits(seed)),
            f32::from_bits(float32_bits(seed.wrapping_add(1) % 20)),
            rounding_value(seed) as f32,
        )),
        112 => bits32(core::intrinsics::fmaf32(
            f32::from_bits(seed as u32),
            f32::from_bits(seed.rotate_left(23) as u32),
            f32::from_bits(seed.rotate_right(11) as u32),
        )),
        113 => {
            let x = f32::from_bits(0x3f80_0001 | ((seed as u32 & 1) << 1));
            let y = f32::from_bits(0x3fc0_0000 | ((seed as u32 & 2) << 30));
            let z = f32::from_bits(1 | ((seed as u32 & 4) << 29));
            bits32(core::intrinsics::fmaf32(x, y, z))
        }
        114 => {
            let x = f32::from_bits(match seed & 3 {
                0 => 1,
                1 => 0x007f_ffff,
                2 => 0x0080_0000,
                _ => 0x0080_0001,
            });
            let z = f32::from_bits(1 | ((seed as u32 & 4) << 29));
            bits32(core::intrinsics::fmaf32(x, 0.5, z))
        }
        115 => {
            let x = f32::from_bits(0x7f7f_ffff | ((seed as u32 & 1) << 31));
            let y = if seed & 2 == 0 {
                2.0
            } else {
                f32::from_bits(0x3f80_0001)
            };
            bits32(core::intrinsics::fmaf32(x, y, -x))
        }
        _ => {
            let selected = if seed & 1 == 0 {
                core::any::TypeId::of::<u64>()
            } else {
                core::any::TypeId::of::<i64>()
            };
            u64::from(selected == core::any::TypeId::of::<u64>())
                | (u64::from(selected == core::any::TypeId::of::<i64>()) << 1)
                | (u64::from(selected == core::any::TypeId::of::<[u64; 2]>()) << 2)
        }
    }
}
