# Step 01 — Project Bootstrap

## Goal

Create the GoFlex monorepo skeleton, go.mod, baseline tooling, and a CI
configuration that runs tests and linting. This is the foundation every later
step depends on.

## Deliverables

1. Git repository initialized at the project root.
2. `go.mod` with module path `github.com/<org>/goflex` (placeholder allowed).
3. Directory skeleton matching the layout in `00-overview.md`.
4. `Makefile` (or `Taskfile`) with targets: `test`, `lint`, `build`, `tidy`.
5. `.golangci.yml` with a sensible default ruleset.
6. `.editorconfig` and `.gitignore` appropriate for Go + Node + build output.
7. GitHub Actions workflow `.github/workflows/ci.yml` running:
   - `go vet`
   - `go test ./...`
   - `golangci-lint run`
   - on Linux and macOS.
8. A smoke package `pkg/version` exposing `Version() string` with a unit test.
9. `README.md` at the repo root summarizing the project and linking to `plan/`.

## Implementation notes

- Use Go 1.22+ as the minimum version. Pin it in `go.mod` and in CI.
- `pkg/version.Version()` should read from a `-ldflags "-X"` variable, falling
  back to `"dev"` when unset. This gives later steps a ready-made place to
  stamp build info.
- `.gitignore` must ignore:
  - `bin/`, `dist/`, `*.out`, `coverage.txt`
  - `node_modules/`, `.goflex/`, `*.generated.go`
- `Makefile` target `make test` should run `go test ./... -race -cover`.
- `golangci-lint` config should enable at minimum: `govet`, `staticcheck`,
  `errcheck`, `ineffassign`, `revive`, `gofmt`, `goimports`.

## File checklist

```text
goflex/
├── .editorconfig
├── .gitignore
├── .golangci.yml
├── .github/workflows/ci.yml
├── Makefile
├── README.md
├── go.mod
├── go.sum
├── pkg/version/version.go
└── pkg/version/version_test.go
```

## Testing scenarios

Each scenario maps to an automated check in CI or a unit test.

### T01.1 — Module builds

- **Given** a fresh clone of the repo.
- **When** running `go build ./...`.
- **Then** it exits with status 0 and produces no build errors.

### T01.2 — Unit tests pass

- `go test ./... -race` exits 0.
- `pkg/version` has a test asserting `Version()` returns `"dev"` when the
  build-time variable is empty, and returns the injected value otherwise
  (tested via a thin indirection so the test can override the variable).

### T01.3 — Linting passes

- `golangci-lint run` exits 0 on the initial tree.

### T01.4 — CI runs on Linux and macOS

- GitHub Actions matrix contains both OSes.
- All matrix jobs succeed on a no-op commit.

### T01.5 — Directory skeleton exists

- An automated test (`TestRepoLayout` in `internal/meta/layout_test.go`)
  walks the filesystem and asserts that each required directory in the
  layout is present, even if empty (an empty `.gitkeep` file is acceptable).

### T01.6 — Makefile targets work

- `make tidy` runs `go mod tidy` without error.
- `make test` runs the full test suite.
- `make lint` runs `golangci-lint`.
- A shell-based integration test script in `scripts/test-makefile.sh`
  invokes each target and asserts exit code 0.

## Acceptance criteria

- All six scenarios above pass in CI.
- A new contributor can run `make test` and `make lint` successfully after a
  fresh clone, with no additional manual setup beyond installing Go and
  `golangci-lint`.
