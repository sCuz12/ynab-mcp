# YNAB MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io/) server that exposes YNAB plans, accounts, categories, payees, months, and transactions. It supports local stdio with a personal token and hosted Streamable HTTP with per-user YNAB OAuth.

## Requirements

- Go 1.25 or later
- For local mode, a YNAB personal access token from **Account Settings → Developer Settings → Personal Access Tokens**

Personal access tokens are for your own account. Hosted mode uses a YNAB OAuth application instead.

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

## Hosted setup

1. Choose a public HTTPS origin, such as `https://budget-tools.example`. The remote MCP URL will be `https://budget-tools.example/mcp`. YNAB requires a public privacy policy for an OAuth application. Its [application requirements](https://api.ynab.com/) also restrict use of “YNAB” in your public app name or domain unless preceded by “for”.
2. In YNAB, open **Account Settings → Developer Settings → New Application**. Set its redirect URI to the exact URL `https://budget-tools.example/oauth/ynab/callback`. Choose a name and description that explain the integration, and provide your privacy policy URL. Save the app and copy its **Client ID** and **Client Secret** into your host's private environment settings. Do not add them to Git or share them in chat. YNAB starts new OAuth apps in restricted mode, allowing up to 25 tokens for other users; request YNAB review before wider distribution.
3. Generate a persistent encryption key with `openssl rand -base64 32`. Save that result as `TOKEN_ENCRYPTION_KEY` in the host's secret settings. Keep a secure backup: losing it makes existing OAuth records unreadable.
4. Deploy the `Dockerfile` to a managed host with HTTPS and a **persistent writable disk** mounted at `/data`. For Render, `render.yaml` is a single-instance Blueprint with the disk and health check; its compute and disk incur charges. Enter `PUBLIC_BASE_URL` and the three credentials when the Blueprint asks for them. On other hosts, set `PUBLIC_BASE_URL` to the HTTPS origin, `YNAB_OAUTH_CLIENT_ID`, `YNAB_OAUTH_CLIENT_SECRET`, `TOKEN_ENCRYPTION_KEY`, and `OAUTH_DB_PATH=/data/oauth.db`. The container uses `MCP_TRANSPORT=http` and listens on the host's `PORT` or 8080.
5. Check `GET /healthz`, then add `https://budget-tools.example/mcp` as a remote MCP connector in your AI client. The client discovers this service's OAuth endpoints, sends the user to YNAB, and receives its own MCP bearer token. Request `ynab:read` for read tools or `ynab:read ynab:write` for all 15 tools. Write access must be granted during YNAB authorization; changing scope requires reconnecting.

OAuth discovery endpoints are `/.well-known/oauth-protected-resource/mcp` and `/.well-known/oauth-authorization-server`. The service supports dynamic client registration, authorization code with PKCE, refresh-token rotation, and token revocation. YNAB tokens and MCP grants are encrypted in the database. Set up backups for the persistent disk and encryption key, and put rate limiting in front of public OAuth endpoints before broad launch. The current Docker setup is intended for one managed-host instance; horizontal scaling requires a shared transactional store.

## Code layout

- `main.go` wires the YNAB client, tool catalog, and stdio server together.
- `internal/mcp/` owns JSON-RPC framing, MCP initialization, and tool dispatch. It has no YNAB or HTTP dependency.
- `internal/tools/` owns public tool schemas and translates arguments into named YNAB operations.
- `internal/ynab/` owns YNAB requests, authentication, response errors, transaction inputs, and domain validation.
- `internal/hosted/` owns remote HTTP transport, OAuth, per-user credentials, and encrypted persistence.

YNAB response envelopes pass through unchanged. Each package has focused tests; transaction tests mock HTTP without contacting YNAB.
