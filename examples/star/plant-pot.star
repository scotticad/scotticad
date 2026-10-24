# plant-pot.star - a plant pot with drainage holes and a matching saucer,
# made with lofts (cad.add_loft).
#
# The pot's outline turns from a circle at the base into a polygon at the
# rim, twisting as it rises. Every outline has the same number of points,
# so the loft joins each point to the same point on the next outline, and
# the polygon's corners sweep up the side as ridges. Inside, a second loft
# offset by the wall thickness hollows it out. It sits in a round saucer,
# shown beside it, on bumps that lift it so the drain holes can drain.
#
# Both print upright with no supports: the pot's base is flat on the bed
# (the bumps are in the saucer, not under the pot), its walls lean out at
# most about 17 degrees, and the rim band's underside slopes at 45.
#
# Change the numbers below and run again. Sizes are in mm.

HEIGHT = 100.0       # pot, from its base to the rim
BASE_R = 35.0        # radius of the round base
TOP_R = 55.0         # radius to the polygon's corners at the rim
SIDES = 6            # corners of the polygon at the rim
TWIST = 40.0         # degrees the polygon turns from base to rim
WALL = 2.4           # side wall thickness
FLOOR = 3.0          # floor thickness
RIM = 1.6            # how far the rim band stands out
RIM_H = 6.0          # rim band height
DRAIN_D = 8.0        # centre drain hole
SMALL_D = 5.0        # ring of smaller drain holes
SMALL_N = 6

SAUCER_GAP = 6.0     # clearance round the pot's base
SAUCER_H = 15.0
SAUCER_WALL = 2.4
SAUCER_FLOOR = 2.4
BUMPS = 6            # bumps on the saucer's floor that the pot sits on
BUMP_H = 3.0         # how far they lift it
BUMP_D = 6.0

POINTS = 180         # points round each outline (a multiple of SIDES)
LEVELS = 30          # outlines from base to rim
POT_COLOR = "#c8643c"
SAUCER_COLOR = "#a8502e"

# --- the pot's outline ----------------------------------------------------

def ease(t):
    """How far the pot has flared out at height fraction t: quickly at
    first, then gently, for a soft belly."""
    return 1 - math.pow(1 - t, 1.5)

def polygon_radius(a):
    """Distance to a regular SIDES-gon's edge (corners at 1) at angle a."""
    seg = 2 * math.pi / SIDES
    return math.cos(math.pi / SIDES) / math.cos(a % seg - seg / 2)

def outline(t):
    """The pot's outline at height fraction t (0 base, 1 rim): a circle
    blending into the polygon, the whole outline turned by the twist. The
    points turn with it, so the polygon's corners stay on points."""
    m = t * t * (3 - 2 * t)                          # circle to polygon
    size = BASE_R + (TOP_R - BASE_R) * ease(t)
    turn = math.radians(TWIST) * t
    pts = []
    for i in range(POINTS):
        a = 2 * math.pi * i / POINTS
        r = size * ((1 - m) + m * polygon_radius(a))
        pts.append((r * math.cos(a + turn), r * math.sin(a + turn)))
    return pts

def offset(pts, d):
    """The closed outline pts moved d outward (negative: inward), square to
    its edges; at corners the two edges' offsets meet (a mitre)."""
    n = len(pts)
    out = []
    for i in range(n):
        p0, p1, p2 = pts[i - 1], pts[i], pts[(i + 1) % n]
        def normal(a, b):
            dx, dy = b[0] - a[0], b[1] - a[1]
            k = math.sqrt(dx * dx + dy * dy)
            return (dy / k, -dx / k)                 # outward, counter-clockwise outline
        n1, n2 = normal(p0, p1), normal(p1, p2)
        mx, my = n1[0] + n2[0], n1[1] + n2[1]
        k = math.sqrt(mx * mx + my * my)
        mx, my = mx / k, my / k
        s = d / (mx * n1[0] + my * n1[1])            # mitre length
        out.append((p1[0] + mx * s, p1[1] + my * s))
    return out

def at(t):
    return HEIGHT * t

def color(id, c):
    cad.set_color(c, ids = [id])
    return id

def copy_of(id, dx, dy, dz = 0.0):
    """A copy of shape id, moved by (dx, dy, dz)."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, dz = dz, ids = [new])
    return new

def ring_of(id, cx, cy, r, n, start = 0.0):
    """Shape id (centred on the ring's first spot) and n - 1 copies of it,
    spaced evenly round a circle of radius r about (cx, cy)."""
    out = [id]
    x0, y0 = cx + r * math.cos(start), cy + r * math.sin(start)
    for k in range(1, n):
        a = start + 2 * math.pi * k / n
        out.append(copy_of(id, cx + r * math.cos(a) - x0, cy + r * math.sin(a) - y0))
    return out

# --- pot ------------------------------------------------------------------

ts = [i / LEVELS for i in range(LEVELS + 1)]
body = cad.add_loft([(at(t), outline(t)) for t in ts])

# the inside: the same outlines moved in by the wall, from the floor up
# (and on past the rim, so it opens the top)
t_floor = FLOOR / HEIGHT
inside_ts = [t_floor] + [t for t in ts if t > t_floor]
inside = cad.add_loft([(at(t), offset(outline(t), -WALL)) for t in inside_ts] +
                      [(at(1) + 1, offset(outline(1), -WALL))])

# the rim band: the top of the pot grown outward, still twisting. Its
# underside slopes out at 45 degrees (from just inside the wall), so it
# prints without supports.
t_band = 1 - RIM_H / HEIGHT
t_slope = t_band - (RIM + 0.3) / HEIGHT
rim_ts = [t_band + (1 - t_band) * i / 4 for i in range(5)]
rim = cad.add_loft([(at(t_slope), offset(outline(t_slope), -0.3))] +
                   [(at(t), offset(outline(t), RIM)) for t in rim_ts])

# drain holes through the floor: one in the middle, and a ring of copies
# of one smaller hole round it
def drain(x, y, d):
    h = cad.add_shape("cylinder", hole = True)
    cad.set_bounds(min = [x - d / 2, y - d / 2, -1], max = [x + d / 2, y + d / 2, FLOOR + 1], ids = [h])
    return h

ring_r = 0.45 * BASE_R
a0 = 2 * math.pi / SMALL_N
drains = [drain(0, 0, DRAIN_D)] + ring_of(drain(ring_r * math.cos(a0), ring_r * math.sin(a0), SMALL_D),
                                          0, 0, ring_r, SMALL_N, a0)
cad.set_hole(True, ids = [inside])

pot = color(cad.group("union", ids = [body, rim, inside] + drains), POT_COLOR)
cad.set_name("plant pot", ids = [pot])

# --- saucer ---------------------------------------------------------------

# round, wide enough round the pot's base, flaring out a little, with
# bumps on its floor for the pot to stand on; set beside the pot
SX = TOP_R + RIM + BASE_R + SAUCER_GAP + 12

def circle(cx, r, n = 120):
    return [(cx + r * math.cos(2 * math.pi * i / n), r * math.sin(2 * math.pi * i / n)) for i in range(n)]

R0 = BASE_R + SAUCER_GAP + SAUCER_WALL     # outside, at the bottom
R1 = R0 + SAUCER_H * 0.3                    # outside, at the top
outside = cad.add_loft([(0, circle(SX, R0)), (SAUCER_H, circle(SX, R1))])
slope = (R1 - R0) / SAUCER_H
hollow = cad.add_loft([(SAUCER_FLOOR, circle(SX, R0 - SAUCER_WALL + slope * SAUCER_FLOOR)),
                       (SAUCER_H + 1, circle(SX, R1 - SAUCER_WALL + slope))], hole = True)
# the bumps, under the pot's base and between its drain holes: one, and
# copies of it round a ring
bump_r = 0.75 * BASE_R
b0 = math.pi / BUMPS
x, y = SX + bump_r * math.cos(b0), bump_r * math.sin(b0)
first = cad.add_shape("cylinder")
cad.set_bounds(min = [x - BUMP_D / 2, y - BUMP_D / 2, SAUCER_FLOOR - 0.5], max = [x + BUMP_D / 2, y + BUMP_D / 2, SAUCER_FLOOR + BUMP_H], ids = [first])
bumps = ring_of(first, SX, 0, bump_r, BUMPS, b0)
# (hollowed first: a hole cuts everything in its own group, bumps too)
dish = cad.group("union", ids = [outside, hollow])
saucer = color(cad.group("union", ids = [dish] + bumps), SAUCER_COLOR)
cad.set_name("saucer", ids = [saucer])

# the pair centred on the build plate
cad.select([pot, saucer])
cad.set_position(x = 0, y = 0, anchor = "center")
cad.drop()
cad.fit_view()
print("pot", cad.node(pot)["bounds"]["size"], "saucer", cad.node(saucer)["bounds"]["size"])
