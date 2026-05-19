package httpadapter

import (
	"errors"
	"net"
	"sync"
)

// errOneShotDone signals that the single connection has already been served.
var errOneShotDone = errors.New("httpadapter: one-shot listener exhausted")

// oneShotListener yields exactly one accepted net.Conn — the connection passed
// to ServeConnection — then returns errOneShotDone. http.Server uses Accept in
// a loop; this lets us reuse the standard server machinery for a single conn.
type oneShotListener struct {
	once sync.Once
	conn net.Conn
	done chan struct{}
	mu   sync.Mutex
}

func (l *oneShotListener) Accept() (net.Conn, error) {
	var conn net.Conn
	l.once.Do(func() {
		conn = l.conn
	})
	if conn != nil {
		return conn, nil
	}
	<-l.done
	return nil, errOneShotDone
}

func (l *oneShotListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

func (l *oneShotListener) Addr() net.Addr {
	if l.conn != nil {
		return l.conn.LocalAddr()
	}
	return &net.TCPAddr{}
}
