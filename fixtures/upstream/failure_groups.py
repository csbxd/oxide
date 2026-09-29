"""Use generated Go diagnostics to choose smaller batches for fresh retries.

This is only a partitioning heuristic. It must never establish a test result,
remove generated code, or replace the export/build/run of either returned group.
Indirect calls can hide dependencies; a missed edge merely causes another build
failure and the ordinary bisection fallback.
"""

from collections import defaultdict
from pathlib import Path
import re


FUNCTION = re.compile(r"^func ([A-Za-z_]\w*)(?=[\[(])")
IDENTIFIER = re.compile(r"\b[A-Za-z_]\w*\b")
ROOT = re.compile(r"^// (\w+) translates (.+)\.$", re.M)
DIAGNOSTIC = re.compile(
    r"^(?:[^\s:]*[/\\])?(oxide_gen_\d+\.go):([1-9]\d*)(?::\d+)?:", re.M
)


def named_failure_groups(log: Path, roots: list[str]) -> list[list[str]] | None:
    """Partition roots mentioned by their original Rust test paths in a log.

    Export/lowering diagnostics can name a nested item inside a test even when
    no Go source was produced. Such a name only suggests a retry partition;
    every returned root must still undergo fresh export, build and execution.
    """
    if len(roots) < 2 or len(set(roots)) != len(roots):
        return None
    try:
        diagnostic = log.read_text(errors="replace")
    except OSError:
        return None
    if diagnostic.startswith("$ "):
        diagnostic = diagnostic.partition("\n")[2]
    suspects, remaining = [], []
    for root in roots:
        module, separator, function = root.rpartition("::")
        if not separator or not function.startswith("__oxide_upstream_"):
            return None
        original = module + "::" + function.removeprefix("__oxide_upstream_")
        # Allow nested functions/closures after the exact test name, but do not
        # confuse test_a with test_ab or a crate path with a longer identifier.
        mentioned = re.search(r"(?<![\w:])" + re.escape(original) + r"(?!\w)", diagnostic)
        (suspects if mentioned else remaining).append(root)
    return [suspects, remaining] if suspects and remaining else None


def failure_groups(generated: Path, log: Path, roots: list[str]) -> list[list[str]] | None:
    """Return [suspected roots, remaining roots], or None for ordinary bisection.

    The root order is preserved within each group. The recognizer relies on the
    existing generator's gofmt output and falls back when diagnostics cannot be
    placed in named functions. Both groups still need actual fresh compilation
    and execution, including singleton suspects.
    """
    if len(roots) < 2 or len(set(roots)) != len(roots):
        return None
    try:
        errors = [(file, int(line)) for file, line in DIAGNOSTIC.findall(log.read_text())]
        if not errors:
            return None
        root_names = {}
        references = defaultdict(set)
        locations = defaultdict(list)
        for path in sorted(generated.glob("oxide_gen_*.go")):
            source = path.read_text()
            root_names.update({rust: go for go, rust in ROOT.findall(source)})
            active = None
            body = []
            for number, line in enumerate(source.splitlines(), 1):
                match = FUNCTION.match(line)
                if match:
                    # Unexpected formatting is not a reason to trust a partial
                    # graph: let the caller use its existing fallback.
                    if active is not None:
                        return None
                    active = (match[1], number)
                    body = []
                if active is None:
                    continue
                body.append(line)
                if line == "}" or (match and line.rstrip().endswith("}")):
                    name, start = active
                    references[name].update(IDENTIFIER.findall("\n".join(body)))
                    locations[path.name].append((start, number, name))
                    active = None
                    body = []
            if active is not None:
                return None
    except (OSError, UnicodeError):
        return None

    if any(root not in root_names or root_names[root] not in references for root in roots):
        return None
    bad = set()
    for file, line in errors:
        matches = [name for start, end, name in locations[file] if start <= line <= end]
        if len(matches) != 1:
            return None
        bad.add(matches[0])

    callers = defaultdict(set)
    for caller, mentioned in references.items():
        for callee in mentioned & references.keys():
            if callee != caller:
                callers[callee].add(caller)
    pending = list(bad)
    while pending:
        for caller in callers[pending.pop()]:
            if caller not in bad:
                bad.add(caller)
                pending.append(caller)
    suspects = [root for root in roots if root_names[root] in bad]
    remaining = [root for root in roots if root_names[root] not in bad]
    return [suspects, remaining] if suspects and remaining else None
