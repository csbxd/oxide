#![allow(dead_code)]

#[inline(never)]
fn vector(seed: u64) -> u64 {
    let mut values = Vec::with_capacity(2);
    for index in 0..8 {
        values.push(seed.wrapping_add(index));
    }
    values.reverse();
    values[0].wrapping_add(values[7])
}

#[inline(never)]
fn boxed(seed: u64) -> u64 {
    let value = Box::new(seed);
    (*value).wrapping_add(17)
}

#[inline(never)]
fn string(seed: u64) -> u64 {
    let mut text = String::from("oxide");
    text.push_str(" rust");
    seed.wrapping_add(text.len() as u64)
}

pub fn conformance(case: u64, seed: u64) -> u64 {
    match case {
        0 => vector(seed),
        1 => boxed(seed),
        _ => string(seed),
    }
}
