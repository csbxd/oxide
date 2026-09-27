//! Native allocation-owner control for the translated renderer chaos test.
use std::alloc::{GlobalAlloc, Layout, System};
use std::path::PathBuf;
use std::sync::atomic::{AtomicIsize, Ordering::Relaxed};

struct Counted;
static LIVE: AtomicIsize = AtomicIsize::new(0);

unsafe impl GlobalAlloc for Counted {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        let p = unsafe { System.alloc(layout) };
        if !p.is_null() {
            LIVE.fetch_add(1, Relaxed);
        }
        p
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        let p = unsafe { System.alloc_zeroed(layout) };
        if !p.is_null() {
            LIVE.fetch_add(1, Relaxed);
        }
        p
    }
    unsafe fn dealloc(&self, p: *mut u8, layout: Layout) {
        unsafe { System.dealloc(p, layout) };
        LIVE.fetch_sub(1, Relaxed);
    }
    unsafe fn realloc(&self, p: *mut u8, layout: Layout, size: usize) -> *mut u8 {
        unsafe { System.realloc(p, layout, size) }
    }
}

#[global_allocator]
static ALLOCATOR: Counted = Counted;

fn main() {
    let mut args = std::env::args_os().skip(1);
    let cases = PathBuf::from(args.next().expect("cases directory"));
    let references = PathBuf::from(args.next().expect("references directory"));
    let output = PathBuf::from(args.next().expect("output PNG"));
    assert!(args.next().is_none());
    let path = output.to_str().unwrap();
    let samples: Vec<_> = [
        "flowchart",
        "sequence",
        "class",
        "upstream/architecture/basic",
        "upstream/pie/basic",
        "upstream/mindmap/basic",
    ]
    .into_iter()
    .map(|name| {
        let input = std::fs::read(cases.join(format!("{name}.mmd"))).unwrap();
        let svg = std::fs::read(references.join(format!("{name}.svg"))).unwrap();
        let png = std::fs::read(references.join(format!("{name}.png"))).unwrap();
        (name, input, svg, png)
    })
    .collect();
    let mut buffer = vec![0; samples.iter().map(|x| x.2.len()).max().unwrap()];
    let new_threads = std::env::var_os("OXIDE_CHAOS_NEW_THREADS").is_some();
    println!("round,format,case,live_before,live_after,delta");
    for round in 0..4 {
        for (name, input, svg, png) in &samples {
            let before = LIVE.load(Relaxed);
            let mut render = || unsafe {
                oxide_renderer_fixture::render_svg(
                    input.as_ptr(),
                    input.len(),
                    buffer.as_mut_ptr(),
                    buffer.len(),
                )
            };
            let n = if new_threads {
                std::thread::scope(|scope| scope.spawn(render).join().unwrap())
            } else {
                render()
            };
            let after = LIVE.load(Relaxed);
            assert_eq!(&buffer[..n], svg);
            println!("{round},svg,{name},{before},{after},{}", after - before);
            let before = LIVE.load(Relaxed);
            let render = || unsafe {
                oxide_renderer_fixture::write_png(
                    input.as_ptr(),
                    input.len(),
                    path.as_ptr(),
                    path.len(),
                )
            };
            if new_threads {
                std::thread::scope(|scope| scope.spawn(render).join().unwrap());
            } else {
                render();
            }
            let after = LIVE.load(Relaxed);
            assert_eq!(&std::fs::read(&output).unwrap(), png);
            println!("{round},png,{name},{before},{after},{}", after - before);
        }
    }
}
