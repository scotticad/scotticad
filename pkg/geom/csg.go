package geom

import "sort"

// BSP-tree constructive solid geometry in the style of csg.js (Evan
// Wallace, MIT), restructured for speed on large meshes:
//
//   - The BSP trees are used only as classifiers (planes, no polygons).
//     Output polygons come solely from clipping each operand's polygon list
//     against the other operand's tree, so a mesh's own tree never chops up
//     its own output.
//   - Every polygon remembers the input triangle it came from. When all
//     fragments of a triangle survive (none was clipped away), the original
//     triangle is emitted instead of its fragments. Without this, nested
//     booleans fragment meshes explosively (an 11k-triangle shell became
//     500k triangles after a few levels of grouping).
//   - Polygons outside the other operand's bounding box cannot be affected
//     and skip clipping entirely.
//   - Tree building picks, among a few candidates, the splitting plane that
//     cuts the fewest polygons. When none divides them (a convex surface,
//     such as a smooth sphere, lies entirely behind each of its own faces,
//     so a tree of its faces is a chain as deep as the mesh is big), it
//     splits at an axis-aligned plane through the middle of the polygons
//     instead, giving each side every polygon that reaches it; the tree
//     stays shallow and building and clipping stay fast.
//
// Polygons keep their colour so multicolour groups and cut surfaces can be
// coloured.

const csgEps = 1e-5

type plane struct {
	n Vec3
	w float64
}

func (p plane) flipped() plane { return plane{p.n.Neg(), -p.w} }

type poly struct {
	v    []Vec3
	pl   plane
	c    Color
	orig int32 // index of the input triangle this fragment came from
}

func (p poly) flipped() poly {
	v := make([]Vec3, len(p.v))
	for i := range p.v {
		v[i] = p.v[len(p.v)-1-i]
	}
	return poly{v, p.pl.flipped(), p.c, p.orig}
}

const (
	coplanar = 0
	front    = 1
	back     = 2
	spanning = 3
)

// classify returns the combined classification of p against pl.
func (pl plane) classify(p *poly) int {
	pt := 0
	for _, v := range p.v {
		t := pl.n.Dot(v) - pl.w
		if t < -csgEps {
			pt |= back
		} else if t > csgEps {
			pt |= front
		}
	}
	return pt
}

func (pl plane) split(p poly, cf, cb, f, b *[]poly) {
	// a polygon is coplanar with its own plane whatever its vertices say:
	// a sliver's vertices can sit further from its plane than csgEps, and
	// splitting it by that plane would go on forever
	switch p.pl {
	case pl:
		*cf = append(*cf, p)
		return
	case pl.flipped():
		*cb = append(*cb, p)
		return
	}
	var buf [16]int
	types := buf[:0]
	pt := 0
	for _, v := range p.v {
		t := pl.n.Dot(v) - pl.w
		ty := coplanar
		if t < -csgEps {
			ty = back
		} else if t > csgEps {
			ty = front
		}
		pt |= ty
		types = append(types, ty)
	}
	switch pt {
	case coplanar:
		if pl.n.Dot(p.pl.n) > 0 {
			*cf = append(*cf, p)
		} else {
			*cb = append(*cb, p)
		}
	case front:
		*f = append(*f, p)
	case back:
		*b = append(*b, p)
	default:
		n := len(p.v)
		fv := make([]Vec3, 0, n+1)
		bv := make([]Vec3, 0, n+1)
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			ti, tj := types[i], types[j]
			vi, vj := p.v[i], p.v[j]
			if ti != back {
				fv = append(fv, vi)
			}
			if ti != front {
				bv = append(bv, vi)
			}
			if ti|tj == spanning {
				t := (pl.w - pl.n.Dot(vi)) / pl.n.Dot(vj.Sub(vi))
				v := vi.Lerp(vj, t)
				fv = append(fv, v)
				bv = append(bv, v)
			}
		}
		if len(fv) >= 3 {
			*f = append(*f, poly{fv, p.pl, p.c, p.orig})
		}
		if len(bv) >= 3 {
			*b = append(*b, poly{bv, p.pl, p.c, p.orig})
		}
	}
}

// DividerMin is the fewest polygons a divider node is used for (below it a
// chain of face planes is cheap). A tuning setting; tests compare results
// with and without dividers through it.
var DividerMin = 64

// node is a BSP tree of planes (a solid classifier). A node's plane is
// normally one of the surface's own faces: in front of it with no subtree
// is outside the solid, behind it with none is inside. A divider node's
// plane only divides space (both sides have subtrees describing the
// surface there).
type node struct {
	pl          plane
	front, back *node
	divider     bool
}

func buildTree(ps []poly) *node {
	if len(ps) == 0 {
		return nil
	}
	pl, chain := choosePlane(ps)
	if chain && len(ps) > DividerMin {
		if n := divide(ps); n != nil {
			return n
		}
	}
	n := &node{pl: pl}
	var f, b, sink []poly
	for _, p := range ps {
		n.pl.split(p, &sink, &sink, &f, &b)
		sink = sink[:0]
	}
	n.front = buildTree(f)
	n.back = buildTree(b)
	return n
}

// divide makes a divider node at an axis-aligned plane through the median
// of the polygons' centres, along their widest spread. Each side gets every
// polygon reaching it: those across the plane are cut in two, those lying in
// it go to both. It returns nil when the plane doesn't divide them.
func divide(ps []poly) *node {
	lo := EmptyBox()
	cs := make([]Vec3, len(ps))
	for i, p := range ps {
		c := Vec3{}
		for _, v := range p.v {
			c = c.Add(v)
		}
		cs[i] = c.Mul(1 / float64(len(p.v)))
		lo = lo.Extend(cs[i])
	}
	axis, size := 0, lo.Size()
	if size.Y > size.Axis(axis) {
		axis = 1
	}
	if size.Z > size.Axis(axis) {
		axis = 2
	}
	// near the median of the vertices, in the widest gap between them: a
	// plane passing close by a vertex cuts slivers whose new corners are
	// near-duplicates of old ones
	var vals []float64
	for _, p := range ps {
		for _, v := range p.v {
			vals = append(vals, v.Axis(axis))
		}
	}
	sort.Float64s(vals)
	median := vals[len(vals)/2]
	// distinct values: vertices are shared, and often lie in rows
	uniq := vals[:1]
	for _, v := range vals[1:] {
		if v != uniq[len(uniq)-1] {
			uniq = append(uniq, v)
		}
	}
	// the nearest gap to the median wide enough that the plane passes no
	// vertex closely
	mid := max(1, sort.SearchFloat64s(uniq, median))
	at, found := 0.0, false
	for k := 0; !found && (mid-k >= 1 || mid+k < len(uniq)); k++ {
		for _, i := range [2]int{mid + k, mid - k} {
			if i >= 1 && i < len(uniq) && uniq[i]-uniq[i-1] >= 4*csgEps {
				at, found = (uniq[i]+uniq[i-1])/2, true
				break
			}
		}
	}
	if !found {
		return nil
	}
	n := V(0, 0, 0).WithAxis(axis, 1)
	pl := plane{n, at}
	var f, b []poly
	for _, p := range ps {
		switch pl.classify(&p) {
		case coplanar:
			f, b = append(f, p), append(b, p)
		case front:
			f = append(f, p)
		case back:
			b = append(b, p)
		default:
			pl.split(p, &f, &b, &f, &b)
		}
	}
	if len(f) == 0 || len(b) == 0 || len(f) == len(ps) || len(b) == len(ps) {
		return nil
	}
	return &node{pl: pl, divider: true, front: buildTree(f), back: buildTree(b)}
}

// choosePlane picks, among a few candidate polygons, the plane that splits
// the fewest of a sample of the polygons. chain reports that the best one
// doesn't divide them at all (none across it, all on one side).
func choosePlane(ps []poly) (pl plane, chain bool) {
	if len(ps) <= 8 {
		return ps[0].pl, false
	}
	const cands, sample = 6, 48
	best, bestScore := ps[0].pl, int(^uint(0)>>1)
	bestChain := false
	stepC := len(ps) / cands
	stepS := len(ps)/sample + 1
	for c := 0; c < cands; c++ {
		pl := ps[c*stepC].pl
		splits, fr, bk := 0, 0, 0
		for i := 0; i < len(ps); i += stepS {
			switch pl.classify(&ps[i]) {
			case spanning:
				splits++
			case front:
				fr++
			case back:
				bk++
			}
		}
		d := fr - bk
		if d < 0 {
			d = -d
		}
		score := splits*8 + d
		if score < bestScore {
			best, bestScore = pl, score
			bestChain = splits == 0 && (fr == 0 || bk == 0)
		}
	}
	return best, bestChain
}

func (n *node) invert() {
	if n == nil {
		return
	}
	n.pl = n.pl.flipped()
	n.front.invert()
	n.back.invert()
	n.front, n.back = n.back, n.front
}

// csgOp carries per-operation bookkeeping: which input triangles lost a
// fragment to clipping.
type csgOp struct {
	lost []bool
	// the solid the tree being clipped against describes, tested directly
	// (and whether the tree is inverted); nil: the tree alone decides
	in  *insideTest
	inv bool
}

// verdict is what the direct test says of p: 1 inside the tree's solid
// (inverted if the tree is), 0 outside, -1 no answer.
func (op *csgOp) verdict(p *poly) int {
	if op.in == nil {
		return -1
	}
	c := Vec3{}
	for _, v := range p.v {
		c = c.Add(v)
	}
	r := op.in.inside(c.Mul(1 / float64(len(p.v))))
	if r >= 0 && op.inv {
		r = 1 - r
	}
	return r
}

// against clips ps by tree n, the tree of the solid in (inverted: inv).
func (op *csgOp) against(n *node, in *insideTest, inv bool, ps []poly) []poly {
	op.in, op.inv = in, inv
	out := op.clip(n, ps)
	op.in = nil
	return out
}

// clip removes the parts of ps inside the solid described by tree n. Where
// a fragment lies clear of the solid's surface, the direct test (if set)
// overrules the tree.
func (op *csgOp) clip(n *node, ps []poly) []poly {
	if n == nil {
		if op.in == nil {
			return ps
		}
		out := ps[:0:0]
		for _, p := range ps {
			if op.verdict(&p) == 1 {
				op.lost[p.orig] = true
				continue
			}
			out = append(out, p)
		}
		return out
	}
	var f, b []poly
	for _, p := range ps {
		n.pl.split(p, &f, &b, &f, &b)
	}
	f = op.clip(n.front, f)
	if n.back != nil || n.divider {
		b = op.clip(n.back, b)
	} else {
		kept := b[:0]
		for _, p := range b {
			if op.verdict(&p) == 0 {
				kept = append(kept, p)
				continue
			}
			op.lost[p.orig] = true
		}
		b = kept
	}
	return append(f, b...)
}

func flipAll(ps []poly) []poly {
	out := make([]poly, len(ps))
	for i, p := range ps {
		out[i] = p.flipped()
	}
	return out
}

func toPolys(m *Mesh, base int) []poly {
	out := make([]poly, 0, len(m.Tris))
	for i, t := range m.Tris {
		nrm := t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0]))
		l := nrm.Len()
		// a sliver thinner than csgEps (its height: twice the area over its
		// longest side) has no usable plane; it encloses nothing, so it is
		// left out
		longest := 0.0
		for k := 0; k < 3; k++ {
			longest = max(longest, t.P[(k+1)%3].Sub(t.P[k]).Len())
		}
		if l < 1e-12 || l < csgEps*longest {
			continue
		}
		nrm = nrm.Mul(1 / l)
		out = append(out, poly{v: []Vec3{t.P[0], t.P[1], t.P[2]}, pl: plane{nrm, nrm.Dot(t.P[0])}, c: t.C, orig: int32(base + i)})
	}
	return out
}

// partition splits polygons into those whose bounds touch box b (active)
// and those that cannot interact with a solid inside b (passive).
func partition(ps []poly, b Box) (active, passive []poly) {
	b.Min = b.Min.Sub(V(csgEps, csgEps, csgEps))
	b.Max = b.Max.Add(V(csgEps, csgEps, csgEps))
	for _, p := range ps {
		pb := EmptyBox()
		for _, v := range p.v {
			pb = pb.Extend(v)
		}
		if pb.Overlaps(b, 0) {
			active = append(active, p)
		} else {
			passive = append(passive, p)
		}
	}
	return
}

// emit turns the surviving polygons into a mesh, replacing the fragments
// of every input triangle that lost nothing by the triangle itself.
func (op *csgOp) emit(ps []poly, src []Tri) *Mesh {
	m := &Mesh{Tris: make([]Tri, 0, len(ps))}
	done := make([]bool, len(op.lost))
	for _, p := range ps {
		o := p.orig
		if !op.lost[o] {
			if done[o] {
				continue
			}
			done[o] = true
			t := src[o]
			if p.pl.n.Dot(t.Normal()) < 0 {
				t.P[1], t.P[2] = t.P[2], t.P[1]
			}
			t.C = p.c
			m.Tris = append(m.Tris, t)
			continue
		}
		for i := 1; i+1 < len(p.v); i++ {
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{p.v[0], p.v[i], p.v[i+1]}, C: p.c})
		}
	}
	return m.Clean()
}

func empty(m *Mesh) bool { return m == nil || len(m.Tris) == 0 }

// setup prepares an operation on a and b.
func setup(a, b *Mesh) (op *csgOp, src []Tri, pa, pb []poly) {
	src = make([]Tri, 0, len(a.Tris)+len(b.Tris))
	src = append(src, a.Tris...)
	src = append(src, b.Tris...)
	op = &csgOp{lost: make([]bool, len(src))}
	return op, src, toPolys(a, 0), toPolys(b, len(a.Tris))
}

// The operations below are csg.js's, written over polygon lists:
// clip(t, ps) removes the parts of ps inside the solid of tree t (outside
// it, once t is inverted). Polygons outside the other operand's bounding
// box are outside that solid, so they are kept or dropped without clipping.

// Union returns a ∪ b.
func Union(a, b *Mesh) *Mesh {
	if empty(a) {
		return b
	}
	if empty(b) {
		return a
	}
	ab, bb := a.Bounds(), b.Bounds()
	if !ab.Overlaps(bb, csgEps) {
		return Merge(a, b)
	}
	op, src, pa, pb := setup(a, b)
	ta, tb := buildTree(pa), buildTree(pb)
	ia, ib := newInsideTest(a), newInsideTest(b)
	aAct, aPas := partition(pa, bb)
	bAct, bPas := partition(pb, ab)
	aAct = op.against(tb, ib, false, aAct) // a outside b
	bAct = op.against(ta, ia, false, bAct) // b outside a
	// drop b's faces coplanar with (and facing like) a's, so each shared
	// face appears once
	bAct = flipAll(op.against(ta, ia, false, flipAll(bAct)))
	out := append(append(aPas, aAct...), append(bPas, bAct...)...)
	return op.emit(out, src)
}

// Subtract returns a − b.
func Subtract(a, b *Mesh) *Mesh {
	if empty(a) || empty(b) {
		return a
	}
	ab, bb := a.Bounds(), b.Bounds()
	if !ab.Overlaps(bb, csgEps) {
		return a
	}
	op, src, pa, pb := setup(a, b)
	ta, tb := buildTree(pa), buildTree(pb)
	ia, ib := newInsideTest(a), newInsideTest(b)
	aAct, aPas := partition(pa, bb)                // a outside b's box is kept as is
	bAct, _ := partition(pb, ab)                   // b outside a's box cuts nothing
	ta.invert()                                    // complement of a
	aL := op.against(tb, ib, false, flipAll(aAct)) // (flipped) a outside b
	bL := op.against(ta, ia, true, bAct)           // b inside a
	bL = flipAll(op.against(ta, ia, true, flipAll(bL)))
	out := append(aPas, flipAll(aL)...)
	out = append(out, flipAll(bL)...) // b's surviving faces become the cut walls
	return op.emit(out, src)
}

// Intersect returns a ∩ b.
func Intersect(a, b *Mesh) *Mesh {
	if empty(a) || empty(b) {
		return &Mesh{}
	}
	ab, bb := a.Bounds(), b.Bounds()
	if !ab.Overlaps(bb, csgEps) {
		return &Mesh{}
	}
	op, src, pa, pb := setup(a, b)
	ta, tb := buildTree(pa), buildTree(pb)
	ia, ib := newInsideTest(a), newInsideTest(b)
	aAct, _ := partition(pa, bb) // anything outside the other's box is
	bAct, _ := partition(pb, ab) // outside the intersection
	ta.invert()
	aL := flipAll(aAct)
	bL := op.against(ta, ia, true, bAct) // b inside a
	tb.invert()
	bL = flipAll(bL)
	aL = op.against(tb, ib, true, aL) // a inside b
	bL = op.against(ta, ia, true, bL) // coplanar faces kept once
	out := append(flipAll(aL), flipAll(bL)...)
	return op.emit(out, src)
}

// UnionAll unions many meshes.
func UnionAll(ms []*Mesh) *Mesh {
	var acc *Mesh
	for _, m := range ms {
		acc = Union(acc, m)
	}
	if acc == nil {
		return &Mesh{}
	}
	return acc
}
