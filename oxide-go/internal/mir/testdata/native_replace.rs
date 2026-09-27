mod replace;
use replace::*;
use std::mem::ManuallyDrop;
use std::panic::{AssertUnwindSafe, catch_unwind};

fn main() {
    setup();
    if std::env::args().nth(1).as_deref() == Some("double") {
        let _ = catch_unwind(|| {
            let mut destination = make_field_panic();
            destination = make_owner(2, false);
            std::hint::black_box(&destination);
        });
        panic!("double panic unexpectedly returned");
    }
    reset();
    let mut owners = ManuallyDrop::new([make_zero(), make_zero()]);
    let left = owners.as_mut_ptr();
    let right = unsafe { left.add(1) };
    assert_eq!(left, right);
    unsafe { *left = core::ptr::read(right); }
    let after = state();
    unsafe { core::ptr::drop_in_place(left); }
    println!("0 {after} 0 {} 0", state());
    for case in 1..5 {
        reset();
        let panic = case == 2 || case == 4;
        let mut destination = make_owner(1, panic);
        let mut source = ManuallyDrop::new(make_owner(2, false));
        let source_ptr = &mut *source as *mut Owner;
        if case >= 3 { source_slot(source_ptr); }
        let caught = catch_unwind(AssertUnwindSafe(|| {
            // Rust evaluates the moved RHS before dropping the old LHS, and
            // writes the new value on the old destructor's unwind path too.
            destination = unsafe { core::ptr::read(source_ptr) };
        }));
        let matches = match caught {
            Err(payload) => payload.downcast::<u64>().is_ok_and(|id| *id == 1),
            Ok(()) => false,
        };
        assert_eq!(matches, panic);
        source_slot(core::ptr::null_mut());
        let after = state();
        let value = inspect(&destination);
        drop(destination);
        println!("{case} {after} {value} {} {}", state(), u64::from(matches));
    }
}
