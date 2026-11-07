# benchy-layered-slow.star - the 3DBenchy calibration boat rebuilt from
# ScottiCAD primitives, as close to the official model as boxes, cylinders
# and cones allow.
#
# A COUNTEREXAMPLE, KEPT TO SHOW WHAT NOT TO DO. It builds the
# hull a slow, inefficient way: 55 stacked layers, each assembled from
# boxes, slabs and cylinders, about 1,060 shapes and groups in all. It
# takes about a minute to run, and the layers leave visible stair-steps on
# the hull. Use benchy-loft.star instead: the same boat with the hull made
# as a few lofts (cad.add_loft) - about 130 shapes, built in a fraction of
# the time, smooth, and closer to the official model.
#
# 3DBenchy is by Creative Tools AB and has been in the public domain (CC0)
# since 2025. The numbers below were measured from the official STL
# (github.com/johan--/3DBenchy, Single-part/3DBenchy.stl) by slicing it;
# the ones the official spec sheet (3dbenchy.com/dimensions) names are
# exact:
#   overall 60.00 x 31.00 x 48.00, cargo box top 15.50 above the bottom
#   cargo box 12.00 x 10.81 outside, 8.00 x 7.00 inside, 9.00 deep
#   chimney 7.00 outside at the top, 3.00 hole, 11.00 deep
#   bridge roof 23.00 long, inclined 5.5 degrees
#   front window 10.50 x 9.50, rear window 9.00 (12.00 flange, 0.30 proud)
#   hawsepipes 4.00 (flange 0.30 proud)
#
# Axes: X runs stern (-) to bow (+), Y is port/starboard, Z is up.
#
# Building blocks:
#   - the hull is 55 half-millimetre layers; each is a trapezoid aft (a
#     box intersected with two slabs turned a few degrees about Z) joined
#     to a pointed arch forward (the overlap of two large cylinders, an
#     ogive)
#   - sloping faces (the sheer line, the decks, the cabin walls, the roof)
#     are slabs: boxes turned to lie along a plane, used as holes or
#     intersected
# Not modelled: the 0.10 mm stern nameplate and its "#3DBenchy" lettering,
# the "CT3D.xyz" keel lettering, the front window's frame, fillets and
# rounded edges. Against the official STL (voxels of 0.25 mm) the result
# overlaps 93.5% by volume, with a mean surface deviation of about 0.12 mm;
# most of the rest is the 0.5 mm stair-stepping of the hull layers.

COLOR = "#e8562a"
SIDES = 64      # facets on cylinders

# --- helpers ------------------------------------------------------------

def box(lo, hi, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def cyl(cx, cy, d, z0, z1, hole = False):
    """A vertical cylinder of diameter d."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d / 2, cy - d / 2, z0], max = [cx + d / 2, cy + d / 2, z1], ids = [id])
    return id

def flared(cx, cy, d0, d1, z0, z1, hole = False):
    """A cone frustum that widens upward from d0 at z0 to d1 at z1 (a cone
    only narrows upward, so build it upside down and mirror it)."""
    id = cad.add_shape("cone", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d1 / 2, cy - d1 / 2, z0], max = [cx + d1 / 2, cy + d1 / 2, z1], ids = [id])
    cad.set_param(id, "top", d0 / 2)
    cad.mirror("z", ids = [id])
    return id

def tilted_cyl(cx, cz, d, length, lean, hole = False):
    """A cylinder on the centreline (y = 0) centred at (cx, cz), its axis
    leaning lean degrees from vertical toward -X."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d / 2, -d / 2, cz - length / 2], max = [cx + d / 2, d / 2, cz + length / 2], ids = [id])
    cad.rotate("y", -lean, ids = [id])
    return id

def level_cyl(c, d, length, heading, hole = False):
    """A horizontal cylinder centred at c whose axis points at heading
    degrees (0 = +X, 90 = +Y)."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.rotate("x", 90, ids = [id])      # axis along Y
    cad.set_bounds(min = [c[0] - d / 2, c[1] - length / 2, c[2] - d / 2], max = [c[0] + d / 2, c[1] + length / 2, c[2] + d / 2], ids = [id])
    cad.rotate("z", heading - 90, ids = [id])
    return id

SLAB = 80.0     # size of the boxes used as half-spaces

def half(p, q, plane, lo, hi, hole = False):
    """A slab filling the left of the line p->q: in the XY plane (spanning
    z lo..hi) or the XZ plane (spanning y lo..hi). Left of p->q is the
    side its direction turns toward going counter-clockwise."""
    dx, dv = q[0] - p[0], q[1] - p[1]
    n = math.sqrt(dx * dx + dv * dv)
    nx, nv = -dv / n, dx / n                       # left normal
    cx = (p[0] + q[0]) / 2 + nx * SLAB / 4
    cv = (p[1] + q[1]) / 2 + nv * SLAB / 4
    deg = math.degrees(math.atan2(dv, dx))
    L, T = SLAB * 2, SLAB / 2
    if plane == "xy":
        id = box([cx - L / 2, cv - T / 2, lo], [cx + L / 2, cv + T / 2, hi], hole)
        cad.rotate("z", deg, ids = [id])
    else:
        id = box([cx - L / 2, lo, cv - T / 2], [cx + L / 2, hi, cv + T / 2], hole)
        cad.rotate("y", -deg, ids = [id])
    return id

def half3(p, n, hole = False):
    """A slab filling the side of the plane through point p that its
    normal n points to (any direction): a box turned about Y, then Z."""
    k = math.sqrt(n[0] * n[0] + n[1] * n[1] + n[2] * n[2])
    nx, ny, nz = n[0] / k, n[1] / k, n[2] / k
    T, L = SLAB / 2, SLAB * 2
    c = [p[0] + nx * T / 2, p[1] + ny * T / 2, p[2] + nz * T / 2]
    id = box([c[0] - T / 2, c[1] - L / 2, c[2] - L / 2], [c[0] + T / 2, c[1] + L / 2, c[2] + L / 2], hole)
    cad.rotate("y", -math.degrees(math.asin(nz)), ids = [id])
    cad.rotate("z", math.degrees(math.atan2(ny, nx)), ids = [id])
    return id

def union(ids):
    return cad.group("union", ids = ids)

def intersect(ids):
    return cad.group("intersect", ids = ids)

def hole(id):
    cad.set_hole(True, ids = [id])
    return id

def mirrored(id):
    """A copy of shape id mirrored across the boat's centreline (y = 0):
    the starboard twin of a port-side part."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.mirror("y", ids = [new])                  # flips it about its own middle...
    cy = cad.node(new)["bounds"]["center"][1]
    cad.move(dy = -2 * cy, ids = [new])           # ...then across to the other side
    return new

def ogive(x0, xt, w, z0, z1):
    """A pointed arch: half-width w at x0 (tangent there), closing to a
    point at xt; the overlap of two cylinders of radius R."""
    R = ((xt - x0) * (xt - x0) + w * w) / (2 * w)
    return intersect([
        cyl(x0, w - R, 2 * R, z0, z1),
        cyl(x0, R - w, 2 * R, z0, z1),
        box([x0, -w - 1, z0 - 1], [xt + 1, w + 1, z1 + 1]),
    ])

def trapezoid(x0, h0, x1, h1, z0, z1):
    """Half-width h0 at x0 changing in a straight line to h1 at x1."""
    m = max(h0, h1) + 1
    return intersect([
        box([x0, -m, z0], [x1, m, z1]),
        half([x1, h1], [x0, h0], "xy", z0 - 1, z1 + 1),
        half([x0, -h0], [x1, -h1], "xy", z0 - 1, z1 + 1),
    ])

def hull_layer(z0, z1, xs, hs, x0, wa, w, xt):
    """A hull layer: straight taper from the transom (xs, half-width hs)
    to x0 (half-width wa), then a pointed arch to the bow tip xt."""
    return union([
        trapezoid(xs, hs, x0 + 0.05, wa, z0, z1),
        ogive(x0, xt, w, z0, z1),
    ])

def cut_above(a, b):
    """A hole removing everything above the line a->b (x, z) between
    their x values."""
    return hole(intersect([
        box([a[0], -20, -5], [b[0], 20, 60]),
        half(a, b, "xz", -20, 20),
    ]))

def cut_below(a, b):
    """A hole removing everything below the line a->b (x, z) between
    their x values."""
    return hole(intersect([
        box([a[0], -20, -5], [b[0], 20, 60]),
        half(b, a, "xz", -20, 20),
    ]))

# --- hull ---------------------------------------------------------------

# One row per half-millimetre layer, measured at its mid-height:
# z0, z1, transom x, transom half-width, x0, aft half-width at x0,
# half-width at x0 (bow arch), bow tip x.
# From z = 12 up the sheer line cuts the aft end, so those layers start at
# x = -8 and the sheer cuts below shape them.
HULL = [
    (0.0, 0.5, -25.12, 6.63, 1.50, 8.21, 8.21, 14.13),
    (0.5, 1.0, -25.36, 6.89, 1.50, 8.57, 8.57, 14.75),
    (1.0, 1.5, -25.59, 7.14, 1.50, 8.93, 8.93, 15.36),
    (1.5, 2.0, -25.83, 7.40, 1.50, 9.29, 9.29, 15.98),
    (2.0, 2.5, -26.06, 7.65, 1.50, 9.64, 9.64, 16.58),
    (2.5, 3.0, -26.29, 7.90, 1.50, 10.00, 10.00, 17.19),
    (3.0, 3.5, -26.52, 8.15, 1.50, 10.34, 10.34, 17.79),
    (3.5, 4.0, -26.74, 8.39, 1.50, 10.69, 10.69, 18.38),
    (4.0, 4.5, -26.96, 8.63, 1.50, 11.02, 11.02, 18.96),
    (4.5, 5.0, -27.18, 8.87, 1.50, 11.35, 11.35, 19.54),
    (5.0, 5.5, -27.50, 9.10, 1.50, 11.67, 11.67, 20.10),
    (5.5, 6.0, -27.70, 9.32, 1.50, 11.99, 11.99, 20.66),
    (6.0, 6.5, -27.90, 9.54, 1.50, 12.29, 12.29, 21.20),
    (6.5, 7.0, -28.09, 9.75, 1.50, 12.58, 12.58, 21.72),
    (7.0, 7.5, -28.27, 9.96, 1.50, 12.86, 12.86, 22.24),
    (7.5, 8.0, -28.44, 10.16, 1.75, 13.13, 13.13, 22.74),
    (8.0, 8.5, -28.50, 10.36, 1.75, 13.38, 13.38, 23.22),
    (8.5, 9.0, -28.60, 10.50, 1.75, 13.52, 13.62, 23.69),
    (9.0, 9.5, -29.16, 10.60, 2.00, 13.75, 13.85, 24.13),
    (9.5, 10.0, -29.17, 10.65, 2.00, 13.96, 14.06, 24.56),
    (10.0, 10.5, -29.17, 10.65, 2.00, 14.15, 14.25, 24.97),
    (10.5, 11.0, -29.11, 10.50, 2.25, 14.33, 14.43, 25.36),
    (11.0, 11.5, -29.17, 11.03, 2.00, 14.70, 14.70, 25.73),
    (11.5, 12.0, -29.17, 10.55, 1.50, 15.07, 15.07, 26.09),
    (12.0, 12.5, -8.00, 14.30, 1.75, 15.12, 15.12, 26.42),
    (12.5, 13.0, -8.00, 14.30, 2.00, 15.25, 15.25, 26.74),
    (13.0, 13.5, -8.00, 14.30, 2.25, 15.30, 15.30, 27.04),
    (13.5, 14.0, -8.00, 14.30, 2.75, 15.30, 15.30, 27.33),
    (14.0, 14.5, -8.00, 15.30, 3.00, 15.30, 15.30, 27.60),
    (14.5, 15.0, -8.00, 15.35, 3.25, 15.35, 15.35, 27.85),
    (15.0, 15.5, -8.00, 15.35, 3.50, 15.35, 15.35, 28.09),
    (15.5, 16.0, -8.00, 15.30, 4.00, 15.30, 15.30, 28.31),
    (16.0, 16.5, -8.00, 15.35, 4.00, 15.35, 15.35, 28.52),
    (16.5, 17.0, -8.00, 15.45, 4.00, 15.45, 15.45, 28.72),
    (17.0, 17.5, -8.00, 15.44, 4.25, 15.44, 15.44, 28.91),
    (17.5, 18.0, -8.00, 15.44, 4.50, 15.44, 15.44, 29.08),
    (18.0, 18.5, -8.00, 15.28, 5.25, 15.28, 15.28, 29.24),
    (18.5, 19.0, -8.00, 15.31, 6.25, 15.30, 15.30, 29.38),
    (19.0, 19.5, -8.00, 15.49, 6.50, 15.30, 15.30, 29.52),
    (19.5, 20.0, -8.00, 15.47, 6.50, 15.30, 15.30, 29.64),
    (20.0, 20.5, -8.00, 15.50, 6.75, 15.30, 15.30, 29.76),
    (20.5, 21.0, -8.00, 15.51, 6.75, 15.30, 15.30, 29.86),
    (21.0, 21.5, -8.00, 15.50, 7.00, 15.30, 15.30, 29.95),
    (21.5, 22.0, -8.00, 15.48, 7.00, 15.30, 15.30, 30.04),
    (22.0, 22.5, -8.00, 15.50, 7.25, 15.30, 15.30, 30.12),
    (22.5, 23.0, -8.00, 14.88, 7.00, 14.88, 14.88, 30.18),
    (23.0, 23.5, -8.00, 14.74, 7.25, 14.74, 14.74, 30.25),
    (23.5, 24.0, -8.00, 14.84, 7.00, 14.84, 14.84, 30.30),
    (24.0, 24.5, -8.00, 13.95, 9.00, 13.95, 13.95, 30.34),
    (24.5, 25.0, -8.00, 12.40, 12.25, 12.40, 12.40, 30.38),
    (25.0, 25.5, -8.00, 10.00, 17.00, 10.00, 10.00, 30.42),
    (25.5, 26.0, -8.00, 10.00, 16.25, 10.00, 10.00, 30.83),
    (26.0, 26.5, -8.00, 10.00, 16.00, 10.00, 10.00, 30.83),
    (26.5, 27.6, -8.00, 10.05, 16.50, 10.05, 10.05, 30.58),
]

DECK = 8.5          # the aft deck and cabin floor
BOW_DECK = 15.0     # the bow deck, before it ramps up
OVERLAP = 0.02      # layers overlap a little so they fuse cleanly
SIDE_WALL = 2.1     # hull side walls above the deck
TRANSOM_WALL = 2.0

BEAM = 31.0         # overall width, at the rubbing strake

layers = [hull_layer(max(z0 - OVERLAP, 0), z1 + OVERLAP, xs, hs, x0, wa, w, xt) for z0, z1, xs, hs, x0, wa, w, xt in HULL]

# the sheer line (top of the hull sides), in straight runs from stern to bow
SHEER = [[-40, 10.72], [-18, 10.77], [-8, 11.75], [0, 13.26], [4.6, 14.6], [9.5, 21.1], [32, 28.1]]
RUNS = [(SHEER[i], SHEER[i + 1]) for i in range(len(SHEER) - 1)]
sheer = [cut_above(a, b) for a, b in RUNS]

# the rubbing strake: a band 1.6 mm tall under the sheer, 0.3 mm proud of
# the hull (it sets the 31.00 overall width). It is the hull grown by 0.3,
# kept only between the sheer and 1.6 mm below it.
STRAKE_H, STRAKE_OUT = 1.6, 0.3
strake_layers = []
for z0, z1, xs, hs, x0, wa, w, xt in HULL:
    if z0 >= 9.0:
        sw = min(w + STRAKE_OUT, BEAM / 2)
        strake_layers.append(hull_layer(z0 - OVERLAP, z1 + OVERLAP, xs, hs + STRAKE_OUT, x0, min(wa + STRAKE_OUT, sw), sw, xt))
strake = union(strake_layers + [cut_above(a, b) for a, b in RUNS] +
               [cut_below([a[0], a[1] - STRAKE_H], [b[0], b[1] - STRAKE_H]) for a, b in RUNS])

# the aft well: the hull layers above the deck, shrunk by the wall
# thickness so the walls follow the hull's flare. Its front climbs from the
# deck at x = 6.4 to the bow deck at x = 10.3 (beside the cabin; the
# cabin's front fills the middle).
well_layers = []
for z0, z1, xs, hs, x0, wa, w, xt in HULL:
    if z0 < DECK or z0 >= 15:
        continue
    if z0 >= 12:
        xs, hs = -29.17, 10.98       # the aft end continues straight up
    top = 45 if z0 == 14.5 else z1 + OVERLAP
    well_layers.append(hull_layer(max(z0 - OVERLAP, DECK), top, xs + TRANSOM_WALL, hs - SIDE_WALL,
                                  x0, wa - SIDE_WALL, w - SIDE_WALL, xt - SIDE_WALL))
aft_well = hole(intersect([
    union(well_layers),
    half([6.4, DECK], [10.3, BOW_DECK], "xz", -20, 20),
]))

# the bow well: bulwarks about 2.3 mm, floor flat at 15 then ramping up to
# the bow at 0.3125 (to 20 at x = 27)
bow_well = hole(intersect([
    ogive(6.0, 28.0, 12.8, BOW_DECK, 45),
    box([6.0, -14, BOW_DECK], [30, 14, 45]),
    half([11, BOW_DECK - 0.45], [31, BOW_DECK - 0.45 + 20 * 0.3125], "xz", -20, 20),
]))

# hawsepipes: 4.00 mm, through the bulwark at 45 degrees, each with a
# flange 0.30 proud of the hull
flange = level_cyl([26.05, 6.55, 20.6], 5.6, 0.6, 45)                  # port...
pipe = level_cyl([25.1, 5.6, 20.6], 4.0, 8, 45, hole = True)
flanges = [flange, mirrored(flange)]                                      # ...and starboard
hawse = [pipe, mirrored(pipe)]

hull = union(layers + [strake] + sheer + [aft_well, bow_well])
hull = union([hull] + flanges + hawse)    # the holes go through the flanges too

# --- stern: fishing rod holder ------------------------------------------

# a 2.0 mm bore in a 3.5 mm boss against the transom, leaning 16.2 degrees
# aft; its axis passes (-25.21, 0, 6.12)
ROD_LEAN = 16.2

def rod_x(z):
    return -25.21 - (z - 6.12) * math.tan(math.radians(ROD_LEAN))

rod_boss = tilted_cyl(rod_x(10.4), 10.4, 3.5, 4.8, ROD_LEAN)

# --- cargo box ----------------------------------------------------------

CARGO_X0 = -17.8    # 10.81 long, 12.00 wide
cargo = box([CARGO_X0, -6.0, DECK - 0.5], [CARGO_X0 + 10.81, 6.0, 15.5])

# --- cabin (bridge) -----------------------------------------------------

CAB_TOP = 38.2      # the walls run up into the roof
CEILING = 36.6
CAB_WALL = 2.0
SPLIT = 1.85        # mid-door: the rear block is narrower than the front
WINDOW_Z = 24.6     # bottom of the front window

# The cabin's faces, as planes: a point on each and its inward normal.
# Sides of the rear block: half-width 7.30 at x = -7, z = 20, widening
# 0.13 per mm forward and 0.027 per mm upward; the front block's sides are
# 8.38 at z = 20, widening upward the same. The rear face leans aft
# (x = -7.2 at z = 16, 0.04 per mm); the front leans forward (x = 11.9 at
# z = 20, 0.115 per mm).
def rear_side(s, inset):
    return half3([-7, s * (7.30 - inset), 20], [0.13, -s, 0.027])

def front_side(s, inset):
    return half3([4, s * (8.38 - inset), 20], [0, -s, 0.027])

def rear_face(inset):
    return half3([-7.2 + inset, 0, 16], [1, 0, 0.04])

def front_face(inset):
    return half3([11.9 - inset, 0, 20], [-1, 0, 0.115])

cabin_shell = union([
    intersect([box([-12, -12, DECK - 0.5], [SPLIT, 12, CAB_TOP]), rear_side(1, 0), rear_side(-1, 0), rear_face(0)]),
    intersect([box([SPLIT - 0.05, -12, DECK - 0.5], [16, 12, CAB_TOP]), front_side(1, 0), front_side(-1, 0), front_face(0)]),
    # inside, open from the floor to the ceiling
    hole(intersect([box([-12, -12, DECK], [SPLIT, 12, CEILING]), rear_side(1, CAB_WALL), rear_side(-1, CAB_WALL), rear_face(CAB_WALL)])),
    hole(intersect([box([SPLIT - 0.05, -12, DECK], [16, 12, CEILING]), front_side(1, CAB_WALL), front_side(-1, CAB_WALL), front_face(2.2)])),
    # front window 10.50 x 9.50
    box([8.0, -5.25, WINDOW_Z], [16, 5.25, WINDOW_Z + 9.5], hole = True),
    # rear window 9.00
    level_cyl([-6.5, 0, 27.3], 9.0, 6, 0, hole = True),
    # arched side doors, through both sides
    box([-2.3, -12, DECK], [6.0, 12, 27.85], hole = True),
    level_cyl([1.85, 0, 27.85], 8.3, 24, 90, hole = True),
])

# the front below the window: a block up to z = 20.5 whose back leans
# (x = 6.5 at the floor, 7.05 at z = 20); on it a half-round console
# (radius 5) with the ship's wheel on its back - a ring 10.0 across with a
# 7.0 hole, cut flat at z = 22.6, and a 2.5 hub, standing 0.5 proud. The
# gap between the console and the front wall drops to z = 20.5.
WHEEL_Z = 19.6
dash = union([
    intersect([box([6.0, -7, DECK], [14, 7, 20.5]), front_face(0), half3([6.5, 0, DECK], [1, 0, -0.0478])]),
    level_cyl([7.7, 0, WHEEL_Z], 10.0, 1.3, 0),                          # console
    union([
        level_cyl([6.8, 0, WHEEL_Z], 10.0, 0.6, 0),                      # wheel rim
        level_cyl([6.8, 0, WHEEL_Z], 7.0, 1.0, 0, hole = True),
        box([6.0, -6, 22.6], [7.5, 6, 26], hole = True),
    ]),
    level_cyl([6.8, 0, WHEEL_Z], 2.5, 0.6, 0),                           # hub
])

# rear window flange: 12.00 across, 0.30 proud of the rear wall
FLANGE_X = -7.2 - 0.04 * (27.3 - 16) - 0.25        # its outer face
rear_flange = union([
    level_cyl([FLANGE_X + 0.25, 0, 27.3], 12.0, 0.5, 0),
    level_cyl([FLANGE_X + 0.25, 0, 27.3], 9.0, 1.0, 0, hole = True),
])

cabin = union([cabin_shell, dash, rear_flange])

# --- roof ---------------------------------------------------------------

# one slab, 2.4 mm thick: its top and underside both slope at 5.5 degrees,
# it widens toward the front as the cabin does, and the cabin's walls run
# up into it
ROOF_X0, ROOF_X1 = -9.0, 14.0        # 23.00 long
ROOF_SLOPE = math.tan(math.radians(5.5))
ROOF_THICK = 2.4

def roof_top(x):
    return 38.1 + (x - ROOF_X0) * ROOF_SLOPE

A, B = ROOF_X0 - 5, ROOF_X1 + 5
roof = union([
    trapezoid(ROOF_X0, 8.7, ROOF_X1, 9.9, 30, 45),
    hole(half([A, roof_top(A)], [B, roof_top(B)], "xz", -20, 20)),                                # above the top
    hole(half([B, roof_top(B) - ROOF_THICK], [A, roof_top(A) - ROOF_THICK], "xz", -20, 20)),      # below the underside
])

# --- chimney ------------------------------------------------------------

CX = -4.0
chimney = union([
    cyl(CX, 0, 6.2, CAB_TOP - 0.4, 39.0),            # base, merging into the roof
    flared(CX, 0, 5.65, 6.0, 38.9, 44.0),            # tapered stack
    flared(CX, 0, 6.0, 7.0, 44.0, 46.0),             # flare to the top
    cyl(CX, 0, 7.0, 46.0, 48.0),                     # 7.00 top
])

# --- assemble -----------------------------------------------------------

boat = union([
    hull, rod_boss, cargo, cabin, roof, chimney,
    # holes through everything
    box([CARGO_X0 + 1.9, -4.0, 15.5 - 9.0], [CARGO_X0 + 8.9, 4.0, 16], hole = True),   # cargo box, 8.00 x 7.00, 9.00 deep
    cyl(CX, 0, 3.0, 48.0 - 11.0, 48.5, hole = True),                         # chimney, 11.00 deep
    tilted_cyl(rod_x(9.5), 9.5, 2.0, 10, ROD_LEAN, hole = True),             # rod holder
    level_cyl([-23.25, 0, 1.5], 2.0, 6.5, 0, hole = True),                   # hole in the keel
])
cad.set_color(COLOR, ids = [boat])
cad.set_name("3DBenchy", ids = [boat])
cad.drop(ids = [boat])
cad.fit_view()
print("3DBenchy built: group", boat, "size", cad.selection()["bounds"]["size"])
