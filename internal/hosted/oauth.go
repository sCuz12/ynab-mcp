package hosted

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const ynabAuthorize = "https://app.ynab.com/oauth/authorize"
const ynabToken = "https://app.ynab.com/oauth/token"

type Config struct {
	BaseURL      string
	YNABClientID string
	YNABSecret   string
	DBPath       string
	Key          string
	HTTPClient   *http.Client
	YNABAuthURL  string // test override
	YNABTokenURL string // test override
	YNABUserURL  string // test override
	YNABAPIURL   string // test override
}

type Service struct {
	cfg   Config
	store *Store
	http  *http.Client
	mu    sync.Mutex // serializes refresh for a YNAB user
}

type oauthClient struct {
	RedirectURIs []string `json:"redirect_uris"`
	ClientName   string   `json:"client_name,omitempty"`
}

type pending struct {
	ClientID, RedirectURI, State, Challenge, Resource, Scope, YNABVerifier string
	Expires                                                                time.Time
}

type authorizationCode struct {
	ClientID, RedirectURI, Challenge, Resource, Scope, UserID string
	Expires                                                   time.Time
}

type mcpToken struct {
	ClientID, Resource, Scope, UserID string
	Expires                           time.Time
}

type ynabCredential struct {
	Access, Refresh string
	Expires         time.Time
}

func New(cfg Config) (*Service, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, errors.New("PUBLIC_BASE_URL must be an HTTPS origin without a path")
	}
	if cfg.YNABClientID == "" || cfg.YNABSecret == "" || cfg.DBPath == "" {
		return nil, errors.New("YNAB_OAUTH_CLIENT_ID, YNAB_OAUTH_CLIENT_SECRET, and OAUTH_DB_PATH are required")
	}
	store, err := OpenStore(cfg.DBPath, cfg.Key)
	if err != nil {
		return nil, err
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.YNABAuthURL == "" {
		cfg.YNABAuthURL = ynabAuthorize
	}
	if cfg.YNABTokenURL == "" {
		cfg.YNABTokenURL = ynabToken
	}
	if cfg.YNABUserURL == "" {
		cfg.YNABUserURL = "https://api.ynab.com/v1/user"
	}
	return &Service{cfg: cfg, store: store, http: cfg.HTTPClient}, nil
}

func (s *Service) Close() error { return s.store.Close() }

func (s *Service) resource() string { return s.cfg.BaseURL + "/mcp" }

func jsonReply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func oauthError(w http.ResponseWriter, status int, code string) {
	jsonReply(w, status, map[string]string{"error": code})
}

func (s *Service) metadata(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, http.StatusOK, map[string]any{
		"issuer": s.cfg.BaseURL, "authorization_endpoint": s.cfg.BaseURL + "/oauth/authorize",
		"token_endpoint": s.cfg.BaseURL + "/oauth/token", "registration_endpoint": s.cfg.BaseURL + "/oauth/register",
		"revocation_endpoint":      s.cfg.BaseURL + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported": []string{"ynab:read", "ynab:write"},
	})
}

func (s *Service) resourceMetadata(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, http.StatusOK, map[string]any{
		"resource": s.resource(), "authorization_servers": []string{s.cfg.BaseURL},
		"scopes_supported": []string{"ynab:read", "ynab:write"},
	})
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "[::1]" || u.Hostname() == "::1")
}

func (s *Service) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var client oauthClient
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&client); err != nil || len(client.RedirectURIs) == 0 || len(client.RedirectURIs) > 10 {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	for _, redirect := range client.RedirectURIs {
		if !validRedirect(redirect) {
			oauthError(w, 400, "invalid_redirect_uri")
			return
		}
	}
	id, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	if err := s.store.put("clients", id, client); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	jsonReply(w, http.StatusCreated, map[string]any{"client_id": id, "client_id_issued_at": time.Now().Unix(), "redirect_uris": client.RedirectURIs, "client_name": client.ClientName, "token_endpoint_auth_method": "none"})
}

func validChallenge(value string) bool {
	if len(value) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil
}

func normalizeScope(scope string) (string, bool) {
	if scope == "" {
		return "ynab:read", true
	}
	parts := strings.Fields(scope)
	if len(parts) == 1 && parts[0] == "ynab:read" {
		return "ynab:read", true
	}
	if len(parts) == 2 && ((parts[0] == "ynab:read" && parts[1] == "ynab:write") || (parts[0] == "ynab:write" && parts[1] == "ynab:read")) {
		return "ynab:read ynab:write", true
	}
	return "", false
}

func (s *Service) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	q := r.URL.Query()
	var client oauthClient
	if err := s.store.get("clients", q.Get("client_id"), &client); err != nil {
		oauthError(w, 400, "invalid_client")
		return
	}
	redirect := q.Get("redirect_uri")
	allowed := false
	for _, u := range client.RedirectURIs {
		if u == redirect {
			allowed = true
		}
	}
	if !allowed {
		oauthError(w, 400, "invalid_redirect_uri")
		return
	}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || !validChallenge(q.Get("code_challenge")) || q.Get("resource") != s.resource() {
		oauthError(w, 400, "invalid_request")
		return
	}
	scope, valid := normalizeScope(q.Get("scope"))
	if !valid {
		oauthError(w, 400, "invalid_scope")
		return
	}
	id, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	verifier, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	p := pending{ClientID: q.Get("client_id"), RedirectURI: redirect, State: q.Get("state"), Challenge: q.Get("code_challenge"), Resource: s.resource(), Scope: scope, YNABVerifier: verifier, Expires: time.Now().Add(10 * time.Minute)}
	if err := s.store.put("pending", digest(id), p); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	u, _ := url.Parse(s.cfg.YNABAuthURL)
	y := u.Query()
	y.Set("client_id", s.cfg.YNABClientID)
	y.Set("redirect_uri", s.cfg.BaseURL+"/oauth/ynab/callback")
	y.Set("response_type", "code")
	y.Set("state", id)
	y.Set("code_challenge", digest(verifier))
	y.Set("code_challenge_method", "S256")
	if scope == "ynab:read" {
		y.Set("scope", "read-only")
	}
	u.RawQuery = y.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	var p pending
	if err := s.store.take("pending", digest(r.URL.Query().Get("state")), &p); err != nil || time.Now().After(p.Expires) {
		oauthError(w, 400, "invalid_state")
		return
	}
	if r.URL.Query().Get("error") != "" {
		oauthError(w, 400, "access_denied")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		oauthError(w, 400, "invalid_request")
		return
	}
	credential, err := s.exchangeYNAB(r.Context(), url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.cfg.BaseURL + "/oauth/ynab/callback"}, "code_verifier": {p.YNABVerifier},
	})
	if err != nil {
		http.Error(w, "YNAB authorization failed", 502)
		return
	}
	userID, err := s.ynabUser(r.Context(), credential.Access)
	if err != nil {
		http.Error(w, "YNAB user lookup failed", 502)
		return
	}
	if err := s.store.put("ynab", digest(userID+":"+p.Scope), credential); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	outCode, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	a := authorizationCode{ClientID: p.ClientID, RedirectURI: p.RedirectURI, Challenge: p.Challenge, Resource: p.Resource, Scope: p.Scope, UserID: userID, Expires: time.Now().Add(5 * time.Minute)}
	if err := s.store.put("codes", digest(outCode), a); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	u, _ := url.Parse(p.RedirectURI)
	q := u.Query()
	q.Set("code", outCode)
	q.Set("state", p.State)
	q.Set("iss", s.cfg.BaseURL)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Service) exchangeYNAB(ctx context.Context, values url.Values) (ynabCredential, error) {
	values.Set("client_id", s.cfg.YNABClientID)
	values.Set("client_secret", s.cfg.YNABSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.YNABTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return ynabCredential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return ynabCredential{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ynabCredential{}, fmt.Errorf("YNAB token HTTP %d", resp.StatusCode)
	}
	var token struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil {
		return ynabCredential{}, err
	}
	if token.Access == "" || token.Refresh == "" || token.Expires <= 0 {
		return ynabCredential{}, errors.New("incomplete YNAB token response")
	}
	return ynabCredential{Access: token.Access, Refresh: token.Refresh, Expires: time.Now().Add(time.Duration(token.Expires) * time.Second)}, nil
}

func (s *Service) ynabUser(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.YNABUserURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("YNAB user HTTP %d", resp.StatusCode)
	}
	var value struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil {
		return "", err
	}
	if value.Data.User.ID == "" {
		return "", errors.New("YNAB user ID missing")
	}
	return value.Data.User.ID, nil
}

func verifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	hash := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(hash[:])), []byte(challenge)) == 1
}

func (s *Service) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	var t mcpToken
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		var code authorizationCode
		if err := s.store.takeIf("codes", digest(r.Form.Get("code")), &code, func() bool {
			return time.Now().Before(code.Expires) && code.ClientID == r.Form.Get("client_id") && code.RedirectURI == r.Form.Get("redirect_uri") && code.Resource == r.Form.Get("resource") && verifyPKCE(r.Form.Get("code_verifier"), code.Challenge)
		}); err != nil {
			oauthError(w, 400, "invalid_grant")
			return
		}
		t = mcpToken{ClientID: code.ClientID, Resource: code.Resource, Scope: code.Scope, UserID: code.UserID}
	case "refresh_token":
		if err := s.store.takeIf("refresh", digest(r.Form.Get("refresh_token")), &t, func() bool {
			return time.Now().Before(t.Expires) && t.ClientID == r.Form.Get("client_id") && t.Resource == r.Form.Get("resource")
		}); err != nil {
			oauthError(w, 400, "invalid_grant")
			return
		}
	default:
		oauthError(w, 400, "unsupported_grant_type")
		return
	}
	access, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	refresh, err := randomID()
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	t.Expires = time.Now().Add(time.Hour)
	if err := s.store.put("access", digest(access), t); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	t.Expires = time.Now().Add(30 * 24 * time.Hour)
	if err := s.store.put("refresh", digest(refresh), t); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	jsonReply(w, 200, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": t.Scope})
}

func (s *Service) revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	key := digest(r.Form.Get("token"))
	var token mcpToken
	for _, bucket := range []string{"access", "refresh"} {
		if s.store.get(bucket, key, &token) == nil && token.ClientID == r.Form.Get("client_id") {
			_ = s.store.delete(bucket, key)
		}
	}
	w.WriteHeader(200)
}

func (s *Service) credential(ctx context.Context, userID, scope string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c ynabCredential
	key := digest(userID + ":" + scope)
	if err := s.store.get("ynab", key, &c); err != nil {
		return "", err
	}
	if time.Until(c.Expires) > time.Minute {
		return c.Access, nil
	}
	updated, err := s.exchangeYNAB(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.Refresh}})
	if err != nil {
		return "", err
	}
	if err := s.store.put("ynab", key, updated); err != nil {
		return "", err
	}
	return updated.Access, nil
}
