"""Require the native oracle and translated renderer to resolve the same closure."""

import json
from pathlib import Path
import subprocess


def renderer_closure(cargo, manifest, upstream, target, env, features):
    command = [str(cargo), "metadata", "--format-version=1", "--locked", "--manifest-path", str(manifest), "--filter-platform", target]
    if features:
        command += ["--features", features]
    metadata = json.loads(subprocess.check_output(command, env=env))
    packages = {p["id"]: p for p in metadata["packages"]}
    nodes = {p["id"]: p for p in metadata["resolve"]["nodes"]}
    start = next(p["id"] for p in packages.values() if Path(p["manifest_path"]).resolve() == upstream.resolve())
    def identity(package):
        return (package["name"], package["version"], package["source"] or "path")
    result, pending, visited = [], [start], set()
    while pending:
        package_id = pending.pop()
        if package_id in visited:
            continue
        visited.add(package_id)
        node, package = nodes[package_id], packages[package_id]
        dependencies = []
        for edge in node["deps"]:
            kinds = [kind for kind in edge["dep_kinds"] if kind["kind"] != "dev"]
            if not kinds:
                continue
            pending.append(edge["pkg"])
            dependencies.append({"alias": edge["name"], "package": identity(packages[edge["pkg"]]),
                                 "kinds": sorted(kinds, key=lambda kind: json.dumps(kind, sort_keys=True))})
        result.append({"package": identity(package), "features": sorted(node["features"]),
                       "dependencies": sorted(dependencies, key=lambda edge: (edge["alias"], edge["package"]))})
    return sorted(result, key=lambda row: row["package"])


def check_dependencies(cargo, oracle, upstream, target, env):
    native = renderer_closure(cargo, oracle, upstream, target, env, "")
    direct = renderer_closure(cargo, upstream, upstream, target, env, "scene")
    if native != direct:
        left = {tuple(row["package"]): row for row in native}
        right = {tuple(row["package"]): row for row in direct}
        changed = [key for key in sorted(left.keys() | right.keys()) if left.get(key) != right.get(key)]
        raise AssertionError(f"native/direct dependency versions, features or edges differ: {changed}")
    return {"target": target, "packages": len(direct), "renderer_closure": direct}
