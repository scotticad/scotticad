package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/app"
)

// fake is a Backend answering describe with the real method table, and
// every other call with fn's reply (default {"ok": true}).
type fake struct {
	mu    sync.Mutex
	calls []string
	fn    func(method string, p map[string]any) (json.RawMessage, error)
}

func (f *fake) Call(method string, p map[string]any) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	fn := f.fn
	f.mu.Unlock()
	if method == "describe" {
		return app.MethodTable()
	}
	if method == "version" {
		return json.RawMessage(`"test"`), nil
	}
	if fn != nil {
		return fn(method, p)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (f *fake) called(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == method {
			return true
		}
	}
	return false
}

// session runs Serve on pipes: send writes a message, next reads a reply.
type session struct {
	t    *testing.T
	in   *io.PipeWriter
	out  chan map[string]any
	done chan error
}

func start(t *testing.T, b Backend, o Options) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &session{t: t, in: inW, out: make(chan map[string]any, 16), done: make(chan error, 1)}
	go func() {
		err := Serve(inR, outW, b, o)
		outW.Close()
		s.done <- err
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("a reply that isn't JSON: %q", sc.Text())
				continue
			}
			s.out <- m
		}
		close(s.out)
	}()
	t.Cleanup(func() { inW.Close() })
	return s
}

func (s *session) send(id any, method string, params any) {
	s.t.Helper()
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		m["id"] = id
	}
	if params != nil {
		m["params"] = params
	}
	b, _ := json.Marshal(m)
	if _, err := s.in.Write(append(b, '\n')); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) next() map[string]any {
	s.t.Helper()
	select {
	case m, ok := <-s.out:
		if !ok {
			s.t.Fatal("the server stopped replying")
		}
		return m
	case <-time.After(10 * time.Second):
		s.t.Fatal("no reply")
	}
	return nil
}

// ask sends a request and returns its result, failing on an error reply.
func (s *session) ask(id any, method string, params any) map[string]any {
	s.t.Helper()
	s.send(id, method, params)
	m := s.next()
	if m["error"] != nil {
		s.t.Fatalf("%s: %v", method, m["error"])
	}
	r, _ := m["result"].(map[string]any)
	return r
}

func (s *session) init(version string) map[string]any {
	return s.ask(0, "initialize", map[string]any{"protocolVersion": version, "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"}})
}

func toolNames(r map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, t := range r["tools"].([]any) {
		t := t.(map[string]any)
		out[t["name"].(string)] = t
	}
	return out
}

func text(r map[string]any) string {
	var b strings.Builder
	for _, c := range r["content"].([]any) {
		if c := c.(map[string]any); c["type"] == "text" {
			b.WriteString(c["text"].(string))
		}
	}
	return b.String()
}

func TestInitializeNegotiatesTheVersion(t *testing.T) {
	s := start(t, &fake{}, Options{Version: "9.9"})
	r := s.init("2025-03-26")
	if r["protocolVersion"] != "2025-03-26" {
		t.Errorf("a version spoken is echoed, got %v", r["protocolVersion"])
	}
	if r["serverInfo"].(map[string]any)["version"] != "9.9" || r["instructions"] == "" {
		t.Errorf("serverInfo and instructions: %v", r)
	}
	s2 := start(t, &fake{}, Options{})
	if r := s2.init("1999-01-01"); r["protocolVersion"] != protocolVersions[len(protocolVersions)-1] {
		t.Errorf("an unknown version gets the newest, got %v", r["protocolVersion"])
	}
}

func TestEveryAPIMethodIsClassified(t *testing.T) {
	b, err := app.MethodTable()
	if err != nil {
		t.Fatal(err)
	}
	var ms []Method
	if err := json.Unmarshal(b, &ms); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, m := range ms {
		have[m.Name] = true
		if _, ok := toolSet[m.Name]; !ok {
			t.Errorf("API method %s isn't in toolSet: decide whether it's a tool", m.Name)
		}
		for _, p := range m.Params {
			if _, ok := schemas[p.Type]; !ok {
				t.Errorf("%s(%s): the API type %q has no JSON Schema", m.Name, p.Name, p.Type)
			}
		}
	}
	for name := range toolSet {
		if !have[name] {
			t.Errorf("toolSet has %s, which the API doesn't", name)
		}
	}
	for _, name := range append(regexp.MustCompile(`\b[a-z]+_[a-z_]+\b`).FindAllString(instructions, -1),
		regexp.MustCompile(`\b[a-z]+_[a-z_]+\b`).FindAllString(screenshotDoc, -1)...) {
		if toolSet[name] == never {
			t.Errorf("the instructions name %s, which isn't a tool", name)
		}
	}
}

func TestToolsListProfilesAndSchemas(t *testing.T) {
	s := start(t, &fake{}, Options{})
	s.init("2025-06-18")
	tools := toolNames(s.ask(1, "tools/list", nil))
	for _, name := range []string{"run_script", "screenshot", "add_shape", "set_bounds", "lay_flat", "undo"} {
		if tools[name] == nil {
			t.Errorf("full lists %s", name)
		}
	}
	for _, name := range []string{"click", "mouse_move", "quit", "describe", "stop_script", "answer_dialogs"} {
		if tools[name] != nil {
			t.Errorf("%s is never a tool", name)
		}
	}
	sb := tools["set_bounds"]["inputSchema"].(map[string]any)
	props := sb["properties"].(map[string]any)
	if props["min"].(map[string]any)["maxItems"] != float64(3) || props["ids"].(map[string]any)["type"] != "array" {
		t.Errorf("set_bounds' schema: %v", sb)
	}
	if req := fmt.Sprint(sb["required"]); req != "[min max]" {
		t.Errorf("set_bounds requires min and max, not %s", req)
	}
	ss := tools["screenshot"]["inputSchema"].(map[string]any)["properties"].(map[string]any)
	if ss["path"] != nil || ss["max_size"] == nil {
		t.Errorf("screenshot takes max_size, not a path: %v", ss)
	}
	an := tools["delete"]["annotations"].(map[string]any)
	if an["destructiveHint"] != true || tools["get_node"]["annotations"].(map[string]any)["readOnlyHint"] != true {
		t.Errorf("annotations: %v", an)
	}

	c := start(t, &fake{}, Options{Tools: "core"})
	c.init("2024-11-05")
	core := toolNames(c.ask(1, "tools/list", nil))
	if core["run_script"] == nil || core["lay_flat"] != nil || len(core) > 25 {
		t.Errorf("core lists %d tools: %v", len(core), core)
	}
	if core["run_script"]["annotations"] != nil {
		t.Error("no annotations before 2025-03-26")
	}
}

func TestToolCalls(t *testing.T) {
	f := &fake{fn: func(method string, p map[string]any) (json.RawMessage, error) {
		switch method {
		case "get_node":
			return json.RawMessage(fmt.Sprintf(`{"id":%v,"kind":"box"}`, p["id"])), nil
		case "delete":
			return nil, errors.New("no node 99")
		case "run_script":
			if strings.Contains(p["code"].(string), "bad") {
				return json.RawMessage(`{"output":"half","error":"<api>:1: bad"}`), nil
			}
			return json.RawMessage(`{"output":"made 9 holes","error":null}`), nil
		}
		return json.RawMessage(`{"ok":true}`), nil
	}}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	r := s.ask(1, "tools/call", map[string]any{"name": "get_node", "arguments": map[string]any{"id": 7}})
	if text(r) != `{"id":7,"kind":"box"}` || r["structuredContent"].(map[string]any)["kind"] != "box" || r["isError"] == true {
		t.Errorf("get_node: %v", r)
	}
	r = s.ask(2, "tools/call", map[string]any{"name": "delete", "arguments": map[string]any{"ids": []int{99}}})
	if r["isError"] != true || text(r) != "no node 99" {
		t.Errorf("a failing call is a tool error: %v", r)
	}
	r = s.ask(3, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "ok"}})
	if text(r) != "made 9 holes" || r["isError"] == true {
		t.Errorf("run_script: %v", r)
	}
	r = s.ask(4, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "bad"}})
	if r["isError"] != true || !strings.Contains(text(r), "<api>:1: bad") || !strings.Contains(text(r), "half") {
		t.Errorf("a script error: %v", r)
	}
	s.send(5, "tools/call", map[string]any{"name": "click", "arguments": map[string]any{}})
	if e := s.next()["error"]; e == nil || e.(map[string]any)["code"] != float64(-32602) {
		t.Errorf("a method that isn't a tool is an unknown tool: %v", e)
	}
}

func TestScreenshotIsAPictureScaledDown(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3200, 2000)))
	f := &fake{fn: func(method string, p map[string]any) (json.RawMessage, error) {
		if method != "screenshot" || len(p) != 0 {
			return nil, fmt.Errorf("unexpected %s %v", method, p)
		}
		return json.Marshal(map[string]any{"width": 1600, "height": 1000, "dpi": 2, "png": buf.Bytes()})
	}}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	r := s.ask(1, "tools/call", map[string]any{"name": "screenshot", "arguments": map[string]any{"max_size": 800}})
	img := r["content"].([]any)[0].(map[string]any)
	if img["type"] != "image" || img["mimeType"] != "image/png" {
		t.Fatalf("an image block: %v", img)
	}
	var data []byte
	json.Unmarshal([]byte(`"`+img["data"].(string)+`"`), &data)
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 800 || cfg.Height != 500 {
		t.Errorf("scaled to 800 × 500, got %v × %v (%v)", cfg.Width, cfg.Height, err)
	}
}

func TestProtocolEdges(t *testing.T) {
	s := start(t, &fake{}, Options{})
	s.init("2025-06-18")
	s.send(nil, "notifications/initialized", nil) // no reply
	s.send("a", "nonsense/method", nil)
	if m := s.next(); m["id"] != "a" || m["error"].(map[string]any)["code"] != float64(-32601) {
		t.Errorf("an unknown method: %v", m)
	}
	s.in.Write([]byte("{not json\n"))
	if m := s.next(); m["error"].(map[string]any)["code"] != float64(-32700) {
		t.Errorf("a parse error: %v", m)
	}
	if r := s.ask(9, "ping", nil); len(r) != 0 {
		t.Errorf("ping: %v", r)
	}
	r := s.ask(10, "resources/list", nil)
	if len(r["resources"].([]any)) != 3 {
		t.Errorf("resources: %v", r)
	}
	s.send(11, "resources/read", map[string]any{"uri": "scotticad://nothing"})
	if m := s.next(); m["error"] == nil {
		t.Errorf("an unknown resource: %v", m)
	}
}

func TestScriptAPIResource(t *testing.T) {
	f := &fake{fn: func(method string, p map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`[{"name":"add_shape","api":"add_shape","params":[{"name":"kind","required":true,"default":null},{"name":"x","default":0}]}]`), nil
	}}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	r := s.ask(1, "resources/read", map[string]any{"uri": "scotticad://script-api"})
	md := r["contents"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(md, "`cad.add_shape(kind, x=0)`: Place a catalogue shape") {
		t.Errorf("the reference: %s", md)
	}
}

func TestCancelStopsTheScriptAndPingStillAnswers(t *testing.T) {
	release := make(chan bool)
	var once sync.Once
	var running atomic.Int32 // ScottiCAD runs one script at a time
	f := &fake{}
	f.fn = func(method string, p map[string]any) (json.RawMessage, error) {
		switch method {
		case "run_script":
			defer running.Add(-1)
			if running.Add(1) > 1 {
				return nil, errors.New("a script is already running")
			}
			if p["code"] == "while True: pass" {
				<-release
				return json.RawMessage(`{"output":"","error":"stopped"}`), nil
			}
			return json.RawMessage(`{"output":"next","error":null}`), nil
		case "stop_script":
			if running.Load() == 0 {
				return json.RawMessage(`{"stopped":false}`), nil
			}
			once.Do(func() { close(release) })
			return json.RawMessage(`{"stopped":true}`), nil
		}
		return json.RawMessage(`{}`), nil
	}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	s.send(1, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "while True: pass"}})
	if r := s.ask(2, "ping", nil); r == nil {
		t.Error("ping during a script")
	}
	for !f.called("run_script") { // cancelled before that, it would never be sent
		time.Sleep(5 * time.Millisecond)
	}
	s.send(nil, "notifications/cancelled", map[string]any{"requestId": 1})
	if r := s.ask(3, "ping", nil); r == nil {
		t.Error("ping after cancelling")
	}
	// the next script waits for the cancelled one to stop
	if r := s.ask(4, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "x"}}); text(r) != "next" {
		t.Errorf("a script straight after cancelling: %v", r)
	}
	if !f.called("stop_script") {
		t.Error("cancelling run_script stops the script")
	}
}

func (f *fake) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == method {
			n++
		}
	}
	return n
}

// A run_script cancelled while it waits for a cancelled one to stop is
// never sent, and stops nothing: stop_script would stop whatever script is
// running.
func TestCancellingAWaitingScriptStopsNothing(t *testing.T) {
	release := make(chan bool)
	f := &fake{}
	f.fn = func(method string, p map[string]any) (json.RawMessage, error) {
		switch method {
		case "run_script":
			<-release // slow to stop
			return json.RawMessage(`{"output":"","error":"stopped"}`), nil
		case "stop_script":
			return json.RawMessage(`{"stopped":true}`), nil
		}
		return json.RawMessage(`{}`), nil
	}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	s.send(1, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "a"}})
	for !f.called("run_script") {
		time.Sleep(5 * time.Millisecond)
	}
	s.send(nil, "notifications/cancelled", map[string]any{"requestId": 1})
	for !f.called("stop_script") {
		time.Sleep(5 * time.Millisecond)
	}
	s.send(2, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "b"}})
	s.ask(3, "ping", nil) // request 2 is waiting by now
	s.send(nil, "notifications/cancelled", map[string]any{"requestId": 2})
	s.ask(4, "ping", nil)
	close(release)
	s.ask(5, "ping", nil)
	if n := f.count("stop_script"); n != 1 {
		t.Errorf("stop_script called %d times, want once", n)
	}
	if n := f.count("run_script"); n != 1 {
		t.Errorf("run_script sent %d times, want once", n)
	}
}

func TestADuplicateRequestIDIsRefused(t *testing.T) {
	release := make(chan bool)
	f := &fake{fn: func(method string, p map[string]any) (json.RawMessage, error) {
		<-release
		return json.RawMessage(`{"output":"done","error":null}`), nil
	}}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	s.send(1, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "a"}})
	for !f.called("run_script") {
		time.Sleep(5 * time.Millisecond)
	}
	s.send(1, "ping", nil)
	if m := s.next(); m["id"] != float64(1) || m["error"] == nil || m["error"].(map[string]any)["code"] != float64(-32600) {
		t.Errorf("a duplicate id: %v", m)
	}
	close(release)
	if m := s.next(); m["id"] != float64(1) || text(m["result"].(map[string]any)) != "done" {
		t.Errorf("the first request's reply: %v", m)
	}
}

func TestEOFStopsTheScriptAndReturns(t *testing.T) {
	release := make(chan bool)
	var once sync.Once
	f := &fake{}
	f.fn = func(method string, p map[string]any) (json.RawMessage, error) {
		switch method {
		case "run_script":
			<-release
		case "stop_script":
			once.Do(func() { close(release) })
		}
		return json.RawMessage(`{"output":"","error":null}`), nil
	}
	s := start(t, f, Options{})
	s.init("2025-06-18")
	s.send(1, "tools/call", map[string]any{"name": "run_script", "arguments": map[string]any{"code": "x"}})
	time.Sleep(50 * time.Millisecond)
	s.in.Close()
	select {
	case err := <-s.done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve didn't return when the client went")
	}
}

func TestTimeout(t *testing.T) {
	f := &fake{fn: func(method string, p map[string]any) (json.RawMessage, error) {
		time.Sleep(time.Second)
		return json.RawMessage(`{}`), nil
	}}
	s := start(t, f, Options{Timeout: 50 * time.Millisecond})
	s.init("2025-06-18")
	r := s.ask(1, "tools/call", map[string]any{"name": "undo", "arguments": map[string]any{}})
	if r["isError"] != true || !strings.Contains(text(r), "didn't answer") {
		t.Errorf("a call that takes too long: %v", r)
	}
}

func TestCheckLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{"127.0.0.1:47800": true, "localhost:1": true, "[::1]:5": true,
		"0.0.0.0:47800": false, "192.168.1.5:47800": false, "example.com:80": false, "nonsense": false} {
		if err := CheckLoopback(addr); (err == nil) != ok {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

// apiServer is a stand-in for ScottiCAD's --api socket.
func apiServer(t *testing.T, ln net.Listener) {
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					var m struct {
						ID     any            `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					json.Unmarshal(sc.Bytes(), &m)
					if m.Method == "slow" {
						time.Sleep(100 * time.Millisecond)
					}
					resp := map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"echo": m.Method}}
					if m.Method == "fail" {
						delete(resp, "result")
						resp["error"] = map[string]any{"code": -32000, "message": "it failed"}
					}
					b, _ := json.Marshal(resp)
					c.Write(append(b, '\n'))
				}
			}()
		}
	}()
}

func TestRemoteConnectsLateAndReconnects(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // not running yet
	r := &Remote{Addr: addr}
	if _, err := r.Call("undo", nil); err == nil || !strings.Contains(err.Error(), "scotticad --api "+addr) {
		t.Fatalf("not running: %v", err)
	}
	ln, err = net.Listen("tcp", addr)
	if err != nil {
		t.Skip("the port was taken meanwhile:", err)
	}
	apiServer(t, ln)
	if res, err := r.Call("undo", nil); err != nil || string(res) != `{"echo":"undo"}` {
		t.Fatalf("started later: %s %v", res, err)
	}
	if _, err := r.Call("fail", nil); err == nil || err.Error() != "it failed" {
		t.Errorf("an error reply: %v", err)
	}
	// a call during another gets a connection of its own; both are kept
	done := make(chan bool)
	go func() { r.Call("slow", nil); done <- true }()
	time.Sleep(20 * time.Millisecond)
	r.Call("undo", nil)
	<-done
	if n := len(r.idle); n != 2 {
		t.Errorf("%d idle connections, not 2", n)
	}
	// ScottiCAD restarts: the kept connections are dead, the next call redials
	ln.Close()
	r.mu.Lock()
	for _, c := range r.idle {
		c.c.Close()
	}
	r.mu.Unlock()
	ln, err = net.Listen("tcp", addr)
	if err != nil {
		t.Skip("the port was taken meanwhile:", err)
	}
	defer ln.Close()
	apiServer(t, ln)
	if res, err := r.Call("redo", nil); err != nil || string(res) != `{"echo":"redo"}` {
		t.Errorf("after a restart: %s %v", res, err)
	}
}

func TestFallbackListsTheToolsBeforeScottiCADStarts(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	fb, _ := app.MethodTable()
	s := start(t, &Remote{Addr: addr}, Options{Fallback: fb})
	s.init("2025-06-18")
	if tools := toolNames(s.ask(1, "tools/list", nil)); tools["run_script"] == nil {
		t.Errorf("the fallback's tools: %v", tools)
	}
	r := s.ask(2, "tools/call", map[string]any{"name": "undo", "arguments": map[string]any{}})
	if r["isError"] != true || !strings.Contains(text(r), "isn't running") {
		t.Errorf("a call while ScottiCAD isn't up: %v", r)
	}
}
