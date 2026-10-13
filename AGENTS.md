# Repository Guidelines

## Project Structure & Module Organization

`main.go` wires the application and keeps `go run .` available. `internal/mcp/` owns stdio JSON-RPC and tool dispatch, `internal/tools/` defines MCP schemas and argument mapping, and `internal/ynab/` implements named YNAB operations, authentication, and validation. Keep protocol details out of `internal/ynab/` and HTTP details out of `internal/mcp/`. Tests live beside the package they exercise. The repository has no generated assets.

## Build, Test, and Development Commands

- `go run .` starts the stdio server; it waits for MCP messages on stdin.
- `go build -o bin/ynab-mcp .` builds a local binary. `bin/` is ignored by Git.
- `go test ./...` runs protocol, tool, and mocked API tests.
- `go vet ./...` runs static checks; `gofmt -w main.go internal/*/*.go` formats Go source.

Set `YNAB_ACCESS_TOKEN` in the environment before running. For a local `.env` file, use `set -a; source .env; set +a` in your shell. Go does not load `.env` automatically.

## Coding Style & Naming Conventions

Use `gofmt` formatting and standard Go naming: short lowercase package names, exported names only for cross-package APIs, and `TestBehavior` test names. Add YNAB endpoint logic to a named client operation instead of constructing HTTP paths in MCP handlers. Use typed transaction inputs; preserve YNAB's response envelope at the MCP boundary.

## Testing Guidelines

Use Go's `testing` package. Test protocol framing in `internal/mcp/`, tool schemas and argument mapping in `internal/tools/`, and request methods and validation in `internal/ynab/`. Mock HTTP with an in-memory transport. Cover invalid inputs and API errors without requiring a live account or token. Run `go test ./...` before requesting review; there is no numeric coverage target.

## Commit & Pull Request Guidelines

There is no Git history establishing a commit convention. Use short imperative subjects such as `Validate transaction dates`. Pull requests should explain behavior changes, link relevant issues, and list checks run. Include screenshots only if a visible interface is added.

## Security & Configuration

Never commit access tokens, account data, or populated `.env` files. Keep the token in the environment or a local secrets manager. Do not log request headers or include the bearer token in errors.
