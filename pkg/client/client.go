// Package client is the public Go client library for the Cloak daemon.
//
// The CLI uses this. The future MCP server and GUI will use the same package
// — never bypass it from within those binaries. That makes pkg/client the
// only place RPC method names are mentioned outside the daemon, so adding a
// new method is a single-place change.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Client is a connection-bound JSON-RPC 2.0 client.
type Client struct {
	socketPath string
	mu         sync.Mutex
	conn       net.Conn
	enc        *json.Encoder
	dec        *bufio.Reader
	nextID     atomic.Int64
}

// Dial opens a fresh connection to socketPath. Call Authenticate before any
// other method requires authentication.
func Dial(ctx context.Context, socketPath string) (*Client, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("client: dial %s: %w", socketPath, err)
	}
	return &Client{
		socketPath: socketPath,
		conn:       conn,
		enc:        json.NewEncoder(conn),
		dec:        bufio.NewReaderSize(conn, 1<<16),
	}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Call performs an RPC and unmarshals the result into out (may be nil).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("client: closed")
	}

	id := c.nextID.Add(1)
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params}

	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(dl)
		defer c.conn.SetDeadline(time.Time{})
	}

	if err := c.enc.Encode(req); err != nil {
		return fmt.Errorf("client: write: %w", err)
	}
	line, err := c.dec.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("client: daemon closed the connection")
		}
		return fmt.Errorf("client: read: %w", err)
	}
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("client: bad response: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

// RPCError is a typed JSON-RPC error returned by Call.
type RPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if hint, _ := e.Data["hint"].(string); hint != "" {
		return fmt.Sprintf("%s: %s", e.Message, hint)
	}
	return e.Message
}

// AppCode returns the application-level error string (e.g. "vault_locked").
// Useful for branching on stable codes.
func (e *RPCError) AppCode() string { return e.Message }

// Is supports errors.Is matching by code string.
func (e *RPCError) Is(target error) bool {
	var other *RPCError
	if errors.As(target, &other) {
		return other.Message == e.Message
	}
	return false
}
