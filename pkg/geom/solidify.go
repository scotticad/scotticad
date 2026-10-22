package geom

import "math"

// Solidify makes an imported mesh a single solid the booleans can work
// with. STL assemblies are often several closed shells that overlap (a
// post standing in a plate, each modelled whole), plus the odd shell with
// no volume (two triangles back to back). A boolean takes each operand's
// faces as the boundary between inside and outside, so a face buried in
// another shell, or a shell with nothing inside, makes it keep and drop
// the wrong pieces: holes in the result, millimetres wide. Here shells
// with no volume are dropped, and closed shells that overlap are unioned;
// cavities (shells inside out) are cut from the union afterwards, and a
// solid lying loose inside a cavity is kept as it is. A mesh whose shells
// don't overlap is returned as it is.
func (m *Mesh) Solidify() *Mesh {
	shells := m.Shells()
	if len(shells) < 2 {
		return m
	}
	var solids, cavities, other []*Mesh
	junk := false
	for _, s := range shells {
		o, nm := s.EdgeCheck(0)
		v := s.Volume()
		switch {
		case o != 0 || nm != 0:
			other = append(other, s) // not closed: nothing can be done
		case math.Abs(v) < 1e-9*math.Max(1, s.Bounds().Size().Len()):
			junk = true // no volume: a sheet, which no solid has
		case v > 0:
			solids = append(solids, s)
		default:
			cavities = append(cavities, s)
		}
	}
	// a solid inside a cavity (a loose part in a hollow) stays apart:
	// unioned with the rest it would be cut away with the cavity
	var loose []*Mesh
	if len(cavities) > 0 {
		tests := make([]*insideTest, len(cavities))
		kept := solids[:0:0]
		for _, s := range solids {
			sb := s.Bounds()
			in := false
			for i, c := range cavities {
				if !boxWithin(sb, c.Bounds()) {
					continue
				}
				if tests[i] == nil {
					tests[i] = newInsideTest(c.Flipped())
				}
				if tests[i].inside(sb.Center()) == 1 {
					in = true
					break
				}
			}
			if in {
				loose = append(loose, s)
			} else {
				kept = append(kept, s)
			}
		}
		solids = kept
	}
	overlap := false
	for i := range solids {
		for j := 0; j < i && !overlap; j++ {
			overlap = solids[i].Bounds().Overlaps(solids[j].Bounds(), csgEps)
		}
	}
	if !overlap {
		if !junk {
			return m
		}
		return Merge(append(append(append(solids, loose...), cavities...), other...)...)
	}
	acc := UnionAll(solids).FixTJunctions()
	for _, c := range cavities {
		acc = Subtract(acc, c.Flipped()).FixTJunctions()
	}
	if len(loose)+len(other) > 0 {
		acc = Merge(append(append([]*Mesh{acc}, loose...), other...)...)
	}
	return acc
}

// boxWithin is whether inner lies inside outer.
func boxWithin(inner, outer Box) bool {
	for k := 0; k < 3; k++ {
		if inner.Min.Axis(k) < outer.Min.Axis(k) || inner.Max.Axis(k) > outer.Max.Axis(k) {
			return false
		}
	}
	return true
}

// Shells splits m into its connected pieces: triangles that share an edge
// (exact end points) are in the same piece. Bodies that only touch at a
// corner are separate pieces. Order follows the first triangle of each.
func (m *Mesh) Shells() []*Mesh {
	parent := make([]int, len(m.Tris))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	owner := map[vedge]int{} // the first triangle on each undirected edge
	for i, t := range m.Tris {
		for k := 0; k < 3; k++ {
			a, b := t.P[k], t.P[(k+1)%3]
			if lessVec(b, a) {
				a, b = b, a
			}
			if j, ok := owner[vedge{a, b}]; ok {
				if x, y := find(i), find(j); x != y {
					parent[x] = y
				}
			} else {
				owner[vedge{a, b}] = i
			}
		}
	}
	idx := map[int]int{}
	var out []*Mesh
	for i, t := range m.Tris {
		r := find(i)
		k, ok := idx[r]
		if !ok {
			k = len(out)
			idx[r] = k
			out = append(out, &Mesh{})
		}
		out[k].Tris = append(out[k].Tris, t)
	}
	return out
}
