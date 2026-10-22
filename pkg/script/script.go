// Package script runs Starlark (a Python dialect) scripts against
// ScottiCAD. Scripts reach the design only through the cad object, whose
// methods mirror the Python client (see Methods); every call goes to the
// automation API, so a script can do what the Python client can do.
//
// Starlark has no file, network or system access of its own; besides
// Starlark's built-in functions (print, len, range, ...), a script sees
// only what the session predeclares: cad, math, json, struct and help.
package script

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"strings"
	"sync"

	starjson "go.starlark.net/lib/json"
	starmath "go.starlark.net/lib/math"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// Caller sends one automation API call: params and the reply are JSON, as
// on the API's socket, so a script sees exactly what the Python client
// sees.
type Caller func(method string, params []byte) ([]byte, error)

// Kind is the kind of a line of output.
type Kind int

const (
	Print  Kind = iota // what print wrote
	Result             // the value of an expression typed at the prompt
	Error              // an error, with its traceback
	Info               // a note from the console itself
	Input              // what was typed at the prompt (echoed)
)

// Line is one line of output. File and Line, when set, point at the
// script line an error came from.
type Line struct {
	Kind Kind
	Text string
	File string
	Line int
}

// options enables what standard Starlark leaves out: while loops,
// recursion, sets, and top-level statements and reassignment, as in a
// Python file or prompt. Runaway loops are stopped by Session.Stop and the
// step limit instead.
var options = &syntax.FileOptions{Set: true, While: true, TopLevelControl: true, GlobalReassign: true, Recursion: true}

// Session is a console session: its prompt's variables persist between
// inputs. One script runs at a time.
type Session struct {
	call Caller
	out  func(Line)
	// MaxSteps, if non-zero, stops a run after that many steps.
	MaxSteps uint64
	// Doc, if set, returns an API method's description (for help).
	Doc func(api string) string

	cad     *starlarkstruct.Module
	globals starlark.StringDict

	mu     sync.Mutex
	thread *starlark.Thread // the running thread, or nil
	// base is the running script's folder: relative paths given to cad
	// methods resolve against it, as the Python client resolves them
	// against its working directory
	base string
}

// NewSession makes a session whose cad object calls through call and
// whose output goes to out (called from the running script's goroutine).
func NewSession(call Caller, out func(Line)) *Session {
	s := &Session{call: call, out: out, globals: starlark.StringDict{}}
	members := starlark.StringDict{}
	for i := range Methods {
		m := &Methods[i]
		members[m.Name] = starlark.NewBuiltin(m.Name, func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
			return s.callMethod(m, args, kw)
		})
	}
	members["call"] = starlark.NewBuiltin("call", s.callAny)
	s.cad = &starlarkstruct.Module{Name: "cad", Members: members}
	return s
}

// Predeclared are the names every script starts with.
func (s *Session) Predeclared() starlark.StringDict {
	return starlark.StringDict{
		"cad":    s.cad,
		"math":   starmath.Module,
		"json":   starjson.Module,
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"help":   starlark.NewBuiltin("help", s.help),
	}
}

// ErrBusy is returned when a script is already running.
var ErrBusy = errors.New("a script is already running")

// Stats describe a finished run.
type Stats struct {
	Steps uint64
}

func (s *Session) begin(name, base string) (*starlark.Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.thread != nil {
		return nil, ErrBusy
	}
	s.base = base
	th := &starlark.Thread{Name: name, Print: func(_ *starlark.Thread, msg string) { s.out(Line{Kind: Print, Text: msg}) }}
	if s.MaxSteps > 0 {
		th.SetMaxExecutionSteps(s.MaxSteps)
	}
	s.thread = th
	return th, nil
}

func (s *Session) end() {
	s.mu.Lock()
	s.thread = nil
	s.mu.Unlock()
}

// Running reports whether a script is running.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.thread != nil
}

// Stop cancels the running script; it stops at its next step (a cad call
// in progress finishes first).
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.thread != nil {
		s.thread.Cancel("stopped")
	}
}

// Prompt runs what was typed at the console prompt. Its variables persist
// in the session; the value of a lone expression is shown (and kept in
// _), as at a Python prompt. Relative paths resolve against base.
func (s *Session) Prompt(src, base string) (Stats, error) {
	th, err := s.begin("console", base)
	if err != nil {
		return Stats{}, err
	}
	defer s.end()
	err = s.prompt(th, src)
	if err != nil {
		s.report(err, "<console>")
	}
	return Stats{Steps: th.ExecutionSteps()}, err
}

func (s *Session) prompt(th *starlark.Thread, src string) error {
	for k, v := range s.Predeclared() {
		if _, ok := s.globals[k]; !ok {
			s.globals[k] = v
		}
	}
	f, err := options.Parse("<console>", src, 0)
	if err != nil {
		return err
	}
	if len(f.Stmts) == 1 {
		if es, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
			v, err := starlark.EvalExprOptions(options, th, es.X, s.globals)
			if err != nil {
				return err
			}
			s.globals["_"] = v
			if v != starlark.None {
				s.out(Line{Kind: Result, Text: v.String()})
			}
			return nil
		}
	}
	return starlark.ExecREPLChunk(f, th, s.globals)
}

// Run runs a whole script, as python runs a file: in its own fresh
// module, not the prompt's. Relative paths resolve against base.
func (s *Session) Run(name, src, base string) (Stats, error) {
	th, err := s.begin(name, base)
	if err != nil {
		return Stats{}, err
	}
	defer s.end()
	_, err = starlark.ExecFileOptions(options, th, name, src, s.Predeclared())
	if err != nil {
		s.report(err, name)
	}
	return Stats{Steps: th.ExecutionSteps()}, err
}

// Complete reports whether src typed at the prompt is ready to run, as a
// Python prompt decides whether Enter runs or continues: a block (a line
// ending in ":") needs an empty last line, and unclosed brackets or
// strings need more input.
func Complete(src string) bool {
	trimmed := strings.TrimRight(src, " \t")
	if strings.TrimSpace(trimmed) == "" {
		return true
	}
	lines := strings.Split(trimmed, "\n")
	// a block (any line opening one, not only the first) runs once an
	// empty line ends it, as at a Python prompt
	if opensBlock(trimmed) && strings.TrimSpace(lines[len(lines)-1]) != "" {
		return false
	}
	_, err := options.Parse("<console>", src, 0)
	if err == nil {
		return true
	}
	var se syntax.Error
	if errors.As(err, &se) {
		// the parser hit the end of the input wanting more
		end := strings.Count(src, "\n") + 1
		if strings.Contains(se.Msg, "unexpected EOF") || strings.Contains(se.Msg, "unexpected end of file") ||
			strings.Contains(se.Msg, "unclosed") || strings.Contains(se.Msg, "got end of file") ||
			(strings.Contains(se.Msg, "got outdent") && int(se.Pos.Line) >= end) {
			return false
		}
	}
	return true // a real syntax error: run it and show the error
}

// opensBlock reports whether a line of src ends in a colon outside
// brackets, strings and comments (a statement opening a block: def, for,
// if, else:, ...). A colon in a dict or a comment doesn't count.
func opensBlock(src string) bool {
	depth := 0
	var quote byte
	last := byte(0) // the last character outside strings and comments
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote || c == '\n' {
				quote = 0
			}
			continue
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			i--
			continue
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth = max(0, depth-1)
		case c == '\n':
			if last == ':' && depth == 0 {
				return true
			}
		}
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			last = c
		}
	}
	return last == ':' && depth == 0
}

// report writes an error as output lines: a traceback for a runtime
// error, the position for a syntax error. The Line points into file.
func (s *Session) report(err error, file string) {
	var ee *starlark.EvalError
	var se syntax.Error
	var rl resolve.ErrorList
	switch {
	case errors.As(err, &ee):
		line := 0
		for _, fr := range ee.CallStack {
			if fr.Pos.Filename() == file {
				line = int(fr.Pos.Line)
			}
		}
		for _, l := range strings.Split(ee.Backtrace(), "\n") {
			s.out(Line{Kind: Error, Text: l, File: file, Line: line})
		}
	case errors.As(err, &se):
		s.out(Line{Kind: Error, Text: fmt.Sprintf("%s: syntax error: %s", se.Pos, se.Msg), File: file, Line: int(se.Pos.Line)})
	case errors.As(err, &rl):
		for _, e := range rl {
			s.out(Line{Kind: Error, Text: fmt.Sprintf("%s: %s", e.Pos, e.Msg), File: file, Line: int(e.Pos.Line)})
		}
	default:
		s.out(Line{Kind: Error, Text: "Error: " + err.Error(), File: file})
	}
}

// --- the cad object -----------------------------------------------------

// callMethod binds a call's arguments to m's parameters (positional, then
// keywords, then defaults), calls the API and shapes the reply as the
// Python client does.
func (s *Session) callMethod(m *Method, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	if len(args) > len(m.Params) {
		return nil, fmt.Errorf("%s() takes at most %d arguments (%d given)", m.Name, len(m.Params), len(args))
	}
	given := make([]starlark.Value, len(m.Params))
	copy(given, args)
	for _, kv := range kw {
		name := string(kv[0].(starlark.String))
		i := -1
		for j, prm := range m.Params {
			if prm.Name == name {
				i = j
			}
		}
		if i < 0 {
			return nil, fmt.Errorf("%s() got an unexpected keyword argument %q", m.Name, name)
		}
		if given[i] != nil {
			return nil, fmt.Errorf("%s() got multiple values for argument %q", m.Name, name)
		}
		given[i] = kv[1]
	}
	params := map[string]any{}
	for i, prm := range m.Params {
		var v any
		if given[i] == nil {
			if prm.Required {
				return nil, fmt.Errorf("%s() missing argument %q", m.Name, prm.Name)
			}
			v = prm.Default
		} else {
			var err error
			if v, err = toGo(given[i]); err != nil {
				return nil, fmt.Errorf("%s(): %s: %v", m.Name, prm.Name, err)
			}
		}
		if v == nil {
			continue // None is left out, as the Python client does
		}
		// as the Python client: an empty optional path, or an empty camera
		// target, is no value (it tests "if path", "if target")
		if s, ok := v.(string); ok && s == "" && prm.Path && !prm.Required {
			continue
		}
		if l, ok := v.([]any); ok && len(l) == 0 && prm.Name == "target" {
			continue
		}
		if prm.Path {
			v = s.abs(v)
		}
		params[prm.Name] = v
	}
	if m.special == "handle" {
		return s.handle(params)
	}
	res, err := s.callAPI(m.API, params)
	if err != nil {
		return nil, err
	}
	switch {
	case m.None:
		return starlark.None, nil
	case m.special == "list":
		if res == starlark.None {
			return starlark.NewList(nil), nil
		}
	case m.special == "xy":
		x, _, _ := field(res, "x")
		y, _, _ := field(res, "y")
		return starlark.Tuple{x, y}, nil
	case m.Result != "":
		v, ok, err := field(res, m.Result)
		if err != nil || !ok {
			return nil, fmt.Errorf("%s: the reply has no %q", m.API, m.Result)
		}
		return v, nil
	}
	return res, nil
}

// callAny is cad.call(method, **params): any API method by name.
func (s *Session) callAny(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var method string
	if err := starlark.UnpackPositionalArgs("call", args, nil, 1, &method); err != nil {
		return nil, err
	}
	params := map[string]any{}
	for _, kv := range kw {
		v, err := toGo(kv[1])
		if err != nil {
			return nil, fmt.Errorf("call(): %s: %v", kv[0], err)
		}
		if v != nil {
			params[string(kv[0].(starlark.String))] = v
		}
	}
	return s.callAPI(method, params)
}

// handle is cad.handle(kind, index): the on-screen position of a
// selection handle.
func (s *Session) handle(params map[string]any) (starlark.Value, error) {
	res, err := s.callAPI("list_handles", map[string]any{})
	if err != nil {
		return nil, err
	}
	kind, _ := params["kind"].(string)
	index := fmt.Sprint(params["index"])
	if l, ok := res.(*starlark.List); ok {
		for i := 0; i < l.Len(); i++ {
			k, _, _ := field(l.Index(i), "kind")
			n, _, _ := field(l.Index(i), "index")
			if ks, ok := k.(starlark.String); ok && string(ks) == kind && n != nil && n.String() == index {
				x, _, _ := field(l.Index(i), "x")
				y, _, _ := field(l.Index(i), "y")
				return starlark.Tuple{x, y}, nil
			}
		}
	}
	return nil, fmt.Errorf("no %s handle %s", kind, index)
}

func (s *Session) callAPI(method string, params map[string]any) (starlark.Value, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	res, err := s.call(method, b)
	if err != nil {
		return nil, err
	}
	return fromJSON(res)
}

func (s *Session) abs(v any) any {
	p, ok := v.(string)
	if !ok || p == "" || filepath.IsAbs(p) {
		return v
	}
	base := s.base
	if base == "" {
		if a, err := filepath.Abs(p); err == nil {
			return a
		}
		return p
	}
	return filepath.Join(base, p)
}

// field returns d[key] for a Starlark dict d.
func field(d starlark.Value, key string) (starlark.Value, bool, error) {
	dict, ok := d.(*starlark.Dict)
	if !ok {
		return starlark.None, false, nil
	}
	return dict.Get(starlark.String(key))
}

// --- values ---------------------------------------------------------------

// toGo converts a Starlark value to JSON-ready Go data. Like the Python
// client's json.dumps, it takes lists, tuples, dicts with string keys,
// strings, numbers, bools and None; any other iterable (a range, a set)
// becomes a list.
func toGo(v starlark.Value) (any, error) {
	n := 0
	return toGoLimited(v, 0, &n)
}

// Limits on what a script can pass: a list that contains itself would
// otherwise recurse until the program dies, and a huge range would be
// built in full with no way to stop it.
const (
	maxDepth  = 64
	maxValues = 1_000_000
)

func toGoLimited(v starlark.Value, depth int, count *int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("nested too deeply (does a list or dict contain itself?)")
	}
	if *count++; *count > maxValues {
		return nil, fmt.Errorf("too many values (more than %d)", maxValues)
	}
	switch v := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Int:
		if i, ok := v.Int64(); ok {
			return i, nil
		}
		return json.Number(v.String()), nil
	case starlark.Float:
		if f := float64(v); math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%v isn't a number ScottiCAD can use", v)
		}
		return float64(v), nil
	case starlark.String:
		return string(v), nil
	case *starlark.Dict:
		out := map[string]any{}
		for _, kv := range v.Items() {
			k, ok := kv[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings, not %s", kv[0].Type())
			}
			x, err := toGoLimited(kv[1], depth+1, count)
			if err != nil {
				return nil, err
			}
			out[string(k)] = x
		}
		return out, nil
	case starlark.Iterable:
		out := []any{}
		it := v.Iterate()
		defer it.Done()
		var x starlark.Value
		for it.Next(&x) {
			g, err := toGoLimited(x, depth+1, count)
			if err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		return out, nil
	}
	return nil, fmt.Errorf("can't pass a %s to ScottiCAD", v.Type())
}

// fromJSON converts a JSON reply to Starlark values the way Python's
// json.loads does: objects become dicts in their key order, numbers
// without a fraction or exponent become ints.
func fromJSON(b []byte) (starlark.Value, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, fmt.Errorf("bad reply: %v", err)
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (starlark.Value, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := t.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(t), nil
	case string:
		return starlark.String(t), nil
	case json.Number:
		s := string(t)
		if !strings.ContainsAny(s, ".eE") {
			if i, ok := new(big.Int).SetString(s, 10); ok {
				return starlark.MakeBigInt(i), nil
			}
		}
		f, err := t.Float64()
		return starlark.Float(f), err
	case json.Delim:
		switch t {
		case '[':
			var elems []starlark.Value
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				elems = append(elems, v)
			}
			_, err := dec.Token()
			return starlark.NewList(elems), err
		case '{':
			d := starlark.NewDict(0)
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				d.SetKey(starlark.String(k.(string)), v)
			}
			_, err := dec.Token()
			return d, err
		}
	}
	return nil, fmt.Errorf("unexpected %v", t)
}

// Signature is how a cad method is called, e.g.
// add_shape(kind, x=0, y=0, z=None, hole=False).
func Signature(m *Method) string {
	var ps []string
	for _, prm := range m.Params {
		if prm.Required {
			ps = append(ps, prm.Name)
			continue
		}
		v := "None"
		if prm.Default != nil {
			b, _ := json.Marshal(prm.Default)
			v = string(b)
			switch prm.Default {
			case true:
				v = "True"
			case false:
				v = "False"
			}
			if _, ok := prm.Default.([]any); ok {
				v = "()"
			}
		}
		ps = append(ps, prm.Name+"="+v)
	}
	return m.Name + "(" + strings.Join(ps, ", ") + ")"
}

// help is help(): the cad methods, or help(cad.method) for one.
func (s *Session) help(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	say := func(f string, a ...any) { s.out(Line{Kind: Print, Text: fmt.Sprintf(f, a...)}) }
	if _, mod := firstArg(args).(*starlarkstruct.Module); len(args) == 0 || mod { // help() or help(cad)
		say("ScottiCAD scripts are Starlark, a dialect of Python: functions, loops,")
		say("lists, dicts and comprehensions as in Python; no classes, try/except or")
		say("import. Also available: math, json, struct and print.")
		say("")
		say("The design is reached through cad (the same methods as the Python client):")
		for i := range Methods {
			say("  cad.%s", Signature(&Methods[i]))
		}
		say("  cad.call(method, **params)  any API method by name; cad.describe() lists them")
		say("")
		say("Lengths are millimetres, angles degrees. \"The current plane\" is the")
		say("construction plane, if one is placed, otherwise the build plate.")
		say("")
		say("help(cad.add_shape) describes one method.")
		return starlark.None, nil
	}
	name := ""
	switch v := args[0].(type) {
	case *starlark.Builtin:
		name = v.Name()
	case starlark.String:
		name = strings.TrimPrefix(string(v), "cad.")
	}
	for i := range Methods {
		m := &Methods[i]
		if m.Name == name {
			say("cad.%s", Signature(m))
			if s.Doc != nil {
				if d := s.Doc(m.API); d != "" {
					say("  %s", d)
				}
			}
			return starlark.None, nil
		}
	}
	if name == "call" {
		say("cad.call(method, **params)")
		say("  Calls any automation API method by name; cad.describe() lists them.")
		return starlark.None, nil
	}
	say("%s", args[0].String())
	return starlark.None, nil
}

func firstArg(args starlark.Tuple) starlark.Value {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}
