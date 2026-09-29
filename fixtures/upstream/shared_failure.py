"""Prove that an emit failure lies in support code required by every root.

The returned evidence can mark a batch blocked, never passed. No indirect calls,
allocation references, guessed call targets, or generated-code edits are used.
"""

from collections import defaultdict, deque
import json
from pathlib import Path


def _edges(function, functions):
    for block, target in function.get("calls", {}).items():
        if isinstance(target, str) and target in functions:
            yield {"kind": "calls", "block": str(block),
                   "caller": function["symbol"], "callee": target}
    for block, assertion in function.get("assert_calls", {}).items():
        target = assertion.get("symbol") if isinstance(assertion, dict) else None
        if isinstance(target, str) and target in functions:
            yield {"kind": "assert_calls", "block": str(block),
                   "caller": function["symbol"], "callee": target}


def _path(functions, start, destination):
    if start not in functions:
        return None
    previous = {start: None}
    pending = deque([start])
    while pending:
        symbol = pending.popleft()
        if symbol == destination:
            edges = []
            while previous[symbol] is not None:
                edge = previous[symbol]
                edges.append(edge)
                symbol = edge["caller"]
            return list(reversed(edges))
        for edge in _edges(functions[symbol], functions):
            target = edge["callee"]
            if target not in previous:
                previous[target] = edge
                pending.append(target)
    return None


def _anchors(program, roots):
    # Generate always emits ProcessExitSymbol independently of selected roots.
    process_exit = program.get("process_exit_symbol")
    if isinstance(process_exit, str) and process_exit:
        yield {"kind": "process_exit_symbol", "symbol": process_exit}

    # Limit API-based proofs to this adapter's common () -> u8 signature.
    # Other public types or a single root's signature cannot justify blocking
    # an entire batch.
    returned = roots[0].get("return")
    if type(returned) is not int or any(root.get("params") != [] or root.get("return") != returned
                                       for root in roots):
        return
    types = [ty for ty in program.get("types", []) if ty.get("id") == returned]
    apis = [ty for ty in program.get("api_types", []) if ty.get("id") == returned]
    if len(types) != 1 or len(apis) != 1:
        return
    if any(ty.get("kind") != "u8" or ty.get("size") != 1 or ty.get("sized") is not True
           for ty in (types[0], apis[0])):
        return
    api = apis[0]
    for member in ("default_symbol", "display_symbol", "drop_symbol"):
        symbol = api.get(member)
        if isinstance(symbol, str) and symbol:
            yield {"kind": "shared_u8_return_api", "type": returned, "member": member, "symbol": symbol}
    debug = api.get("debug")
    if isinstance(debug, dict):
        for member in ("argument_symbol", "arguments_symbol", "format_symbol"):
            symbol = debug.get(member)
            if isinstance(symbol, str) and symbol:
                yield {"kind": "shared_u8_return_api", "type": returned,
                       "member": "debug." + member, "symbol": symbol}


def shared_failure(mir: Path, log: Path) -> dict | None:
    """Return a verifiable shared-support dependency path, otherwise None.

    Evidence includes the unique failed function, every affected root, an
    always-required support symbol and each direct MIR call/assert-call edge.
    The caller must preserve this as a nonpassing blocked outcome; old passing
    baseline cases must still fail their regression gate when blocked.
    """
    try:
        program = json.loads(mir.read_text())
        diagnostic = log.read_text(errors="replace")
        if diagnostic.startswith("$ "):
            diagnostic = diagnostic.partition("\n")[2]
        functions = {}
        by_name = defaultdict(list)
        for function in program["functions"]:
            symbol, name = function["symbol"], function["name"]
            if not isinstance(symbol, str) or not symbol or symbol in functions or not isinstance(name, str) or not name:
                return None
            functions[symbol] = function
            by_name[name].append(symbol)
        roots = program.get("roots")
        if not isinstance(roots, list) or not roots:
            return None
        if any(not isinstance(root.get("name"), str) or not root["name"] or root.get("symbol") not in functions
               for root in roots):
            return None
        if len({root["name"] for root in roots}) != len(roots):
            return None

        matched = {}
        for line in diagnostic.splitlines():
            for name in by_name:
                if line.startswith(name + ": "):
                    matched[name] = line
        if len(matched) != 1:
            return None
        name, message = next(iter(matched.items()))
        if len(by_name[name]) != 1:
            return None
        failed = by_name[name][0]
        for anchor in _anchors(program, roots):
            edges = _path(functions, anchor["symbol"], failed)
            if edges is None:
                continue
            symbols = [anchor["symbol"]] + [edge["callee"] for edge in edges]
            return {"diagnostic": message,
                    "failed_function": {"name": name, "symbol": failed},
                    "anchor": anchor,
                    "path": [{"name": functions[symbol]["name"], "symbol": symbol} for symbol in symbols],
                    "edges": edges,
                    "roots": [root["name"] for root in roots]}
    except (OSError, ValueError, KeyError, TypeError, AttributeError):
        return None
    return None
