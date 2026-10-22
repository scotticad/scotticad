package editor

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const mmPerInch = 25.4

// Format shows a length (mm) in the design's units. Labels on the canvas
// use two decimals in mm and three in inches; inspector fields use two.
func Format(mm float64, units string, field bool) string {
	if units == "in" {
		if field {
			// up to four places (1/16" is 0.0625), at least two
			s := strings.TrimRight(fmt.Sprintf("%.4f", mm/mmPerInch), "0")
			if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 < 2 {
				s += strings.Repeat("0", 2-(len(s)-i-1))
			}
			return s
		}
		return fmt.Sprintf("%.3f", mm/mmPerInch)
	}
	return fmt.Sprintf("%.2f", mm)
}

// Parse reads a typed length in the design's units and returns mm. It
// accepts decimals and fractions ("1/8", "1 1/2", "-3/4"), and a suffix
// (mm, cm, in, ") overrides the units. Arithmetic is not supported.
func Parse(s string, units string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty value")
	}
	for _, suf := range []struct {
		s string
		u string
	}{{"mm", "mm"}, {"cm", "cm"}, {"in", "in"}, {"\"", "in"}} {
		if strings.HasSuffix(s, suf.s) {
			s = strings.TrimSpace(strings.TrimSuffix(s, suf.s))
			units = suf.u
			break
		}
	}
	v, err := parseNumber(s)
	if err != nil {
		return 0, err
	}
	switch units {
	case "in":
		v *= mmPerInch
	case "cm":
		v *= 10
	}
	if math.IsNaN(v) || math.IsInf(v, 0) { // also a huge number made larger by the units
		return 0, fmt.Errorf("bad number %q", s)
	}
	return v, nil
}

func parseNumber(s string) (float64, error) {
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimSpace(s[1:])
	} else if strings.HasPrefix(s, "+") {
		s = strings.TrimSpace(s[1:])
	}
	var total float64
	parts := strings.Fields(s)
	if len(parts) == 0 || len(parts) > 2 {
		return 0, fmt.Errorf("bad number %q", s)
	}
	for _, p := range parts {
		// the sign is only in front; no digit separators or hex floats
		// (which ParseFloat would take: "1_0", "0x1p4")
		if strings.ContainsAny(p, "_xXpP") || (strings.ContainsAny(p, "+-") && !strings.ContainsAny(p, "eE")) {
			return 0, fmt.Errorf("bad number %q", s)
		}
	}
	// two parts are a whole number and a fraction ("1 1/2"), nothing else
	if len(parts) == 2 && (strings.ContainsAny(parts[0], "./eE") || !strings.Contains(parts[1], "/")) {
		return 0, fmt.Errorf("bad number %q", s)
	}
	for _, p := range parts {
		if i := strings.Index(p, "/"); i >= 0 {
			a, err1 := strconv.ParseFloat(p[:i], 64)
			b, err2 := strconv.ParseFloat(p[i+1:], 64)
			if err1 != nil || err2 != nil || b == 0 {
				return 0, fmt.Errorf("bad fraction %q", p)
			}
			total += a / b
		} else {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return 0, fmt.Errorf("bad number %q", p)
			}
			total += v
		}
	}
	if math.IsNaN(total) || math.IsInf(total, 0) {
		return 0, fmt.Errorf("bad number %q", s)
	}
	if neg {
		total = -total
	}
	return total, nil
}

// SnapOption is an entry in the Snap Grid dropdown.
type SnapOption struct {
	Label string
	MM    float64
}

// SnapOptions lists the snap values for the unit system.
func SnapOptions(units string) []SnapOption {
	if units == "in" {
		return []SnapOption{{"Off", 0}, {"1/64 in", mmPerInch / 64}, {"1/32 in", mmPerInch / 32}, {"1/16 in", mmPerInch / 16},
			{"1/8 in", mmPerInch / 8}, {"1/4 in", mmPerInch / 4}, {"1/2 in", mmPerInch / 2}, {"1 in", mmPerInch}}
	}
	return []SnapOption{{"Off", 0}, {"0.1 mm", 0.1}, {"0.25 mm", 0.25}, {"0.5 mm", 0.5}, {"1.0 mm", 1},
		{"2.0 mm", 2}, {"5.0 mm", 5}, {"10.0 mm", 10}}
}

// SnapLabel returns the dropdown label for the current snap value.
func SnapLabel(mm float64, units string) string {
	for _, o := range SnapOptions(units) {
		if math.Abs(o.MM-mm) < 1e-6 {
			return o.Label
		}
	}
	return Format(mm, units, true)
}

// Snap rounds v (mm) to the snap grid; step 0 disables snapping.
func Snap(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	return math.Round(v/step) * step
}
