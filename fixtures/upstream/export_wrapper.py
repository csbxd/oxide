#!/usr/bin/env python3
"""Keep explicit Oxide test exports metadata-only within Cargo's wrapper protocol.

Cargo adds --emit=dep-info,link to test targets. Appending --emit=metadata does
not replace those modes, so explicit exports must remove every earlier emit
argument. All semantic compiler flags and non-export invocations are retained.
"""

import os
import sys


def metadata_export_arguments(arguments: list[str]) -> list[str]:
    """Change emit modes only when oxide-rs has an explicit export destination."""
    if not any(argument.startswith("--oxide-export=") for argument in arguments):
        return list(arguments)
    forwarded = []
    index = 0
    while index < len(arguments):
        argument = arguments[index]
        if argument == "--emit":
            if index + 1 == len(arguments) or arguments[index + 1].startswith("-"):
                raise ValueError("--emit requires a value")
            index += 2
            continue
        if argument.startswith("--emit="):
            index += 1
            continue
        forwarded.append(argument)
        index += 1
    return [*forwarded, "--emit=metadata"]


def main() -> int:
    frontend = os.environ.get("OXIDE_UPSTREAM_FRONTEND")
    if not frontend:
        print("upstream export wrapper: OXIDE_UPSTREAM_FRONTEND is required", file=sys.stderr)
        return 2
    try:
        arguments = metadata_export_arguments(sys.argv[1:])
        # Keep Cargo's real-rustc argv[1], exit status, signals and process tree.
        os.execv(frontend, [frontend, *arguments])
    except (OSError, ValueError) as error:
        print(f"upstream export wrapper: {error}", file=sys.stderr)
        return 1
    return 0  # execv replaces this process on success.


if __name__ == "__main__":
    sys.exit(main())
