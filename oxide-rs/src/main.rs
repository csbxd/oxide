#![feature(rustc_private)]

extern crate rustc_abi;
extern crate rustc_driver;
extern crate rustc_hir;
extern crate rustc_interface;
extern crate rustc_middle;
extern crate rustc_public;
extern crate serde_json;

mod export;
mod roots;

use std::env;
use std::path::PathBuf;

fn main() {
    let mut args: Vec<String> = env::args().collect();
    let sysroot = env!("OXIDE_SYSROOT");
    if args.get(1).map(String::as_str) == Some("--print-sysroot") {
        println!("{sysroot}");
        return;
    }
    // Cargo's RUSTC_WRAPPER protocol supplies the real compiler as argv[1].
    let wrapper = args.get(1).is_some_and(|s| s.ends_with("rustc"));
    let output = if wrapper {
        args.remove(1);
        // Cargo fingerprints these final-crate arguments. A fresh destination
        // forces export even when Cargo would otherwise reuse a compiled crate.
        let mut explicit_output = None;
        args.retain(|arg| {
            if let Some(path) = arg.strip_prefix("--oxide-export=") {
                explicit_output = Some(PathBuf::from(path));
                false
            } else {
                true
            }
        });
        explicit_output.or_else(|| {
            env::var_os("OXIDE_EXPORT").map(PathBuf::from).filter(|p| {
                !p.as_os_str().is_empty() && env::var_os("CARGO_PRIMARY_PACKAGE").is_some()
            })
        })
    } else if args.get(1).map(String::as_str) == Some("--export") {
        args.remove(1);
        let path = PathBuf::from(args.remove(1));
        if args.get(1).map(String::as_str) == Some("--") {
            args.remove(1);
        }
        Some(path)
    } else {
        eprintln!("usage: oxide-rs --export FILE -- RUSTC_ARGS...");
        std::process::exit(2);
    };
    if !args
        .iter()
        .any(|a| a == "--sysroot" || a.starts_with("--sysroot="))
    {
        args.extend(["--sysroot".into(), sysroot.into()]);
    }
    let Some(output) = output else {
        let status = std::process::Command::new(format!("{sysroot}/bin/rustc"))
            .args(&args[1..])
            .status()
            .expect("run pinned rustc");
        std::process::exit(status.code().unwrap_or(1));
    };
    force_panic_runtime(&mut args);
    let result = rustc_public::run_with_tcx!(&args, |tcx| -> ControlFlow<String> {
        match export::program(tcx, &output) {
            Ok(()) => ControlFlow::Continue(()),
            Err(e) => ControlFlow::Break(e),
        }
    });
    if let Err(e) = result {
        eprintln!("oxide-rs: {e:?}");
        std::process::exit(1);
    }
}

// Cargo supplies the selected build-std runtime as an unused, non-prelude
// dependency. Loading it during export provides the definitions normally
// selected by the native linker, even when the input is only an rlib.
fn force_panic_runtime(args: &mut [String]) {
    let mut strategy = "unwind";
    for (index, arg) in args.iter().enumerate() {
        let option = if arg == "-C" {
            args.get(index + 1).map(String::as_str)
        } else {
            arg.strip_prefix("-C")
        };
        if let Some(value) = option.and_then(|value| value.strip_prefix("panic=")) {
            strategy = value;
        }
    }
    let runtime = match strategy {
        "unwind" => "panic_unwind",
        "abort" => "panic_abort",
        _ => return,
    };
    for index in 1..args.len() {
        let (prefix, value) = if args[index - 1] == "--extern" {
            ("", args[index].as_str())
        } else if let Some(value) = args[index].strip_prefix("--extern=") {
            ("--extern=", value)
        } else {
            continue;
        };
        let (options, dependency) = value.split_once(':').unwrap_or(("", value));
        if dependency.split('=').next() != Some(runtime)
            || options.split(',').any(|option| option == "force")
        {
            continue;
        }
        let options = if options.is_empty() {
            "force".into()
        } else {
            format!("{options},force")
        };
        args[index] = format!("{prefix}{options}:{dependency}");
    }
}
