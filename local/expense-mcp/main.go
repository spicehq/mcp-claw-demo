// expense-mcp — stdio MCP server that wraps the expense-api HTTP service.
//
// Spice supervises this process over stdio (see `expense_reports` in
// spicepod.yaml). It implements just enough of the MCP protocol
// (newline-delimited JSON-RPC 2.0) to expose three tools, each of which
// translates into an HTTP call against the expense-api:
//
//   list_expense_reports     →  GET  /reports
//   file_expense_report      →  POST /reports
//   approve_expense_report   →  POST /reports/{id}/approve
//
// EXPENSE_API_URL controls the upstream base URL. Defaults to
// http://127.0.0.1:8093 (where docker compose publishes the service).
//
// stdlib only — no module downloads required to run the demo.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	mcpProtocolVersion = "2024-11-05"
	serverName         = "expense-reports"
	serverVersion      = "0.1.0"
)

var (
	apiBase    string
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// --- JSON-RPC framing -----------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// --- MCP shapes -----------------------------------------------------------

type toolSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required,omitempty"`
}

type tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema toolSchema `json:"inputSchema"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// --- Tool catalog ---------------------------------------------------------

func tools() []tool {
	return []tool{
		{
			Name:        "list_expense_reports",
			Description: "List every expense report currently in the system, with status (filed / approved) and submitter.",
			InputSchema: toolSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
		{
			Name:        "file_expense_report",
			Description: "File a new expense report. Returns the created report including its assigned id.",
			InputSchema: toolSchema{
				Type: "object",
				Properties: map[string]any{
					"submitter":   map[string]any{"type": "string", "description": "Email of the employee filing the report."},
					"amount":      map[string]any{"type": "number", "description": "Amount in the report currency. Must be > 0."},
					"currency":    map[string]any{"type": "string", "description": "ISO 4217 currency code, e.g. \"USD\"."},
					"category":    map[string]any{"type": "string", "description": "Expense category, e.g. travel, software, meals."},
					"description": map[string]any{"type": "string", "description": "Free-text justification."},
				},
				Required: []string{"submitter", "amount", "currency"},
			},
		},
		{
			Name:        "approve_expense_report",
			Description: "Approve a previously-filed expense report by id. Returns an error if the report is already approved.",
			InputSchema: toolSchema{
				Type: "object",
				Properties: map[string]any{
					"id":       map[string]any{"type": "string", "description": "The report id returned by file_expense_report."},
					"approver": map[string]any{"type": "string", "description": "Email of the manager approving the report."},
				},
				Required: []string{"id", "approver"},
			},
		},
	}
}

// --- Tool implementations -------------------------------------------------

func callAPI(method, path string, body any) (string, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, apiBase+path, reqBody)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("expense-api %s %s → %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	// Trim trailing whitespace — the REST service uses json.NewEncoder
	// which appends a newline, and embedding that into the MCP `text`
	// content block forces clients to deal with control chars inside a
	// JSON-encoded string.
	return strings.TrimSpace(string(respBody)), nil
}

func handleToolCall(name string, args map[string]any) toolResult {
	var (
		text string
		err  error
	)
	switch name {
	case "list_expense_reports":
		text, err = callAPI("GET", "/reports", nil)
	case "file_expense_report":
		text, err = callAPI("POST", "/reports", args)
	case "approve_expense_report":
		id, _ := args["id"].(string)
		if id == "" {
			err = fmt.Errorf("missing required argument: id")
			break
		}
		payload := map[string]any{"approver": args["approver"]}
		text, err = callAPI("POST", "/reports/"+id+"/approve", payload)
	default:
		err = fmt.Errorf("unknown tool: %s", name)
	}
	if err != nil {
		return toolResult{
			Content: []contentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	return toolResult{Content: []contentBlock{{Type: "text", Text: text}}}
}

// --- JSON-RPC dispatch ----------------------------------------------------

func dispatch(req rpcRequest) *rpcResponse {
	// Notifications (no id) get no response.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    serverName,
					"version": serverVersion,
				},
			},
		}

	case "notifications/initialized", "initialized":
		return nil // notification

	case "tools/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools()},
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return &rpcResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &rpcError{Code: -32602, Message: "invalid params: " + err.Error()},
				}
			}
		}
		result := handleToolCall(params.Name, params.Arguments)
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	case "ping":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}

	default:
		if isNotification {
			return nil
		}
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func main() {
	apiBase = os.Getenv("EXPENSE_API_URL")
	if apiBase == "" {
		apiBase = "http://127.0.0.1:8093"
	}

	// Newline-delimited JSON-RPC over stdio (MCP stdio transport).
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := bytes.TrimSpace(in.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Best-effort error response — log and continue.
			fmt.Fprintf(os.Stderr, "expense-mcp: bad request: %v: %s\n", err, line)
			continue
		}
		resp := dispatch(req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "expense-mcp: encode error: %v\n", err)
			return
		}
		_ = out.Flush()
	}
	if err := in.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "expense-mcp: stdin error: %v\n", err)
	}
}
