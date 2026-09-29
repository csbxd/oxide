#![feature(test)]
#![no_std]

extern crate std;
extern crate test as __oxide_test;

use oxide_upstream_macros::case;

#[case]
fn success() {
    assert_eq!(2 + 2, 4);
}

#[case]
#[should_panic(expected = "needle")]
fn expected_message() {
    panic!("haystack needle haystack");
}

#[case]
#[should_panic(expected = r#"needle"#)]
#[ignore = "deliberately failing input for the adapter"]
fn wrong_message() {
    panic!("different message");
}

#[case]
#[should_panic = "needle"]
fn name_value_message() {
    panic!("haystack needle haystack");
}

#[case]
#[should_panic = r#"needle"#]
#[ignore = "deliberately failing input for the adapter"]
fn name_value_wrong_message() {
    panic!("different message");
}

#[case]
#[ignore = "deliberately failing input for the adapter"]
fn result_error() -> Result<(), &'static str> {
    Err("failed result")
}

#[case]
fn result_success() -> Result<(), &'static str> {
    Ok(())
}

#[case]
#[ignore = "deliberately failing input for the adapter"]
fn unexpected_panic() {
    panic!("unexpected");
}

#[case]
#[should_panic]
fn any_payload() {
    std::panic::panic_any(42u8);
}

#[case]
#[should_panic]
#[ignore = "deliberately failing input for the adapter"]
fn missing_panic() {}

#[case]
#[should_panic(expected = "needle")]
fn owned_message() {
    std::panic::panic_any(std::string::String::from("owned needle payload"));
}

#[case]
#[cfg_attr(all(), should_panic(expected = "needle"))]
fn conditional_attribute() {
    panic!("needle");
}

#[case]
#[should_panic(expected = "needle")]
#[ignore = "deliberately failing input for the adapter"]
fn non_string_message() {
    std::panic::panic_any(42u8);
}

#[case]
#[cfg(any())]
fn disabled() {
    intentionally_undefined();
}

#[case]
#[cfg_attr(all(), cfg(any()))]
fn conditionally_disabled() {
    intentionally_undefined();
}

macro_rules! forwarded_cases {
    ($name:ident, [$($attribute:meta),* $(,)?], $body:block) => {
        #[case]
        $(#[$attribute])*
        fn $name() $body
    };
}

macro_rules! relay_attribute {
    ($name:ident, $attribute:meta) => {
        forwarded_cases!($name, [$attribute], { panic!("needle") });
    };
}

macro_rules! nested_attribute {
    ($name:ident, $attribute:meta) => {
        relay_attribute!($name, $attribute);
    };
}

macro_rules! forwarded_literal {
    ($name:ident, $message:literal, [$($attribute:meta),*]) => {
        forwarded_cases!($name, [should_panic(expected = $message), $($attribute),*], {
            panic!("needle");
        });
    };
}

macro_rules! forwarded_legacy_literal {
    ($name:ident, $message:literal, [$($attribute:meta),*]) => {
        forwarded_cases!($name, [should_panic = $message, $($attribute),*], {
            panic!("needle");
        });
    };
}

// The upstream slice_index generators use `$expect_msg:expr` even though the
// value is a string literal. This creates an opaque group inside the options,
// without wrapping the whole should_panic attribute in a meta fragment.
macro_rules! forwarded_expression {
    ($name:ident, $message:expr, [$($attribute:meta),*], $body:block) => {
        #[case]
        #[should_panic(expected = $message)]
        $(#[$attribute])*
        fn $name() $body
    };
}

forwarded_cases!(forwarded_bare, [should_panic], { panic!("needle") });
forwarded_cases!(
    forwarded_missing,
    [should_panic, ignore = "adapter failure input"],
    {}
);
forwarded_cases!(forwarded_expected, [should_panic(expected = "needle")], {
    panic!("needle")
});
forwarded_cases!(
    forwarded_wrong_message,
    [
        should_panic(expected = "needle"),
        ignore = "adapter failure input"
    ],
    { panic!("other") }
);
forwarded_cases!(
    forwarded_expected_missing,
    [
        should_panic(expected = "needle"),
        ignore = "adapter failure input"
    ],
    {}
);
forwarded_cases!(forwarded_legacy, [should_panic = "needle"], {
    panic!("needle")
});
forwarded_cases!(
    forwarded_legacy_wrong,
    [should_panic = "needle", ignore = "adapter failure input"],
    { panic!("other") }
);
forwarded_cases!(
    forwarded_ignored,
    [ignore = "upstream ignore remains intact"],
    { panic!("unexpected") }
);
forwarded_cases!(forwarded_disabled, [cfg(any())], {
    intentionally_undefined();
});
forwarded_cases!(forwarded_cfg_disabled, [cfg_attr(all(), cfg(any()))], {
    intentionally_undefined();
});
forwarded_cases!(
    forwarded_cfg_panic,
    [cfg_attr(all(), should_panic(expected = "needle"))],
    { panic!("needle") }
);
nested_attribute!(forwarded_nested, should_panic(expected = "needle"));
forwarded_literal!(forwarded_literal_match, "needle", []);
forwarded_literal!(
    forwarded_literal_wrong,
    "other",
    [ignore = "adapter failure input"]
);
forwarded_legacy_literal!(forwarded_legacy_literal_match, r#"needle"#, []);
forwarded_legacy_literal!(
    forwarded_legacy_literal_wrong,
    "other",
    [ignore = "adapter failure input"]
);
forwarded_expression!(forwarded_expression_match, "needle", [], {
    panic!("needle");
});
forwarded_expression!(
    forwarded_expression_wrong,
    "needle",
    [ignore = "adapter failure input"],
    {
        panic!("other");
    }
);
forwarded_expression!(
    forwarded_expression_missing,
    "needle",
    [ignore = "adapter failure input"],
    {}
);

#[test]
fn adapters_preserve_libtest_semantics() {
    assert_eq!(__oxide_upstream_success(), 0);
    assert_eq!(__oxide_upstream_expected_message(), 0);
    assert_eq!(__oxide_upstream_wrong_message(), 4);
    assert_eq!(__oxide_upstream_name_value_message(), 0);
    assert_eq!(__oxide_upstream_name_value_wrong_message(), 4);
    assert_eq!(__oxide_upstream_result_error(), 1);
    assert_eq!(__oxide_upstream_result_success(), 0);
    assert_eq!(__oxide_upstream_unexpected_panic(), 2);
    assert_eq!(__oxide_upstream_any_payload(), 0);
    assert_eq!(__oxide_upstream_missing_panic(), 3);
    assert_eq!(__oxide_upstream_owned_message(), 0);
    assert_eq!(__oxide_upstream_conditional_attribute(), 0);
    assert_eq!(__oxide_upstream_non_string_message(), 4);
}

#[test]
fn forwarded_attributes_preserve_libtest_semantics() {
    assert_eq!(__oxide_upstream_forwarded_bare(), 0);
    assert_eq!(__oxide_upstream_forwarded_missing(), 3);
    assert_eq!(__oxide_upstream_forwarded_expected(), 0);
    assert_eq!(__oxide_upstream_forwarded_wrong_message(), 4);
    assert_eq!(__oxide_upstream_forwarded_expected_missing(), 3);
    assert_eq!(__oxide_upstream_forwarded_legacy(), 0);
    assert_eq!(__oxide_upstream_forwarded_legacy_wrong(), 4);
    assert_eq!(__oxide_upstream_forwarded_ignored(), 2);
    assert_eq!(__oxide_upstream_forwarded_cfg_panic(), 0);
    assert_eq!(__oxide_upstream_forwarded_nested(), 0);
    assert_eq!(__oxide_upstream_forwarded_literal_match(), 0);
    assert_eq!(__oxide_upstream_forwarded_literal_wrong(), 4);
    assert_eq!(__oxide_upstream_forwarded_legacy_literal_match(), 0);
    assert_eq!(__oxide_upstream_forwarded_legacy_literal_wrong(), 4);
    assert_eq!(__oxide_upstream_forwarded_expression_match(), 0);
    assert_eq!(__oxide_upstream_forwarded_expression_wrong(), 4);
    assert_eq!(__oxide_upstream_forwarded_expression_missing(), 3);
}
