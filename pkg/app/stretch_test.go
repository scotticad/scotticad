package app

import (
	"math"
	"testing"
)

// The Stretch tool's amount box: a sign adds or takes out, a plain number
// is the length to reach; inches when the design is in inches.
func TestParseStretch(t *testing.T) {
	for _, c := range []struct {
		text, units string
		ok          bool
		want        stretchState
	}{
		{"+10", "mm", true, stretchState{amount: 10}},
		{"-4.5", "mm", true, stretchState{amount: -4.5}},
		{"− 3", "mm", true, stretchState{amount: -3}},
		{"60", "mm", true, stretchState{byLength: true, length: 60}},
		{"+1", "in", true, stretchState{amount: 25.4}},
		{"2\"", "in", true, stretchState{byLength: true, length: 50.8}},
		{"0", "mm", false, stretchState{}},
		{"+", "mm", false, stretchState{}},
		{"--3", "mm", false, stretchState{}},
		{"ten", "mm", false, stretchState{}},
	} {
		got, ok := parseStretch(c.text, c.units)
		if ok != c.ok || ok && (got.byLength != c.want.byLength || math.Abs(got.amount-c.want.amount) > 1e-9 || math.Abs(got.length-c.want.length) > 1e-9) {
			t.Errorf("%q (%s): %+v %v, want %+v %v", c.text, c.units, got, ok, c.want, c.ok)
		}
	}
}
