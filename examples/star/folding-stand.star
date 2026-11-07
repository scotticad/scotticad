# folding-stand.star - a phone or tablet stand that prints flat, in one
# piece, with its hinges already assembled: no supports, no screws, no glue.
#
# HOW IT WORKS. Three parts lie side by side in one flat slab, joined by two
# print-in-place hinges:
#   - the BASE, a frame that stays on the desk;
#   - the PANEL the device leans on, lying in a cut-out at the front of the
#     base, hinged to it;
#   - the PROP, lying in a window in the panel, hinged to the panel.
# To stand it up, lift the panel's far end up and over the hinge, so the
# panel leans back over the base, then swing the prop out from behind it
# and drop its foot into one of the notches in the base. Each notch holds
# the panel at one angle (ANGLES); the angle is engraved beside it. The
# device stands in front of the panel, its bottom edge on the base's side
# rails, held by two lips, and a charging cable passes down through the
# open front of the frame. Fold it the other way to carry it flat.
#
# THE HINGES. Each hinge is a row of knuckles, alternately on one part and
# the other, with a cone-shaped pin on each fixed knuckle reaching into a
# cone-shaped socket in its moving neighbour. The cones have 45-degree
# sides, so they print without supports, and the clearance (CLEAR) is the
# same all round. The hinge axis is level with the top of the slab, not
# its middle, so the parts can swing nearly 180 degrees without hitting
# each other. Each knuckle's underside on the side facing the other part
# is cut at 45 degrees too, so nothing overhangs.
#
# PRINTING. Print it as it comes, flat on the bed: 0.2 mm layers, 3 walls,
# 15% infill. A brim isn't needed. When it's done, flex each hinge a few
# times to free it. If the hinges are fused, raise CLEAR by 0.1 mm; if
# they're loose, lower it. The base and the moving parts have their own
# colours (COLORS) for printers that change filament by part.
#
# The phone size fits a 180 x 180 mm bed; the tablet size needs about 225
# mm. Basic shapes and outlines only. Sizes are in mm.

SIZE = "phone"           # "phone" or "tablet": a key of SIZES
DEVICE = 12.0            # the device's thickness, in its case
ANGLES = [50, 57, 64, 71]    # the panel's angles from the desk, in degrees:
                             # one notch each, shallowest first
CLEAR = 0.4              # gap between moving parts (0.3-0.5 for most printers)
LABELS = True            # engrave each notch's angle beside it
COLORS = ("#2f3640", "#e1b12c")   # the base; the panel and prop

# W: the panel's width; P: its length from the hinge; PIVOT: how far up the
# panel the prop is hinged; PROP: the prop's length; PROP_W: its width;
# RAIL: the base's side rails; T: the slab's thickness.
SIZES = {
    "phone": {"W": 60.0, "P": 95.0, "PIVOT": 44.0, "PROP": 44.0, "PROP_W": 40.0, "RAIL": 12.0, "T": 4.0},
    "tablet": {"W": 84.0, "P": 128.0, "PIVOT": 60.0, "PROP": 60.0, "PROP_W": 56.0, "RAIL": 15.0, "T": 5.0},
}

FRONT = 6.0              # the base's front bar
END = 6.5                # the panel's end beyond the prop window
NOTCH = 1.6              # notch depth
LIP_H = 7.0              # the device lips' height above the base
LABEL_DEPTH = 0.6
SIDES = 40               # cones and knuckle outlines

S = SIZES[SIZE]
T = S["T"]
R = T                    # knuckle radius: a knuckle is as tall as two slabs
A = R + CLEAR            # a part's edge from a hinge axis it doesn't own
W, P, PIVOT, PROP, PW = S["W"], S["P"], S["PIVOT"], S["PROP"], S["PROP_W"]
HALF = W / 2 + CLEAR + S["RAIL"]   # the base's half-width

# --- helpers ----------------------------------------------------------------

def box(lo, hi, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def union(ids, name = ""):
    g = cad.group("union", ids = ids)
    if name:
        cad.set_name(name, ids = [g])
    return g

def copy_of(id, dx = 0.0):
    cad.select([id])
    new = cad.duplicate()[0]
    cad.clear_selection()
    if dx:
        cad.move(dx = dx, ids = [new])
    return new

def along_x(pts, x0, x1):
    """An outline drawn in the (y, z) plane, extruded along x from x0 to x1."""
    id = cad.add_outline(pts, z0 = 0, z1 = x1 - x0)
    cad.rotate("x", 90, ids = [id])     # outline y up to z
    cad.rotate("z", 90, ids = [id])     # outline x round to y, extrusion along x
    ys = [p[0] for p in pts]
    zs = [p[1] for p in pts]
    cad.set_bounds(min = [x0, min(ys), min(zs)], max = [x1, max(ys), max(zs)], ids = [id])
    return id

def cone_x(x_base, y, z, r, h, dir, hole = False):
    """A 45-degree cone lying along x: base radius r at x_base, its point h
    away in direction dir (+1 or -1)."""
    id = cad.add_shape("cone", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_param(id, "top", 0)
    cad.rotate("y", 90 * dir, ids = [id])   # the point toward +x or -x
    xa, xb = x_base, x_base + h * dir
    cad.set_bounds(min = [min(xa, xb), y - r, z - r], max = [max(xa, xb), y + r, z + r], ids = [id])
    return id

# --- the hinge ----------------------------------------------------------------
# A hinge's axis runs along x at height T, at y = y0. The FIXED part lies on
# the +y side of it, the MOVING part on the -y side; the moving part swings
# up and over toward +y. Knuckles alternate fixed, moving, fixed..., an odd
# number, so the fixed part holds both ends.

def knuckle(y0, x0, x1, outer):
    """A knuckle from x0 to x1: a circle round the axis, its lower quarter
    on the side facing the other part (outer: +1 or -1) cut at 45 degrees."""
    pts = []
    skip = (270, 360) if outer > 0 else (180, 270)
    for i in range(SIDES):
        a = 360.0 * i / SIDES
        if a > skip[0] and a < skip[1]:
            continue
        pts.append((y0 + R * math.cos(math.radians(a)), T + R * math.sin(math.radians(a))))
    return along_x(pts, x0, x1)

def hinge(y0, x0, x1, n, fixed, moving):
    """Knuckles for a hinge from x0 to x1 in n segments; appends the fixed
    part's pieces to the list fixed and the moving part's to moving."""
    L = (x1 - x0 - (n - 1) * CLEAR) / n
    rp = min(0.6 * R, L / 2 - 1.0)       # pin radius = pin length (45 degrees)
    k = CLEAR * math.sqrt(2.0)           # a 45-degree surface moved CLEAR away
    segs = [(x0 + i * (L + CLEAR), x0 + i * (L + CLEAR) + L) for i in range(n)]
    for i in range(n):
        a, b = segs[i]
        if i % 2 == 0:
            # fixed: knuckle, a bridge back to the fixed part, and a pin into
            # each moving neighbour
            parts = [knuckle(y0, a, b, -1), box([a, y0, 0], [b, y0 + A + 0.6, T])]
            if i > 0:
                parts.append(cone_x(a + 0.2, y0, T, rp + 0.2, rp + 0.2, -1))
            if i < n - 1:
                parts.append(cone_x(b - 0.2, y0, T, rp + 0.2, rp + 0.2, 1))
            fixed.append(union(parts))
        else:
            # moving: knuckle, a bridge to the moving part, and a socket for
            # the pin from each side, CLEAR away from it all round
            parts = [knuckle(y0, a, b, 1), box([a, y0 - A - 0.6, 0], [b, y0, T])]
            for face, dir in ((a, 1), (b, -1)):
                # the pin's face is CLEAR outside this knuckle's face
                pin_face = face - dir * CLEAR
                start = face - dir * 0.2
                tip = pin_face + dir * (rp + k)
                parts.append(cone_x(start, y0, T, (tip - start) * dir, (tip - start) * dir, dir, hole = True))
            moving.append(union(parts))

# --- where the prop's foot lands ---------------------------------------------
# Side view (y, z). The panel turns 180 - angle about hinge 1 (y = 0); the
# prop, hinged PIVOT up the panel, turns beta about hinge 2 first. Each
# turn lifts the moving part's far end, toward +y.

def turn(p, c, deg):
    w = math.radians(deg)
    u, v = p[0] - c[0], p[1] - c[1]
    return (c[0] + u * math.cos(w) + v * math.sin(w), c[1] - u * math.sin(w) + v * math.cos(w))

def foot(angle, beta):
    """The prop's two foot corners, standing, with the panel at angle and
    the prop turned beta: its top-face corner, then its bottom-face corner."""
    out = []
    for z in (T, 0.0):
        p = turn((-(PIVOT + PROP), z), (-PIVOT, T), beta)
        out.append(turn(p, (0.0, T), 180 - angle))
    return out

def notch(angle):
    """The notch for angle: (front, back) in y, and the prop's turn beta
    standing in it. The prop turns until its lowest corner meets the notch
    floor; the notch reaches from just in front of that corner to just
    behind where the foot's end crosses the base's top (or its other
    corner, if both are below it)."""
    floor = T - NOTCH
    lo, hi = 0.0, 179.0
    for i in range(1, 1790):
        f = foot(angle, i * 0.1)
        if min(f[0][1], f[1][1]) <= floor:
            lo, hi = i * 0.1 - 0.1, i * 0.1
            break
    for i in range(40):
        m = (lo + hi) / 2
        f = foot(angle, m)
        if min(f[0][1], f[1][1]) > floor:
            lo = m
        else:
            hi = m
    c0, c1 = foot(angle, lo)
    low, high = (c0, c1) if c0[1] < c1[1] else (c1, c0)
    if high[1] <= T:
        back = high[0]
    else:
        back = low[0] + (high[0] - low[0]) * (T - low[1]) / (high[1] - low[1])
    # in front, the notch reaches past wherever the prop's sides dip below
    # the base's top on their way up from the foot
    pivot = turn((-PIVOT, T), (0.0, T), 180 - angle)
    d = (pivot[0] - c0[0], pivot[1] - c0[1])
    l = math.sqrt(d[0] * d[0] + d[1] * d[1])
    front = min(low[0], back)
    for c in (c0, c1):
        for k in range(80):
            y, z = c[0] + d[0] / l * k * 0.25, c[1] + d[1] / l * k * 0.25
            if z < T:
                front = min(front, y)
    return (front - 0.8, max(low[0], back) + 0.3, lo)

notches = [notch(a) for a in ANGLES]
BACK = max([n[1] for n in notches]) + 8.0    # the base's back edge
for i in range(len(notches) - 1):
    if notches[i + 1][1] > notches[i][0] - 2.0:
        fail("the notches for %d and %d degrees overlap: space ANGLES further apart" % (ANGLES[i], ANGLES[i + 1]))

# --- the parts ---------------------------------------------------------------

base_bits, panel_bits, prop_bits = [], [], []

# hinge 1: the base (fixed) and the panel (moving), across the panel's width
hinge(0.0, -W / 2, W / 2, 5, base_bits, panel_bits)
# hinge 2: the panel (fixed) and the prop (moving), across the prop's width
hinge(-PIVOT, -PW / 2, PW / 2, 3, panel_bits, prop_bits)

# the base: a slab with a cut-out for the panel, the notches, and the labels
cuts = [box([-W / 2 - CLEAR, -P - CLEAR, -1], [W / 2 + CLEAR, A, T + 1], hole = True)]
for i in range(len(ANGLES)):
    y0, y1 = notches[i][0], notches[i][1]
    cuts.append(box([-PW / 2 - 1.5, y0, T - NOTCH], [PW / 2 + 1.5, y1, T + 1], hole = True))
    if LABELS:
        t = cad.add_shape("text", hole = True)
        cad.set_text(t, str(ANGLES[i]))
        w, h, _ = cad.node(t)["bounds"]["size"]
        f = min(4.0 / h, (HALF - PW / 2 - 6) / w)
        cx, cy = (PW / 2 + HALF) / 2 + 1, (y0 + y1) / 2
        cad.set_bounds(min = [cx - w * f / 2, cy - h * f / 2, T - LABEL_DEPTH], max = [cx + w * f / 2, cy + h * f / 2, T + 1], ids = [t])
        cuts.append(t)
slab = union([box([-HALF, -P - CLEAR - FRONT, 0], [HALF, BACK, T])] + cuts)

# the lips that stop the device sliding forward, on the side rails, far
# enough forward for the device at the shallowest angle
lip_y = -(T + DEVICE) / math.sin(math.radians(ANGLES[0])) - 1.0
lip = box([W / 2 + CLEAR + 1.0, lip_y - 3.0, T - 0.5], [HALF - 1.0, lip_y, T + LIP_H])
cad.set_param(lip, "radius", 1.0)
lips = [lip, copy_of(lip, -(W / 2 + CLEAR + HALF))]

base = union([slab] + lips + base_bits, "Base")

# the panel: a slab with a window for the prop
window = box([-PW / 2 - CLEAR, -PIVOT - PROP - CLEAR, -1], [PW / 2 + CLEAR, -PIVOT + A, T + 1], hole = True)
panel_slab = union([box([-W / 2, -P, 0], [W / 2, -A, T]), window])
panel = union([panel_slab] + panel_bits, "Panel")

# the prop
prop = union([box([-PW / 2, -PIVOT - PROP, 0], [PW / 2, -PIVOT - A, T])] + prop_bits, "Prop")

cad.set_color(COLORS[0], ids = [base])
cad.set_color(COLORS[1], ids = [panel, prop])
stand = cad.group("bundle", ids = [base, panel, prop])
cad.set_name("Folding stand (%s)" % SIZE, ids = [stand])

size = cad.node(stand)["bounds"]["size"]
print("%s stand: %s x %s x %s mm, notches for %s degrees" % (
    SIZE, str(math.round(size[0])), str(math.round(size[1])), str(math.round(size[2] * 10) / 10),
    ", ".join([str(a) for a in ANGLES])))
