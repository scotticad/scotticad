package editor

import (
	"math"
	"testing"
)

// Fuzz test for typed lengths. Run with
//
//	go test ./pkg/editor -run '^$' -fuzz '^FuzzParseUnits$' -fuzztime 60s

func FuzzParseUnits(f *testing.F) {
	for _, s := range []string{
		"12", "12.5", "-3", "+4", "1/8", "1 1/2", "-3/4", "- 3/4", "1/0", "0/0", "2mm", "2 cm", "1in", "1.5\"",
		"1e3", "1e-3", "-1e308", "1e308in", "1e309", "nan", "inf", "-Inf", "infinity", "0x1p-2", "1_000",
		"1 1", "1.5 1/2", "1/2/3", "1 -1/2", "", "  ", "mm", "\"", "1/8in", "3 1/2 in", "1e400/1e400", "1/1e-320",
	} {
		f.Add(s, false, false)
		f.Add(s, true, true)
	}
	f.Fuzz(func(t *testing.T, s string, inch, field bool) {
		units := "mm"
		if inch {
			units = "in"
		}
		v, err := Parse(s, units)
		if err != nil {
			return
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("Parse(%q, %s) = %v", s, units, v)
		}
		f1 := Format(v, units, field)
		v2, err := Parse(f1, units)
		if err != nil {
			t.Fatalf("Parse(%q) = %v, but its own Format %q doesn't parse: %v", s, v, f1, err)
		}
		if f2 := Format(v2, units, field); f2 != f1 {
			t.Fatalf("Format not stable: %q -> %v -> %q -> %v -> %q", s, v, f1, v2, f2)
		}
	})
}
