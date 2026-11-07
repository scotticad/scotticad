// Package mcp serves ScottiCAD to AI assistants over the Model Context
// Protocol: JSON-RPC 2.0 on stdin and stdout, one message per line. The
// tools are the automation API's methods (see docs/api.md), listed from
// its describe reply and forwarded to a Backend: the editor in the same
// process (scotticad --mcp) or a ScottiCAD's --api socket
// (scotticad --mcp-connect, see Remote).
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"
)

// Backend carries API calls: params in, the reply's result as JSON out.
type Backend interface {
	Call(method string, params map[string]any) (json.RawMessage, error)
}

// Method and Param are an API method as describe lists it.
type Method struct {
	Name   string  `json:"name"`
	Doc    string  `json:"doc"`
	Params []Param `json:"params"`
}

type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Doc      string `json:"doc"`
	Optional bool   `json:"optional,omitempty"`
}

// Options configure Serve.
type Options struct {
	Version string // ScottiCAD's, for serverInfo
	Tools   string // the tools listed: "full" (the default) or "core"
	// Fallback is a describe reply to list the tools from while the
	// backend can't answer (connect mode, ScottiCAD not started yet).
	Fallback []byte
	// Timeout bounds every call but run_script's (which has Stop); 0 is
	// DefaultTimeout.
	Timeout time.Duration
	Log     io.Writer // diagnostics; nil discards them
}

// DefaultTimeout is how long a call may take before the assistant is told
// ScottiCAD is busy.
const DefaultTimeout = 60 * time.Second

// protocolVersions are the MCP revisions spoken, oldest first.
var protocolVersions = []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}

type server struct {
	b   Backend
	o   Options
	out io.Writer
	wmu sync.Mutex // one message at a time on out

	mu       sync.Mutex
	version  string           // the negotiated protocol revision
	methods  []Method         // describe's reply, once the backend has given it
	inflight map[string]*call // requests being handled, by their JSON id
	wg       sync.WaitGroup
}

type call struct {
	tool      string // a tools/call's tool
	cancelled bool
	sent      bool          // a run_script passed on to ScottiCAD
	done      chan struct{} // closed once the request has been handled
}

// Serve answers MCP messages from in on out until in ends (the client has
// gone). A script still running then is stopped, and Serve returns once
// every call in progress has finished.
func Serve(in io.Reader, out io.Writer, b Backend, o Options) error {
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Tools == "" {
		o.Tools = "full"
	}
	if o.Tools != "full" && o.Tools != "core" {
		return fmt.Errorf("the tools are full or core, not %q", o.Tools)
	}
	s := &server{b: b, o: o, out: out, version: protocolVersions[len(protocolVersions)-1], inflight: map[string]*call{}}
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			s.handleLine(line)
		}
		if err != nil {
			s.stopScripts()
			s.wg.Wait()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (s *server) logf(f string, a ...any) {
	if s.o.Log != nil {
		fmt.Fprintf(s.o.Log, "scotticad mcp: "+f+"\n", a...)
	}
}

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

func errorf(code int, f string, a ...any) *rpcError {
	return &rpcError{code, fmt.Sprintf(f, a...)}
}

func (s *server) handleLine(line []byte) {
	var m message
	if line[0] == '[' {
		s.send(nil, nil, errorf(-32600, "batches aren't supported"))
		return
	}
	if err := json.Unmarshal(line, &m); err != nil {
		s.send(nil, nil, errorf(-32700, "parse error: %v", err))
		return
	}
	if m.Method == "" {
		return // a reply: this server sends no requests
	}
	if len(m.ID) == 0 || string(m.ID) == "null" {
		s.notification(m)
		return
	}
	key := string(m.ID)
	c := &call{done: make(chan struct{})}
	if m.Method == "tools/call" {
		var p struct {
			Name string `json:"name"`
		}
		json.Unmarshal(m.Params, &p)
		c.tool = p.Name
	}
	s.mu.Lock()
	if s.inflight[key] != nil {
		s.mu.Unlock()
		s.send(m.ID, nil, errorf(-32600, "request id %s is already in use", key))
		return
	}
	s.inflight[key] = c
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		res, err := s.request(m, c)
		s.mu.Lock()
		delete(s.inflight, key)
		cancelled := c.cancelled
		s.mu.Unlock()
		close(c.done)
		if !cancelled { // a cancelled request gets no reply
			s.send(m.ID, res, err)
		}
	}()
}

func (s *server) notification(m message) {
	switch m.Method {
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		json.Unmarshal(m.Params, &p)
		// marked here, not in the goroutine: a run_script read next must
		// see it and wait for this one's script to stop
		s.mu.Lock()
		c := s.inflight[string(p.RequestID)]
		if c != nil {
			c.cancelled = true
		}
		s.mu.Unlock()
		if c != nil {
			go s.cancel(c)
		}
	}
	// notifications/initialized and the rest need nothing
}

// cancel abandons a request: it gets no reply, and a run_script's script
// is stopped.
func (s *server) cancel(c *call) {
	s.mu.Lock()
	c.cancelled = true
	sent := c.sent
	s.mu.Unlock()
	// Not yet sent, it never will be. stop_script stops whatever script is
	// running, which would then be another one, or the user's.
	if c.tool != "run_script" || !sent {
		return
	}
	// The script may not have started yet (its call is queued for the UI
	// thread), when stop_script finds nothing to stop: ask again until it
	// does, or the call ends (runScript doesn't start a cancelled one).
	for {
		var res struct {
			Stopped bool `json:"stopped"`
		}
		r, err := s.b.Call("stop_script", nil)
		if err != nil || (json.Unmarshal(r, &res) == nil && res.Stopped) {
			return
		}
		select {
		case <-c.done:
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// awaitCancelledScripts waits for the cancelled run_scripts still in
// progress to end (ScottiCAD runs one script at a time, and a stop takes
// effect in its own time), then reports whether c was cancelled meanwhile.
// If it wasn't, c is marked sent: the caller passes it on.
func (s *server) awaitCancelledScripts(c *call) bool {
	s.mu.Lock()
	var stopping []chan struct{}
	for _, o := range s.inflight {
		if o != c && o.tool == "run_script" && o.cancelled {
			stopping = append(stopping, o.done)
		}
	}
	s.mu.Unlock()
	for _, done := range stopping {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c.sent = !c.cancelled
	return c.cancelled
}

// stopScripts stops the run_scripts in progress when the client goes.
func (s *server) stopScripts() {
	s.mu.Lock()
	var scripts []*call
	for _, c := range s.inflight {
		if c.tool == "run_script" {
			scripts = append(scripts, c)
		}
	}
	s.mu.Unlock()
	for _, c := range scripts {
		s.cancel(c)
	}
}

func (s *server) send(id json.RawMessage, result any, err error) {
	resp := map[string]any{"jsonrpc": "2.0", "id": id} // a nil id marshals as null
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = &rpcError{-32603, err.Error()}
		}
		resp["error"] = re
	} else {
		if result == nil {
			result = map[string]any{}
		}
		resp["result"] = result
	}
	b, merr := json.Marshal(resp)
	if merr != nil {
		b, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": resp["id"],
			"error": rpcError{-32603, "the reply can't be sent: " + merr.Error()}})
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if _, err := s.out.Write(append(b, '\n')); err != nil {
		s.logf("writing a reply: %v", err)
	}
}

func (s *server) request(m message, c *call) (any, error) {
	switch m.Method {
	case "initialize":
		return s.initialize(m.Params), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools, err := s.tools()
		if err != nil {
			return nil, err
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, errorf(-32602, "tools/call params: %v", err)
		}
		return s.callTool(c, p.Name, p.Arguments)
	case "resources/list":
		return map[string]any{"resources": resourceList}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(m.Params, &p)
		return s.readResource(p.URI)
	}
	return nil, errorf(-32601, "method not found: %s", m.Method)
}

func (s *server) initialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(params, &p)
	v := protocolVersions[len(protocolVersions)-1]
	if slices.Contains(protocolVersions, p.ProtocolVersion) {
		v = p.ProtocolVersion
	}
	s.mu.Lock()
	s.version = v
	s.mu.Unlock()
	return map[string]any{
		"protocolVersion": v,
		"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
		"serverInfo":      map[string]any{"name": "scotticad", "version": s.o.Version},
		"instructions":    instructions,
	}
}

// atLeast reports whether the negotiated revision is v or later (the
// revision strings are dates, so they compare as strings).
func (s *server) atLeast(v string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version >= v
}

// call runs one API call, giving up on the reply after timeout (0: wait
// for as long as it takes). The call itself carries on in ScottiCAD.
func (s *server) call(method string, p map[string]any, timeout time.Duration) (json.RawMessage, error) {
	type reply struct {
		r   json.RawMessage
		err error
	}
	ch := make(chan reply, 1)
	go func() {
		r, err := s.b.Call(method, p)
		ch <- reply{r, err}
	}()
	if timeout <= 0 {
		x := <-ch
		return x.r, x.err
	}
	select {
	case x := <-ch:
		return x.r, x.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("ScottiCAD didn't answer within %v: it may be busy, or waiting for an answer to a dialog in its window", timeout)
	}
}

// listMethods is describe's reply: from the backend once it has answered,
// else the fallback.
func (s *server) listMethods() ([]Method, error) {
	s.mu.Lock()
	ms := s.methods
	s.mu.Unlock()
	if ms != nil {
		return ms, nil
	}
	r, err := s.call("describe", nil, 10*time.Second)
	if err == nil {
		if err = json.Unmarshal(r, &ms); err == nil {
			s.mu.Lock()
			s.methods = ms
			s.mu.Unlock()
			s.checkVersion()
			return ms, nil
		}
	}
	if s.o.Fallback == nil {
		return nil, err
	}
	s.logf("listing the tools this build knows: %v", err)
	if err := json.Unmarshal(s.o.Fallback, &ms); err != nil {
		return nil, err
	}
	return ms, nil
}

// checkVersion warns when the ScottiCAD answering isn't this build.
func (s *server) checkVersion() {
	r, err := s.call("version", nil, 10*time.Second)
	var v string
	if err == nil && json.Unmarshal(r, &v) == nil && s.o.Version != "" && v != s.o.Version {
		s.logf("connected to ScottiCAD %s; this is %s, so the tools are the running one's", v, s.o.Version)
	}
}
