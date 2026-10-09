# JSON Schema for gtr.json

`gtr.schema.json` is the schema for version 1 of the manifest (draft 2020-12).

Editor hints: add
`"$schema": "https://raw.githubusercontent.com/shibisty/gtr/main/schema/gtr.schema.json"`
to gtr.json (the packages of the orm, passport, migration and faker families do), or in
VS Code:

```json
"json.schemas": [{ "fileMatch": ["gtr.json"], "url": "./schema/gtr.schema.json" }]
```

- `examples/` — valid manifests: an application, a driver, a workspace root.

Reserved for later (valid in the schema, not used by gtr-manager yet): `build`, `test`,
`env`, and `pre<name>`/`post<name>` scripts.
- `testdata/invalid/` — manifests the schema must reject.

gtr-manager checks the fields it uses when it reads a manifest; to validate a whole file
against the schema:

```bash
pip install jsonschema
python -c "import json,glob,jsonschema as j; s=json.load(open('gtr.schema.json')); v=j.Draft202012Validator(s); \
[print(f, list(v.iter_errors(json.load(open(f)))) == []) for f in glob.glob('examples/*.json')+glob.glob('testdata/invalid/*.json')]"
```

Expected: `True` for `examples/` and `False` for `testdata/invalid/`.

Manifests written by early gtr-manager versions are migrated on install, with a warning:
`entrypoint` is dropped (see `build.entry`), `engine` → `engines.go`, `website` → `homepage`,
the dependency key `github:user/repo` → an import path plus the source in the value (ADR-0003).
