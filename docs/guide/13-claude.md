# 13. Claude

`gtr mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server. With it,
Claude works on a gtr project through gtr — installing packages, running scripts and tests,
reading `gtr.json` — instead of calling `go get` and `go mod tidy` on a `go.mod` that gtr
generates. It sees the same errors and hints you see in the terminal (ADR-0011).

## Claude Code

In the project folder:

```bash
claude mcp add gtr -- gtr mcp
```

Or install the plugin — the server plus a skill that tells Claude how gtr projects work:

```
/plugin marketplace add shibisty/gtr
/plugin install gtr@gtr
```

(`/plugin marketplace add C:\path\to\gtr` installs it from a local clone.)

## Claude Desktop

```bash
cd myproject
gtr mcp bundle -o gtr.mcpb
```

Open `gtr.mcpb` with Claude Desktop (double-click, or drag it into the window) and choose the
project folder in the extension's settings. Or add the server by hand:

```bash
gtr mcp --print-config
```

prints the entry for `claude_desktop_config.json` (Settings → Developer → Edit Config, then
restart Claude Desktop) with the full path of `gtr` and the project folder.

## What Claude can do

| | Tools | |
|---|---|---|
| look | `project_info`, `list`, `why`, `outdated`, `search`, `info`, `env`, `doctor` | read only |
| change | `add`, `remove`, `install`, `update`, `sync`, `link`, `unlink`, `version_bump` | Claude asks you first |
| run | `run`, `test`, `build` | with a time limit; `test` returns results and coverage per package |
| release | `publish` | only a dry run unless allowed |

Claude can also read `gtr://gtr.json`, `gtr://gtr.lock` and the `gtr.json` schema.

- The server works in one project: the folder it starts in, moved up to the nearest
  `gtr.json` or workspace root (`--root <dir>` to choose). It refuses folders outside it;
  `dir` reaches workspace members.
- Publishing is off: `gtr mcp --allow-publish` (the extension's *Allow publishing*
  setting) lets `publish` upload. `--allow-self-install` adds a tool that installs Go
  versions.
- Long-running scripts (`dev`, a server's `start`) stop at the time limit.

Try: "run the tests with coverage", "which packages are outdated?", "add faker as a dev
dependency and write a test that uses it".

Next: [troubleshooting](14-troubleshooting.md).
