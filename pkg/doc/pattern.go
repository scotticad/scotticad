package doc

import (
	"errors"
	"fmt"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// A pattern group shows its children repeated: in rows and columns, or
// round a circle. It holds one set of children and the numbers (in Params),
// not the copies, so changing a child or a number changes every copy. Like
// a bundle it keeps its parts apart: each copy keeps its colours, and a
// hole in it stays a hole, cutting whatever the pattern is grouped with.
//
// The copies are laid out in the group's own space: columns along its X,
// rows along its Y, a circle about its Z. The first copy is the children
// where they are; a circle's centre is Radius to its -X.
//
// A ScottiCAD from before patterns reads the mode as a union, so it shows
// the first copy alone, and keeps the numbers when it saves.
const ModePattern = "pattern"

// A pattern's kinds (its "type" setting).
const (
	PatternGrid   = 0
	PatternCircle = 1
)

// maxSpacing bounds a pattern's lengths, so a file's numbers can't put a
// copy where its matrix overflows.
const maxSpacing = 10000

var offOn = []string{"Off", "On"}

// patternParams are a pattern's settings, each as the Properties panel
// presents it; keys are those of Node.Params.
var patternParams = []shapes.Param{
	{Key: "type", Label: "Pattern", Kind: shapes.Choice, Min: 0, Max: 1, Step: 1, Default: PatternGrid, Int: true, SizeAxis: -1, Options: []string{"Grid", "Circle"}},
	{Key: "columns", Label: "Columns", Kind: shapes.Slider, Min: 1, Max: 50, Step: 1, Default: 3, Int: true, SizeAxis: -1},
	{Key: "rows", Label: "Rows", Kind: shapes.Slider, Min: 1, Max: 50, Step: 1, Default: 1, Int: true, SizeAxis: -1},
	{Key: "dx", Label: "Column Spacing", Kind: shapes.Number, Min: -maxSpacing, Max: maxSpacing, Step: 1, Default: 20, SizeAxis: -1, Length: true},
	{Key: "dy", Label: "Row Spacing", Kind: shapes.Number, Min: -maxSpacing, Max: maxSpacing, Step: 1, Default: 20, SizeAxis: -1, Length: true},
	{Key: "stagger", Label: "Stagger Rows", Kind: shapes.Choice, Min: 0, Max: 1, Step: 1, Default: 0, Int: true, SizeAxis: -1, Options: offOn},
	{Key: "count", Label: "Copies", Kind: shapes.Slider, Min: 1, Max: 200, Step: 1, Default: 6, Int: true, SizeAxis: -1},
	{Key: "radius", Label: "Radius", Kind: shapes.Number, Min: 0, Max: maxSpacing, Step: 1, Default: 20, SizeAxis: -1, Length: true},
	{Key: "angle", Label: "Angle", Kind: shapes.Slider, Min: 1, Max: 360, Step: 1, Default: 360, SizeAxis: -1},
	{Key: "turn", Label: "Turn Copies", Kind: shapes.Choice, Min: 0, Max: 1, Step: 1, Default: 1, Int: true, SizeAxis: -1, Options: offOn},
}

// which kind of pattern each setting belongs to (the type: both)
var patternParamOf = map[string]int{"columns": PatternGrid, "rows": PatternGrid, "dx": PatternGrid, "dy": PatternGrid, "stagger": PatternGrid,
	"count": PatternCircle, "radius": PatternCircle, "angle": PatternCircle, "turn": PatternCircle}

// PatternParam is the pattern setting named key.
func PatternParam(key string) (*shapes.Param, bool) {
	for i := range patternParams {
		if patternParams[i].Key == key {
			return &patternParams[i], true
		}
	}
	return nil, false
}

// PatternParams are the settings of pattern n's kind, in the panel's order.
func (n *Node) PatternParams() []shapes.Param {
	typ := int(n.patternNum("type"))
	var out []shapes.Param
	for _, p := range patternParams {
		if of, ok := patternParamOf[p.Key]; !ok || of == typ {
			out = append(out, p)
		}
	}
	return out
}

// IsPattern reports a pattern group.
func (n *Node) IsPattern() bool { return n.IsGroup() && n.GroupMode == ModePattern }

// patternNum is a pattern setting as stored, made safe: the default when
// it is missing or not a number, within its range, whole if it counts.
func (n *Node) patternNum(key string) float64 {
	p, ok := PatternParam(key)
	if !ok {
		return 0
	}
	v, ok := n.Params[key]
	if !ok || math.IsNaN(v) {
		v = p.Default
	}
	v = math.Max(p.Min, math.Min(p.Max, v))
	if p.Int {
		v = math.Round(v)
	}
	return v
}

// patternScale is what a stored length is multiplied by on the way to the
// parent's space: a pattern that has been stretched has its spacing
// stretched with it. Columns and a circle's radius run along the group's
// X, rows along its Y.
func (n *Node) patternScale(key string) float64 {
	axis := -1
	switch key {
	case "dx", "radius":
		axis = 0
	case "dy":
		axis = 1
	}
	if axis < 0 {
		return 1
	}
	if s := n.M.Col(axis).Len(); s > 1e-12 && !math.IsInf(s, 0) && !math.IsNaN(s) {
		return s
	}
	return 1
}

// PatternValue is a pattern setting as the panel and the API give it:
// lengths as they measure in the group's parent space.
func (n *Node) PatternValue(key string) float64 {
	return n.patternNum(key) * n.patternScale(key)
}

// PatternValues are all of a pattern's settings (see PatternValue).
func (n *Node) PatternValues() map[string]float64 {
	out := map[string]float64{}
	for _, p := range patternParams {
		out[p.Key] = n.PatternValue(p.Key)
	}
	return out
}

// SetPatternParam sets a pattern setting from a value as PatternValue
// gives it.
func SetPatternParam(n *Node, key string, v float64) error {
	p, ok := PatternParam(key)
	if !ok {
		return fmt.Errorf("a pattern has no setting %q", key)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be a number", key)
	}
	v /= n.patternScale(key)
	v = math.Max(p.Min, math.Min(p.Max, v))
	if p.Int {
		v = math.Round(v)
	}
	if n.Params == nil {
		n.Params = map[string]float64{}
	}
	was, had := n.Params[key]
	n.Params[key] = v
	if n.wantedCopies()*n.shapesShown() > MaxPatternShapes {
		if n.Params[key] = was; !had {
			delete(n.Params, key)
		}
		return fmt.Errorf("that would show more than %d shapes (patterns inside a pattern multiply)", MaxPatternShapes)
	}
	return nil
}

// MaxPatternShapes is the most shapes a pattern shows, counting the
// patterns inside it: a pattern of patterns multiplies, and 50 × 50 of
// 50 × 50 is six million. Past it a pattern shows fewer copies than its
// settings ask for (a design file can hold any settings; SetPatternParam
// refuses them).
const MaxPatternShapes = 20000

// shapesShown is how many shapes one copy of n's children comes to, with
// the patterns among them counted in full (never more than a little over
// MaxPatternShapes: enough to compare with it).
func (n *Node) shapesShown() int {
	total := 0
	for _, c := range n.Children {
		k := 1
		if c.IsGroup() {
			// (c's copies capped as Copies caps them, from the one count:
			// asking Copies would count c's children again, which doubles
			// the work at each level of nesting)
			if k = c.shapesShown(); c.IsPattern() {
				k *= min(len(c.copies()), max(1, MaxPatternShapes/k))
			}
		}
		if total += k; total > MaxPatternShapes {
			return MaxPatternShapes + 1
		}
	}
	return max(1, total)
}

// wantedCopies is how many copies a pattern's settings ask for.
func (n *Node) wantedCopies() int {
	if int(n.patternNum("type")) == PatternCircle {
		return int(n.patternNum("count"))
	}
	return int(n.patternNum("columns")) * int(n.patternNum("rows"))
}

// Copies are where a pattern shows its children, as transforms in the
// group's own space, the first the identity. nil for any other node (its
// children are shown once, where they are).
func (n *Node) Copies() []geom.Mat4 {
	if !n.IsPattern() {
		return nil
	}
	out := n.copies()
	if most := max(1, MaxPatternShapes/n.shapesShown()); len(out) > most {
		out = out[:most]
	}
	return out
}

// copies is Copies as the settings ask, however many that makes.
func (n *Node) copies() []geom.Mat4 {
	if int(n.patternNum("type")) == PatternCircle {
		count := int(n.patternNum("count"))
		r, sweep := n.patternNum("radius"), n.patternNum("angle")
		// a full turn is shared out among the copies; an arc has a copy at
		// each end
		step := sweep / float64(count)
		if sweep < 360 && count > 1 {
			step = sweep / float64(count-1)
		}
		turn := n.patternNum("turn") != 0
		c := geom.V(-r, 0, 0)
		out := []geom.Mat4{geom.Identity()}
		for i := 1; i < count; i++ {
			rot := geom.RotateZ(step * float64(i) * math.Pi / 180)
			if turn {
				out = append(out, geom.Translate(c).Mul(rot).Mul(geom.Translate(c.Neg())))
			} else {
				out = append(out, geom.Translate(rot.Point(c.Neg()).Add(c)))
			}
		}
		return out
	}
	cols, rows := int(n.patternNum("columns")), int(n.patternNum("rows"))
	dx, dy := n.patternNum("dx"), n.patternNum("dy")
	stagger := n.patternNum("stagger") != 0
	out := make([]geom.Mat4, 0, cols*rows)
	for j := 0; j < rows; j++ {
		off := 0.0
		if stagger && j%2 == 1 {
			off = dx / 2
		}
		for i := 0; i < cols; i++ {
			out = append(out, geom.Translate(geom.V(float64(i)*dx+off, float64(j)*dy, 0)))
		}
	}
	return out
}

// eachCopy calls f with the transform of each place n's children are
// shown: xf itself, or for a pattern xf with each copy's.
func (n *Node) eachCopy(xf geom.Mat4, f func(geom.Mat4)) {
	if !n.IsPattern() {
		f(xf)
		return
	}
	for _, t := range n.Copies() {
		f(xf.Mul(t))
	}
}

// expanded is pattern n as a plain bundle holding every copy (the copies
// share their originals' ids: for a caller that renumbers what it keeps).
func (n *Node) expanded() *Node {
	c := *n
	c.GroupMode, c.Params, c.Children = ModeBundle, nil, nil
	for i, t := range n.Copies() {
		for _, ch := range n.Children {
			k := ch.Clone()
			if i > 0 {
				k.M = t.Mul(k.M)
			}
			c.Children = append(c.Children, k)
		}
	}
	return &c
}

// Pattern wraps top-level nodes (one or more) in a pattern group: a row of
// three to begin with, spaced by the nodes' size, with a circle's radius
// set to suit as well. The nodes stay where they are, as the first copy.
func (d *Document) Pattern(ids []int) (*Node, error) {
	ns := d.FindAll(ids)
	if len(ns) == 0 {
		return nil, errors.New("select a shape to repeat")
	}
	size := Bounds(ns).Size()
	// one and a half times the size, in whole millimetres: a gap of half
	// a shape between copies
	space := func(s float64) float64 {
		if math.IsNaN(s) || math.IsInf(s, 0) || s <= 0 {
			return 20
		}
		return math.Min(maxSpacing, math.Max(1, math.Ceil(s*1.5-1e-9)))
	}
	g := d.group(ns, ModePattern)
	g.Params = map[string]float64{}
	for _, p := range patternParams {
		g.Params[p.Key] = p.Default
	}
	g.Params["dx"], g.Params["dy"] = space(size.X), space(size.Y)
	g.Params["radius"] = space(math.Max(size.X, size.Y))
	return g, nil
}
