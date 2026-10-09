# JSON Schema for gtr.json

`gtr.schema.json` is the schema for version 1 of the manifest (draft 2020-12).

Editor hints: add `"$schema": "https://gtr.dev/schema/gtr.schema.json"` to gtr.json
(until the domain is live, use a relative path to the file), or in VS Code:

```json
"json.schemas": [{ "fileMatch": ["gtr.json"], "url": "./schema/gtr.schema.json" }]
```

- `examples/` — valid manifests: an application, a driver, a workspace root.
- `testdata/invalid/` — manifests the schema must reject.

Validation (until `gtr` ships built-in validation in stage 3):

```bash
pip install jsonschema
python -c "import json,glob,jsonschema as j; s=json.load(open('gtr.schema.json')); v=j.Draft202012Validator(s); \
[print(f, list(v.iter_errors(json.load(open(f)))) == []) for f in glob.glob('examples/*.json')+glob.glob('testdata/invalid/*.json')]"
```

Expected: `True` for `examples/` and `False` for `testdata/invalid/`.

Changes compared to the current gtr-manager manifest:
`entrypoint` → `build.entry`, `engine` → `engines.go`, `website` → `homepage`,
the dependency key `github:user/repo` → an import path plus the source in the value (ADR-0003).
