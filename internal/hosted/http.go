package hosted

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"ynab-mcp/internal/tools"
	"ynab-mcp/internal/ynab"
)

// Handler serves OAuth discovery, authorization, and stateless MCP over HTTPS.
// A reverse proxy or managed host must terminate TLS before forwarding requests.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.metadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.resourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.resourceMetadata)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("GET /oauth/authorize", s.authorize)
	mux.HandleFunc("GET /oauth/ynab/callback", s.callback)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("POST /oauth/revoke", s.revoke)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/mcp", s.mcp)
	return withRequestLogging(mux)
}

func (s *Service) mcp(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		logOutcome(r, "mcp_auth", "rejected", "missing_bearer")
		s.challenge(w)
		return
	}
	var grant mcpToken
	if err := s.store.get("access", digest(parts[1]), &grant); err != nil || time.Now().After(grant.Expires) || grant.Resource != s.resource() {
		logOutcome(r, "mcp_auth", "rejected", "invalid_or_expired_bearer")
		s.challenge(w)
		return
	}
	ynabToken, err := s.credential(r.Context(), grant.UserID, grant.Scope)
	if err != nil {
		logOutcome(r, "mcp_auth", "failed", "ynab_session_unavailable")
		jsonReply(w, 503, map[string]string{"error": "ynab_session_unavailable"})
		return
	}
	client := ynab.NewClient(ynabToken, s.http, s.cfg.YNABAPIURL)
	server := sdk.NewServer(&sdk.Implementation{Name: "ynab-mcp", Version: "0.1.0"}, nil)
	for _, tool := range tools.New(client) {
		if strings.HasPrefix(tool.Name, "create_") || strings.HasPrefix(tool.Name, "update_") || strings.HasPrefix(tool.Name, "delete_") {
			if !strings.Contains(grant.Scope, "ynab:write") {
				continue
			}
		}
		tool := tool
		server.AddTool(&sdk.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			started := time.Now()
			data, err := tool.Handle(ctx, json.RawMessage(req.Params.Arguments))
			if err != nil {
				logToolOutcome(r, tool.Name, "failed", time.Since(started))
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}, IsError: true}, nil
			}
			logToolOutcome(r, tool.Name, "succeeded", time.Since(started))
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(data)}}}, nil
		})
	}
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	handler.ServeHTTP(w, r)
}

func (s *Service) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+s.cfg.BaseURL+`/.well-known/oauth-protected-resource/mcp"`)
	oauthError(w, http.StatusUnauthorized, "invalid_token")
}
