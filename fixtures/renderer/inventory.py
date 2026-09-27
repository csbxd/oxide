"""Rebuild the public API inventory from pinned rustdoc and compiler output."""

import argparse
import collections
import hashlib
import json
from pathlib import Path
import subprocess
import tomllib

from check_api import cli_names

HERE = Path(__file__).resolve().parent


def discover(path, name):
    path = Path(path)
    doc = json.loads(path.read_text())
    index = doc['index']
    paths = collections.defaultdict(set)
    exports = []
    visited = set()

    def walk(item_id, path):
        item_id = str(item_id)
        if (item_id, path) in visited:
            return
        visited.add((item_id, path))
        item = index[item_id]
        kind, value = next(iter(item['inner'].items()))
        if kind == 'use':
            target = value['id']
            exports.append({'path': path, 'source': value['source'], 'glob': value['is_glob'], 'target_id': target})
            if target is not None and str(target) in index:
                if value['is_glob']:
                    for child in index[str(target)]['inner']['module']['items']:
                        c = index[str(child)]
                        if c['visibility'] == 'public':
                            label = c['name'] or c.get('inner', {}).get('use', {}).get('name')
                            walk(child, path.rsplit('::', 1)[0] + '::' + label)
                else:
                    walk(target, path)
            return
        paths[item_id].add(path)
        if kind == 'module':
            for child in value['items']:
                c = index[str(child)]
                if c['visibility'] != 'public':
                    continue
                label = c['name'] or c.get('inner', {}).get('use', {}).get('name')
                walk(child, path + '::' + label)

    root = index[str(doc['root'])]
    walk(doc['root'], root['name'])

    def trait_path(t):
        p = doc['paths'].get(str(t['id']))
        path = '::'.join(p['path']) if p else t['path']
        args = t.get('args')
        if args and 'angle_bracketed' in args:
            params = [ty(a['type']) if 'type' in a else a.get('lifetime', str(a)) for a in args['angle_bracketed']['args']]
            if params:
                path += '<' + ', '.join(params) + '>'
        return path

    def ty(t):
        if t is None:
            return '()'
        if isinstance(t, str):
            return t
        k, v = next(iter(t.items()))
        if k in ('primitive', 'generic'):
            return v
        if k == 'resolved_path':
            args = v.get('args')
            params = []
            if args and 'angle_bracketed' in args:
                for a in args['angle_bracketed']['args']:
                    if 'type' in a:
                        params.append(ty(a['type']))
                    elif 'lifetime' in a:
                        params.append(a['lifetime'])
                    else:
                        params.append(str(a))
            return v['path'] + ('<' + ', '.join(params) + '>' if params else '')
        if k == 'borrowed_ref':
            return '&' + (v['lifetime'] + ' ' if v['lifetime'] else '') + ('mut ' if v['is_mutable'] else '') + ty(v['type'])
        if k == 'raw_pointer':
            return ('*mut ' if v['is_mutable'] else '*const ') + ty(v['type'])
        if k == 'tuple':
            return '(' + ', '.join(ty(x) for x in v) + (',' if len(v) == 1 else '') + ')'
        if k == 'slice':
            return '[' + ty(v) + ']'
        if k == 'array':
            return '[' + ty(v['type']) + '; ' + v['len'] + ']'
        if k == 'qualified_path':
            return '<' + ty(v['self_type']) + (' as ' + v['trait']['path'] if v.get('trait') else '') + '>::' + v['name']
        if k == 'impl_trait':
            return 'impl ' + repr(v)
        return repr(t)

    records = []
    types = []
    traits = []

    def add(item, category, call_paths, owner=None, impl=None):
        f = item['inner']['function']
        generics = f['generics']['params'] + (impl['generics']['params'] if impl else [])
        non_lifetime = [x for x in generics if next(iter(x['kind'])) != 'lifetime']
        call_paths = sorted(call_paths, key=lambda s: (len(s.split('::')), s))
        records.append({'id': call_paths[0], 'aliases': call_paths, 'category': category,
                        'owner': owner, 'name': item['name'], 'span': item['span'],
                        'generic': bool(non_lifetime), 'generic_parameters': generics,
                        'signature': 'fn ' + item['name'] + '(' + ', '.join(n + ': ' + ty(t) for n, t in f['sig']['inputs']) + ') -> ' + ty(f['sig']['output']),
                        'rustdoc_signature': f['sig'], 'where_predicates': f['generics']['where_predicates'],
                        'header': f['header'], 'docs': item['docs'], 'attrs': item['attrs']})

    for item_id, public_paths in paths.items():
        item = index[item_id]
        if item['crate_id'] != 0:
            continue
        kind, value = next(iter(item['inner'].items()))
        if kind == 'function':
            add(item, 'free', public_paths)
        if kind not in ('struct', 'enum', 'union', 'type_alias'):
            continue
        names = sorted(public_paths)
        types.append({'paths': names, 'kind': kind, 'span': item['span'], 'item_id': int(item_id)})
        for impl_id in value.get('impls', []):
            impl_item = index[str(impl_id)]
            impl = impl_item['inner']['impl']
            if impl_item['crate_id'] != 0 or impl['is_synthetic'] or impl.get('blanket_impl') is not None:
                continue
            self_type = impl['for'].get('resolved_path')
            if not self_type or str(self_type['id']) != item_id:
                continue
            trait = impl.get('trait')
            derived = 'automatically_derived' in impl_item['attrs']
            category = 'derived_trait' if derived else ('explicit_trait' if trait else 'inherent')
            methods = []
            for child in impl['items']:
                method = index[str(child)]
                if 'function' not in method['inner']:
                    continue
                if trait is None and method['visibility'] != 'public':
                    continue
                methods.append(method['name'])
                aliases = [('<' + p + ' as ' + trait_path(trait) + '>::' if trait else p + '::') + method['name'] for p in names]
                add(method, category, aliases, names[0], impl)
            if trait:
                traits.append({'owner_paths': names, 'trait': trait_path(trait), 'derived': derived,
                               'methods': methods, 'span': impl_item['span'], 'generics': impl['generics']})
    records.sort(key=lambda x: x['id'])
    result = {'configuration': name, 'rustdoc_format': doc['format_version'], 'target': doc['target']['triple'],
              'raw_sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
              'modules': sorted(p for i, ps in paths.items() if 'module' in index[i]['inner'] for p in ps),
              'functions': records, 'types': types, 'trait_impls': traits, 'reexports': exports,
              'counts': dict(collections.Counter(x['category'] for x in records))}
    return result, doc


def constructors(discovered, doc, base_paths):
    index, result = doc['index'], []
    for item in discovered['types']:
        owner = index[str(item['item_id'])]
        kind = owner['inner'].get('struct', {}).get('kind')
        candidates = []
        if isinstance(kind, dict) and 'tuple' in kind:
            fields = kind['tuple']
            if all(field is not None and index[str(field)]['visibility'] == 'public' for field in fields):
                candidates.append((owner, item['paths'], fields, 'tuple_struct'))
        for variant_id in owner['inner'].get('enum', {}).get('variants', []):
            variant = index[str(variant_id)]
            kind = variant['inner']['variant']['kind']
            if isinstance(kind, dict) and 'tuple' in kind:
                candidates.append((variant, [p+'::'+variant['name'] for p in item['paths']], kind['tuple'], 'tuple_variant'))
        generic_params = next(iter(owner['inner'].values())).get('generics', {}).get('params', [])
        for value, aliases, fields, origin in candidates:
            aliases = sorted(aliases, key=lambda p: (len(p.split('::')), p))
            result.append({'id': aliases[0], 'kind': 'constructor', 'origin': origin,
                'aliases': aliases, 'generic': any(next(iter(p['kind'])) != 'lifetime' for p in generic_params),
                'features': ['scene'] if not set(item['paths']) & base_paths else [],
                'source': value['span'], 'argument_types': [index[str(f)]['inner']['struct_field'] for f in fields]})
    return result


def attach_probes(definitions, mapping, native_names, process_names):
    used = set()
    for definition in definitions:
        key = definition['id']
        if key not in mapping:
            assert definition['kind'] == 'derived_trait', f'unmapped public API: {key}; add an explicit api-probes.json entry'
            probes = {'probes': [], 'process_probes': [], 'probe_status': 'not_individually_probed'}
        else:
            used.add(key)
            probes = mapping[key]
        assert set(probes) == {'probes', 'process_probes', 'probe_status'}, key
        assert set(probes['probes']) <= native_names, f'unknown native probe: {key}'
        assert set(probes['process_probes']) <= process_names, f'unknown CLI probe: {key}'
        if definition['kind'] != 'derived_trait':
            assert probes['probes'] or probes['process_probes'], f'public API lacks a probe: {key}'
        definition.update(probes)
    assert used == set(mapping), f'stale probe mappings: {sorted(set(mapping) - used)}'


def trait_key(name, trait):
    owner = name[1:].split(' as ', 1)[0] if name.startswith('<') else ''
    return owner, name.rsplit('::', 1)[-1], trait.split('<', 1)[0]


def build_inventory(default_path, scene_path, api_path, cases_path, upstream, probes_path):
    base, _ = discover(default_path, 'default')
    scene, raw = discover(scene_path, 'scene')
    assert base['rustdoc_format'] == scene['rustdoc_format']
    assert base['target'] == scene['target'], 'rustdoc targets differ'
    compiler = json.loads(Path(api_path).read_text())
    native = json.loads(Path(cases_path).read_text())
    probes = json.loads(Path(probes_path).read_text())
    assert set(probes['process_cases']) == cli_names(HERE / 'test_cli.py'), 'CLI mapping is stale'
    native_names = {case['name'] for case in native}
    assert len(native_names) == len(native), 'duplicate native case'
    assert sorted(case['id'] for case in native) == list(range(len(native))), 'invalid native case IDs'
    definitions = []
    base_ids = {function['id'] for function in base['functions']}
    for function in scene['functions']:
        features = ['scene'] if function['id'] not in base_ids else []
        if function['span'] and function['span']['filename'] == 'src/cli.rs':
            features.append('cli')
        if function['name'] == 'write_output_png':
            features.append('png')
        kind = function['category']
        definitions.append({'id': function['id'], 'kind': kind,
            'origin': 'derived' if kind == 'derived_trait' else 'handwritten',
            'aliases': function['aliases'], 'generic': function['generic'],
            'generic_parameters': function['generic_parameters'], 'features': features,
            'signature': function['signature'], 'source': function['span']})
    base_paths = {path for item in base['types'] for path in item['paths']}
    definitions.extend(constructors(scene, raw, base_paths))
    attach_probes(definitions, probes['definitions'], native_names, set(probes['process_cases']))
    by_alias = {alias: d for d in definitions for alias in d['aliases']}
    by_trait = {}
    for definition in definitions:
        if 'trait' not in definition['kind']:
            continue
        trait = definition['id'].split(' as ', 1)[1].rsplit('>::', 1)[0]
        for alias in definition['aliases']:
            by_trait[trait_key(alias, trait)] = definition
    public = []
    for function in compiler['public_api']:
        name = function['name']
        definition = by_alias.get(name)
        if definition is None and function['kind'] == 'trait_method':
            definition = by_trait.get(trait_key(name, function['trait_definition']))
        if function['kind'] != 'trait_method':
            assert definition is not None, f'compiler callable absent from rustdoc: {name}'
        public.append({key: function[key] for key in ('name', 'definition', 'kind', 'owner', 'implementation', 'trait_definition', 'status') if key in function} |
            {'features': definition['features'] if definition else (['scene'] if any(p in name for p in ('::Scene', '::Paint', '::Color', '::BlendMode', '::PathCommand', '::FillRule', '::GradientStop')) else []),
             'generic': function['status'] == 'requires_monomorphization', 'must_export': function['status'] == 'monomorphic',
             'probes': definition['probes'] if definition else [], 'process_probes': definition['process_probes'] if definition else [],
             'probe_status': definition['probe_status'] if definition else 'inherited_default_not_individually_probed'})
    actual = {item['name']: item for item in public}
    kinds = {'free': 'function', 'inherent': 'inherent_method', 'constructor': 'constructor'}
    for definition in definitions:
        if definition['kind'] in kinds:
            for alias in definition['aliases']:
                assert alias in actual and actual[alias]['kind'] == kinds[definition['kind']], f'rustdoc/compiler callable mismatch: {alias}'
    upstream = Path(upstream)
    package = tomllib.loads((upstream / 'Cargo.toml').read_text())['package']
    commit = subprocess.check_output(['git', '-C', str(upstream), 'rev-parse', 'HEAD'], text=True).strip()
    return {'schema_version': 1, 'source': {'crate': package['name'], 'version': package['version'],
        'repository': package['repository'], 'commit': commit, 'compiler': compiler['compiler'],
        'target': compiler['target'], 'rustdoc_format': scene['rustdoc_format'],
        'rustdoc_sha256': {'default': base['raw_sha256'], 'scene': scene['raw_sha256']},
        'api_rs_sha256': hashlib.sha256((HERE / 'api.rs').read_bytes()).hexdigest()},
        'features': ['cli', 'png', 'scene'], 'default_features_preserved': True,
        'counts': {'default': base['counts'], 'default_scene': scene['counts'],
            'tuple_struct_constructors': sum(d['origin'] == 'tuple_struct' for d in definitions),
            'tuple_variant_constructors': sum(d['origin'] == 'tuple_variant' for d in definitions),
            'public_types_default': len(base['types']), 'public_types_scene': len(scene['types']),
            'public_api_paths': len(public),
            'public_api_by_kind': dict(collections.Counter(d['kind'] for d in public)),
            'public_api_by_status': dict(collections.Counter(d['status'] for d in public))},
        'functions': sorted(definitions, key=lambda d: d['id']), 'public_api': public,
        'reexports': scene['reexports'], 'public_types': scene['types'],
        'probe_cases': [case['name'] for case in native], 'process_cases': probes['process_cases'],
        'scope': ['Rustdoc public reachability traverses module children and pub-use aliases, including types reexported from private modules.',
            'Compiler inventory includes monomorphic provided trait defaults; generic declarations require explicit concrete Rust witnesses.',
            'Unit and named-field variants are not function constructors. Blanket and synthetic auto-trait impls are not enumerated as local callable definitions.',
            'Export completeness is not behavioral coverage of every derived/default trait method or every implementation branch.']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--default', type=Path, required=True, help='default-features rustdoc JSON')
    parser.add_argument('--scene', type=Path, required=True, help='default + scene rustdoc JSON')
    parser.add_argument('--api', type=Path, required=True, help='small oxide.mir.api.json sidecar')
    parser.add_argument('--cases', type=Path, required=True, help='native API cases.json')
    parser.add_argument('--upstream', type=Path, default=HERE.parents[2] / 'mermaid-rs-renderer')
    parser.add_argument('--probes', type=Path, default=HERE / 'api-probes.json')
    parser.add_argument('--output', type=Path, default=HERE / 'api-inventory.json')
    parser.add_argument('--check', action='store_true', help='compare against output without replacing it')
    args = parser.parse_args()
    inventory = build_inventory(args.default, args.scene, args.api, args.cases, args.upstream, args.probes)
    if args.check:
        expected = json.loads(args.output.read_text())
        assert inventory == expected, f'inventory differs in {[key for key in inventory if inventory[key] != expected.get(key)]}'
    else:
        args.output.write_text(json.dumps(inventory, indent=2, ensure_ascii=False) + '\n')
    print(json.dumps({'counts': inventory['counts'], 'probe_cases': len(inventory['probe_cases']), 'check': args.check}, indent=2))


if __name__ == '__main__':
    main()
