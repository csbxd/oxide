#![no_std]
#![feature(fn_traits, unboxed_closures)]

pub fn shared() -> u32 {
    11
}
pub fn identity<T>(value: T) -> T {
    value
}
pub fn r#type() -> u32 {
    67
}

mod hidden {
    pub fn exposed() -> u32 {
        23
    }
    pub fn not_exported() -> u32 {
        31
    }

    #[derive(Clone, Copy, Default, PartialEq, Eq)]
    pub struct Point {
        pub x: u32,
    }
    impl Point {
        pub fn new(x: u32) -> Self {
            Self { x }
        }
        pub fn value(&self) -> u32 {
            self.x
        }
        pub fn generic<T>(&self, value: T) -> T {
            value
        }
        fn private(&self) -> u32 {
            self.x
        }
    }

    pub struct Unreachable;
    impl Unreachable {
        pub fn method() -> u32 {
            37
        }
    }
    pub trait PrivateTrait {
        fn helper(&self) -> u32;
    }
    impl PrivateTrait for Point {
        fn helper(&self) -> u32 {
            self.x
        }
    }
}
pub use hidden::{Point, exposed as renamed};
pub type PointAlias = Point;

pub trait Measure {
    fn measure(&self) -> u32;
    fn default_method(&self) -> u32 {
        self.measure()
    }
    fn generic_method<T>(&self, value: T) -> T {
        value
    }
}
impl Measure for Point {
    fn measure(&self) -> u32 {
        self.x
    }
}
pub trait Pair<A, B> {
    fn pair(&self) -> u32;
}
impl Pair<u8, u16> for Point {
    fn pair(&self) -> u32 {
        self.x
    }
}

pub trait Conditional {
    fn sized_only(&self) -> u32
    where
        Self: Clone,
    {
        5
    }
}
pub struct NotClone;
impl Conditional for NotClone {}
impl Drop for NotClone {
    fn drop(&mut self) {}
}

pub struct Generic<T>(pub T);
impl Generic<u8> {
    pub fn specialized() -> u32 {
        8
    }
}
impl Generic<u16> {
    pub fn specialized() -> u32 {
        16
    }
}
impl<T> Generic<T> {
    pub fn borrowed(&self) -> &T {
        &self.0
    }
}
pub type Byte = Generic<u8>;
pub struct Tuple(pub u32, pub u16);
pub struct PrivateTuple(u32);
pub type TupleAlias = Tuple;
pub enum Choice {
    Unit,
    Named { x: u32 },
    Tuple(u32),
    Empty(),
}
pub type ChoiceAlias = Choice;

pub struct DropParameter;
impl Drop for DropParameter {
    fn drop(&mut self) {}
}
pub fn consume_drop(value: DropParameter) {
    drop(value);
}
pub struct BorrowOnly;
impl Drop for BorrowOnly {
    fn drop(&mut self) {}
}
pub fn borrow_only(value: &BorrowOnly) -> *const BorrowOnly {
    value
}
pub fn raw_only(value: *mut BorrowOnly) -> *mut BorrowOnly {
    value
}
pub struct CallOnce;
impl FnOnce<(DropParameter, u32)> for CallOnce {
    type Output = u32;
    extern "rust-call" fn call_once(self, args: (DropParameter, u32)) -> u32 {
        args.1
    }
}

pub mod left {
    pub fn same() -> u32 {
        41
    }
}
pub mod right {
    pub fn same() -> u32 {
        43
    }
}

macro_rules! make_api {
    () => {
        pub fn expanded() -> u32 {
            47
        }
    };
}
make_api!();
