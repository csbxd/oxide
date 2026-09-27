#!/usr/bin/env python3
"""Generate direct Go ownership checks from public Rust type IDs and Go declarations.

Usage: write_owned_test.py --api CACHE/oxide.mir.api.json --go CACHE/go
Reads only the small API sidecar and generated Go source, never the full MIR.
The output requires memory.counters and calls real public roots plus DropT<ID>.
"""

import argparse
import json
from pathlib import Path
import re
import subprocess


ROOTS = {
    "theme": ("oxide_renderer_fixture::Theme::dark", "Theme_Dark", 0),
    "config": ("<oxide_renderer_fixture::Config as core::default::Default>::default", "Config_As_Core_Default_Default_Default", 0),
    "options": ("oxide_renderer_fixture::RenderOptions::modern", "RenderOptions_Modern", 0),
    "parse": ("oxide_renderer_fixture::parse_mermaid", "ParseMermaid", 1),
    "render": ("oxide_renderer_fixture::render", "Render", 1),
    "scene": ("oxide_renderer_fixture::render_scene", "RenderScene", 2),
}


def declarations(directory, wanted):
    found, package = {}, None
    for path in sorted(directory.glob("oxide_gen_*.go")):
        comments = []
        with path.open() as source:
            for line in source:
                if line.startswith("package "):
                    name = line.split()[1]
                    assert package in (None, name), "generated package names differ"
                    package = name
                if line.startswith("//"):
                    comments.append(line.strip())
                    continue
                if line.startswith("func "):
                    name = line[5:].split("(", 1)[0]
                    if name in wanted:
                        match = re.fullmatch(r"func (\w+)\((.*)\)\s*(\w*)\s*\{\s*", line)
                        assert match, f"unsupported generated declaration: {line.strip()}"
                        params = [tuple(part.strip().split(None, 1)) for part in match[2].split(",")]
                        assert all(len(p) == 2 for p in params), line
                        assert name not in found, f"duplicate Go function {name}"
                        found[name] = {"params": params, "return": match[3], "comments": "\n".join(comments)}
                comments = []
    assert package and set(found) == wanted, f"missing Go declarations: {sorted(wanted - found.keys())}"
    return package, found


def str_fields(directory, name):
    # Consume the generated scalar-pair declaration, rather than inventing byte
    # offsets or assuming that a Go string has Rust's representation.
    marker = f"type {name} = struct {{"
    for path in sorted(directory.glob("oxide_gen_*.go")):
        with path.open() as source:
            for line in source:
                if line.strip() != marker:
                    continue
                fields = []
                for line in source:
                    if line.strip() == "}":
                        break
                    field, kind = line.strip().split(None, 1)
                    if field != "_":
                        fields.append((field, kind))
                assert fields == [("A", "uintptr"), ("B", "uint64")], f"unsupported Rust str ABI: {name} {fields}"
                return fields
    raise AssertionError(f"Rust str representation missing: {name}")


def indirect_layout(declaration, label):
    match = re.search(rf"// {label} points to (\d+) (?:writable|initialized) bytes with Rust alignment (\d+)\.", declaration["comments"])
    assert match, f"missing compiler size/alignment for indirect {label}"
    return int(match[1]), int(match[2])


def write_owned_test(api_path, directory):
    directory = Path(directory)
    api = json.loads(Path(api_path).read_text())
    assert api["target"] in ("aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu")
    roots = {r["name"]: r for r in api["roots"]}
    assert len(roots) == len(api["roots"]), "duplicate public root"
    assert "public_drop_types" in api, "sidecar lacks public drop metadata; re-export Rust"
    drops = {d["type"]: d for d in api["public_drop_types"]}
    assert len(drops) == len(api["public_drop_types"]), "duplicate public drop type"
    selected = {}
    for key, (path, name, arity) in ROOTS.items():
        root = roots[path]
        assert "params" in root and "return" in root, f"sidecar lacks logical Rust signature: {path}"
        assert len(root["params"]) == arity, f"Rust signature changed: {path}"
        result = root["return"]
        assert type(result) is int and result in drops, f"owned return lacks compiler drop: {path}"
        assert drops[result]["symbol"], f"empty compiler drop symbol: {result}"
        selected[key] = {"root": root, "go": name, "drop": f"DropT{result}", "type": result}
    assert selected["scene"]["root"]["params"][1] == selected["options"]["type"], "scene options ownership type mismatch"
    assert len({selected[k]["root"]["params"][0] for k in ("parse", "render", "scene")}) == 1, "source Rust types differ"
    wanted = {value[key] for value in selected.values() for key in ("go", "drop")}
    package, functions = declarations(directory, wanted)
    source_types = set()
    for value in selected.values():
        declaration, drop = functions[value["go"]], functions[value["drop"]]
        assert declaration["params"][0] == ("ctx", "*oxide.Context")
        assert drop["params"][0] == ("ctx", "*oxide.Context") and len(drop["params"]) == 2 and not drop["return"], f"unexpected destructor declaration: {value['drop']}"
        params = declaration["params"][1:]
        value["indirect"] = bool(params and params[0][0] == "result")
        if value["indirect"]:
            assert params[0][1] == "uintptr" and not declaration["return"]
            assert drop["params"][1][1] == "uintptr"
            value["layout"] = indirect_layout(declaration, "result")
            assert value["layout"] == indirect_layout(drop, "value"), "result/drop layout mismatch"
            params = params[1:]
        else:
            assert declaration["return"], f"missing Go return: {value['go']}"
            assert declaration["return"] == drop["params"][1][1], f"Go return/drop representations differ: {value['go']} {value['drop']}"
        assert len(params) == len(value["root"]["params"]), f"Go/Rust arity differs: {value['go']}"
        value["params"] = params
        if params:
            source_types.add(params[0][1])
    assert len(source_types) == 1, "source Go representations differ"
    source_type = source_types.pop()
    str_fields(directory, source_type)

    cases = [("theme", "theme", False), ("config", "config", False), ("options", "options", False),
             ("parse_ok", "parse", False), ("parse_error", "parse", True),
             ("render_ok", "render", False), ("render_error", "render", True), ("scene", "scene", False)]
    bodies, rows = [], []
    for case, key, invalid in cases:
        value = selected[key]
        setup, calls, arguments = [], [], ["ctx"]
        if value["indirect"]:
            size, align = value["layout"]
            setup.append(f"value := ctx.Alloc({size}, {align})")
            arguments.append("value")
        if value["params"]:
            arguments.append("input.invalid" if invalid else "input.valid")
        if key == "scene":
            options = selected["options"]
            if options["indirect"]:
                size, align = options["layout"]
                setup.append(f"options := ctx.Alloc({size}, {align})")
                calls.append(f"{options['go']}(ctx, options)")
            else:
                calls.append(f"options := {options['go']}(ctx)")
            arguments.append("options")
        expression = f"{value['go']}({', '.join(arguments)})"
        calls.append(expression if value["indirect"] else f"value := {expression}")
        bodies.append(f"""
func owned_{case}(ctx *oxide.Context, input ownedInputs) ownedObservation {{
    mark := ctx.Mark()
    defer ctx.Restore(mark)
    {'; '.join(setup)}
    frame := ctx.Mark()
    before := oxide.HeapStats()
    {'; '.join(calls)}
    retained := oxide.HeapStats()
    returnedFrame := ctx.Mark()
    // This consumes the returned ownership, including all raw-bit copies.
    // Scene also consumes options: it must never be separately dropped here.
    {value['drop']}(ctx, value)
    return ownedObservation{{before, retained, oxide.HeapStats(), frame, returnedFrame, ctx.Mark(), ctx.Failed()}}
}}
""")
        rows.append(f'{{{json.dumps(case)}, {json.dumps(value["drop"])}, owned_{case}}}')

    source = HEADER.replace("@PACKAGE@", package).replace("@SOURCE_TYPE@", source_type)
    source += "\nvar ownedCases = []ownedCase{\n" + ",\n".join(rows) + ",\n}\n"
    source += "".join(bodies)
    output = directory / "owned_return_test.go"
    output.write_text(source)
    subprocess.run(["gofmt", "-w", str(output)], check=True)
    return {"target": api["target"], "cases": len(cases), "drop_types": sorted({v["type"] for v in selected.values()}), "output": str(output)}


HEADER = '''//go:build memory.counters

// Code generated by fixtures/renderer/write_owned_test.py; DO NOT EDIT.
package @PACKAGE@

import (
    "runtime"
    "testing"
    "unsafe"

    oxide "github.com/csbxd/oxide/oxide-go/runtime"
    "modernc.org/libc"
)

type ownedInputs struct { valid, invalid @SOURCE_TYPE@ }
type ownedObservation struct {
    before, retained, after oxide.HeapSnapshot
    frame, returnedFrame, droppedFrame oxide.Mark
    failed bool
}
type ownedCase struct {
    name, drop string
    invoke func(*oxide.Context, ownedInputs) ownedObservation
}

func ownedInput(ctx *oxide.Context) ownedInputs {
    put := func(s string) @SOURCE_TYPE@ {
        p := ctx.Alloc(uintptr(len(s)), 1)
        copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(s)), s)
        return @SOURCE_TYPE@{A: p, B: uint64(len(s))}
    }
    return ownedInputs{put("flowchart LR\\n A[Alpha] -->|go| B{Beta}\\n"), put("not a diagram")}
}

func ownedFrame(t *testing.T, item ownedCase, ctx *oxide.Context, mark oxide.Mark, result ownedObservation) {
    t.Helper()
    if result.failed || result.returnedFrame != result.frame || result.droppedFrame != result.frame || ctx.Mark() != mark {
        t.Fatalf("%s: return/drop did not restore the Context frame or panic state", item.name)
    }
}

func TestPublicOwnedReturns(t *testing.T) {
    defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
    // Fixed warm-up, never an adaptive baseline: regex's owner plus eight
    // thread-ID shards. Every path runs from each of nine fresh Contexts.
    for warm := 0; warm < 9; warm++ {
        func() {
            ctx := oxide.NewContext()
            defer func() { if err := ctx.Close(); err != nil { t.Error(err) } }()
            input := ownedInput(ctx)
            mark := ctx.Mark()
            for _, item := range ownedCases {
                result := item.invoke(ctx, input)
                ownedFrame(t, item, ctx, mark, result)
            }
        }()
    }
    baseline := oxide.HeapStats()
    libcBaseline := libc.MemStat()
    t.Logf("fixed baseline: heap=%+v libc=%+v", baseline, libcBaseline)
    for _, item := range ownedCases {
        t.Run(item.name, func(t *testing.T) {
            ctx := oxide.NewContext()
            defer func() {
                if err := ctx.Close(); err != nil { t.Error(err) }
                if got := oxide.HeapStats(); got.LiveAllocations != baseline.LiveAllocations {
                    t.Errorf("%s: after Context.Close live Rust owners %+v, baseline %+v", item.name, got, baseline)
                }
                if got := libc.MemStat(); got != libcBaseline {
                    t.Errorf("%s: after Context.Close libc %+v, baseline %+v", item.name, got, libcBaseline)
                }
            }()
            input := ownedInput(ctx)
            mark := ctx.Mark()
            // Warm this Context's frame/TLS setup outside the Go allocation
            // window. Its closure must still match the fixed process baseline.
            warm := item.invoke(ctx, input)
            ownedFrame(t, item, ctx, mark, warm)
            localBaseline := oxide.HeapStats().LiveAllocations
            for call := 0; call < 3; call++ {
                var before, after runtime.MemStats
                runtime.ReadMemStats(&before)
                result := item.invoke(ctx, input)
                runtime.ReadMemStats(&after)
                ownedFrame(t, item, ctx, mark, result)
                if result.before.LiveAllocations != localBaseline || result.after.LiveAllocations != localBaseline {
                    t.Fatalf("call %d: %s failed to release returned owner: before=%+v retained=%+v after=%+v baseline=%d", call, item.drop, result.before, result.retained, result.after, localBaseline)
                }
                if result.retained.LiveAllocations <= result.before.LiveAllocations {
                    t.Fatalf("call %d: ownership oracle observed no live returned allocation: %+v -> %+v", call, result.before, result.retained)
                }
                allocations, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc
                if allocations != 0 || bytes != 0 {
                    t.Fatalf("call %d: %d Go allocations, %d Go bytes", call, allocations, bytes)
                }
                t.Logf("call=%d helper=%s retained=%d released=%d raw-Go-objects=%d raw-Go-bytes=%d", call, item.drop, result.retained.LiveAllocations-result.before.LiveAllocations, result.retained.LiveAllocations-result.after.LiveAllocations, allocations, bytes)
            }
        })
    }
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", required=True, type=Path)
    parser.add_argument("--go", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(write_owned_test(args.api, args.go), indent=2))


if __name__ == "__main__":
    main()
