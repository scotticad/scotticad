package geom

import "math"

// The repair's bookkeeping: corners numbered by exact position, and edges
// counted between numbers. Hashing a float position (or a pair of them,
// for an edge) in a Go map is most of what a repair of a large mesh costs;
// these tables hash each position once and edges as one 64-bit key.

// vertexIndex numbers a mesh's corners by exact position, in the order
// they first appear, triangle by triangle: positions equal as floats share
// a number (-0 and +0 alike, as with a map[Vec3] key), and a position with
// a NaN in it is never equal to anything, so each of its corners has a
// number of its own.
type vertexIndex struct {
	pos   []Vec3     // by number
	tri   [][3]int32 // each triangle's corners' numbers
	slots []int32    // hash table: number+1, 0 empty
	shift uint
}

func posHash(p Vec3) uint64 {
	z := func(f float64) uint64 {
		if f == 0 {
			f = 0 // -0 hashes as +0, as it compares equal
		}
		return math.Float64bits(f)
	}
	h := z(p.X)*0x9E3779B97F4A7C15 ^ z(p.Y)*0xC2B2AE3D27D4EB4F ^ z(p.Z)*0x165667B19E3779F9
	return h ^ h>>29
}

func tableBits(n int) uint {
	bits := uint(4)
	for 1<<bits < 2*n {
		bits++
	}
	return bits
}

func indexMesh(m *Mesh) *vertexIndex {
	bits := tableBits(3 * len(m.Tris))
	vi := &vertexIndex{
		pos:   make([]Vec3, 0, len(m.Tris)),
		tri:   make([][3]int32, len(m.Tris)),
		slots: make([]int32, 1<<bits),
		shift: 64 - bits,
	}
	mask := len(vi.slots) - 1
	for i, t := range m.Tris {
		for k, p := range t.P {
			if p != p { // NaN: equal to nothing
				vi.tri[i][k] = int32(len(vi.pos))
				vi.pos = append(vi.pos, p)
				continue
			}
			s := int(posHash(p) >> vi.shift)
			for {
				v := vi.slots[s]
				if v == 0 {
					vi.slots[s] = int32(len(vi.pos)) + 1
					vi.tri[i][k] = int32(len(vi.pos))
					vi.pos = append(vi.pos, p)
					break
				}
				if vi.pos[v-1] == p {
					vi.tri[i][k] = v - 1
					break
				}
				s = (s + 1) & mask
			}
		}
	}
	return vi
}

// cellTable maps weld grid cells to vertices; a cell, once set, is never
// changed. Most cells a weld looks in are empty (it looks in the 27 around
// a new vertex): a bit per hash, small enough to stay in cache, answers
// most of those without the table.
type cellTable struct {
	keys  [][3]int64
	vals  []int32 // vertex+1, 0 empty
	shift uint
	seen  []uint64 // bit h>>seenShift set: some cell with hash h may be set
	sbits uint
}

func newCellTable(n int) *cellTable {
	bits := tableBits(n)
	sbits := bits + 3 // 16 bits per cell the table can hold
	return &cellTable{keys: make([][3]int64, 1<<bits), vals: make([]int32, 1<<bits), shift: 64 - bits,
		seen: make([]uint64, 1<<(sbits-6)), sbits: sbits}
}

func cellHash(k [3]int64) uint64 {
	h := uint64(k[0])*0x9E3779B97F4A7C15 ^ uint64(k[1])*0xC2B2AE3D27D4EB4F ^ uint64(k[2])*0x165667B19E3779F9
	h ^= h >> 29
	return h * 0x9E3779B97F4A7C15
}

func (c *cellTable) slot(k [3]int64, h uint64) int {
	mask := len(c.keys) - 1
	s := int(h >> c.shift)
	for c.vals[s] != 0 && c.keys[s] != k {
		s = (s + 1) & mask
	}
	return s
}

func (c *cellTable) get(k [3]int64) (int, bool) {
	h := cellHash(k)
	if b := h >> (64 - c.sbits); c.seen[b>>6]&(1<<(b&63)) == 0 {
		return -1, false
	}
	v := c.vals[c.slot(k, h)]
	return int(v) - 1, v != 0
}

// set gives an empty cell a vertex.
func (c *cellTable) set(k [3]int64, v int) {
	h := cellHash(k)
	b := h >> (64 - c.sbits)
	c.seen[b>>6] |= 1 << (b & 63)
	s := c.slot(k, h)
	c.keys[s], c.vals[s] = k, int32(v)+1
}

// edgeTable counts directed edges between numbered vertices, and keeps the
// last triangle (in mesh order) to use each.
type edgeTable struct {
	keys  []uint64 // a<<32 | b; emptyEdge where unused
	count []int32
	last  []int32
	shift uint
}

const emptyEdge = ^uint64(0)

func newEdgeTable(n int) *edgeTable {
	bits := tableBits(n)
	e := &edgeTable{keys: make([]uint64, 1<<bits), count: make([]int32, 1<<bits), last: make([]int32, 1<<bits), shift: 64 - bits}
	for i := range e.keys {
		e.keys[i] = emptyEdge
	}
	return e
}

// edgesOf counts the edges of triangles given by vertex numbers; an edge
// whose ends are one vertex (a triangle of no area) is left out.
func edgesOf(tris [][3]int32) *edgeTable {
	e := newEdgeTable(3 * len(tris))
	for i, t := range tris {
		for k := 0; k < 3; k++ {
			if a, b := t[k], t[(k+1)%3]; a != b {
				s := e.slot(a, b)
				e.count[s]++
				e.last[s] = int32(i)
			}
		}
	}
	return e
}

// slot is where edge a-b is, or goes.
func (e *edgeTable) slot(a, b int32) int {
	k := uint64(uint32(a))<<32 | uint64(uint32(b))
	mask := len(e.keys) - 1
	s := int((k * 0x9E3779B97F4A7C15) >> e.shift)
	for {
		switch e.keys[s] {
		case k:
			return s
		case emptyEdge:
			e.keys[s] = k
			return s
		}
		s = (s + 1) & mask
	}
}

// get is how many times edge a-b is used.
func (e *edgeTable) get(a, b int32) int32 {
	k := uint64(uint32(a))<<32 | uint64(uint32(b))
	mask := len(e.keys) - 1
	s := int((k * 0x9E3779B97F4A7C15) >> e.shift)
	for {
		switch e.keys[s] {
		case k:
			return e.count[s]
		case emptyEdge:
			return 0
		}
		s = (s + 1) & mask
	}
}

// open reports whether edge a-b isn't closed: used a different number of
// times each way.
func (e *edgeTable) open(a, b int32) bool { return e.get(a, b) != e.get(b, a) }

// each calls f for every edge counted, in no particular order.
func (e *edgeTable) each(f func(a, b, n int32)) {
	for s, k := range e.keys {
		if k != emptyEdge {
			f(int32(k>>32), int32(uint32(k)), e.count[s])
		}
	}
}

// openVertices flags the vertices on edges the mesh doesn't close (used a
// different number of times each way, or folded: used more than once each
// way), and counts them.
func openVertices(vi *vertexIndex) ([]bool, int) {
	e := edgesOf(vi.tri)
	flag := make([]bool, len(vi.pos))
	n := 0
	mark := func(v int32) {
		if !flag[v] {
			flag[v] = true
			n++
		}
	}
	e.each(func(a, b, c int32) {
		if r := e.get(b, a); r != c || c > 1 {
			mark(a)
			mark(b)
		}
	})
	return flag, n
}
