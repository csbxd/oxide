#![no_std]
pub use dependency::Tuple as RenamedTuple;
pub use dependency::left as renamed_module;
pub use dependency::*;
pub use dependency::{Point as RenamedPoint, shared as alias};
pub mod nested {
    pub use dependency::{Point, renamed};
}

mod private {
    pub fn not_exported() -> u32 {
        51
    }
}
pub(crate) fn crate_only() -> u32 {
    53
}
fn selected_private() -> u32 {
    59
}

pub mod cycle {
    pub use crate::cycle as again;
    pub fn leaf() -> u32 {
        61
    }
}
