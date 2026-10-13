# YNAB MCP Server

A local [Model Context Protocol](https://modelcontextprotocol.io/) server that exposes YNAB plans, accounts, categories, payees, months, and transactions to an MCP client. It uses stdio and the [YNAB API v1](https://api.ynab.com/).

## Requirements

- Go 1.23 or later
- A YNAB personal access token from **Account Settings → Developer Settings → Personal Access Tokens**

Personal access tokens are intended for your own account. This server does not implement OAuth for other users.

## Build and run

```sh
go build -o bin/ynab-mcp .
YNAB_ACCESS_TOKEN='your-token' ./bin/ynab-mcp
```

For local development with a `.env` file containing `YNAB_ACCESS_TOKEN=...`, export its variables before starting Go:

```sh
set -a
source .env
set +a
go run .
```

The process reads newline-delimited MCP JSON-RPC messages on stdin and writes responses on stdout. It does not print a startup message. For an MCP host, configure an absolute path to the built binary and supply the token through the host's private environment settings. For example:

```json
{
  "mcpServers": {
    "ynab": {
      "command": "/absolute/path/to/ynab-mcp/bin/ynab-mcp",
      "env": { "YNAB_ACCESS_TOKEN": "your-token" }
    }
  }
}
```

Keep your token out of committed configuration files and shell history. Client configuration formats vary; adapt the example to your host.

## Tools

Read tools: `list_plans`, `get_plan`, `list_accounts`, `get_account`, `list_categories`, `get_category`, `list_payees`, `list_months`, `get_month`, `list_transactions`, `list_account_transactions`, and `get_transaction`.

Write tools: `create_transaction`, `update_transaction`, and `delete_transaction`. These modify your YNAB data when called. Transaction amounts use **milliunits**: `-12500` is an outflow of 12.50 units of the plan currency. Dates use `YYYY-MM-DD`. Use `list_plans` to find a plan ID, or pass `last-used` as `plan_id`. List tools that accept `last_knowledge_of_server` can return only changes since an earlier response.

`list_transactions` and `list_account_transactions` default to YNAB's last-year window unless `since_date` is supplied. Prefer narrow date ranges and specific resource tools to limit response size and API calls. YNAB limits a token to 200 requests per hour; a rate-limit response appears as a tool error.

## Development

```sh
go test ./...      # protocol, validation, and mocked API tests
go vet ./...       # static checks
gofmt -w *.go      # format source
```

The tests use an in-memory HTTP transport and never call a live YNAB account.

## Code layout

- `main.go` wires the YNAB client, tool catalog, and stdio server together.
- `internal/mcp/` owns JSON-RPC framing, MCP initialization, and tool dispatch. It has no YNAB or HTTP dependency.
- `internal/tools/` owns public tool schemas and translates arguments into named YNAB operations.
- `internal/ynab/` owns YNAB requests, authentication, response errors, transaction inputs, and domain validation.

YNAB response envelopes pass through unchanged. Each package has focused tests; transaction tests mock HTTP without contacting YNAB.
