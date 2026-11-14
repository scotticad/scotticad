package shapes

// A QR code encoder for the QR Code shape: byte mode, versions 1 to 10
// (21 to 57 modules a side; larger codes have modules too small to
// print), all four error correction levels, and the mask chosen by the
// standard's penalty rules (ISO/IEC 18004).

// QR error correction levels, as the shape's dropdown orders them.
const (
	qrLow = iota
	qrMedium
	qrQuartile
	qrHigh
)

const qrMaxVersion = 10

// qrBlocks is, for each version and level, the error correction
// codewords per block and the blocks' data codewords: n1 blocks of d1,
// then n2 of d1+1.
type qrBlocks struct{ ec, n1, d1, n2 int }

var qrTable = [qrMaxVersion + 1][4]qrBlocks{
	1:  {{7, 1, 19, 0}, {10, 1, 16, 0}, {13, 1, 13, 0}, {17, 1, 9, 0}},
	2:  {{10, 1, 34, 0}, {16, 1, 28, 0}, {22, 1, 22, 0}, {28, 1, 16, 0}},
	3:  {{15, 1, 55, 0}, {26, 1, 44, 0}, {18, 2, 17, 0}, {22, 2, 13, 0}},
	4:  {{20, 1, 80, 0}, {18, 2, 32, 0}, {26, 2, 24, 0}, {16, 4, 9, 0}},
	5:  {{26, 1, 108, 0}, {24, 2, 43, 0}, {18, 2, 15, 2}, {22, 2, 11, 2}},
	6:  {{18, 2, 68, 0}, {16, 4, 27, 0}, {24, 4, 19, 0}, {28, 4, 15, 0}},
	7:  {{20, 2, 78, 0}, {18, 4, 31, 0}, {18, 2, 14, 4}, {26, 4, 13, 1}},
	8:  {{24, 2, 97, 0}, {22, 2, 38, 2}, {22, 4, 18, 2}, {26, 4, 14, 2}},
	9:  {{30, 2, 116, 0}, {22, 3, 36, 2}, {20, 4, 16, 4}, {24, 4, 12, 4}},
	10: {{18, 2, 68, 2}, {26, 4, 43, 1}, {24, 6, 19, 2}, {28, 6, 15, 2}},
}

// qrAlign are the alignment patterns' centre rows and columns.
var qrAlign = [qrMaxVersion + 1][]int{
	2: {6, 18}, 3: {6, 22}, 4: {6, 26}, 5: {6, 30}, 6: {6, 34},
	7: {6, 22, 38}, 8: {6, 24, 42}, 9: {6, 26, 46}, 10: {6, 28, 50},
}

func (b qrBlocks) dataCodewords() int { return b.n1*b.d1 + b.n2*(b.d1+1) }

// qrCapacity is how many bytes a version holds at a level.
func qrCapacity(ver, level int) int {
	count := 8
	if ver >= 10 {
		count = 16
	}
	return (qrTable[ver][level].dataCodewords()*8 - 4 - count) / 8
}

// QRMaxBytes is the most a QR Code shape holds at a level (version 10).
func QRMaxBytes(level int) int { return qrCapacity(qrMaxVersion, level) }

// qrSymbol is a QR code's modules, [row][column], true for dark.
type qrSymbol [][]bool

// qrEncode makes the smallest code holding data at the level, its mask
// chosen by penalty; ok is false if data is too long for version 10.
func qrEncode(data []byte, level int) (qrSymbol, bool) {
	for ver := 1; ver <= qrMaxVersion; ver++ {
		if len(data) <= qrCapacity(ver, level) {
			best, bestScore := qrSymbol(nil), 0
			for mask := 0; mask < 8; mask++ {
				s := qrEncodeAt(data, level, ver, mask)
				if p := s.penalty(); best == nil || p < bestScore {
					best, bestScore = s, p
				}
			}
			return best, true
		}
	}
	return nil, false
}

// qrEncodeAt makes the code at a given version and mask (data must fit).
func qrEncodeAt(data []byte, level, ver, mask int) qrSymbol {
	q := newQRGrid(ver)
	q.drawFunctionPatterns(level, mask)
	q.drawCodewords(qrCodewords(data, level, ver))
	q.applyMask(mask)
	return q.mod
}

// --- codewords -------------------------------------------------------------------------

// qrCodewords is the data in byte mode, padded, split into blocks with
// their error correction, and interleaved.
func qrCodewords(data []byte, level, ver int) []byte {
	tb := qrTable[ver][level]
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, v>>i&1 == 1)
		}
	}
	put(4, 4) // byte mode
	if ver >= 10 {
		put(len(data), 16)
	} else {
		put(len(data), 8)
	}
	for _, b := range data {
		put(int(b), 8)
	}
	capBits := tb.dataCodewords() * 8
	put(0, min(4, capBits-len(bits))) // terminator
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	for pad := 0; len(bits) < capBits; pad++ {
		put([]int{0xEC, 0x11}[pad%2], 8)
	}
	cw := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b {
			cw[i/8] |= 0x80 >> (i % 8)
		}
	}
	// blocks, each with its error correction
	var dataBlocks, ecBlocks [][]byte
	gen := rsGenerator(tb.ec)
	at := 0
	for i := 0; i < tb.n1+tb.n2; i++ {
		n := tb.d1
		if i >= tb.n1 {
			n++
		}
		blk := cw[at : at+n]
		at += n
		dataBlocks = append(dataBlocks, blk)
		ecBlocks = append(ecBlocks, rsRemainder(blk, gen))
	}
	var out []byte
	for i := 0; i < tb.d1+1; i++ {
		for _, b := range dataBlocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < tb.ec; i++ {
		for _, b := range ecBlocks {
			out = append(out, b[i])
		}
	}
	return out
}

// GF(256) arithmetic with the QR polynomial x^8 + x^4 + x^3 + x^2 + 1.
func gfMul(x, y byte) byte {
	var z byte
	for i := 7; i >= 0; i-- {
		hi := z&0x80 != 0
		z <<= 1
		if hi {
			z ^= 0x1D
		}
		if y>>i&1 != 0 {
			z ^= x
		}
	}
	return z
}

// rsGenerator is the Reed-Solomon generator polynomial of the given
// degree, highest power first, its leading 1 left out.
func rsGenerator(degree int) []byte {
	g := make([]byte, degree)
	g[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range g {
			g[j] = gfMul(g[j], root)
			if j+1 < len(g) {
				g[j] ^= g[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return g
}

// rsRemainder is data's error correction codewords.
func rsRemainder(data, gen []byte) []byte {
	r := make([]byte, len(gen))
	for _, b := range data {
		f := b ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i := range r {
			r[i] ^= gfMul(gen[i], f)
		}
	}
	return r
}

// --- the grid --------------------------------------------------------------------------

type qrGrid struct {
	n   int
	mod qrSymbol // dark modules
	fn  [][]bool // function patterns, which data and masks leave alone
	ver int
}

func newQRGrid(ver int) *qrGrid {
	n := 17 + 4*ver
	q := &qrGrid{n: n, ver: ver, mod: make(qrSymbol, n), fn: make([][]bool, n)}
	for i := range q.mod {
		q.mod[i] = make([]bool, n)
		q.fn[i] = make([]bool, n)
	}
	return q
}

// set marks the function module at column x, row y.
func (q *qrGrid) set(x, y int, dark bool) {
	q.mod[y][x] = dark
	q.fn[y][x] = true
}

func (q *qrGrid) drawFunctionPatterns(level, mask int) {
	n := q.n
	for i := 0; i < n; i++ { // timing
		q.set(6, i, i%2 == 0)
		q.set(i, 6, i%2 == 0)
	}
	for _, c := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} { // finders and separators
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if x >= 0 && x < n && y >= 0 && y < n {
					d := max(abs(dx), abs(dy))
					q.set(x, y, d != 2 && d != 4)
				}
			}
		}
	}
	al := qrAlign[q.ver]
	for i, ay := range al {
		for j, ax := range al {
			if i == 0 && j == 0 || i == 0 && j == len(al)-1 || i == len(al)-1 && j == 0 {
				continue // a finder is there
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.set(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	q.drawFormat(level, mask)
	if q.ver >= 7 {
		rem := q.ver
		for i := 0; i < 12; i++ {
			rem = rem<<1 ^ (rem>>11)*0x1F25
		}
		bits := q.ver<<12 | rem
		for i := 0; i < 18; i++ {
			dark := bits>>i&1 == 1
			a, b := n-11+i%3, i/3
			q.set(a, b, dark)
			q.set(b, a, dark)
		}
	}
}

// qrLevelBits are the levels' two format bits.
var qrLevelBits = [4]int{qrLow: 1, qrMedium: 0, qrQuartile: 3, qrHigh: 2}

func (q *qrGrid) drawFormat(level, mask int) {
	data := qrLevelBits[level]<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return bits>>i&1 == 1 }
	n := q.n
	for i := 0; i <= 5; i++ {
		q.set(8, i, bit(i))
	}
	q.set(8, 7, bit(6))
	q.set(8, 8, bit(7))
	q.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		q.set(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.set(8, n-15+i, bit(i))
	}
	q.set(8, n-8, true) // the dark module
}

// drawCodewords lays the codewords' bits up and down the two-column
// strips from the right, skipping function modules.
func (q *qrGrid) drawCodewords(cw []byte) {
	i := 0
	for right := q.n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // the vertical timing pattern
		}
		for vert := 0; vert < q.n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = q.n - 1 - vert // upward
				}
				if !q.fn[y][x] && i < len(cw)*8 {
					q.mod[y][x] = cw[i>>3]>>(7-i&7)&1 == 1
					i++
				}
			}
		}
	}
}

func qrMaskAt(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (q *qrGrid) applyMask(mask int) {
	for y := 0; y < q.n; y++ {
		for x := 0; x < q.n; x++ {
			if !q.fn[y][x] && qrMaskAt(mask, x, y) {
				q.mod[y][x] = !q.mod[y][x]
			}
		}
	}
}

// --- penalty ---------------------------------------------------------------------------

// penalty scores a symbol by the standard's four rules: long runs, 2 × 2
// blocks, finder-like patterns, and dark/light imbalance. Lower is better.
func (s qrSymbol) penalty() int {
	n := len(s)
	at := func(r, c int, cols bool) bool {
		if cols {
			return s[c][r]
		}
		return s[r][c]
	}
	score := 0
	for _, cols := range []bool{false, true} {
		for r := 0; r < n; r++ {
			run := 1
			for c := 1; c <= n; c++ {
				if c < n && at(r, c, cols) == at(r, c-1, cols) {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			// 1:1:3:1:1 with four light modules on either side
			for c := 0; c+11 <= n; c++ {
				a, b := true, true
				for k, want := range [11]bool{true, false, true, true, true, false, true, false, false, false, false} {
					if at(r, c+k, cols) != want {
						a = false
					}
					if at(r, c+10-k, cols) != want {
						b = false
					}
				}
				if a {
					score += 40
				}
				if b {
					score += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if s[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n && s[y][x] == s[y][x+1] && s[y][x] == s[y+1][x] && s[y][x] == s[y+1][x+1] {
				score += 3
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	if k > 0 {
		score += k * 10
	}
	return score
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
