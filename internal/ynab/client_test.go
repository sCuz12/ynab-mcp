package ynab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func ptr[T any](value T) *T { return &value }

func TestTransactionWriteRequests(t *testing.T) {
	var requests []*http.Request
	var bodies []map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		if r.Body != nil {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			bodies = append(bodies, body)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"transaction":{"id":"tx-1"}}}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("test-token", httpClient, "https://api.example.test/v1")
	ctx := context.Background()
	_, err := client.CreateTransaction(ctx, "last-used", TransactionFields{
		AccountID: ptr("account-1"), Date: ptr("2026-01-02"), Amount: ptr(Milliunits(-12500)), Approved: ptr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UpdateTransaction(ctx, "last-used", "tx-1", TransactionFields{Memo: ptr("Coffee")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DeleteTransaction(ctx, "last-used", "tx-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /v1/plans/last-used/transactions", "PUT /v1/plans/last-used/transactions/tx-1", "DELETE /v1/plans/last-used/transactions/tx-1",
	}
	if len(requests) != len(want) {
		t.Fatalf("got %d requests", len(requests))
	}
	for i, r := range requests {
		if r.Method+" "+r.URL.RequestURI() != want[i] {
			t.Errorf("request %d: wrong route", i)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("request %d: missing bearer token", i)
		}
	}
	created := bodies[0]["transaction"].(map[string]any)
	if created["amount"] != float64(-12500) || created["approved"] != false {
		t.Errorf("create payload changed: %v", created)
	}
	if len(created) != 4 {
		t.Errorf("unexpected create fields: %v", created)
	}
	updated := bodies[1]["transaction"].(map[string]any)
	if len(updated) != 1 || updated["memo"] != "Coffee" {
		t.Errorf("update payload changed: %v", updated)
	}
}

func TestValidationBeforeRequest(t *testing.T) {
	called := false
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})}
	client := NewClient("token", httpClient, "https://api.example.test/v1")
	_, err := client.CreateTransaction(context.Background(), "last-used", TransactionFields{
		AccountID: ptr("account-1"), Date: ptr("2026-02-30"), Amount: ptr(Milliunits(0)),
	})
	if err == nil {
		t.Fatal("invalid date accepted")
	}
	_, err = client.UpdateTransaction(context.Background(), "last-used", "tx-1", TransactionFields{})
	if err == nil {
		t.Fatal("empty update accepted")
	}
	_, err = client.ListTransactions(context.Background(), "last-used", TransactionQuery{Type: "invalid"})
	if err == nil {
		t.Fatal("invalid transaction filter accepted")
	}
	if called {
		t.Fatal("invalid request reached API")
	}
}

func TestAPIErrorHidesToken(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`{"error":{"name":"too_many_requests","detail":"Too many requests"}}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("secret-token", httpClient, "https://api.example.test/v1")
	_, err := client.ListPlans(context.Background())
	if err == nil || !strings.Contains(err.Error(), "YNAB 429 too_many_requests") {
		t.Fatalf("wrong error: %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatal("token exposed in error")
	}
}
