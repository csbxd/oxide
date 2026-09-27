#!/usr/bin/env python3
"""Install Oxide's pinned compiler in the repository cache, without rustup."""
import concurrent.futures
import hashlib
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile
import tomllib
import urllib.request

DATE = "2026-09-15"
ROOT = Path(__file__).resolve().parent.parent
CACHE = ROOT / ".cache"
PREFIX = CACHE / ("rust-" + DATE)


def install():
    arch = {"aarch64": "aarch64", "x86_64": "x86_64"}[platform.machine()]
    host = arch + "-unknown-linux-gnu"
    manifest = urllib.request.urlopen(
        f"https://static.rust-lang.org/dist/{DATE}/channel-rust-nightly.toml"
    ).read()
    packages = tomllib.loads(manifest.decode())["pkg"]
    components = [("rustc", host), ("cargo", host), ("rust-src", "*"), ("rustc-dev", host), ("llvm-tools-preview", host),
                  ("rust-std", "aarch64-unknown-linux-gnu"),
                  ("rust-std", "x86_64-unknown-linux-gnu")]
    CACHE.mkdir(exist_ok=True)
    (CACHE / "channel-rust-nightly.toml").write_bytes(manifest)

    def missing(component):
        name, target = component
        marker = name + ("-" + target if name == "rust-std" else "")
        return not (PREFIX / "lib/rustlib" / ("manifest-" + marker)).exists()

    components = [component for component in components if missing(component)]

    def download(component):
        name, target = component
        info = packages[name]["target"][target]
        if not info["available"]:
            raise RuntimeError(f"{name} unavailable for {target}")
        path = CACHE / info["xz_url"].rsplit("/", 1)[-1]
        if not path.exists() or hashlib.file_digest(path.open("rb"), "sha256").hexdigest() != info["xz_hash"]:
            print("Downloading", path.name, flush=True)
            urllib.request.urlretrieve(info["xz_url"], path)
        with path.open("rb") as f:
            if hashlib.file_digest(f, "sha256").hexdigest() != info["xz_hash"]:
                raise RuntimeError(f"checksum mismatch: {path}")
        return path

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        archives = list(pool.map(download, components))
    for archive in archives:
        with tempfile.TemporaryDirectory(dir=CACHE) as work:
            with tarfile.open(archive) as tf:
                tf.extractall(work, filter="data")
            installer = next(Path(work).glob("*/install.sh"))
            subprocess.run(["sh", str(installer), "--disable-ldconfig",
                            "--prefix=" + str(PREFIX)], check=True)
    subprocess.run([str(PREFIX / "bin/rustc"), "-vV"], check=True)


if __name__ == "__main__":
    install()
