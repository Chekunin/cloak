// Package errs defines the stable string error codes exchanged between the
// daemon and CLI over JSON-RPC. New codes must be added with care: the CLI and
// future MCP server branch on them.
package errs

import (
	"errors"
	"fmt"
)

const (
	CodeVaultLocked             = "vault_locked"
	CodeVaultNotInitialized     = "vault_not_initialized"
	CodeVaultAlreadyInitialized = "vault_already_initialized"
	CodeUnauthorized            = "unauthorized"
	CodeForbidden               = "forbidden"
	CodeNotFound                = "not_found"
	CodeInvalidRequest          = "invalid_request"
	CodeNameConflict            = "name_conflict"
	CodeAdapterError            = "adapter_error"
	CodeEndpointError           = "endpoint_error"
	CodeInternalError           = "internal_error"
)

// Coded is an error carrying a stable IPC error code plus an optional hint
// surfaced to the client.
type Coded struct {
	Code    string
	Message string
	Hint    string
	cause   error
}

func New(code, message string) *Coded {
	return &Coded{Code: code, Message: message}
}

func Newf(code, format string, args ...any) *Coded {
	return &Coded{Code: code, Message: fmt.Sprintf(format, args...)}
}

func Wrap(code string, err error) *Coded {
	if err == nil {
		return nil
	}
	return &Coded{Code: code, Message: err.Error(), cause: err}
}

func (c *Coded) Error() string {
	if c.Hint != "" {
		return fmt.Sprintf("%s: %s (%s)", c.Code, c.Message, c.Hint)
	}
	return fmt.Sprintf("%s: %s", c.Code, c.Message)
}

func (c *Coded) Unwrap() error { return c.cause }

// WithHint attaches a hint string returned to the client as error.data.hint.
func (c *Coded) WithHint(hint string) *Coded {
	c.Hint = hint
	return c
}

// As reports whether err is a *Coded with the given code.
func As(err error, code string) bool {
	var c *Coded
	if errors.As(err, &c) {
		return c.Code == code
	}
	return false
}

// Code returns the embedded code if err is a *Coded, otherwise CodeInternalError.
func Code(err error) string {
	var c *Coded
	if errors.As(err, &c) {
		return c.Code
	}
	if err == nil {
		return ""
	}
	return CodeInternalError
}
