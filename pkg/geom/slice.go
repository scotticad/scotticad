package geom

import (
	"bufio"
	"fmt"
	"io"
	"math"
)

// Slice cuts m with the plane z = h and returns the closed outlines where
// the plane meets it (each a polygon in XY, without repeating its first
// point): outer outlines counter-clockwise, holes clockwise, seen from
// above. Collinear points are dropped. m's triangles must face outwards.
func Slice(m *Mesh, h float64) [][]Vec3 {
	type seg struct{ a, b Vec3 }
	var segs []seg
	for _, t := range m.Tris {
		var d [3]float64
		for i, p := range t.P {
			d[i] = p.Z - h
			if d[i] == 0 {
				d[i] = 1e-12 // a vertex on the plane counts as above it
			}
		}
		var pts []Vec3
		for i := 0; i < 3; i++ {
			j := (i + 1) % 3
			if (d[i] < 0) != (d[j] < 0) {
				s := d[i] / (d[i] - d[j])
				p := t.P[i].Lerp(t.P[j], s)
				p.Z = h
				pts = append(pts, p)
			}
		}
		if len(pts) == 2 && pts[0].Sub(pts[1]).Len() > 1e-9 {
			// direct it with the solid on its left, seen from above: the
			// solid lies against the triangle's normal
			d := pts[1].Sub(pts[0])
			if V(0, 0, 1).Cross(d).Dot(t.Normal()) > 0 {
				pts[0], pts[1] = pts[1], pts[0]
			}
			segs = append(segs, seg{pts[0], pts[1]})
		}
	}
	// chain segments end to end through shared (welded) points
	key := func(p Vec3) [2]int64 {
		return [2]int64{int64(math.Round(p.X * 1e5)), int64(math.Round(p.Y * 1e5))}
	}
	from := map[[2]int64][]int{} // segments by start point
	for i, s := range segs {
		from[key(s.a)] = append(from[key(s.a)], i)
	}
	used := make([]bool, len(segs))
	var loops [][]Vec3
	for i := range segs {
		if used[i] {
			continue
		}
		used[i] = true
		loop := []Vec3{segs[i].a}
		cur, start := segs[i].b, key(segs[i].a)
		in := segs[i].b.Sub(segs[i].a)
		for key(cur) != start {
			loop = append(loop, cur)
			// where outlines touch at a point, take the sharpest left
			// turn: that keeps to this outline's own solid
			next, best := -1, math.Inf(-1)
			for _, j := range from[key(cur)] {
				if used[j] {
					continue
				}
				out := segs[j].b.Sub(segs[j].a)
				turn := math.Atan2(in.Cross(out).Z, in.X*out.X+in.Y*out.Y)
				if turn > best {
					next, best = j, turn
				}
			}
			if next < 0 {
				break // open chain (non-watertight input): keep what we have
			}
			used[next] = true
			in = segs[next].b.Sub(segs[next].a)
			cur = segs[next].b
		}
		if l := simplifyLoop(loop); len(l) >= 3 {
			loops = append(loops, l)
		}
	}
	return loops
}

// simplifyLoop drops points that lie on the line through their neighbours.
func simplifyLoop(pts []Vec3) []Vec3 {
	for changed := true; changed && len(pts) > 3; {
		changed = false
		for i := 0; i < len(pts) && len(pts) > 3; i++ {
			p, q, r := pts[(i+len(pts)-1)%len(pts)], pts[i], pts[(i+1)%len(pts)]
			if math.Abs(q.Sub(p).Cross(r.Sub(q)).Z) < 1e-9*math.Max(1, r.Sub(p).Len()) {
				pts = append(pts[:i:i], pts[i+1:]...)
				changed = true
				i--
			}
		}
	}
	return pts
}

// WriteSVG writes outlines (in millimetres, XY) as an SVG for laser
// cutting: one path, hairline black stroke, no fill, Y flipped so the
// drawing reads as seen from above.
func WriteSVG(w io.Writer, loops [][]Vec3) error {
	b := EmptyBox()
	for _, l := range loops {
		for _, p := range l {
			b = b.Extend(p)
		}
	}
	if b.IsEmpty() {
		return fmt.Errorf("nothing to draw")
	}
	const margin = 1.0
	minX, maxY := b.Min.X-margin, b.Max.Y+margin
	wd, ht := b.Size().X+2*margin, b.Size().Y+2*margin
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(bw, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%gmm\" height=\"%gmm\" viewBox=\"0 0 %g %g\">\n", r3(wd), r3(ht), r3(wd), r3(ht))
	fmt.Fprintf(bw, "<path fill=\"none\" stroke=\"#000000\" stroke-width=\"0.1\" fill-rule=\"evenodd\" d=\"")
	for _, l := range loops {
		for i, p := range l {
			c := "L"
			if i == 0 {
				c = "M"
			}
			fmt.Fprintf(bw, "%s%g %g ", c, r3(p.X-minX), r3(maxY-p.Y))
		}
		fmt.Fprintf(bw, "Z ")
	}
	fmt.Fprintf(bw, "\"/>\n</svg>\n")
	return bw.Flush()
}

func r3(v float64) float64 { return math.Round(v*1000) / 1000 }
