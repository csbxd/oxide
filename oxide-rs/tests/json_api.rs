#![allow(dead_code, unexpected_cfgs)]

use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
pub struct Packet {
    pub title: String,
    pub count: u128,
    pub values: Vec<i64>,
    #[cfg(feature = "json")]
    pub extra: serde_json::Value,
}

#[derive(Serialize)]
pub struct EncodeOnly { pub value: u64 }

// Direct serialization preserves declaration order and f32 precision. Going
// through serde_json::Value sorts object keys and stores the number as f64.
#[derive(Serialize)]
pub struct FloatOrder { pub z: f32, pub a: u8 }

#[derive(Deserialize)]
pub struct DecodeOnly { pub value: u64 }

pub struct Neither { pub value: u64 }
pub type Borrowed = &'static str;

// No serde_json call exists in this library configuration. Compiler-resolved
// operations must work without a project helper instantiating the generics.
pub fn inspect(packet: &Packet) -> usize { packet.title.len() + packet.values.len() }

#[cfg(not(feature = "json"))]
pub mod serde_json {
    pub struct Error;
    pub fn to_string<T>(_: &T) -> String { "unrelated module".into() }
    pub fn from_str<T>(_: &str) -> Result<T, Error> { Err(Error) }
    pub fn to_value<T>(_: T) -> Result<(), Error> { Err(Error) }
}

#[cfg(oxide_native)]
fn main() {
    let source = r#"{"title":"oxide 雪","count":1267650600228229401496703205379,"values":[-7,0,42],"extra":{"enabled":true,"items":[null,1,"x"]}}"#;
    let value: Packet = serde_json::from_str(source).unwrap();
    let output = serde_json::to_string(&value).unwrap();
    let again: Packet = serde_json::from_str(&output).unwrap();
    assert_eq!(again.title, "oxide 雪");
    assert_eq!(again.count, (1u128 << 100) + 3);
    assert_eq!(again.values, [-7, 0, 42]);
    println!("{output}");
    println!("{}", serde_json::to_string(&EncodeOnly { value: 91 }).unwrap());
    let decoded: DecodeOnly = serde_json::from_str(r#"{"value":17}"#).unwrap();
    println!("{}", decoded.value);
    let borrowed: &str = serde_json::from_str(r#""borrowed""#).unwrap();
    println!("{borrowed}");
    println!("{}", serde_json::from_str::<Packet>(r#"{"title":7}"#).err().unwrap());
    let floating = FloatOrder { z: 0.1, a: 7 };
    let direct = serde_json::to_string(&floating).unwrap();
    let through_value = serde_json::to_string(&serde_json::to_value(&floating).unwrap()).unwrap();
    assert_ne!(direct, through_value);
    println!("{direct}|{through_value}");
}
