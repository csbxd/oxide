#!/bin/sh
set -eu
oxide_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
oxide_sysroot="$oxide_root/.cache/rust-2026-09-15"
if [ ! -x "$oxide_sysroot/bin/rustc" ] || [ ! -x "$oxide_sysroot/bin/cargo" ] || [ ! -f "$oxide_sysroot/lib/rustlib/src/rust/library/Cargo.toml" ]; then
    python3 "$oxide_root/oxide-rs/bootstrap.py"
fi
OXIDE_SYSROOT="$oxide_sysroot" RUSTC="$oxide_sysroot/bin/rustc" \
    RUSTFLAGS="-C link-arg=-Wl,-rpath,$oxide_sysroot/lib" \
    "$oxide_sysroot/bin/cargo" build --manifest-path "$oxide_root/oxide-rs/Cargo.toml" \
    --target-dir "$oxide_root/.cache/frontend"
mkdir -p "$oxide_root/bin"
oxide_binary=$(mktemp "$oxide_root/bin/.oxide-rs.XXXXXX")
trap 'rm -f "$oxide_binary"' EXIT HUP INT TERM
cp "$oxide_root/.cache/frontend/debug/oxide-rs" "$oxide_binary"
chmod 755 "$oxide_binary"
mv -f "$oxide_binary" "$oxide_root/bin/oxide-rs"
