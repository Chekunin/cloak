package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/rs/zerolog"

	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/errs"
	"github.com/Chekunin/cloak/internal/store"
)

// HandlerFunc is invoked per RPC method.
type HandlerFunc func(ctx context.Context, sess *Session, params json.RawMessage) (any, error)

// Server is the JSON-RPC 2.0 server bound to a Unix socket.
type Server struct {
	socketPath string
	listener   net.Listener
	handlers   map[string]HandlerFunc
	requireAuth map[string]bool // method → must be authenticated
	store      *store.Store
	audit      *audit.Logger
	log        zerolog.Logger
	wg         sync.WaitGroup
}

// New constructs a Server.
func New(socketPath string, store *store.Store, audit *audit.Logger, log zerolog.Logger) *Server {
	return &Server{
		socketPath:  socketPath,
		handlers:    map[string]HandlerFunc{},
		requireAuth: map[string]bool{},
		store:       store,
		audit:       audit,
		log:         log,
	}
}

// Register adds a handler for method. If authRequired is true, the session
// must have completed `hello` successfully before invocation.
func (s *Server) Register(method string, h HandlerFunc, authRequired bool) {
	s.handlers[method] = h
	s.requireAuth[method] = authRequired
}

// Start binds the socket and begins accepting. Returns once the listener is
// ready; goroutines run until Stop is called or ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o700); err != nil {
		return fmt.Errorf("ipc: mkdir: %w", err)
	}
	// Best-effort: remove any stale socket file.
	if runtime.GOOS != "windows" {
		_ = os.Remove(s.socketPath)
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("ipc: listen: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("ipc: chmod: %w", err)
	}
	s.listener = ln
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	return nil
}

// Stop closes the listener and waits for accept loops to drain.
func (s *Server) Stop() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	if runtime.GOOS != "windows" {
		_ = os.Remove(s.socketPath)
	}
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.log.Warn().Err(err).Msg("ipc: accept")
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.serveConn(ctx, c)
		}(conn)
	}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	sess := &Session{conn: conn, pid: peerPID(conn)}
	br := bufio.NewReaderSize(conn, 1<<16)
	enc := json.NewEncoder(conn)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			s.handleLine(ctx, sess, line, enc)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				s.log.Debug().Err(err).Msg("ipc: read")
			}
			return
		}
	}
}

func (s *Server) handleLine(ctx context.Context, sess *Session, line []byte, enc *json.Encoder) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		_ = enc.Encode(Response{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: rpcCodeParseError, Message: "parse error"},
		})
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		_ = enc.Encode(errorResponse(req.ID, rpcCodeInvalidRequest, "invalid request", nil))
		return
	}
	h, ok := s.handlers[req.Method]
	if !ok {
		_ = enc.Encode(errorResponse(req.ID, rpcCodeMethodNotFound, "method not found: "+req.Method, nil))
		return
	}
	if s.requireAuth[req.Method] && !sess.IsAuthenticated() {
		_ = enc.Encode(errorResponse(req.ID, rpcCodeApplication, errs.CodeUnauthorized,
			mustMarshal(map[string]any{"hint": "send `hello` first"})))
		return
	}
	result, err := h(ctx, sess, req.Params)
	if err != nil {
		_ = enc.Encode(toRPCError(req.ID, err))
		return
	}
	resBytes, mErr := json.Marshal(result)
	if mErr != nil {
		_ = enc.Encode(errorResponse(req.ID, rpcCodeInternal, "marshal result: "+mErr.Error(), nil))
		return
	}
	_ = enc.Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: resBytes})
}

func errorResponse(id json.RawMessage, code int, message string, data json.RawMessage) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message, Data: data},
	}
}

func toRPCError(id json.RawMessage, err error) Response {
	var c *errs.Coded
	if errors.As(err, &c) {
		data := map[string]any{}
		if c.Hint != "" {
			data["hint"] = c.Hint
		}
		var raw json.RawMessage
		if len(data) > 0 {
			raw = mustMarshal(data)
		}
		return errorResponse(id, rpcCodeApplication, c.Code, raw)
	}
	return errorResponse(id, rpcCodeInternal, err.Error(), nil)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
