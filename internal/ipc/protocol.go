// Package ipc implements the local-only JSON-RPC 2.0 server (Section 3.5)
// over a Unix domain socket. The IPC protocol is the only interface between
// the daemon and any client (the CLI, future MCP server, future GUI).
package ipc

import (
	"encoding/json"
)

// Request is a JSON-RPC 2.0 request frame (or notification when ID is null).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response frame. Either Result or Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the error object embedded in a Response.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 codes. Application codes start at -32000.
const (
	rpcCodeParseError     = -32700
	rpcCodeInvalidRequest = -32600
	rpcCodeMethodNotFound = -32601
	rpcCodeInvalidParams  = -32602
	rpcCodeInternal       = -32603
	rpcCodeApplication    = -32000
)
