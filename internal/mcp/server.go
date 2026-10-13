// Package mcp implements the stdio JSON-RPC portion of the MCP server.
// It has no knowledge of YNAB or HTTP.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"slices"
)

type Handler func(context.Context, json.RawMessage) (json.RawMessage, error)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handle      Handler
}

type Server struct {
	tools  []Tool
	byName map[string]Tool
}

func NewServer(tools []Tool) *Server {
	byName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	return &Server{tools: tools, byName: byName}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func rpcError(id json.RawMessage, code int, message string) any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
}

func rpcResult(id json.RawMessage, result any) any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

// Serve reads and writes one JSON-RPC message per line. Only responses go to output.
func (s *Server) Serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	writer := bufio.NewWriter(output)
	defer writer.Flush()
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if err := writeRPC(writer, rpcError(nil, -32700, "Invalid JSON")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}
		if err := writeRPC(writer, s.handle(context.Background(), req)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func writeRPC(w *bufio.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

func (s *Server) handle(ctx context.Context, req request) any {
	if req.JSONRPC != "2.0" {
		return rpcError(req.ID, -32600, "Expected JSON-RPC 2.0")
	}
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcError(req.ID, -32602, "Invalid initialize parameters")
		}
		version := params.ProtocolVersion
		if !slices.Contains([]string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}, version) {
			version = "2025-11-25"
		}
		return rpcResult(req.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "ynab-mcp", "version": "0.1.0"},
		})
	case "ping":
		return rpcResult(req.ID, map[string]any{})
	case "tools/list":
		listed := make([]any, 0, len(s.tools))
		for _, tool := range s.tools {
			listed = append(listed, map[string]any{
				"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema,
			})
		}
		return rpcResult(req.ID, map[string]any{"tools": listed})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return rpcError(req.ID, -32602, "Invalid tool call parameters")
		}
		tool, ok := s.byName[params.Name]
		if !ok {
			return rpcError(req.ID, -32602, "Unknown tool: "+params.Name)
		}
		data, err := tool.Handle(ctx, params.Arguments)
		if err != nil {
			return rpcResult(req.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true})
		}
		return rpcResult(req.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": string(data)}}})
	default:
		return rpcError(req.ID, -32601, "Method not found")
	}
}
