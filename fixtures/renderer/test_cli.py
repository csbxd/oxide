#!/usr/bin/env python3
"""Compare the actual Rust and translated CLI in isolated processes."""

import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess


FLOW = "flowchart LR\n A[開始] --> B{Ready?}\n B --> C[Done]\n"
MARKDOWN = "# Two diagrams\n```mermaid\nflowchart LR; A-->B\n```\n```mermaid\npie\n\"One\": 2\n\"Two\": 3\n```\n"
CONFIG = '{"themeVariables":{"primaryColor":"#cdeeff"},"flowchart":{"nodeSpacing":37}}'

# Inputs, argv, exit status, stdout, stderr and every created file are checked.
# CLI parsing and rendering are always the original translated Rust functions.
CASES = [
    ("help", ["--help"], "", {}, 0),
    ("version", ["--version"], "", {}, 0),
    ("unknown-flag", ["--not-a-renderer-option"], "", {}, 2),
    ("bad-ratio", ["--preferredAspectRatio", "0:9"], FLOW, {}, 2),
    ("empty-input", [], "", {}, 1),
    ("missing-file", ["-i", "absent.mmd"], "", {}, 1),
    ("stdin-svg", ["-i", "-"], FLOW, {}, 0),
    ("implicit-stdin", [], FLOW, {}, 0),
    ("svg-file", ["-i", "input.mmd", "-o", "output.svg"], "", {"input.mmd": FLOW}, 0),
    ("png-file", ["-i", "input.mmd", "-e", "png", "-o", "output.png", "-w", "640", "-H", "360"], "", {"input.mmd": FLOW}, 0),
    ("png-missing-output", ["-e", "png"], FLOW, {}, 1),
    ("size", ["--size", "-w", "800", "-H", "450"], FLOW, {}, 0),
    ("options", ["-t", "dark", "--nodeSpacing", "60", "--rankSpacing", "75", "--preferredAspectRatio", "16:9", "--fastText"], FLOW, {}, 0),
    ("config", ["-c", "config.json", "-t", "forest"], FLOW, {"config.json": CONFIG}, 0),
    ("invalid-config", ["-c", "config.json"], FLOW, {"config.json": "{"}, 1),
    ("layout-dumps", ["--layoutEngine", "dagre", "--dumpLayout", "layout.json", "--dumpLayeredLayout", "layered.json"], FLOW, {}, 0),
    ("timing", ["--timing"], FLOW, {}, 0),
    ("markdown", ["-i", "input.md", "-o", "out/", "--dumpLayout", "layouts/"], "", {"input.md": MARKDOWN}, 0),
    ("markdown-size", ["-i", "input.md", "--size"], "", {"input.md": MARKDOWN}, 0),
    ("markdown-empty", ["-i", "input.md"], "", {"input.md": "# No Mermaid blocks\n"}, 1),
]


def encode(data):
    return base64.b64encode(data).decode("ascii")


def timing_contract(data):
    timing = json.loads(data)
    keys = {"parse_us", "layout_us", "render_us", "total_us", "layout_stage_us"}
    stage_keys = {"layered_layout_us", "port_assignment_us", "edge_routing_us", "label_placement_us", "total_us"}
    assert set(timing) == keys, timing
    stages = timing["layout_stage_us"]
    assert set(stages) == stage_keys, stages
    for value in [timing[key] for key in keys - {"layout_stage_us"}] + list(stages.values()):
        assert type(value) is int and value >= 0, timing
    assert timing["total_us"] == timing["parse_us"] + timing["layout_us"] + timing["render_us"], timing
    assert timing["total_us"] > 0, timing
    assert stages["total_us"] == sum(stages[key] for key in stage_keys - {"total_us"}), timing
    assert stages["total_us"] <= timing["layout_us"], timing
    # Wall-clock durations cannot be equal across two real executions. Retain
    # the complete measured payload beside the normalized contract report.
    return b"validated real nonnegative timing fields and exact totals\n"


def execute(binary, destination):
    results = {}
    env = dict(os.environ, NO_COLOR="1", CLICOLOR="0", TERM="dumb", RUST_BACKTRACE="0")
    for name, argv, stdin, files, exit_code in CASES:
        work = destination / name
        if work.exists():
            shutil.rmtree(work)
        work.mkdir(parents=True)
        for relative, content in files.items():
            path = work / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        result = subprocess.run(["mmdr", *argv], executable=str(binary), input=stdin.encode(), capture_output=True, cwd=work, env=env, timeout=90)
        artifacts = {str(path.relative_to(work)): encode(path.read_bytes()) for path in sorted(work.rglob("*")) if path.is_file()}
        (destination / (name + ".stdout")).write_bytes(result.stdout)
        (destination / (name + ".stderr")).write_bytes(result.stderr)
        assert result.returncode == exit_code, (name, result.returncode, exit_code, result.stderr)
        stderr = timing_contract(result.stderr) if name == "timing" else result.stderr
        results[name] = {"exit": result.returncode, "stdout": encode(result.stdout), "stderr": encode(stderr), "files": artifacts}
        print(f"{name}: exit={result.returncode}, stdout={len(result.stdout)}, stderr={len(result.stderr)}, files={len(artifacts)}", flush=True)
    (destination / "results.json").write_text(json.dumps(results, indent=2, sort_keys=True))
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", type=Path)
    parser.add_argument("--go", type=Path)
    parser.add_argument("--cache", type=Path, required=True)
    args = parser.parse_args()
    if not args.native and not args.go:
        parser.error("--native or --go is required")
    cache = args.cache.resolve()
    if args.native:
        expected = execute(args.native.resolve(), cache / "native")
    else:
        expected = json.loads((cache / "native" / "results.json").read_text())
    assert set(expected) == {case[0] for case in CASES}, "stale native CLI cases; regenerate references"
    if args.go:
        got = execute(args.go.resolve(), cache / "go")
        assert set(got) == set(expected), "CLI case inventory differs"
        for name in expected:
            assert got[name] == expected[name], f"{name}: CLI differs from native Rust; inspect {cache}"
        print(f"{len(expected)} CLI cases match native Rust exit/stdout/stderr/files", flush=True)


if __name__ == "__main__":
    main()
