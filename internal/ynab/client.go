// Package ynab implements named operations against the YNAB API.
package ynab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.ynab.com/v1"

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

func NewClient(token string, client *http.Client, baseURL string) *Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{token: token, http: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func segment(name, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%s must be a nonempty string", name)
	}
	return url.PathEscape(value), nil
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	if c.token == "" {
		return nil, fmt.Errorf("YNAB_ACCESS_TOKEN is not set")
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("YNAB request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("reading YNAB response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError struct {
			Error struct {
				Name   string `json:"name"`
				Detail string `json:"detail"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &apiError) == nil && apiError.Error.Name != "" {
			return nil, fmt.Errorf("YNAB %d %s: %s", resp.StatusCode, apiError.Error.Name, apiError.Error.Detail)
		}
		return nil, fmt.Errorf("YNAB returned HTTP %d", resp.StatusCode)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("YNAB returned invalid JSON")
	}
	return data, nil
}
