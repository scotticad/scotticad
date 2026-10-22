package script

import (
	"testing"
	"time"
)

// Fuzz tests. Run one with, e.g.
//
//	go test ./pkg/script -run '^$' -fuzz '^FuzzComplete$' -fuzztime 60s

// fzWithin runs f and fails if it doesn't finish within d. A panic in f
// crashes the test binary with f's own stack, which the fuzzer records as
// a crasher like any other.
func fzWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s: still running after %v", what, d)
	}
}

var fzScripts = []string{
	"", "1", "x = 1", "def f(x):\n    return x\n", "def f(x):\n    return x\n\n", "if True:", "if True:\n    pass\n",
	"for i in range(3):\n    print(i)\n", "[1,\n2", "{'a': 1,", "\"\"\"doc", "'abc", "x = {'a': 1}  # c:", "s = 'a:'",
	"while True:\n    pass\n", "l = []\nl.append(l)\ncad.select(l)", "d = {}\nd[\"a\"] = d\ncad.call(\"x\", p=d)",
	"cad.select(range(300000000))", "cad.move(dx=float(\"nan\"))", "cad.call(\"x\", a=[1, 2.5, 'x', None, True, {'k': (1, 2)}])",
	"cad.call(\"x\", a=struct(b=1, c=[1]))", "cad.call(\"x\", a=math.inf)", "cad.call(\"x\", a=1 << 70)",
	"cad.call(\"x\", a={1: 2})", "cad.call(\"x\", a=set([1]))", "r = cad.call(\"x\", a=[[[[[[1]]]]]])\nprint(r)",
	"cad.add_shape(kind='box', x=1, y=2)", "help(cad.add_shape)", "help()", "x = cad.call('echo', v=json.decode('[1,2,{\"a\":null}]'))",
	"else:", "  x", "x = (", "\t", "#:", "a = [x for x in range(10) if x:", "lambda: 1",
}

func FuzzComplete(f *testing.F) {
	for _, s := range fzScripts {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		fzWithin(t, 2*time.Second, "Complete", func() { Complete(src) })
	})
}

// fzEcho is a fake API that answers every call with its own params, so
// that values go through toGo, JSON and fromJSON and back to the script.
func fzEcho(method string, params []byte) ([]byte, error) {
	switch method {
	case "list_handles":
		return []byte(`[{"kind": "corner", "index": 0, "x": 1, "y": 2}]`), nil
	case "bad_json":
		return []byte(`{"a": [1, 2`), nil
	}
	return params, nil
}

func FuzzSession(f *testing.F) {
	for _, s := range fzScripts {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, src string, prompt bool) {
		if len(src) > 4096 {
			return
		}
		s := NewSession(fzEcho, func(Line) {})
		s.MaxSteps = 100000
		done := make(chan struct{})
		go func() {
			defer close(done)
			if prompt {
				s.Prompt(src, "")
				s.Prompt("_", "")
			} else {
				s.Run("fuzz.star", src, "")
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			s.Stop()
			select {
			case <-done:
				// stopped once asked: a slow but stoppable script is not
				// a hang
			case <-time.After(5 * time.Second):
				t.Fatalf("script still running 5 s after Stop (MaxSteps %d)", s.MaxSteps)
			}
		}
	})
}
