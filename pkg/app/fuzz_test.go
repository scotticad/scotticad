//go:build windows

package app

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Fuzz tests for the automation API's plain functions (no window needed).
// Run one with, e.g.
//
//	go test ./pkg/app -run '^$' -fuzz '^FuzzParseChord$' -fuzztime 60s

func FuzzParseChord(f *testing.F) {
	for _, s := range []string{"ctrl+z", "Ctrl+Shift+Z", "ctrl++", "+", "ctrl+-", "alt+f5", "shift", "a+b", "ctrl+", "++", "+++", "ctrl+ +", " Delete ", "control+alt+del", "é", "f1+ctrl", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		vk, _, err := parseChord(s)
		if err != nil {
			return
		}
		if vk <= 0 || vk > 0xff {
			t.Fatalf("parseChord(%q): key code %d", s, vk)
		}
	})
}

func FuzzCheckParams(f *testing.F) {
	for i := range apiMethods {
		m := apiMethods[i]
		p := map[string]any{}
		for _, prm := range m.Params {
			switch prm.Type {
			case "float", "int":
				p[prm.Name] = 1.5
			case "bool":
				p[prm.Name] = true
			case "string":
				p[prm.Name] = "x"
			case "string[]":
				p[prm.Name] = []any{"a", "b"}
			case "int[]":
				p[prm.Name] = []any{1.0, 2.0}
			case "float[3]":
				p[prm.Name] = []any{1.0, 2.0, 3.0}
			}
		}
		b, _ := json.Marshal(p)
		f.Add(uint16(i), b)
	}
	f.Add(uint16(0), []byte(`{"x": " 1e999 ", "y": "nan", "z": "0x10", "ids": [1, "2"], "p": [1, 2], "q": null}`))
	f.Add(uint16(0), []byte(`{"ids": 1e300, "v": [1e308, -1e308, 0], "n": "1 2"}`))
	f.Fuzz(func(t *testing.T, mi uint16, raw []byte) {
		if len(apiMethods) == 0 {
			t.Skip()
		}
		m := apiMethods[int(mi)%len(apiMethods)]
		p := args{}
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		if err := checkParams(m, p); err != nil {
			return
		}
		// what checkParams lets through, the handlers read as given (not
		// as their defaults)
		for _, prm := range m.Params {
			v, ok := p[prm.Name]
			if !ok {
				if !prm.Optional {
					t.Fatalf("%s: required %q missing but accepted", m.Name, prm.Name)
				}
				continue
			}
			switch prm.Type {
			case "float", "int":
				if x := p.f(prm.Name, math.NaN()); math.IsNaN(x) || math.IsInf(x, 0) {
					t.Fatalf("%s: %q = %#v accepted as a number but reads as %v", m.Name, prm.Name, v, x)
				}
				p.i(prm.Name, 0)
			case "float[3]":
				if _, ok := p.vec(prm.Name); !ok {
					t.Fatalf("%s: %q = %#v accepted as a point but doesn't read as one", m.Name, prm.Name, v)
				}
			case "int[]":
				if ids := p.ids(prm.Name); ids == nil {
					t.Fatalf("%s: %q = %#v accepted as ids but reads as none", m.Name, prm.Name, v)
				}
			case "string":
				if p.s(prm.Name, "\x00default") == "\x00default" && !strings.Contains(v.(string), "\x00default") {
					t.Fatalf("%s: %q = %#v accepted but reads as the default", m.Name, prm.Name, v)
				}
			}
		}
	})
}
