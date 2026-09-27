#![no_std]
pub mod sys {
    pub mod args {
        pub mod unix {
            pub mod imp {
                pub fn argc_argv() -> (isize, *const *const u8) {
                    (7, core::ptr::null())
                }
            }
        }
    }
}
