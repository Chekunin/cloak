package adapters

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
)

// ProxyStats accumulates bytes proxied in each direction.
type ProxyStats struct {
	BytesClientToUpstream int64
	BytesUpstreamToClient int64
}

// Proxy performs bidirectional byte copy between client and upstream. Returns
// when either side closes or ctx is cancelled. v2 will replace this with a
// message-aware loop; keeping it as a function call so adapters can swap it
// out without restructuring.
func Proxy(ctx context.Context, client, upstream net.Conn) (ProxyStats, error) {
	var stats ProxyStats
	errCh := make(chan error, 2)

	// client → upstream
	go func() {
		n, err := io.Copy(upstream, client)
		atomic.StoreInt64(&stats.BytesClientToUpstream, n)
		// half-close upstream so the other goroutine drains
		if cw, ok := upstream.(closeWriter); ok {
			_ = cw.CloseWrite()
		} else {
			_ = upstream.Close()
		}
		errCh <- err
	}()
	// upstream → client
	go func() {
		n, err := io.Copy(client, upstream)
		atomic.StoreInt64(&stats.BytesUpstreamToClient, n)
		if cw, ok := client.(closeWriter); ok {
			_ = cw.CloseWrite()
		} else {
			_ = client.Close()
		}
		errCh <- err
	}()

	select {
	case err1 := <-errCh:
		// wait for the second goroutine but don't block forever; closing the
		// conns above unblocks it.
		_ = client.Close()
		_ = upstream.Close()
		<-errCh
		if isBenign(err1) {
			return stats, nil
		}
		return stats, err1
	case <-ctx.Done():
		_ = client.Close()
		_ = upstream.Close()
		<-errCh
		<-errCh
		return stats, ctx.Err()
	}
}

type closeWriter interface {
	CloseWrite() error
}

func isBenign(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	// "use of closed network connection" — expected when one half-closes.
	return false
}
