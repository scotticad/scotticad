package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Computers & networking: fixed models (no parameters) built from boxes,
// cylinders and balls. Each is built once, in millimetres; its size is
// the model's own size.

// --- building blocks ------------------------------------------------------------

func cuboid(x0, y0, z0, x1, y1, z1 float64) *geom.Mesh {
	c := geom.V((x0+x1)/2, (y0+y1)/2, (z0+z1)/2)
	s := geom.V(x1-x0, y1-y0, z1-z0)
	return extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {-0.5, 0.5}}).Transform(geom.Translate(c).Mul(geom.Scale(s)))
}

// cylZ is a cylinder along Z.
func cylZ(cx, cy, z0, z1, r float64, n int) *geom.Mesh {
	return revolve([][2]float64{{0, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {0, 0.5}}, n, false).
		Transform(geom.Translate(geom.V(cx, cy, (z0+z1)/2)).Mul(geom.Scale(geom.V(2*r, 2*r, z1-z0))))
}

// cylY is a cylinder along Y.
func cylY(cx, y0, y1, cz, r float64, n int) *geom.Mesh {
	return revolve([][2]float64{{0, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {0, 0.5}}, n, false).
		Transform(geom.Translate(geom.V(cx, (y0+y1)/2, cz)).Mul(geom.RotateX(math.Pi / 2)).Mul(geom.Scale(geom.V(2*r, 2*r, y1-y0))))
}

func ball(c geom.Vec3, r float64, n int) *geom.Mesh {
	return revolve(arc(0, 0, r, -math.Pi/2, math.Pi/2, n/2), n, false).Transform(geom.Translate(c))
}

// union joins overlapping parts; cut subtracts (disjoint) cutters.
func union(ms ...*geom.Mesh) *geom.Mesh { return geom.UnionAll(ms) }

func cut(a *geom.Mesh, cutters ...*geom.Mesh) *geom.Mesh {
	return geom.Subtract(a, geom.Merge(cutters...))
}

func done(m *geom.Mesh) *geom.Mesh { return m.FixTJunctions().Clean().Oriented() }

// --- models ------------------------------------------------------------------------

func laptopModel() *geom.Mesh {
	base := cuboid(-17, -12, 0, 17, 12, 1.6)
	base = cut(base,
		cuboid(-15, -1, 1.2, 15, 10, 1.7), // keyboard well
		cuboid(-5, -10, 1.35, 5, -3, 1.7), // touchpad
	)
	var keys []*geom.Mesh
	for r := 0; r < 4; r++ {
		for c := 0; c < 13; c++ {
			x := -14.6 + float64(c)*2.25
			y := -0.6 + float64(r)*2.6
			keys = append(keys, cuboid(x, y, 1.1, x+1.9, y+2.2, 1.5))
		}
	}
	base = geom.Union(base, geom.Merge(keys...))
	// the lid, opened 105° about the hinge at the back
	lid := cut(cuboid(-17, -23, 0, 17, 0, 1), cuboid(-15.5, -21.5, -0.1, 15.5, -1.5, 0.35))
	lid = lid.Transform(geom.Translate(geom.V(0, 11.6, 1.2)).Mul(geom.RotateX(-105 * math.Pi / 180)))
	return done(geom.Union(base, lid))
}

func monitorModel() *geom.Mesh {
	panel := cut(cuboid(-20, -1, 8, 20, 1.5, 34), cuboid(-18.5, -1.1, 9.5, 18.5, -0.6, 32.5))
	return done(union(
		panel,
		cuboid(-10, 1.4, 14, 10, 3.5, 28),  // back
		cuboid(-2.5, 2.5, 1, 2.5, 4.5, 20), // neck
		cuboid(-10, -4, 0, 10, 9, 1.2),     // foot
	))
}

func keyboardModel() *geom.Mesh {
	base := cuboid(-22, -7.5, 0, 22, 7.5, 1.8)
	var keys []*geom.Mesh
	key := func(x0, x1, y0 float64) { keys = append(keys, cuboid(x0, y0, 1.6, x1, y0+2.3, 2.8)) }
	for r := 1; r < 5; r++ {
		y := -6.8 + float64(r)*2.8
		for c := 0; c < 14; c++ {
			x := -21 + float64(c)*3
			key(x, x+2.5, y)
		}
	}
	// bottom row: three keys each side of the space bar
	for c := 0; c < 3; c++ {
		x := -21 + float64(c)*3
		key(x, x+2.5, -6.8)
		key(-x-2.5, -x, -6.8)
	}
	key(-11.5, 11.5, -6.8)
	return done(geom.Union(base, geom.Merge(keys...)))
}

func mouseModel() *geom.Mesh {
	prof := append([][2]float64{{0, 0}}, arc(0, 0, 0.5, 0, math.Pi/2, 16)...)
	body := revolve(prof, 40, false).Transform(geom.Scale(geom.V(12, 20, 14)))
	body = cut(body, cuboid(-0.3, 1, 3, 0.3, 11, 8)) // between the buttons
	wheel := cylZ(0, 0, -0.4, 0.4, 1.2, 20).Transform(geom.Translate(geom.V(0, 4, 6.1)).Mul(geom.RotateY(math.Pi / 2)))
	return done(geom.Union(body, wheel))
}

func desktopModel() *geom.Mesh {
	tower := cuboid(-5, -11, 0, 5, 11, 24)
	var cutters []*geom.Mesh
	cutters = append(cutters, cuboid(-3.5, -11.1, 19, 3.5, -10.5, 21), cuboid(-3.5, -11.1, 16, 3.5, -10.5, 18)) // drive bays
	for i := 0; i < 6; i++ {
		z := 2 + float64(i)*1.2
		cutters = append(cutters, cuboid(-3.5, -11.1, z, 3.5, -10.6, z+0.5)) // vents
	}
	tower = cut(tower, cutters...)
	return done(geom.Union(tower, cylY(0, -11.4, -10.5, 12, 1, 20))) // power button
}

func serverModel() *geom.Mesh {
	body := cuboid(-24, -15, 0, 24, 15, 4.4)
	var bays []*geom.Mesh
	for i := 0; i < 8; i++ {
		x := -22 + float64(i)*5
		bays = append(bays, cuboid(x, -15.1, 0.8, x+4, -14.4, 3.6))
	}
	bays = append(bays, cylY(19, -15.1, -14.5, 2.2, 0.5, 12), cylY(21, -15.1, -14.5, 2.2, 0.5, 12)) // LEDs
	body = cut(body, bays...)
	return done(union(body,
		cuboid(-26, -15, 0.3, -23.5, -14, 4.1), // rack ears
		cuboid(23.5, -15, 0.3, 26, -14, 4.1),
	))
}

func rackModel() *geom.Mesh {
	cab := cut(cuboid(-12, -12, 0, 12, 12, 48), cuboid(-10.5, -12.1, 2, 10.5, 10, 46))
	var units []*geom.Mesh
	for i := 0; i < 5; i++ {
		z := 3.5 + float64(i)*8.3
		u := cuboid(-10.6, -11.5, z, 10.6, 10.1, z+6.5)
		var slots []*geom.Mesh
		for j := 0; j < 4; j++ {
			x := -9 + float64(j)*4.5
			slots = append(slots, cuboid(x, -11.6, z+1.2, x+3.5, -11, z+5.3))
		}
		units = append(units, cut(u, slots...))
	}
	return done(geom.Union(cab, geom.Merge(units...)))
}

func routerModel() *geom.Mesh {
	var leds []*geom.Mesh
	for i := 0; i < 6; i++ {
		x := -7 + float64(i)*2.8
		leds = append(leds, cuboid(x, -8.1, 2, x+1, -7.6, 2.8))
	}
	body := cut(cuboid(-12, -8, 0, 12, 8, 4), leds...)
	return done(union(body,
		cylZ(-9, 6, 3.5, 18, 0.8, 16),
		cylZ(0, 6, 3.5, 18, 0.8, 16),
		cylZ(9, 6, 3.5, 18, 0.8, 16),
	))
}

func switchModel() *geom.Mesh {
	var ports []*geom.Mesh
	for i := 0; i < 12; i++ {
		x := -20 + float64(i)*3
		ports = append(ports, cuboid(x, -10.1, 0.5, x+2.4, -8, 1.5), cuboid(x, -10.1, 1.9, x+2.4, -8, 2.9))
	}
	ports = append(ports, cuboid(16.5, -10.1, 1.2, 18.5, -8.5, 2.2), cuboid(19, -10.1, 1.2, 21, -8.5, 2.2)) // SFP
	body := cut(cuboid(-22, -10, 0, 22, 10, 3.5), ports...)
	return done(union(body,
		cuboid(-24, -10, 0.3, -21.5, -9, 3.2),
		cuboid(21.5, -10, 0.3, 24, -9, 3.2),
	))
}

func cloudModel() *geom.Mesh {
	puffs := union(
		ball(geom.V(0, 0, 6), 6, 28),
		ball(geom.V(-7, 0, 4), 4.5, 24),
		ball(geom.V(7, 0, 4.5), 5, 24),
		ball(geom.V(-3, 0.5, 9), 4.5, 24),
	)
	flat := geom.Intersect(puffs, cuboid(-20, -20, 0.8, 20, 20, 30))
	return done(flat.Transform(geom.Scale(geom.V(1, 0.7, 1))))
}

func databaseModel() *geom.Mesh {
	return done(union(
		cylZ(0, 0, 0, 4, 8, 48),
		cylZ(0, 0, 5, 9, 8, 48),
		cylZ(0, 0, 10, 14, 8, 48),
		cylZ(0, 0, 0.5, 13.5, 7.5, 48),
	))
}

func wifiModel() *geom.Mesh {
	contours := [][]Pt{}
	var dot []Pt
	for i := 0; i < 32; i++ {
		a := 2 * math.Pi * float64(i) / 32
		dot = append(dot, Pt{1.5 * math.Cos(a), 1.5 * math.Sin(a)})
	}
	contours = append(contours, dot)
	for _, rr := range [][2]float64{{3.5, 5.5}, {7.5, 9.5}, {11.5, 13.5}} {
		var s []Pt
		const n = 24
		for i := 0; i <= n; i++ {
			a := math.Pi/4 + math.Pi/2*float64(i)/n
			s = append(s, Pt{rr[1] * math.Cos(a), rr[1] * math.Sin(a)})
		}
		for i := n; i >= 0; i-- {
			a := math.Pi/4 + math.Pi/2*float64(i)/n
			s = append(s, Pt{rr[0] * math.Cos(a), rr[0] * math.Sin(a)})
		}
		contours = append(contours, s)
	}
	return done(ExtrudeContours(contours, 0, 3))
}

func laserPrinterModel() *geom.Mesh {
	body := cut(cuboid(-15, -13, 0, 15, 13, 14),
		cuboid(-11, -9, 11, 11, 8, 14.1),        // output tray
		cuboid(-13, -13.1, 3.8, 13, -12.6, 4.2), // paper drawer seam
		cuboid(-4, -13.1, 1.5, 4, -12.4, 3),     // drawer handle
	)
	return done(union(body,
		cuboid(-8.5, -6, 10.8, 8.5, 7, 11.4), // printed page
		cuboid(9, -12.5, 13.5, 14, -8, 15),   // control panel
	))
}

func printer3DModel() *geom.Mesh {
	spool := cut(cylY(0, -1.5, 1.5, 35.5, 5, 32), cylY(0, -1.6, 1.6, 35.5, 1.2, 16))
	return done(union(
		cuboid(-16, -14, 0, 16, 14, 2),       // base
		cuboid(-3, -8, 1.8, 3, 8, 3.6),       // bed carriage
		cuboid(-11, -11, 3.5, 11, 11, 4.2),   // bed
		cuboid(-15, -1.5, 1.8, -12, 1.5, 30), // uprights
		cuboid(12, -1.5, 1.8, 15, 1.5, 30),
		cuboid(-15, -1.5, 28, 15, 1.5, 31),       // top bar
		cuboid(-12.2, -0.8, 14, 12.2, 0.8, 15.5), // X rail
		cuboid(-2.5, -3.5, 11, 2.5, 1, 17),       // print head
		cylZ(0, -2, 9.5, 11.2, 0.6, 12),          // nozzle
		spool,
	))
}

// --- catalogue --------------------------------------------------------------------

// TechKinds lists the computers & networking shapes in library order.
var TechKinds []string

func registerModel(id, label, color string, model func() *geom.Mesh) {
	m := model()
	b := m.Bounds()
	m = m.Transform(geom.Translate(b.Center().Neg()))
	register(&Kind{ID: id, Label: label, Color: color, Size: b.Size(),
		build: func(s geom.Vec3, v Values) *geom.Mesh { return m.FitTo(s) }})
	TechKinds = append(TechKinds, id)
}

func init() {
	registerModel("laptop", "Laptop", "#5a6470", laptopModel)
	registerModel("monitor", "Monitor", "#2b3138", monitorModel)
	registerModel("keyboard", "Keyboard", "#3a4048", keyboardModel)
	registerModel("mouse", "Mouse", "#8a939c", mouseModel)
	registerModel("desktop", "Desktop PC", "#3a4048", desktopModel)
	registerModel("server", "Rackmount Server", "#4a5560", serverModel)
	registerModel("rack", "Equipment Rack", "#23282e", rackModel)
	registerModel("router", "Router", "#e8eaed", routerModel)
	registerModel("switch", "Network Switch", "#1c6fb8", switchModel)
	registerModel("cloud", "Cloud", "#9fd2f2", cloudModel)
	registerModel("database", "Database", "#2f8f5b", databaseModel)
	registerModel("wifi", "Wi-Fi", "#1c9ad6", wifiModel)
	registerModel("laserprinter", "Laser Printer", "#d8dbe0", laserPrinterModel)
	registerModel("printer3d", "3D Printer", "#f08a24", printer3DModel)
}
