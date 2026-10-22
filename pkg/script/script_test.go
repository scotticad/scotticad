package script

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI records calls and answers a few methods like the real API.
type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	next  int
}

func (f *fakeAPI) call(method string, params []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+" "+string(params))
	switch method {
	case "add_shape":
		var p map[string]any
		json.Unmarshal(params, &p)
		if p["kind"] == "nonsense" {
			return nil, errors.New("unknown shape kind \"nonsense\"")
		}
		f.next++
		return json.Marshal(map[string]any{"id": f.next})
	case "get_node":
		return []byte(`{"id": 3, "size": [20, 20, 10.5], "name": "Box", "hole": false}`), nil
	case "project":
		return []byte(`{"x": 12.5, "y": 40}`), nil
	case "list_handles":
		return []byte(`[{"kind": "corner", "index": 0, "x": 1, "y": 2}, {"kind": "top", "index": 0, "x": 5, "y": 6}]`), nil
	case "notes":
		return []byte(`null`), nil
	case "save":
		return params, nil // {"path": ...}, as the API replies
	case "undo":
		return []byte(`{"done": true}`), nil
	}
	return []byte(`{"ok": true}`), nil
}

func session(t *testing.T) (*Session, *fakeAPI, func() []Line) {
	f := &fakeAPI{}
	var mu sync.Mutex
	var out []Line
	s := NewSession(f.call, func(l Line) { mu.Lock(); out = append(out, l); mu.Unlock() })
	return s, f, func() []Line { mu.Lock(); defer mu.Unlock(); return append([]Line(nil), out...) }
}

// Values a script can't pass are an error in the script, not the end of
// the program: a list that contains itself (endless recursion), a huge
// range (built with no way to stop it), NaN.
func TestUnpassableValues(t *testing.T) {
	for _, src := range []string{
		"l = []\nl.append(l)\ncad.select(l)",
		"d = {}\nd[\"a\"] = d\ncad.call(\"x\", p=d)",
		"cad.select(range(300000000))",
		"cad.move(dx=float(\"nan\"))",
	} {
		s, _, _ := session(t)
		done := make(chan error, 1)
		go func() { _, err := s.Run("t.star", src, ""); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%q ran", src)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: still running", src)
		}
	}
}

func TestArgumentsMatchThePythonClient(t *testing.T) {
	s, f, out := session(t)
	_, err := s.Run("t.star", `
b = cad.add_shape("box", x=10)
c = cad.add_shape("cylinder", 5, 6, hole=True)
cad.move(dz=3, ids=[b, c])
cad.set_size(x=50)
cad.select(range(1, 3))
cad.save("part.scotticad")
n = cad.node(3)
print(b, c, n["size"], type(n["size"][0]), type(n["size"][2]))
print(cad.project(1, 2, 3), cad.handle("top"), cad.notes(), cad.undo())
print(cad.call("set_snap", mm=0.5, ignored=None))
`, `C:\work`)
	if err != nil {
		t.Fatal(err, out())
	}
	want := []string{
		`add_shape {"hole":false,"kind":"box","x":10,"y":0}`,
		`add_shape {"hole":true,"kind":"cylinder","x":5,"y":6}`,
		`move {"dx":0,"dy":0,"dz":3,"ids":[1,2]}`,
		`set_size {"uniform":false,"x":50}`,
		`select {"ids":[1,2]}`,
		`save {"path":"C:\\work\\part.scotticad"}`,
		`get_node {"id":3}`,
		`project {"x":1,"y":2,"z":3}`,
		`list_handles {}`,
		`notes {}`,
		`undo {}`,
		`set_snap {"mm":0.5}`,
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s", strings.Join(f.calls, "\n"))
	}
	got := out()
	if len(got) != 3 || got[0].Text != `1 2 [20, 20, 10.5] int float` ||
		got[1].Text != `(12.5, 40) (5, 6) [] True` || got[2].Text != `{"ok": True}` {
		t.Fatalf("output %+v", got)
	}
}

func TestArgumentErrors(t *testing.T) {
	for src, msg := range map[string]string{
		`cad.add_shape()`:                `missing argument "kind"`,
		`cad.add_shape("box", colour=1)`: `unexpected keyword argument "colour"`,
		`cad.add_shape("box", 1, x=2)`:   `multiple values for argument "x"`,
		`cad.rotate("x", 1, None, 4)`:    `takes at most 3 arguments`,
		`cad.add_shape("nonsense")`:      `unknown shape kind`,
		`cad.set_color(cad.add_shape)`:   `can't pass a builtin_function_or_method`,
		`cad.handle("corner", 3)`:        `no corner handle 3`,
	} {
		s, _, out := session(t)
		if _, err := s.Run("t.star", src, ""); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: error %v, want %q", src, err, msg)
		}
		if ls := out(); len(ls) == 0 || ls[0].Kind != Error {
			t.Errorf("%s: no error output: %+v", src, ls)
		}
	}
}

func TestErrorPointsAtTheLine(t *testing.T) {
	s, _, out := session(t)
	s.Run("bracket.star", "x = 1\n\ndef f():\n    return cad.add_shape(\"nonsense\")\n\nf()\n", "")
	ls := out()
	last := ls[len(ls)-1]
	if last.Line != 4 || last.File != "bracket.star" || !strings.Contains(last.Text, "Error in add_shape: unknown shape kind") {
		t.Fatalf("%+v", ls)
	}
	s.Run("syntax.star", "x = 1\ny = (2\n", "")
	if l := out()[len(out())-1]; l.Kind != Error || l.Line == 0 || !strings.Contains(l.Text, "syntax error") {
		t.Fatalf("%+v", l)
	}
}

func TestPromptKeepsVariablesAndShowsValues(t *testing.T) {
	s, _, out := session(t)
	for _, src := range []string{"w = 6", "def area(h):\n    return w * h\n", "area(7)", "_ + 1", "x = None", "'mm'"} {
		if _, err := s.Prompt(src, ""); err != nil {
			t.Fatal(src, err)
		}
	}
	var res []string
	for _, l := range out() {
		res = append(res, l.Text)
	}
	if strings.Join(res, ",") != `42,43,"mm"` {
		t.Fatalf("%v", res)
	}
	// a script file runs in its own module, not the prompt's
	if _, err := s.Run("f.star", "print(w)", ""); err == nil {
		t.Fatal("a script saw the prompt's variables")
	}
}

func TestWhileRecursionAndLimits(t *testing.T) {
	s, _, out := session(t)
	if _, err := s.Run("t.star", `
i = 0
while i * i < 1000:
    i += 1
def fact(k):
    return 1 if k <= 1 else k * fact(k - 1)
print(i, fact(20), sorted(set([3, 1, 3])), math.sqrt(16), json.encode({"a": 1}))
`, ""); err != nil {
		t.Fatal(err, out())
	}
	if got := out()[0].Text; got != `32 2432902008176640000 [1, 3] 4.0 {"a":1}` {
		t.Fatal(got)
	}
	s.MaxSteps = 100000
	if _, err := s.Run("loop.star", "while True:\n    pass\n", ""); err == nil || !strings.Contains(err.Error(), "too many steps") {
		t.Fatal(err)
	}
	s.MaxSteps = 0
	done := make(chan error)
	go func() { _, err := s.Run("loop.star", "while True:\n    pass\n", ""); done <- err }()
	for !s.Running() {
		time.Sleep(time.Millisecond)
	}
	if _, err := s.Prompt("1", ""); !errors.Is(err, ErrBusy) {
		t.Fatalf("second run: %v", err)
	}
	s.Stop()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "stopped") {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop didn't stop the loop")
	}
}

func TestNoFileOrSystemAccess(t *testing.T) {
	s, _, _ := session(t)
	for _, src := range []string{`open("x")`, `load("os.star", "x")`, `import os`, `os.remove("x")`} {
		if _, err := s.Run("t.star", src, ""); err == nil {
			t.Errorf("%s ran", src)
		}
	}
}

func TestComplete(t *testing.T) {
	for src, want := range map[string]bool{
		"x = 1":                       true,
		"cad.add_shape(\"box\"":       false,
		"for i in range(3):":          false,
		"for i in range(3):\n    i":   false,
		"for i in range(3):\n    i\n": true,
		"def f():\n    return 1\n\n":  true,
		"s = \"\"\"abc":               false,
		"x = = 1":                     true, // a real error: run and report it
		"":                            true,
		// a block opened after the first line still waits for its end
		"x = 1\nfor i in range(3):\n    a = i":   false,
		"x = 1\nfor i in range(3):\n    a = i\n": true,
		// colons in a comment, a dict or a string open nothing
		"x = 1 # note:":       true,
		"d = {\"a\":\n1}":     true,
		"s = \"if x:\"":       true,
		"if x:\n    a\nelse:": false,
	} {
		if got := Complete(src); got != want {
			t.Errorf("Complete(%q) = %v, want %v", src, got, want)
		}
	}
}
