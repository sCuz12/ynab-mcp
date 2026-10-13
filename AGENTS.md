# Repository Guidelines

## Project Structure & Module Organization

`main.go` selects local stdio or hosted HTTP mode. `internal/mcp/` owns stdio JSON-RPC, `internal/tools/` defines schemas, `internal/ynab/` implements API operations, and `internal/hosted/` owns OAuth, remote MCP, and encrypted storage. Keep HTTP details out of `internal/mcp/` and YNAB endpoint logic in `internal/ynab/`. Tests live beside the package they exercise.

## Build, Test, and Development Commands

- `go run .` starts the stdio server; it waits for MCP messages on stdin.
- `MCP_TRANSPORT=http go run .` starts hosted mode when its OAuth environment variables are set; see `README.md`.
- `go build -o bin/ynab-mcp .` builds a local binary. `bin/` is ignored by Git.
- `go test ./...` runs protocol, tool, and mocked API tests.
- `go vet ./...` runs static checks; `gofmt -w main.go internal/*/*.go` formats Go source.

Set `YNAB_ACCESS_TOKEN` for local stdio mode. Hosted mode uses YNAB OAuth variables and a persistent database. For a local `.env` file, use `set -a; source .env; set +a`; Go does not load it automatically.

## Coding Style & Naming Conventions

Use `gofmt` formatting and standard Go naming: short lowercase package names, exported names only for cross-package APIs, and `TestBehavior` test names. Add YNAB endpoint logic to a named client operation instead of constructing HTTP paths in MCP handlers. Use typed transaction inputs; preserve YNAB's response envelope at the MCP boundary.

## Testing Guidelines

Use Go's `testing` package. Test stdio framing in `internal/mcp/`, schemas in `internal/tools/`, YNAB operations in `internal/ynab/`, and OAuth plus remote MCP in `internal/hosted/`. Mock HTTP with an in-memory transport. Cover invalid inputs and API errors without a live account. Run `go test ./...` before review; there is no numeric coverage target.

## Commit & Pull Request Guidelines

There is no Git history establishing a commit convention. Use short imperative subjects such as `Validate transaction dates`. Pull requests should explain behavior changes, link relevant issues, and list checks run. Include screenshots only if a visible interface is added.

## Security & Configuration

Never commit access tokens, OAuth client secrets, encryption keys, account data, populated `.env` files, or OAuth database files. Keep credentials in the host's secret manager. Do not log request headers or include bearer tokens in errors. Hosted mode requires HTTPS and a persistent single-instance disk.
