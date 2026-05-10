# Step 02 — CLI Skeleton (`goflex`)

## Goal

Build the `goflex` CLI as the single entry point for all framework commands.
At this step the CLI only wires up command parsing, help output, and stub
implementations. Later steps fill in the real behavior.

## Deliverables

1. `cmd/goflex/main.go` — CLI binary entry.
2. `internal/cli/` — command definitions and dispatch logic.
3. Subcommand stubs (all returning "not yet implemented" but with parsed flags):
   - `goflex new <name>` — scaffold new app.
   - `goflex dev` — run dev server.
   - `goflex build` — build production artifacts.
   - `goflex generate` — run code generators (API client, etc.).
   - `goflex db <subcommand>` — db migrations (`migrate`, `rollback`, `create`).
   - `goflex version` — print version.
4. Built binary installable via `go install ./cmd/goflex`.
5. Makefile target `make install` that runs `go install ./cmd/goflex`.

## Implementation notes

- Use `github.com/spf13/cobra` for command parsing. It's the de facto Go CLI
  library and supports the nested command shape we need (`goflex db migrate`).
- Each subcommand lives in its own file under `internal/cli/` (e.g.
  `new.go`, `dev.go`, `build.go`, `generate.go`, `db.go`, `version.go`).
- Commands must declare their flags in a `Flags()` function that the test
  suite can introspect.
- Exit codes:
  - `0` success
  - `1` generic failure
  - `2` usage error (unknown command or missing required flag)
- All stubs must log a clear message like
  `"goflex dev: not yet implemented (step 14)"` and exit 0 so CI can still
  invoke them during early development.
- The CLI should respect `GOFLEX_DEBUG=1` and print additional diagnostic
  output when set. Add a `pkg/log` package (thin wrapper) for this.

## Command surface (v0)

```text
goflex new <name>         [--template <name>] [--module <path>]
goflex dev                [--addr :3000] [--no-open]
goflex build              [--out dist/] [--minify]
goflex generate           [--only api|routes|all]
goflex db migrate         [--step N]
goflex db rollback        [--step N]
goflex db create <name>
goflex version
```

## Testing scenarios

### T02.1 — Help output works

- `goflex --help` exits 0 and includes each subcommand name in its output.
- `goflex db --help` lists `migrate`, `rollback`, `create`.
- Asserted in `internal/cli/cli_test.go` via Cobra's built-in test helpers
  (`cmd.SetOut`, `cmd.SetArgs`, `cmd.Execute`).

### T02.2 — Unknown command returns usage error

- `goflex bogus` exits with status 2.
- Stderr contains `unknown command "bogus"`.

### T02.3 — Each stub runs and exits 0

A table-driven test runs each command with minimal args (e.g. `goflex new x`,
`goflex dev`, `goflex build`, `goflex generate`, `goflex db migrate`,
`goflex db create foo`, `goflex version`) and asserts:

- Exit code is 0.
- Stdout contains `"not yet implemented"` (except `version`).
- `version` prints a non-empty string containing the version from
  `pkg/version`.

### T02.4 — Flags parse correctly

- `goflex dev --addr :4000` sets an internal struct field to `":4000"`.
- `goflex build --out ./out --minify` sets `Out="./out"`, `Minify=true`.
- `goflex db migrate --step 2` sets `Step=2`.
- Each subcommand exposes a `parsedConfig()` accessor the test can read
  after `cmd.Execute()`.

### T02.5 — Missing required args

- `goflex new` (no name) exits with status 2 and stderr contains
  `requires exactly 1 argument`.
- `goflex db create` (no name) exits with status 2.

### T02.6 — Debug mode

- With `GOFLEX_DEBUG=1`, running any command prints at least one
  `level=debug` line to stderr.
- Without it, no debug lines appear.

### T02.7 — `go install` produces a working binary

- A shell integration test runs `go install ./cmd/goflex`, then invokes
  `goflex version` from `$GOPATH/bin` and asserts exit code 0.

## Acceptance criteria

- All scenarios T02.1–T02.7 pass in CI.
- The CLI compiles without warnings.
- No subcommand is missing from the top-level help.
- Developers can run `goflex --help` and see a clear table of commands.
