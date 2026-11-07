package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Remote is the Backend of connect mode: it forwards calls to the API of a
// ScottiCAD started with --api ADDR. It connects when first called and
// again after ScottiCAD restarts, so the two can start in either order.
// The socket answers one call at a time, so a call in progress (a long
// script) gets a connection to itself, and the next one another.
type Remote struct {
	Addr string

	mu   sync.Mutex
	idle []*remoteConn
	id   atomic.Int64
}

type remoteConn struct {
	c net.Conn
	r *bufio.Reader
}

// CheckLoopback refuses an address other machines could serve: the API
// has no authentication, and is only ever served on loopback.
func CheckLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("%s isn't a loopback address (such as 127.0.0.1): the API is only for this machine", addr)
	}
	return nil
}

func (r *Remote) notRunning(err error) error {
	return fmt.Errorf("ScottiCAD isn't running with its API on %s (%v). Ask the user to start it with: scotticad --api %s", r.Addr, err, r.Addr)
}

// Call sends one call and waits for its reply.
func (r *Remote) Call(method string, params map[string]any) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	line, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": r.id.Add(1), "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	for {
		c, reused, err := r.get()
		if err != nil {
			return nil, r.notRunning(err)
		}
		reply, err := c.roundTrip(line)
		if err != nil {
			c.c.Close()
			// the kept connections all died with an earlier ScottiCAD: try
			// the next, until a new one
			if reused {
				continue
			}
			return nil, fmt.Errorf("the connection to ScottiCAD on %s broke: %v", r.Addr, err)
		}
		r.put(c)
		var m struct {
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(reply, &m); err != nil {
			return nil, fmt.Errorf("ScottiCAD's reply: %v", err)
		}
		if m.Error != nil {
			return nil, errors.New(m.Error.Message)
		}
		return m.Result, nil
	}
}

func (c *remoteConn) roundTrip(line []byte) ([]byte, error) {
	if _, err := c.c.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	return c.r.ReadBytes('\n')
}

// get returns an idle connection, or a new one; reused says which.
func (r *Remote) get() (*remoteConn, bool, error) {
	r.mu.Lock()
	if n := len(r.idle); n > 0 {
		c := r.idle[n-1]
		r.idle = r.idle[:n-1]
		r.mu.Unlock()
		return c, true, nil
	}
	r.mu.Unlock()
	c, err := net.DialTimeout("tcp", r.Addr, 2*time.Second)
	if err != nil {
		return nil, false, err
	}
	return &remoteConn{c, bufio.NewReaderSize(c, 1<<20)}, false, nil
}

func (r *Remote) put(c *remoteConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.idle) >= 4 {
		c.c.Close()
		return
	}
	r.idle = append(r.idle, c)
}
