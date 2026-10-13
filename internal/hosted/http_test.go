package hosted

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testService(t *testing.T) *Service {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/token":
			if r.FormValue("client_secret") != "private-secret" {
				t.Error("missing YNAB client secret")
			}
			body = `{"access_token":"ynab-access","refresh_token":"ynab-refresh","expires_in":7200}`
		case "/user":
			if r.Header.Get("Authorization") != "Bearer ynab-access" {
				t.Error("bad YNAB user authorization")
			}
			body = `{"data":{"user":{"id":"user-1"}}}`
		case "/v1/plans":
			if r.Header.Get("Authorization") != "Bearer ynab-access" {
				t.Error("bad YNAB API authorization")
			}
			body = `{"data":{"plans":[{"id":"plan-1"}]}}`
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	s, err := New(Config{BaseURL: "https://budget.example", YNABClientID: "app-id", YNABSecret: "private-secret", DBPath: filepath.Join(t.TempDir(), "oauth.db"), Key: key, HTTPClient: client, YNABAuthURL: "https://ynab.test/authorize", YNABTokenURL: "https://ynab.test/token", YNABUserURL: "https://ynab.test/user", YNABAPIURL: "https://ynab.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func request(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestOAuthFlowAndAuthenticatedMCP(t *testing.T) {
	s := testService(t)
	h := s.Handler()
	if got := request(t, h, "POST", "/mcp", "{}"); got.Code != 401 || !strings.Contains(got.Header().Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("unauthorized request: %d %s", got.Code, got.Body.String())
	}
	registration := httptest.NewRecorder()
	h.ServeHTTP(registration, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"redirect_uris":["https://client.example/callback"]}`)))
	if registration.Code != 201 {
		t.Fatalf("registration: %d %s", registration.Code, registration.Body.String())
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(registration.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	q := url.Values{"client_id": {registered.ClientID}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {s.resource()}, "state": {"client-state"}, "scope": {"ynab:read ynab:write"}}
	authorized := request(t, h, "GET", "/oauth/authorize?"+q.Encode(), "")
	if authorized.Code != 302 {
		t.Fatalf("authorize: %d %s", authorized.Code, authorized.Body.String())
	}
	ynabLocation, _ := url.Parse(authorized.Header().Get("Location"))
	if ynabLocation.Query().Get("client_id") != "app-id" || ynabLocation.Query().Get("code_challenge") == "" {
		t.Fatal("YNAB authorization missing parameters")
	}
	callback := request(t, h, "GET", "/oauth/ynab/callback?code=fake-code&state="+url.QueryEscape(ynabLocation.Query().Get("state")), "")
	if callback.Code != 302 {
		t.Fatalf("callback: %d %s", callback.Code, callback.Body.String())
	}
	clientLocation, _ := url.Parse(callback.Header().Get("Location"))
	if clientLocation.Query().Get("state") != "client-state" {
		t.Fatal("client state was not preserved")
	}
	code := clientLocation.Query().Get("code")
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {registered.ClientID}, "redirect_uri": {"https://client.example/callback"}, "resource": {s.resource()}, "code": {code}, "code_verifier": {verifier}}
	issued := request(t, h, "POST", "/oauth/token", form.Encode())
	if issued.Code != 200 {
		t.Fatalf("token: %d %s", issued.Code, issued.Body.String())
	}
	var tokens struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := json.Unmarshal(issued.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.Access == "" || tokens.Refresh == "" {
		t.Fatal("tokens missing")
	}
	if replay := request(t, h, "POST", "/oauth/token", form.Encode()); replay.Code != 400 {
		t.Fatal("authorization code replay accepted")
	}
	call := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_plans","arguments":{}}}`))
	call.Header.Set("Authorization", "Bearer "+tokens.Access)
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Accept", "application/json, text/event-stream")
	call.Header.Set("MCP-Protocol-Version", "2025-11-25")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, call)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "plan-1") {
		t.Fatalf("MCP call: %d %s", response.Code, response.Body.String())
	}
	list := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	list.Header = call.Header.Clone()
	listed := httptest.NewRecorder()
	h.ServeHTTP(listed, list)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), "create_transaction") || !strings.Contains(listed.Body.String(), "delete_transaction") {
		t.Fatalf("write tools missing: %d %s", listed.Code, listed.Body.String())
	}
	refreshForm := url.Values{"grant_type": {"refresh_token"}, "client_id": {registered.ClientID}, "resource": {s.resource()}, "refresh_token": {tokens.Refresh}}
	if refreshed := request(t, h, "POST", "/oauth/token", refreshForm.Encode()); refreshed.Code != 200 {
		t.Fatalf("refresh: %d %s", refreshed.Code, refreshed.Body.String())
	}
	if replay := request(t, h, "POST", "/oauth/token", refreshForm.Encode()); replay.Code != 400 {
		t.Fatal("refresh token replay accepted")
	}
}

func TestRejectsUntrustedRedirectAndBadPKCE(t *testing.T) {
	s := testService(t)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"redirect_uris":["http://other.example/callback"]}`)))
	if w.Code != 400 {
		t.Fatalf("insecure redirect accepted: %d", w.Code)
	}
	if verifyPKCE("short", strings.Repeat("a", 43)) {
		t.Fatal("short verifier accepted")
	}
	if verifyPKCE(strings.Repeat("v", 43), strings.Repeat("a", 43)) {
		t.Fatal("wrong verifier accepted")
	}
}

func TestReadOnlyGrantHidesWriteTools(t *testing.T) {
	s := testService(t)
	access := "mcp-read-only"
	grant := mcpToken{Resource: s.resource(), Scope: "ynab:read", UserID: "user-1", Expires: time.Now().Add(time.Hour)}
	if err := s.store.put("access", digest(access), grant); err != nil {
		t.Fatal(err)
	}
	if err := s.store.put("ynab", digest("user-1:ynab:read"), ynabCredential{Access: "ynab-access", Refresh: "ynab-refresh", Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	r.Header.Set("Authorization", "Bearer "+access)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "list_transactions") || strings.Contains(w.Body.String(), "create_transaction") {
		t.Fatalf("read-only tools: %d %s", w.Code, w.Body.String())
	}
}
