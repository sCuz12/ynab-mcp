package hosted

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type requestIDKey struct{}

type loggedResponse struct {
	http.ResponseWriter
	status int
}

func (w *loggedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggedResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *loggedResponse) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *loggedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func requestLabel(path string) string {
	switch path {
	case "/mcp", "/oauth/register", "/oauth/authorize", "/oauth/ynab/callback", "/oauth/token", "/oauth/revoke", "/healthz", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
		return path
	default:
		return "unknown"
	}
}

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := randomID()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		start := time.Now()
		response := &loggedResponse{ResponseWriter: w}
		next.ServeHTTP(response, r)
		if response.status == 0 {
			response.status = http.StatusOK
		}
		if r.URL.Path != "/healthz" {
			slog.InfoContext(r.Context(), "http_request", "request_id", id, "method", r.Method, "route", requestLabel(r.URL.Path), "status", response.status, "duration_ms", time.Since(start).Milliseconds())
		}
	})
}

func logOutcome(r *http.Request, event, result, reason string) {
	slog.InfoContext(r.Context(), event, "request_id", r.Context().Value(requestIDKey{}), "result", result, "reason", reason)
}

func logStateOutcome(r *http.Request, result, reason, state string) {
	slog.InfoContext(r.Context(), "oauth_state", "request_id", r.Context().Value(requestIDKey{}), "result", result, "reason", reason, "state_ref", stateRef(state))
}

func logToolOutcome(r *http.Request, name, result string, duration time.Duration) {
	slog.InfoContext(r.Context(), "mcp_tool", "request_id", r.Context().Value(requestIDKey{}), "tool", name, "result", result, "duration_ms", duration.Milliseconds())
}

func stateRef(state string) string {
	if state == "" {
		return ""
	}
	return digest(state)[:12]
}
