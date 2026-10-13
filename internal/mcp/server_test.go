package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestServeProtocol(t *testing.T) {
	server := NewServer([]Tool{{
		Name: "example", Description: "Example tool", InputSchema: map[string]any{"type": "object"},
		Handle: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"data":{"ok":true}}`), nil
		},
	}})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"example","arguments":{}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d responses, want 3", len(lines))
	}
	var initialize, listed, called map[string]any
	for index, target := range []*map[string]any{&initialize, &listed, &called} {
		if err := json.Unmarshal([]byte(lines[index]), target); err != nil {
			t.Fatal(err)
		}
	}
	if initialize["result"].(map[string]any)["protocolVersion"] != "2025-11-25" {
		t.Fatal("wrong protocol version")
	}
	if len(listed["result"].(map[string]any)["tools"].([]any)) != 1 {
		t.Fatal("wrong tool list")
	}
	content := called["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if content != `{"data":{"ok":true}}` {
		t.Fatalf("unexpected tool response: %s", content)
	}
}

func TestToolErrors(t *testing.T) {
	server := NewServer([]Tool{{
		Name: "fails", Handle: func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, errors.New("failed") },
	}})
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fails"}}`+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["result"].(map[string]any)["isError"] != true {
		t.Fatal("tool failure must use isError")
	}
}
