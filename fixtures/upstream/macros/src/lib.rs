//! Adapt upstream Rust tests without changing their bodies or oxide itself.
//!
//! The ordinary `#[test]` item remains available to libtest. The additional
//! private function is an oxide export root, so each test can be translated
//! independently without making rustc's entire test descriptor table reachable.

use proc_macro::{Delimiter, TokenStream, TokenTree};

/// Keep a native libtest test and add `__oxide_upstream_<name>() -> u8`.
///
/// Status codes: 0 passes, 1 is a failed `Termination` result, 2 is an unexpected
/// panic, 3 is a missing expected panic, and 4 is a mismatched panic message.
#[proc_macro_attribute]
pub fn case(arguments: TokenStream, item: TokenStream) -> TokenStream {
    if !arguments.is_empty() {
        return error("oxide upstream case does not accept arguments");
    }
    let tokens = flatten_forwarded(item.clone());
    let Some(name) = tokens.windows(2).find_map(|pair| match pair {
        [TokenTree::Ident(keyword), TokenTree::Ident(name)] if keyword.to_string() == "fn" => {
            Some(name.to_string())
        }
        _ => None,
    }) else {
        return error("oxide upstream case requires a test function");
    };

    let mut configuration = TokenStream::new();
    let mut should_panic = false;
    let mut expected = None;
    for pair in tokens.windows(2) {
        let [TokenTree::Punct(hash), TokenTree::Group(attribute)] = pair else {
            continue;
        };
        if hash.as_char() != '#' || attribute.delimiter() != Delimiter::Bracket {
            continue;
        }
        let values = flatten_forwarded(attribute.stream());
        let Some(TokenTree::Ident(kind)) = values.first() else {
            continue;
        };
        match kind.to_string().as_str() {
            // rustc usually evaluates these before invoking us. Preserve any
            // remaining configuration on the helper as well as on the test.
            "cfg" | "cfg_attr" => configuration.extend(pair.iter().cloned()),
            "should_panic" => {
                should_panic = true;
                if let [_, TokenTree::Punct(eq), TokenTree::Literal(value)] = values.as_slice()
                    && eq.as_char() == '='
                {
                    // libtest also accepts the legacy name-value spelling.
                    expected = Some(value.to_string());
                } else if let Some(TokenTree::Group(options)) = values.get(1) {
                    let options = flatten_forwarded(options.stream());
                    for option in options.windows(3) {
                        if let [
                            TokenTree::Ident(key),
                            TokenTree::Punct(eq),
                            TokenTree::Literal(value),
                        ] = option
                            && key.to_string() == "expected"
                            && eq.as_char() == '='
                        {
                            expected = Some(value.to_string());
                        }
                    }
                }
            }
            _ => {}
        }
    }

    let outcome = match (should_panic, expected) {
        (false, _) => "match outcome { Ok(Ok(())) => 0, Ok(Err(_)) => 1, Err(_) => 2 }".into(),
        (true, None) => "match outcome { Err(_) => 0, Ok(_) => 3 }".into(),
        (true, Some(expected)) => format!(
            "match outcome {{
                Err(payload) => {{
                    let message = payload.downcast_ref::<::std::string::String>()
                        .map(|value| value.as_str())
                        .or_else(|| payload.downcast_ref::<&str>().copied());
                    if message.is_some_and(|value| value.contains({expected})) {{ 0 }} else {{ 4 }}
                }}
                Ok(_) => 3,
            }}"
        ),
    };
    let helper = format!(
        "{configuration}
        #[allow(dead_code, non_snake_case)]
        fn __oxide_upstream_{}() -> u8 {{
            let outcome = ::std::panic::catch_unwind(::std::panic::AssertUnwindSafe(|| {{
                crate::__oxide_test::assert_test_result({name}())
            }}));
            {outcome}
        }}",
        name.strip_prefix("r#").unwrap_or(&name),
    );
    let mut result: TokenStream = "#[test]".parse().unwrap();
    result.extend(item);
    result.extend(helper.parse::<TokenStream>().unwrap());
    result
}

fn error(message: &str) -> TokenStream {
    format!("compile_error!({message:?});").parse().unwrap()
}

// macro_rules! forwards opaque fragments such as `$attribute:meta` inside
// delimiter-less groups. They are transparent to rustc's attribute parser.
// Flatten only our inspection view: returning the original item preserves its
// tokens, hygiene, attributes and body exactly as supplied by the upstream test.
fn flatten_forwarded(stream: TokenStream) -> Vec<TokenTree> {
    let mut tokens = Vec::new();
    for token in stream {
        match token {
            TokenTree::Group(group) if group.delimiter() == Delimiter::None => {
                tokens.extend(flatten_forwarded(group.stream()));
            }
            token => tokens.push(token),
        }
    }
    tokens
}
