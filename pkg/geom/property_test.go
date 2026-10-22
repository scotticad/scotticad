package geom

// Property-based tests: seeded random cases checked against invariants
// (CSG volume identities and watertightness, matrix algebra, ray hits,
// file round trips). Deterministic; each test runs in a few seconds.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// ---- random solids ----

// ptBox is an axis-aligned box [lo, hi].
func ptBox(lo, hi Vec3) *Mesh {
	return box(lo.Add(hi).Mul(0.5), hi.Sub(lo))
}

// ptPrism is a closed regular n-gon prism of radius r and height h, centred
// at the origin (its own pivot), axis along Z.
func ptPrism(r, h float64, n int) *Mesh {
	return cyl(0, 0, r, h, n).Transform(Translate(V(0, 0, -h/2))).Oriented()
}

// ptTet is a tetrahedron with the given corners, oriented outward.
func ptTet(a, b, c, d Vec3) *Mesh {
	m := &Mesh{Tris: []Tri{
		{P: [3]Vec3{a, b, c}}, {P: [3]Vec3{a, c, d}}, {P: [3]Vec3{a, d, b}}, {P: [3]Vec3{b, d, c}},
	}}
	return m.Oriented()
}

func ptRandRot(r *rand.Rand) Mat4 {
	ax := V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64()).Norm()
	return RotateAround(ax, r.Float64()*2*math.Pi)
}

func ptRandVec(r *rand.Rand, s float64) Vec3 {
	return V((r.Float64()*2-1)*s, (r.Float64()*2-1)*s, (r.Float64()*2-1)*s)
}

// ptRandSolid is a random convex solid (box, prism or tetrahedron) of size
// roughly 1..20, posed randomly near the origin.
func ptRandSolid(r *rand.Rand) (*Mesh, string) {
	var m *Mesh
	var name string
	switch r.Intn(3) {
	case 0:
		s := V(1+r.Float64()*19, 1+r.Float64()*19, 1+r.Float64()*19)
		m, name = box(V(0, 0, 0), s), fmt.Sprintf("box%v", s)
	case 1:
		rad, h, n := 1+r.Float64()*9, 1+r.Float64()*19, 3+r.Intn(30)
		m, name = ptPrism(rad, h, n), fmt.Sprintf("prism(r=%g,h=%g,n=%d)", rad, h, n)
	default:
		for {
			a, b, c, d := ptRandVec(r, 10), ptRandVec(r, 10), ptRandVec(r, 10), ptRandVec(r, 10)
			if math.Abs(b.Sub(a).Cross(c.Sub(a)).Dot(d.Sub(a))) > 50 {
				m, name = ptTet(a, b, c, d), fmt.Sprintf("tet(%v,%v,%v,%v)", a, b, c, d)
				break
			}
		}
	}
	rot := ptRandRot(r)
	tr := ptRandVec(r, 5)
	return m.Transform(Translate(tr).Mul(rot)), fmt.Sprintf("%s rot%v tr%v", name, rot, tr)
}

// csgCase is a pair of solids; manifold reports that a ∪ b, a ∩ b and
// a − b are all 2-manifold solids (not, e.g., two boxes touching along an
// edge).
type csgCase struct {
	name     string
	a, b     *Mesh
	manifold bool
}

func ptCases(r *rand.Rand, n int) []csgCase {
	var cs []csgCase
	for i := 0; i < n; i++ {
		kind := i % 8
		switch kind {
		case 7: // nearly coincident: a copy shifted well below csgEps (seen
			// as coincident) or well above it; the band in between is
			// TestPropertyCSGNearCoincidentBand
			a, an := ptRandSolid(r)
			e := -9 + 3.5*r.Float64() // 1e-9 .. 3e-6
			if r.Intn(2) == 0 {
				e = -2 + 1.5*r.Float64() // 0.01 .. 0.3
			}
			d := V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64()).Norm().Mul(math.Pow(10, e))
			cs = append(cs, csgCase{fmt.Sprintf("near-coincident |shift|=%.3g A=%s shift %v", d.Len(), an, d), a, a.Transform(Translate(d)), true})
			continue
		case 0, 1: // random general position
			a, an := ptRandSolid(r)
			b, bn := ptRandSolid(r)
			cs = append(cs, csgCase{fmt.Sprintf("general A=%s B=%s", an, bn), a, b, true})
		case 2: // grid-aligned boxes sharing face planes (overlapping, coincident faces), in a common rotated frame
			rot := Identity()
			if r.Intn(2) == 0 {
				rot = ptRandRot(r)
			}
			g := func() float64 { return float64(r.Intn(6)) }
			alo := V(g(), g(), g())
			ahi := alo.Add(V(1+g(), 1+g(), 1+g()))
			// b shares at least one coordinate plane with a on each axis
			pick := func(lo, hi float64) (float64, float64) {
				switch r.Intn(4) {
				case 0:
					return lo, hi
				case 1:
					return lo, lo + 1 + float64(r.Intn(8))
				case 2:
					return hi - 1 - float64(r.Intn(8)), hi
				default:
					l := lo - float64(r.Intn(3))
					return l, hi + float64(r.Intn(3))
				}
			}
			bx0, bx1 := pick(alo.X, ahi.X)
			by0, by1 := pick(alo.Y, ahi.Y)
			bz0, bz1 := pick(alo.Z, ahi.Z)
			a := ptBox(alo, ahi).Transform(rot)
			b := ptBox(V(bx0, by0, bz0), V(bx1, by1, bz1)).Transform(rot)
			cs = append(cs, csgCase{fmt.Sprintf("coplanar A=[%v %v] B=[%v %v] rot%v", alo, ahi, V(bx0, by0, bz0), V(bx1, by1, bz1), rot), a, b, true})
		case 3: // touching at a face (full or partial)
			rot := ptRandRot(r)
			lo := V(0, 0, 0)
			hi := V(1+float64(r.Intn(5)), 1+float64(r.Intn(5)), 1+float64(r.Intn(5)))
			ax := r.Intn(3)
			blo := V(float64(r.Intn(3))-1, float64(r.Intn(3))-1, float64(r.Intn(3))-1).WithAxis(ax, hi.Axis(ax))
			bhi := blo.Add(V(2+float64(r.Intn(4)), 2+float64(r.Intn(4)), 2+float64(r.Intn(4))))
			a := ptBox(lo, hi).Transform(rot)
			b := ptBox(blo, bhi).Transform(rot)
			cs = append(cs, csgCase{fmt.Sprintf("touching A=[%v %v] B=[%v %v] rot%v", lo, hi, blo, bhi, rot), a, b, true})
		case 4: // one inside the other (strictly or sharing faces)
			a, an := ptRandSolid(r)
			s := 0.2 + 0.6*r.Float64()
			// scaled copy about the centroid of a convex solid is inside it
			cen := Vec3{}
			for _, t := range a.Tris {
				cen = cen.Add(t.P[0]).Add(t.P[1]).Add(t.P[2])
			}
			cen = cen.Mul(1 / float64(3*len(a.Tris)))
			b := a.Transform(Translate(cen).Mul(Scale(V(s, s, s))).Mul(Translate(cen.Neg())))
			if r.Intn(2) == 0 {
				a, b = b, a
			}
			cs = append(cs, csgCase{fmt.Sprintf("nested A=%s scale %g", an, s), a, b, true})
		case 5: // identical operands
			a, an := ptRandSolid(r)
			cs = append(cs, csgCase{"identical " + an, a, a, true})
		case 6: // shared edge only: geometry is non-manifold, check volumes only
			rot := ptRandRot(r)
			a := ptBox(V(0, 0, 0), V(2, 2, 2)).Transform(rot)
			b := ptBox(V(2, 2, float64(r.Intn(2))), V(4, 4, 3)).Transform(rot)
			cs = append(cs, csgCase{fmt.Sprintf("edge-touching rot%v", rot), a, b, false})
		}
	}
	return cs
}

func ptWatertight(m *Mesh) (int, int) {
	return m.FixTJunctions().EdgeCheck(1e-7)
}

func runCSGProperties(t *testing.T, seed int64, n int) {
	r := rand.New(rand.NewSource(seed))
	partial := 0
	defer func() { t.Logf("seed %d: %d of %d cases partially overlap", seed, partial, n) }()
	for i, c := range ptCases(r, n) {
		va, vb := c.a.Volume(), c.b.Volume()
		tol := 1e-6 * math.Max(1, math.Max(va, vb))
		if vx := Intersect(c.a, c.b).Volume(); vx > tol && vx < math.Min(va, vb)-tol {
			partial++
		}
		u, x, s := Union(c.a, c.b), Intersect(c.a, c.b), Subtract(c.a, c.b)
		vu, vx, vs := u.Volume(), x.Volume(), s.Volume()
		fail := func(f string, args ...any) {
			t.Errorf("seed %d case %d: %s\n\tcase: %s", seed, i, fmt.Sprintf(f, args...), c.name)
		}
		if math.Abs(vu+vx-va-vb) > tol {
			fail("V(A∪B)+V(A∩B)=%.9g+%.9g, V(A)+V(B)=%.9g+%.9g", vu, vx, va, vb)
		}
		if math.Abs(vs-(va-vx)) > tol {
			fail("V(A−B)=%.9g, V(A)−V(A∩B)=%.9g", vs, va-vx)
		}
		if vu < -tol || vx < -tol || vs < -tol {
			fail("negative volume: ∪ %g ∩ %g − %g", vu, vx, vs)
		}
		if vu2 := Union(c.b, c.a).Volume(); math.Abs(vu2-vu) > tol {
			fail("∪ not commutative: %.9g vs %.9g", vu, vu2)
		}
		if vx2 := Intersect(c.b, c.a).Volume(); math.Abs(vx2-vx) > tol {
			fail("∩ not commutative: %.9g vs %.9g", vx, vx2)
		}
		if c.manifold {
			for name, m := range map[string]*Mesh{"union": u, "intersect": x, "subtract": s} {
				if o, nm := ptWatertight(m); o != 0 || nm != 0 {
					fail("%s not watertight: %d open, %d non-manifold (%d tris)", name, o, nm, len(m.Tris))
				}
			}
		}
	}
}

func TestPropertyCSGIdentities(t *testing.T) {
	runCSGProperties(t, 1, 700)
}

// The same identities with divider nodes forced on (DividerMin lowered so
// even small meshes are split by axis planes).
func TestPropertyCSGIdentitiesWithDividers(t *testing.T) {
	saved := DividerMin
	defer func() { DividerMin = saved }()
	DividerMin = 4
	runCSGProperties(t, 2, 400)
	// finer meshes so dividers really happen
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 20; i++ {
		a := ptPrism(5+r.Float64()*5, 4+r.Float64()*10, 40+r.Intn(40)).Transform(ptRandRot(r))
		b := ptPrism(3+r.Float64()*5, 4+r.Float64()*10, 40+r.Intn(40)).Transform(Translate(ptRandVec(r, 5)).Mul(ptRandRot(r)))
		va, vb := a.Volume(), b.Volume()
		u, x, s := Union(a, b), Intersect(a, b), Subtract(a, b)
		tol := 1e-6 * math.Max(va, vb)
		if d := u.Volume() + x.Volume() - va - vb; math.Abs(d) > tol {
			t.Errorf("prisms case %d: inclusion-exclusion off by %g", i, d)
		}
		if d := s.Volume() - (va - x.Volume()); math.Abs(d) > tol {
			t.Errorf("prisms case %d: difference off by %g", i, d)
		}
		for name, m := range map[string]*Mesh{"union": u, "intersect": x, "subtract": s} {
			if o, nm := ptWatertight(m); o != 0 || nm != 0 {
				t.Errorf("prisms case %d: %s %d open, %d non-manifold", i, name, o, nm)
			}
		}
	}
}

// Booleans of boolean results (as nested groups produce): each step keeps
// the volume identities and stays watertight. Inputs here are CSG outputs
// with T-junctions and many slivers, which primitives never have.
func TestPropertyCSGChained(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 60; i++ {
		acc, desc := ptRandSolid(r)
		trail := "A0=" + desc
		for step := 1; step <= 4; step++ {
			b, bn := ptRandSolid(r)
			trail += fmt.Sprintf(" ; B%d=%s", step, bn)
			va, vb := acc.Volume(), b.Volume()
			tol := 1e-6 * math.Max(1, math.Max(va, vb))
			u, x, s := Union(acc, b), Intersect(acc, b), Subtract(acc, b)
			if d := u.Volume() + x.Volume() - va - vb; math.Abs(d) > tol {
				t.Errorf("case %d step %d: inclusion-exclusion off by %g\n%s", i, step, d, trail)
			}
			if d := s.Volume() - (va - x.Volume()); math.Abs(d) > tol {
				t.Errorf("case %d step %d: difference off by %g\n%s", i, step, d, trail)
			}
			for name, m := range map[string]*Mesh{"union": u, "intersect": x, "subtract": s} {
				if o, nm := ptWatertight(m); o != 0 || nm != 0 {
					t.Errorf("case %d step %d: %s %d open, %d non-manifold\n%s", i, step, name, o, nm, trail)
				}
			}
			switch r.Intn(3) {
			case 0:
				acc = u
			case 1:
				acc = s
			default:
				acc = x
			}
			if len(acc.Tris) == 0 {
				break
			}
		}
	}
}

// Operands offset by a little more than csgEps (1e-5 .. 1e-3 mm, e.g. two
// float32 STL imports meant to be flush) should still give watertight
// results satisfying the volume identities. Faces that close may be taken
// as one (the repair stitches slits up to stitchTol wide), which moves the
// surface by up to the offset: union and intersection may each be off by
// the offset times the area, no more.
func TestPropertyCSGNearCoincidentBand(t *testing.T) {
	r := rand.New(rand.NewSource(12))
	var fails []string
	check := func(name string, a, b *Mesh) {
		va, vb := a.Volume(), b.Volume()
		area := 0.0
		for _, tr := range a.Tris {
			area += tr.P[1].Sub(tr.P[0]).Cross(tr.P[2].Sub(tr.P[0])).Len() / 2
		}
		shift := b.Bounds().Min.Sub(a.Bounds().Min).Len()
		tol := 1e-6*math.Max(1, math.Max(va, vb)) + 2*shift*area
		u, x, s := Union(a, b), Intersect(a, b), Subtract(a, b)
		if d := u.Volume() + x.Volume() - va - vb; math.Abs(d) > tol {
			fails = append(fails, fmt.Sprintf("%s: inclusion-exclusion off by %g", name, d))
		}
		for op, m := range map[string]*Mesh{"union": u, "intersect": x, "subtract": s} {
			if o, nm := ptWatertight(m); o != 0 || nm != 0 {
				fails = append(fails, fmt.Sprintf("%s: %s %d open, %d non-manifold", name, op, o, nm))
			}
		}
	}
	// the smallest case: an axis-aligned 10 mm cube and a copy moved by
	// (2e-5, 3e-5, 1e-5), whose union, intersection and difference are
	// prone to open and non-manifold edges
	cubeA := box(V(0, 0, 0), V(10, 10, 10))
	check("cube10 shifted (2e-5,3e-5,1e-5)", cubeA, cubeA.Transform(Translate(V(2e-5, 3e-5, 1e-5))))
	for i := 0; i < 150; i++ {
		a, an := ptRandSolid(r)
		d := V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64()).Norm().Mul(math.Pow(10, -5+2*r.Float64()))
		check(fmt.Sprintf("case %d |shift|=%.3g A=%s shift %v", i, d.Len(), an, d), a, a.Transform(Translate(d)))
	}
	for _, f := range fails {
		t.Error(f)
	}
}

// A∪A = A, A∩A = A, A−A = ∅ in volume.
func TestPropertyCSGSelf(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < 150; i++ {
		a, name := ptRandSolid(r)
		va := a.Volume()
		tol := 1e-6 * math.Max(1, va)
		if va <= 0 {
			t.Fatalf("case %d (%s): input volume %g", i, name, va)
		}
		if v := Union(a, a).Volume(); math.Abs(v-va) > tol {
			t.Errorf("case %d (%s): V(A∪A)=%.9g, V(A)=%.9g", i, name, v, va)
		}
		if v := Intersect(a, a).Volume(); math.Abs(v-va) > tol {
			t.Errorf("case %d (%s): V(A∩A)=%.9g, V(A)=%.9g", i, name, v, va)
		}
		if m := Subtract(a, a); math.Abs(m.Volume()) > tol {
			t.Errorf("case %d (%s): V(A−A)=%.9g (%d tris)", i, name, m.Volume(), len(m.Tris))
		}
	}
}

// ---- matrices ----

func TestPropertyInverse(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for i := 0; i < 2000; i++ {
		var m Mat4
		if i%2 == 0 {
			for k := 0; k < 12; k++ {
				m[k] = r.Float64()*4 - 2
			}
			m[12], m[13], m[14], m[15] = r.Float64()*100-50, r.Float64()*100-50, r.Float64()*100-50, 1
		} else {
			s := V(0.01+r.Float64()*100, 0.01+r.Float64()*100, 0.01+r.Float64()*100)
			if r.Intn(2) == 0 {
				s.X = -s.X
			}
			m = Translate(ptRandVec(r, 500)).Mul(ptRandRot(r)).Mul(Scale(s)).Mul(ptRandRot(r))
		}
		if math.Abs(m.Det3()) < 1e-3 {
			continue
		}
		inv, ok := m.Inverse()
		if !ok {
			t.Fatalf("case %d: %v reported singular (det3 %g)", i, m, m.Det3())
		}
		p := inv.Mul(m)
		id := Identity()
		// tolerate error in proportion to ‖M‖·‖M⁻¹‖ (the conditioning)
		for k := range p {
			if math.Abs(p[k]-id[k]) > 1e-8*math.Max(1, normInf(m)*normInf(inv)) {
				t.Fatalf("case %d: inv(M)·M[%d]=%g\nM=%v\ninv=%v", i, k, p[k], m, inv)
			}
		}
	}
}

func normInf(m Mat4) float64 {
	s := 0.0
	for _, v := range m {
		s = math.Max(s, math.Abs(v))
	}
	return s
}

func TestPropertyRotateFromTo(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	for i := 0; i < 5000; i++ {
		f := V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64())
		var to Vec3
		switch i % 4 {
		case 0:
			to = f.Neg().Mul(0.1 + r.Float64()*10) // exactly antiparallel
		case 1:
			to = f.Mul(0.1 + r.Float64()*10) // parallel
		case 2:
			// nearly antiparallel
			to = f.Neg().Add(V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64()).Mul(1e-7 * f.Len()))
		default:
			to = V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64())
		}
		if i%40 == 0 { // axis-aligned, including ±X (the cross-product fallback)
			ax := r.Intn(3)
			f = V(0, 0, 0).WithAxis(ax, 1+r.Float64())
			to = f.Mul(-1)
		}
		m := RotateFromTo(f, to)
		got := m.Dir(f.Norm())
		if got.Sub(to.Norm()).Len() > 1e-6 {
			t.Fatalf("case %d: RotateFromTo(%v, %v) maps f to %v", i, f, to, got)
		}
		if d := m.Det3(); math.Abs(d-1) > 1e-9 || !m.IsOrthogonal() {
			t.Fatalf("case %d: not a rotation (det %g)", i, d)
		}
	}
}

func TestPropertyBoxTransform(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 2000; i++ {
		lo := ptRandVec(r, 50)
		b := Box{lo, lo.Add(V(r.Float64()*30, r.Float64()*30, r.Float64()*30))}
		m := Translate(ptRandVec(r, 50)).Mul(ptRandRot(r)).Mul(Scale(V(r.Float64()*5-2.5, r.Float64()*5, r.Float64()*5)))
		tb := b.Transform(m)
		in := func(p Vec3) bool {
			const e = 1e-9
			return p.X >= tb.Min.X-e && p.X <= tb.Max.X+e && p.Y >= tb.Min.Y-e && p.Y <= tb.Max.Y+e && p.Z >= tb.Min.Z-e && p.Z <= tb.Max.Z+e
		}
		for _, c := range b.Corners() {
			if !in(m.Point(c)) {
				t.Fatalf("case %d: corner %v → %v outside %v", i, c, m.Point(c), tb)
			}
		}
		for k := 0; k < 10; k++ {
			p := V(lo.X+r.Float64()*(b.Max.X-lo.X), lo.Y+r.Float64()*(b.Max.Y-lo.Y), lo.Z+r.Float64()*(b.Max.Z-lo.Z))
			if !in(m.Point(p)) {
				t.Fatalf("case %d: interior point %v → %v outside %v", i, p, m.Point(p), tb)
			}
		}
	}
	if !EmptyBox().Transform(Translate(V(1, 2, 3))).IsEmpty() {
		t.Fatal("empty box transformed is not empty")
	}
}

// bruteHit intersects a ray with a triangle via plane intersection and
// edge-side tests. It returns the distance, whether it hit, and how close
// the hit is to the triangle's border (relative), so ambiguous cases can be
// skipped.
func bruteHit(ray Ray, a, b, c Vec3) (float64, bool, float64) {
	n := b.Sub(a).Cross(c.Sub(a))
	den := n.Dot(ray.D)
	if math.Abs(den) < 1e-9*n.Len()*ray.D.Len() {
		return 0, false, 0
	}
	tt := n.Dot(a.Sub(ray.O)) / den
	p := ray.At(tt)
	nn := n.Dot(n)
	w0 := b.Sub(a).Cross(p.Sub(a)).Dot(n) / nn
	w1 := c.Sub(b).Cross(p.Sub(b)).Dot(n) / nn
	w2 := a.Sub(c).Cross(p.Sub(c)).Dot(n) / nn
	margin := math.Min(math.Abs(w0), math.Min(math.Abs(w1), math.Abs(w2)))
	return tt, w0 >= 0 && w1 >= 0 && w2 >= 0 && tt > 1e-9, margin
}

func TestPropertyRayMeshWhere(t *testing.T) {
	r := rand.New(rand.NewSource(8))
	checked := 0
	for i := 0; i < 400; i++ {
		m := &Mesh{}
		for k := 0; k < 30; k++ {
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{ptRandVec(r, 20), ptRandVec(r, 20), ptRandVec(r, 20)}})
		}
		ray := Ray{ptRandVec(r, 40), V(r.NormFloat64(), r.NormFloat64(), r.NormFloat64())}
		if i%2 == 0 { // aim at a triangle
			tr := m.Tris[r.Intn(len(m.Tris))]
			ray.D = tr.P[0].Add(tr.P[1]).Add(tr.P[2]).Mul(1.0 / 3).Sub(ray.O)
		}
		cut := r.Float64() * 30
		var keep func(float64) bool
		if i%3 == 0 {
			keep = func(d float64) bool { return d > cut }
		}
		best, ambiguous := math.Inf(1), false
		var bestN Vec3
		for _, tr := range m.Tris {
			d, ok, margin := bruteHit(ray, tr.P[0], tr.P[1], tr.P[2])
			if margin < 1e-7 || (keep != nil && math.Abs(d-cut) < 1e-7) {
				ambiguous = true
			}
			if ok && d < best && (keep == nil || keep(d)) {
				best, bestN = d, tr.Normal()
			}
		}
		if ambiguous {
			continue
		}
		checked++
		d, n, ok := ray.MeshWhere(m, keep)
		if ok != !math.IsInf(best, 1) {
			t.Fatalf("case %d: MeshWhere hit=%v, brute force hit=%v (dist %g)", i, ok, !math.IsInf(best, 1), best)
		}
		if ok && (math.Abs(d-best) > 1e-7*math.Max(1, best) || n.Sub(bestN).Len() > 1e-9) {
			t.Fatalf("case %d: MeshWhere %g %v, brute force %g %v", i, d, n, best, bestN)
		}
	}
	if checked < 300 {
		t.Fatalf("only %d unambiguous cases", checked)
	}
}

// ---- file round trips ----

func ptRandMesh(r *rand.Rand) *Mesh {
	a, _ := ptRandSolid(r)
	b, _ := ptRandSolid(r)
	m := Union(a, b)
	cols := []Color{{1, 0, 0}, {0, 0.5, 1}, {0.2, 0.2, 0.2}}
	for i := range m.Tris {
		m.Tris[i].C = cols[r.Intn(len(cols))]
	}
	return m.Transform(Translate(ptRandVec(r, 1000)))
}

func TestPropertySTLOBJRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	for i := 0; i < 100; i++ {
		m := ptRandMesh(r)
		// volume about the mesh's centre (about the origin, float32 rounding
		// of coordinates ~1000 mm away swamps it)
		cen := m.Bounds().Center()
		vol := func(x *Mesh) float64 { return x.Transform(Translate(cen.Neg())).Volume() }
		v := vol(m)
		// float32 coordinates near 1000 mm are good to ~6e-5 mm; volume
		// error ~ area × that
		area := 0.0
		for _, tr := range m.Tris {
			area += tr.Area()
		}
		bb := m.Bounds()
		mag := math.Max(bb.Max.Len(), bb.Min.Len())
		tol := area*mag*1.2e-7 + 1e-6

		var sb bytes.Buffer
		if err := WriteSTL(&sb, m, "prop"); err != nil {
			t.Fatal(err)
		}
		if sb.Len() != 84+50*len(m.Tris) {
			t.Fatalf("case %d: STL length %d for %d tris", i, sb.Len(), len(m.Tris))
		}
		s, err := ReadSTL(sb.Bytes())
		if err != nil {
			t.Fatalf("case %d: ReadSTL: %v", i, err)
		}
		if len(s.Tris) != len(m.Tris) || math.Abs(vol(s)-v) > tol {
			t.Errorf("case %d: STL %d→%d tris, volume %.9g→%.9g (tol %g)", i, len(m.Tris), len(s.Tris), v, vol(s), tol)
		}

		var ob, mb bytes.Buffer
		if err := WriteOBJ(&ob, &mb, m, "x.mtl"); err != nil {
			t.Fatal(err)
		}
		o, err := ReadOBJ(ob.Bytes())
		if err != nil {
			t.Fatalf("case %d: ReadOBJ: %v", i, err)
		}
		// %g prints shortest float64 representation: exact
		if len(o.Tris) != len(m.Tris) || math.Abs(vol(o)-v) > 1e-9*math.Max(1, math.Abs(v)) {
			t.Errorf("case %d: OBJ %d→%d tris, volume %.12g→%.12g", i, len(m.Tris), len(o.Tris), v, vol(o))
		}
		for k := range o.Tris {
			if o.Tris[k].P != m.Tris[k].P {
				t.Fatalf("case %d: OBJ triangle %d %v → %v", i, k, m.Tris[k].P, o.Tris[k].P)
			}
		}
	}
}

func TestPropertyGLBStructure(t *testing.T) {
	r := rand.New(rand.NewSource(10))
	for i := 0; i < 100; i++ {
		m := ptRandMesh(r)
		var buf bytes.Buffer
		if err := WriteGLB(&buf, m, fmt.Sprintf("n%d\"q", i)); err != nil {
			t.Fatal(err)
		}
		d := buf.Bytes()
		u := func(o int) uint32 { return binary.LittleEndian.Uint32(d[o:]) }
		if len(d) < 28 || u(0) != 0x46546C67 || u(4) != 2 || int(u(8)) != len(d) {
			t.Fatalf("case %d: bad header (len %d, header length %d)", i, len(d), u(8))
		}
		jl := int(u(12))
		if u(16) != 0x4E4F534A || jl%4 != 0 || 20+jl+8 > len(d) {
			t.Fatalf("case %d: bad JSON chunk (len %d)", i, jl)
		}
		bo := 20 + jl
		bl := int(u(bo))
		if u(bo+4) != 0x004E4942 || bl%4 != 0 || bo+8+bl != len(d) {
			t.Fatalf("case %d: bad BIN chunk (len %d, file %d)", i, bl, len(d))
		}
		var doc struct {
			Buffers     []struct{ ByteLength int } `json:"buffers"`
			BufferViews []struct {
				ByteOffset, ByteLength int
			} `json:"bufferViews"`
			Accessors []struct {
				BufferView, Count int
				Min, Max          []float64
			} `json:"accessors"`
			Meshes []struct {
				Primitives []struct {
					Attributes map[string]int
					Material   int
				}
			} `json:"meshes"`
			Materials []any `json:"materials"`
		}
		if err := json.Unmarshal(d[20:20+jl], &doc); err != nil {
			t.Fatalf("case %d: JSON: %v", i, err)
		}
		if len(doc.Buffers) != 1 || doc.Buffers[0].ByteLength > bl || bl-doc.Buffers[0].ByteLength > 3 {
			t.Fatalf("case %d: buffer byteLength %v vs chunk %d", i, doc.Buffers, bl)
		}
		for _, v := range doc.BufferViews {
			if v.ByteOffset < 0 || v.ByteOffset+v.ByteLength > doc.Buffers[0].ByteLength {
				t.Fatalf("case %d: bufferView %v out of range", i, v)
			}
		}
		verts := 0
		for _, p := range doc.Meshes[0].Primitives {
			pa := doc.Accessors[p.Attributes["POSITION"]]
			na := doc.Accessors[p.Attributes["NORMAL"]]
			if pa.Count != na.Count || pa.Count%3 != 0 || doc.BufferViews[pa.BufferView].ByteLength != 12*pa.Count {
				t.Fatalf("case %d: accessor counts %d/%d", i, pa.Count, na.Count)
			}
			if p.Material >= len(doc.Materials) || len(pa.Min) != 3 || len(pa.Max) != 3 {
				t.Fatalf("case %d: primitive %v", i, p)
			}
			verts += pa.Count
		}
		if verts != 3*len(m.Clean().Tris) {
			t.Fatalf("case %d: %d vertices for %d triangles", i, verts, len(m.Tris))
		}
	}
}
