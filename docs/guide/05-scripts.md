# 5. Scripts and .env

## Scripts

`scripts` in `gtr.json` are named commands, as in npm:

```json
"scripts": {
    "start": "go run .",
    "test": "go test ./...",
    "lint": "go vet ./... && gofmt -l .",
    "migrate": "go run . migrate",
    "pretest": "gtr run lint"
}
```

```bash
gtr run                         # "start" (also: gtr start)
gtr run migrate
gtr migrate                     # the same: a name that is not a gtr command runs the script
gtr run test -- -run TestLogin -v   # arguments after -- go to the script
gtr run -r test                 # in every workspace member that has "test"
```

- A script runs in the directory of the nearest `gtr.json`, through the system shell
  (`sh` on Linux and macOS, `cmd` on Windows), so `&&` works everywhere.
- `pre<name>` and `post<name>` run before and after `<name>` when they exist.
- The script's exit code is gtr's exit code, so CI fails when the script fails.
- `go` inside a script is the project's Go, with gtr's environment: dependencies come from
  `gtr_modules/` and go errors get a gtr hint (see [Troubleshooting](14-troubleshooting.md)).
- A mistyped name gets a suggestion: `script "tset" is not defined; did you mean "test"?`
- Without a script of that name, `gtr run test`, `build`, `dev`, `install` and `ci` run the
  built-in command of the same name.

## .env

Variables from `.env` and then `.env.local` in the project directory are added to the
environment of scripts and of `gtr dev`. Variables already set in your environment win.
In a workspace member, the root's files are read first.

```
# .env — shared defaults, committed (or .env.example)
APP_URL=http://localhost:8080
DB_HOST=localhost
DB_URL="postgres://app@${DB_HOST}/app"   # ${VAR} and \n work in double quotes
LITERAL='$not_expanded'                  # single quotes: as written
```

```
# .env.local — your secrets, never committed
DB_PASSWORD=...
```

`gtr run --no-env <name>` skips the files.

Keep secrets out of `.env` if it is committed; `gtr publish` never includes `.env*` files
in a package except `.env.example`.

Next: [test, build, dev](06-test-build-dev.md).
