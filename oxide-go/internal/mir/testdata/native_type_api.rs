#![feature(ptr_metadata, layout_for_ptr)]

#[path = "../../../../oxide-rs/tests/type_api.rs"]
mod api;
use api::*;
use std::ffi::OsString;
use std::os::unix::ffi::OsStringExt;

fn main() {
    println!("{:?}", make().unwrap());
    let mut document = Document::default();
    document.title = "直接调用 雪".into();
    document.nodes = vec![Node { id: 7, label: Some("甲".into()) }, Node { id: 9, label: None }];
    document.outcome = Err(vec![0, 127, 255]);
    println!("{document:?}");
    println!("{:?}", Event::Text("事件".into()));
    println!("{:?}", Event::Record { name: "点".into(), points: vec![] });
    println!("{:?}", Event::Empty);
    let path = std::path::PathBuf::from(OsString::from_vec(vec![b'a', 255, b'z']));
    println!("{:?}", owned_path(path));
    println!("{:?}", Aligned { values: vec![(); 17] });
    println!("{:?}", PrivateOwner::default());
    let mut maps = make_maps();
    for node in maps.hash.values_mut() { node.id += 10; }
    for node in maps.tree.values_mut() { node.id += 10; }
    println!("{:?}", maps.hash["hash"]);
    println!("{:?}", maps.tree["tree"]);
    for length in [0usize, 3] {
        // Ask the native compiler for this private-tail DST's layout. The
        // prototype supplies metadata only; no reference to dangling data is made.
        let prototype = std::ptr::from_raw_parts_mut::<PrivateTail>(
            std::ptr::NonNull::<u16>::dangling().as_ptr().cast::<()>(), length);
        unsafe {
            let layout = std::alloc::Layout::for_value_raw(prototype);
            let data = std::alloc::alloc_zeroed(layout);
            assert!(!data.is_null());
            let value = std::ptr::from_raw_parts_mut::<PrivateTail>(data.cast::<()>(), length);
            (*value).head = 19;
            let borrowed = dst_ref(&*value);
            let raw = dst_raw(value);
            assert!(std::ptr::addr_eq(borrowed, value));
            assert!(std::ptr::addr_eq(raw, value));
            assert_eq!(std::ptr::metadata(borrowed), length);
            assert_eq!(std::ptr::metadata(raw), length);
            println!("dst {length} {} {} {}", std::mem::size_of_val(borrowed), std::mem::align_of_val(borrowed), borrowed.head);
            std::alloc::dealloc(data, layout);
        }
    }
    for number in [0, 37] {
        let mut value = make_debug(number);
        let original = &*value as *const dyn std::fmt::Debug;
        let borrowed = dyn_ref(&*value);
        assert!(std::ptr::eq(borrowed, original));
        assert_eq!(std::mem::size_of_val(borrowed), std::mem::size_of::<DebugValue>());
        assert_eq!(std::mem::align_of_val(borrowed), std::mem::align_of::<DebugValue>());
        let mutable = dyn_mut(&mut *value);
        assert!(std::ptr::eq(mutable, original));
        println!("{mutable:?}");
    }
}
