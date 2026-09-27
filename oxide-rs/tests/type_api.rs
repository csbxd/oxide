#![allow(dead_code, unexpected_cfgs)]

use std::collections::{BTreeMap, HashMap};
use std::ffi::{OsStr, OsString};
use std::path::{Path, PathBuf};

mod hidden {
    #[derive(Debug, Default)]
    pub struct Secret {
        pub text: String,
    }
}

#[derive(Debug, Default)]
pub struct Node {
    pub id: u32,
    pub label: Option<String>,
}

#[derive(Debug, Default)]
pub struct MapOwner {
    pub hash: HashMap<String, Node>,
    pub tree: BTreeMap<String, Node>,
}
pub fn make_maps() -> MapOwner {
    let mut maps = MapOwner::default();
    maps.hash.insert("hash".into(), Node { id: 3, label: None });
    maps.tree.insert("tree".into(), Node { id: 5, label: None });
    maps
}
pub fn next_hash(value: &HashMap<String, Node>) -> Option<(&String, &Node)> {
    value.iter().next()
}
pub fn next_hash_mut(value: &mut HashMap<String, Node>) -> Option<(&String, &mut Node)> {
    value.iter_mut().next()
}
pub fn next_tree(value: &BTreeMap<String, Node>) -> Option<(&String, &Node)> {
    value.iter().next()
}
pub fn next_tree_mut(value: &mut BTreeMap<String, Node>) -> Option<(&String, &mut Node)> {
    value.iter_mut().next()
}

#[derive(Debug)]
pub struct Document {
    pub title: String,
    pub nodes: Vec<Node>,
    pub outcome: Result<String, Vec<u8>>,
    private: u64,
}
impl Default for Document {
    fn default() -> Self {
        Self {
            title: String::new(),
            nodes: Vec::new(),
            outcome: Ok(String::new()),
            private: 0,
        }
    }
}

// A public field is operable; a private field does not reveal its own public fields.
#[derive(Debug, Default)]
pub struct PrivateOwner {
    secret: hidden::Secret,
    pub value: u64,
}

#[repr(C, packed)]
pub struct Packed {
    pub byte: u8,
    pub wide: u64,
}
pub struct PrivateTail {
    pub head: u8,
    tail: [u16],
}

#[repr(align(64))]
#[derive(Debug)]
pub struct DebugValue {
    pub number: u64,
    pub text: String,
}

pub fn make_debug(number: u64) -> Box<dyn std::fmt::Debug> {
    Box::new(DebugValue { number, text: format!("动态 {number}") })
}

#[repr(align(64))]
#[derive(Debug, Default)]
pub struct Aligned {
    pub values: Vec<()>,
}

#[derive(Debug)]
pub enum Event {
    Empty,
    Text(String),
    Record { name: String, points: Vec<Node> },
}

pub type Names = Vec<String>;
pub type Outcome = Result<Document, String>;
pub type Bytes = [u8];
pub type Word = u64;

pub fn make() -> Outcome {
    Ok(Document {
        title: "雪".into(),
        nodes: Vec::new(),
        outcome: Ok("yes".into()),
        private: 9,
    })
}
pub fn text(value: &str) -> usize {
    value.len()
}
pub fn nodes(value: &[Node]) -> usize {
    value.len()
}
pub fn mutable(value: &mut [u8]) -> usize {
    value.len()
}
pub fn raw(value: *const [Node]) -> *const [Node] {
    value
}
pub fn path(value: &Path) -> &OsStr {
    value.as_os_str()
}
pub fn owned_path(value: PathBuf) -> OsString {
    value.into_os_string()
}
pub fn boxed(value: Box<Node>) -> Box<Node> {
    value
}
pub fn dynamic(value: Box<dyn std::fmt::Debug>) -> Box<dyn std::fmt::Debug> {
    value
}
pub fn dyn_ref(value: &dyn std::fmt::Debug) -> &dyn std::fmt::Debug {
    value
}
pub fn dyn_mut(value: &mut dyn std::fmt::Debug) -> &mut dyn std::fmt::Debug {
    value
}
pub fn dst_ref(value: &PrivateTail) -> &PrivateTail {
    value
}
pub fn dst_raw(value: *mut PrivateTail) -> *mut PrivateTail {
    value
}
pub fn debug_event(value: &Event) -> String {
    format!("{:?}", *value)
}

pub mod spoof {
    pub struct String(pub u64);
    pub struct Vec(pub u64);
    pub struct Path(pub [u8]);
}

#[cfg(oxide_native)]
fn main() {
    // Offsets supplied by the exporter are checked against real initialized Rust
    // storage; this test does not assume Vec/String's field order.
    let offsets: Vec<usize> = std::env::args()
        .skip(1)
        .map(|value| value.parse().unwrap())
        .collect();
    assert_eq!(offsets.len(), 7);
    unsafe fn read<T>(value: &T, offset: usize) -> usize {
        unsafe {
            (value as *const T as *const u8)
                .add(offset)
                .cast::<usize>()
                .read_unaligned()
        }
    }
    let mut string = String::with_capacity(29);
    string.push_str("oxide雪");
    let mut vector: Vec<Node> = Vec::with_capacity(11);
    vector.extend([
        Node { id: 3, label: None },
        Node { id: 5, label: None },
        Node { id: 7, label: None },
    ]);
    unsafe {
        assert_eq!(read(&string, offsets[0]), string.as_ptr() as usize);
        assert_eq!(read(&string, offsets[1]), string.len());
        assert_eq!(read(&string, offsets[2]), string.capacity());
        assert_eq!(read(&vector, offsets[3]), vector.as_ptr() as usize);
        assert_eq!(read(&vector, offsets[4]), vector.len());
        assert_eq!(read(&vector, offsets[5]), vector.capacity());
    }
    let event = Event::Text("oxide雪".into());
    fn check_template(args: std::fmt::Arguments<'_>, offset: usize) {
        unsafe {
            let template = read(&args, offset) as *const u8;
            assert_eq!(std::slice::from_raw_parts(template, 2), &[192, 0]);
        }
    }
    check_template(format_args!("{:?}", event), offsets[6]);
    println!(
        "{} {} {} {} {} {}",
        std::mem::size_of::<Document>(),
        std::mem::align_of::<Document>(),
        std::mem::offset_of!(Document, title),
        std::mem::offset_of!(Document, nodes),
        std::mem::offset_of!(Document, outcome),
        std::mem::offset_of!(Document, private)
    );
}
