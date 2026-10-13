package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ynab-mcp/internal/ynab"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestToolCatalog(t *testing.T) {
	registered := New(ynab.NewClient("", nil, ""))
	want := []string{
		"list_plans", "get_plan", "list_accounts", "get_account", "list_categories", "get_category",
		"list_payees", "list_months", "get_month", "list_transactions", "list_account_transactions",
		"get_transaction", "create_transaction", "update_transaction", "delete_transaction",
	}
	required := [][]string{
		{}, {"plan_id"}, {"plan_id"}, {"plan_id", "account_id"}, {"plan_id"},
		{"plan_id", "category_id"}, {"plan_id"}, {"plan_id"}, {"plan_id", "month"},
		{"plan_id"}, {"plan_id", "account_id"}, {"plan_id", "transaction_id"},
		{"plan_id", "account_id", "date", "amount"}, {"plan_id", "transaction_id"},
		{"plan_id", "transaction_id"},
	}
	if len(registered) != len(want) {
		t.Fatalf("got %d tools, want %d", len(registered), len(want))
	}
	for index, tool := range registered {
		if tool.Name != want[index] {
			t.Errorf("tool %d: got %q, want %q", index, tool.Name, want[index])
		}
		if tool.InputSchema["type"] != "object" || tool.InputSchema["additionalProperties"] != false {
			t.Errorf("invalid schema for %s", tool.Name)
		}
		gotRequired, ok := tool.InputSchema["required"].([]string)
		if !ok {
			t.Errorf("required must be an array for %s", tool.Name)
		} else if strings.Join(gotRequired, ",") != strings.Join(required[index], ",") {
			t.Errorf("wrong required fields for %s: %v", tool.Name, gotRequired)
		}
		properties := tool.InputSchema["properties"].(map[string]any)
		for _, name := range required[index] {
			if _, exists := properties[name]; !exists {
				t.Errorf("%s schema missing %s", tool.Name, name)
			}
		}
	}
}

func TestWriteToolMapping(t *testing.T) {
	var got []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		entry := r.Method + " " + r.URL.RequestURI()
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			entry += " " + string(body)
		}
		got = append(got, entry)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"transaction":{"id":"tx-1"}}}`)), Header: make(http.Header)}, nil
	})}
	registered := New(ynab.NewClient("token", client, "https://api.example.test/v1"))
	cases := []struct {
		index int
		args  string
	}{
		{12, `{"plan_id":"last-used","account_id":"account-1","date":"2026-01-02","amount":-12500,"approved":false}`},
		{13, `{"plan_id":"last-used","transaction_id":"tx-1","memo":"Coffee"}`},
		{14, `{"plan_id":"last-used","transaction_id":"tx-1"}`},
	}
	for _, tc := range cases {
		if _, err := registered[tc.index].Handle(context.Background(), json.RawMessage(tc.args)); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 3 || !strings.HasPrefix(got[0], "POST /v1/plans/last-used/transactions ") ||
		!strings.HasPrefix(got[1], "PUT /v1/plans/last-used/transactions/tx-1 ") ||
		got[2] != "DELETE /v1/plans/last-used/transactions/tx-1" {
		t.Fatalf("wrong write requests: %v", got)
	}
	if !strings.Contains(got[0], `"amount":-12500`) || !strings.Contains(got[0], `"approved":false`) ||
		!strings.Contains(got[1], `"memo":"Coffee"`) {
		t.Fatalf("wrong write payloads: %v", got)
	}
	if _, err := registered[12].Handle(context.Background(), json.RawMessage(`{"plan_id":"last-used","account_id":"account-1","date":"2026-01-02","amount":1.5}`)); err == nil {
		t.Fatal("fractional milliunits accepted")
	}
}

func TestToolArgumentMapping(t *testing.T) {
	var got string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Method + " " + r.URL.RequestURI()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"transactions":[]}}`)), Header: make(http.Header)}, nil
	})}
	registered := New(ynab.NewClient("token", client, "https://api.example.test/v1"))
	tool := registered[9]
	data, err := tool.Handle(context.Background(), json.RawMessage(`{"plan_id":"last-used","since_date":"2026-01-01","type":"unapproved","last_knowledge_of_server":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != "GET /v1/plans/last-used/transactions?last_knowledge_of_server=2&since_date=2026-01-01&type=unapproved" {
		t.Fatalf("wrong request: %s", got)
	}
	if string(data) != `{"data":{"transactions":[]}}` {
		t.Fatalf("response changed: %s", data)
	}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"plan_id":"last-used","since_date":"bad"}`)); err == nil {
		t.Fatal("invalid date accepted")
	}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"plan_id":"last-used","unknown":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
