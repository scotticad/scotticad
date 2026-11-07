# crank-reducer.star - a 40:1 hand-crank gearbox that prints in one piece,
# already assembled: no supports, no screws, no bearings.
#
# HOW IT WORKS. Turn the crank (or knob: HANDLE) on top and the dial ring
# round the outside creeps round once for every 40 turns: one tick on the
# dial per turn. It is a "split ring" planetary gearbox, two planetary
# stages sharing their planets:
#   - the CRANK turns a SUN gear at the bottom of the middle;
#   - three long PLANET gears mesh with the sun, and with the FIXED RING in
#     the base round the bottom half, so they roll round inside it;
#   - the planets' top halves mesh with the DIAL RING, which has a few more
#     teeth (OUT_RING) than the fixed ring (RING). As the planets roll round,
#     the dial ring falls behind them by those few teeth each time round:
#     that difference is what makes the big reduction.
# The ratio is (1 + RING / SUN) / (1 - RING / OUT_RING): 4 / 0.1 = 40 here.
#
# THE CAGE holds the planets and the crank in place. It is an open lid over
# the gears on three pins and three posts: each pin rises from the bed
# through a planet, as its axle, and each post rises between two planets.
# The lid is a hub round the crank's shaft with an arm out to each pin and
# post, so the planets can be seen turning between them. The crank is held
# at the top by the hub and at the bottom by the three planets round the
# sun. The cage turns with the planets, at a quarter of the crank's speed;
# nothing drives it or is driven by it.
#
# THE GEARS. Both stages use ScottiCAD's Helical Gear and Ring Gear shapes
# with herringbone teeth. The top stage's teeth are a little smaller (M2,
# worked out below) so its ring, with more teeth, meshes the same planets at
# the same distance. Each planet is turned so its bottom teeth fit the sun
# and the fixed ring and its top teeth fit the dial ring; with these tooth
# counts no two planets are alike. Every gear here has an even number of
# teeth, so its axis is the middle of its shape and it can be placed and
# turned by its bounds.
#
# PRINTING WITHOUT SUPPORTS. Nothing is printed over a gap on a flat face:
#   - the dial ring sits on the fixed ring on a 45-degree cone, not a flat
#     ledge, with the gap (CLEAR) square to the cone, and a diamond ridge
#     round its outside runs in a groove in the base's wall, so it can't
#     lift or rock;
#   - the dial is the one part that doesn't start on the bed. Its underside
#     zigzags round the ring at 45 degrees (NOTCH), so it starts as a row
#     of small points printed onto the bottom of the groove, G above it,
#     and grows out from them. They may stick lightly; work the crank, or
#     give the dial a twist against the base, to break them free;
#   - the top halves of the dial ring's and the planets' teeth start with a
#     45-degree chamfer;
#   - the lid's underside rises at 45 degrees from the tops of the pins and
#     posts, everywhere: over each planet it is a cone, and each planet's
#     top is countersunk to match; round the outside it rises over a
#     45-degree chamfer on the dial ring's inner edge;
#   - the crank's knob flares out from its shaft at 45 degrees.
# Print it as it comes: 0.2 mm layers, 3 walls, 15% infill, no brim, and
# turn on elephant's-foot compensation. When it's done, hold the base and
# work the crank back and forth until it turns freely. If it's stuck, raise
# CLEAR by 0.1 mm; if it's sloppy, lower it. Each part has its own colour
# (COLORS) for printers that change filament by part. The cage's pins and
# posts stand on the table, like the base: they turn slowly, under nothing
# but the cage's own weight.
#
# About 95 mm across the base and 47 mm tall (43 with HANDLE = "knob").
# Sizes are in mm.

MODULE = 1.5          # the bottom stage's tooth size
SUN = 12              # teeth: the sun,
PLANET = 12           # each planet,
OUT_RING = 40         # and the dial ring (the fixed ring has SUN + 2 x PLANET)
PLANETS = 3
STAGE_LOW = 10.0      # the bottom stage's height
STAGE_HIGH = 10.0     # the top stage's height
HELIX = 30            # the herringbone's angle, in degrees
CLEAR = 0.4           # gap between moving parts (0.3-0.5 for most printers)
TIP_GAP = 0.3         # gap between each tooth's tip and the root it turns in
PIN_R = 3.0           # the cage's pins, the planets' axles
LID = 1.6             # the lid's least thickness
RIM = 6.0             # the fixed ring's wall outside its tooth roots
RIDGE = 2.0           # how far the dial's ridge reaches into the base's groove
WALL = 2.0            # the base's wall outside the groove
NOTCH = 2.0           # the zigzag under the dial: how high its peaks rise above its points
ARM = 5.0             # the width of the lid's arms
FLANGE = 8.0          # the base's flange outside its wall,
FLANGE_H = 4.0        # and its thickness
MOUNT_HOLES = True    # three screw holes in the flange
COLORS = {"Base": "#2f3640", "Dial": "#e1b12c", "Crank": "#c23616", "Planet": "#c8ccd0",
          "Cage": "#487eb0"}

HANDLE = "crank"      # "crank": a knob with a peg to wind it by; "knob": a taller
                      # fluted knob turned with the fingers, at the cost of slower
                      # winding
KNOB_R = 13.0         # the knob
KNOB_H = 4.0          # its height with the crank's peg, or
KNOB_TALL = 10.0      # without it
PEG_R = 4.0           # the crank's peg
PEG_H = 10.0
SIDES = 64

# --- the gears' sizes ---------------------------------------------------------

RING = SUN + 2 * PLANET
for z in (SUN, PLANET, RING, OUT_RING):
    if z % 2:
        fail("use even tooth counts: %d is odd" % z)
if (SUN + RING) % PLANETS:
    fail("the planets can't be evenly spaced: SUN + RING must divide by PLANETS")
if OUT_RING == RING:
    fail("OUT_RING must differ from RING, or nothing turns")
if HANDLE not in ("crank", "knob"):
    fail('HANDLE must be "crank" or "knob"')

C = CLEAR / 2                                 # each tooth face is cut back by half the gap
M2 = MODULE * (RING - PLANET) / (OUT_RING - PLANET)   # the top stage's module
ORBIT = MODULE * (SUN + PLANET) / 2           # the planets' axes from the middle
RATIO = (1 + RING / SUN) / (1 - RING / OUT_RING)
HT = STAGE_LOW                                # where the stages meet
H = STAGE_LOW + STAGE_HIGH                    # the top of the top stage's full teeth
LAP = 0.5                                     # the top stage reaches this far down into the bottom one
G = CLEAR * math.sqrt(2)                      # a gap of CLEAR square to a 45-degree cone, measured upright

def pitch(m, z):
    return m * z / 2

ROOT_A = pitch(MODULE, RING) + MODULE + TIP_GAP     # the fixed ring's tooth roots
ROOT_B = pitch(M2, OUT_RING) + M2 + TIP_GAP         # the dial ring's
TIP_A = pitch(MODULE, RING) - MODULE                # the fixed ring's tooth tips
TIP_B = pitch(M2, OUT_RING) - M2                    # the dial ring's
R = ROOT_A + RIM                                    # both rings' outside
SUN_TIP = pitch(MODULE, SUN) + MODULE
P_TIP_A = pitch(MODULE, PLANET) + MODULE            # a planet's tips, from its axis: bottom stage,
P_TIP_B = pitch(M2, PLANET) + M2                    # and top
PLANET_TIP = ORBIT + P_TIP_A                        # the planets' reach, bottom stage
ROOT_P2 = pitch(M2, PLANET) - M2 - TIP_GAP          # the planets' top-stage roots
SUN_ROOT = pitch(MODULE, SUN) - MODULE - TIP_GAP
SHAFT_R = min(SUN_ROOT - 0.4, ORBIT - P_TIP_B - CLEAR)
BORE = PIN_R + CLEAR                                # the hole through each planet
if BORE + 1.5 > ROOT_P2:
    fail("the planets' walls round their pins would be under 1.5 mm: lower PIN_R")

# The cone between the rings: the dial ring's underside is z = HT + G + K - r,
# which also chamfers its teeth. K is as far out as the planets' bottom teeth
# reach (or the dial ring's tooth roots, if those are further), so the dial
# ring is at least G above the planets wherever they turn. The fixed ring's
# top is G below the dial ring's underside.
K = max(ROOT_B, PLANET_TIP)
APEX_A = HT + K
APEX_B = APEX_A + G
BASE_TOP = APEX_A - R                         # the fixed ring's top at its outside
if BASE_TOP < 1.2:
    fail("the base would be under 1.2 mm thick under the dial: raise STAGE_LOW or lower RIM")

# The dial's ridge: a diamond round its outside, R to R + RIDGE, from its
# lowest edge at Z_LOW (where its underside cone reaches R) up to Z_LOW +
# 2 RIDGE. The base's wall round it has a groove G clear of it on both
# faces, so the dial can't lift or rock, and a lip over it up to LIP_TOP.
Z_LOW = APEX_B - R
LIP_BOT = Z_LOW + 2 * RIDGE + G - CLEAR       # the lip's underside at its inner edge, R + CLEAR
LIP_TOP = LIP_BOT + 1.5
R_WALL = R + RIDGE + G + WALL                 # the wall's outside

# --- the cage's sizes ---------------------------------------------------------
#
# The lid grows out from the tops of the pins and posts (at Z_PIN) at 45
# degrees: it is everywhere within z - Z_PIN of one of them. Over a planet
# that is a cone round its pin, z = Z_PIN + p - PIN_R at p from the planet's
# axis, and the planet's top is countersunk G below it. Straight out from a
# pin the same surface is z = Z_PIN + r - ORBIT - PIN_R at r from the
# middle; the dial ring is chamfered G below that, round its whole inner
# edge, and Z_PIN is set so the chamfer reaches its tooth tips at H.

Z_PIN = H + G - (TIP_B - ORBIT - PIN_R)
DIAL_CUT = Z_PIN - ORBIT - PIN_R - G          # the dial ring keeps to z <= DIAL_CUT + r
LID_R = ORBIT + P_TIP_B + CLEAR + 1.2         # the lid's outside

# a post between each pair of planets, as big as fits clear of the sun, the
# rings and the planets
POST_AT = (SUN_TIP + min(TIP_A, TIP_B)) / 2
post_to_planet = math.sqrt(POST_AT * POST_AT + ORBIT * ORBIT - 2 * POST_AT * ORBIT * math.cos(math.pi / PLANETS))
POST_R = min(POST_AT - SUN_TIP, min(TIP_A, TIP_B) - POST_AT, post_to_planet - max(P_TIP_A, P_TIP_B)) - CLEAR - 0.3
if POST_R < 2:
    fail("no room for the cage's posts between the planets")

def feet():
    """The pins and posts: (x, y, radius) of each."""
    out = []
    for i in range(PLANETS):
        th = 2 * math.pi * i / PLANETS
        out.append((ORBIT * math.cos(th), ORBIT * math.sin(th), PIN_R))
        th += math.pi / PLANETS
        out.append((POST_AT * math.cos(th), POST_AT * math.sin(th), POST_R))
    return out

# The lid must be LID thick over the planets, and round the shaft's hole,
# where it is furthest from the pins and posts.
around_shaft = 0
for i in range(360):
    a = 2 * math.pi * i / 360
    x, y = (SHAFT_R + CLEAR) * math.cos(a), (SHAFT_R + CLEAR) * math.sin(a)
    around_shaft = max(around_shaft, min([math.sqrt((x - fx) * (x - fx) + (y - fy) * (y - fy)) - fr for fx, fy, fr in feet()]))
Z_TOP = Z_PIN + max(P_TIP_B + CLEAR - PIN_R, around_shaft) + LID   # the lid's and the dial's top

# --- helpers ------------------------------------------------------------------

def shape(kind, lo, hi, hole = False):
    id = cad.add_shape(kind, hole = hole)
    if kind in ("cylinder", "cone"):
        cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def disc(x, y, r, z0, z1, hole = False):
    return shape("cylinder", [x - r, y - r, z0], [x + r, y + r, z1], hole)

def cone(apex_z, base_z, hole = False, up = True, x = 0, y = 0):
    """A 45-degree cone on a vertical axis through (x, y): its point at
    apex_z, its base at base_z, pointing up (up) or down."""
    h = abs(apex_z - base_z)
    id = shape("cone", [x - h, y - h, min(apex_z, base_z)], [x + h, y + h, max(apex_z, base_z)], hole)
    if not up:
        cad.rotate("x", 180, ids = [id])
    return id

def group(mode, ids, name = ""):
    g = cad.group("union", ids = ids)
    if mode == "intersect":
        cad.set_group_mode("intersect", ids = [g])
    if name:
        cad.set_name(name, ids = [g])
        cad.set_color(COLORS[name], ids = [g])
    return g

def hole(mode, ids):
    """A group of shapes, used as a hole."""
    g = group(mode, ids)
    cad.set_hole(True, ids = [g])
    return g

def place(id, x, y, z0, turn):
    """Turn a gear about its axis (turn, in radians) and put its axis at
    (x, y), its bottom at z0."""
    if turn:
        cad.rotate("z", math.degrees(turn), ids = [id])
    b = cad.node(id)["bounds"]
    c = b["center"]
    cad.move(dx = x - c[0], dy = y - c[1], dz = z0 - b["min"][2], ids = [id])
    return id

def gear(kind, m, teeth, height, helix, params = {}):
    id = cad.add_shape(kind)
    for key, value in [("module", m), ("teeth", teeth), ("thickness", height), ("helix", helix),
                       ("herringbone", 1), ("clearance", C), ("tipgap", TIP_GAP)] + list(params.items()):
        cad.set_param(id, key, value)
    return id

def ring_outline(r, n = 160):
    return [[r * math.cos(2 * math.pi * i / n), r * math.sin(2 * math.pi * i / n)] for i in range(n)]

def round_loft(profile, hole = False):
    """A solid turned about the Z axis: profile is [[z, radius], ...], z rising."""
    return cad.add_loft([[z, ring_outline(r)] for z, r in profile], hole = hole)

# --- the base: the fixed ring and its flange ------------------------------------

ring_a = gear("ringgear", MODULE, RING, HT, -HELIX, {"rim": R - ROOT_A})
# its spaces start half a tooth past its teeth, which face the planets' gaps
place(ring_a, 0, 0, 0, math.pi * (PLANET + 1) / RING)
ring_a = group("intersect", [ring_a, cone(APEX_A, 0)])
# the wall, with the groove for the dial's ridge: G below its lower face,
# G above its upper face, then CLEAR beside the dial up to the lip's top
wall = cad.add_outline([ring_outline(R_WALL), ring_outline(R - 1)], z0 = 0, z1 = LIP_TOP)
groove = round_loft([[BASE_TOP, R], [Z_LOW + RIDGE, R + RIDGE + G], [LIP_BOT, R + CLEAR], [LIP_TOP + 1, R + CLEAR]], hole = True)
flange = cad.add_outline([ring_outline(R_WALL + FLANGE), ring_outline(R_WALL - 0.5)], z0 = 0, z1 = FLANGE_H)
base_parts = [ring_a, group("union", [wall, groove]), flange]
# the pointer, at +Y on the lip, aimed at the dial's ticks and stopping just
# below them, so it doesn't hide the one it points at
tip = R + CLEAR + 0.2
base_parts.append(cad.add_outline([[0, tip], [3, R_WALL - 0.2], [-3, R_WALL - 0.2]], z0 = LIP_TOP - 0.01, z1 = Z_TOP - 7))
if MOUNT_HOLES:
    for a in (30, 150, 270):
        x, y = (R_WALL + FLANGE / 2) * math.cos(math.radians(a)), (R_WALL + FLANGE / 2) * math.sin(math.radians(a))
        base_parts.append(disc(x, y, 1.7, -1, FLANGE_H + 1, hole = True))
base = group("union", base_parts, "Base")

# --- the dial: the output ring ----------------------------------------------------

ring_b = gear("ringgear", M2, OUT_RING, Z_TOP - HT + LAP, -HELIX, {"rim": R - ROOT_B})
place(ring_b, 0, 0, HT - LAP, 0)
skirt = cad.add_outline([ring_outline(R), ring_outline(PLANET_TIP + CLEAR)], z0 = 0, z1 = HT)
ridge = round_loft([[Z_LOW, R], [Z_LOW + RIDGE, R + RIDGE], [Z_LOW + 2 * RIDGE, R]])
dial_parts = [ring_b, skirt, ridge, cone(APEX_B, 0, hole = True)]
# The zigzag: the dial's underside is raised by h, rising at 45 degrees
# round the ring from SPOTS points to peaks between them, so only those
# points are G above the base. Each is printed onto the base and may stick
# there; the rest of the dial grows out from them at 45 degrees. The
# underside is z = Z_LOW + |r - R| + h, cut away by two lofts: outside R
# (r > R + w) and inside it (r < R - w), w = z - Z_LOW - h.
SPOTS = int(math.round(math.pi * R / NOTCH))
def zigzag(z, side):
    pts = []
    for k in range(4 * SPOTS):
        a = 2 * math.pi * k / (4 * SPOTS)
        u = k % 4 / 4.0
        h = math.pi * R / SPOTS * (1 - abs(2 * u - 1))
        r = R + side * (z - Z_LOW - h)
        pts.append([r * math.cos(a), r * math.sin(a)])
    return pts
r_in, r_out = TIP_B - 1, R + RIDGE + 1
z0, z1 = Z_LOW - 1, Z_LOW + (R - r_in) + NOTCH + 1
outside = cad.add_loft([[z0, zigzag(z0, 1)], [z1, zigzag(z1, 1)]], hole = True)
dial_parts.append(hole("union", [disc(0, 0, r_out, z0, z1), outside]))
inside = cad.add_loft([[z0, zigzag(z0, -1)], [z1, zigzag(z1, -1)]])
dial_parts.append(hole("union", [inside, disc(0, 0, r_in, z0 - 1, z1 + 1, hole = True)]))
# its inner edge: chamfered under the lid, then straight up beside it
dial_parts.append(hole("intersect", [disc(0, 0, LID_R + CLEAR, HT, Z_TOP + 1), cone(DIAL_CUT, Z_TOP + 1, up = False)]))
# a tick per turn of the crank, longer every tenth, and the tens engraved
ticks = int(math.round(abs(RATIO)))
for i in range(ticks):
    a = 2 * math.pi * i / ticks
    tick = shape("box", [R - 1.0, -0.4, Z_TOP - (6 if i % 10 == 0 else 3)], [R + 1, 0.4, Z_TOP + 1], hole = True)
    cad.rotate("z", math.degrees(a) + 90, ids = [tick])
    b = cad.node(tick)["bounds"]["center"]
    cad.move(dx = R * math.cos(a + math.pi / 2) - b[0], dy = R * math.sin(a + math.pi / 2) - b[1], ids = [tick])
    dial_parts.append(tick)
    if i % 10 == 0:
        t = cad.add_shape("text", hole = True)
        cad.set_text(t, str(i))
        rt = (LID_R + CLEAR + R - 1) / 2
        x, y = rt * math.cos(a + math.pi / 2), rt * math.sin(a + math.pi / 2)
        cad.set_bounds(min = [x - 2 * len(str(i)), y - 2, Z_TOP - 0.6], max = [x + 2 * len(str(i)), y + 2, Z_TOP + 1], ids = [t])
        cad.rotate("z", math.degrees(a), ids = [t])
        dial_parts.append(t)
dial = group("union", dial_parts, "Dial")

# --- the crank: sun, shaft, knob and handle ------------------------------------------

sun = place(gear("helicalgear", MODULE, SUN, HT, HELIX, {"bore": 0}), 0, 0, 0, 0)
z_flare = Z_TOP + G                              # the shaft's top, clear of the lid
z_knob = z_flare + KNOB_R - SHAFT_R              # the flare's top
crank_parts = [sun, disc(0, 0, SHAFT_R, HT - 1, z_flare + 0.01)]
flare = shape("cone", [-KNOB_R, -KNOB_R, z_flare], [KNOB_R, KNOB_R, z_knob])
cad.set_param(flare, "top", SHAFT_R)
cad.rotate("x", 180, ids = [flare])                # wide end up
crank_parts.append(flare)
knob_top = z_knob + (KNOB_H if HANDLE == "crank" else KNOB_TALL)
crank_parts.append(disc(0, 0, KNOB_R, z_knob - 0.01, knob_top))
if HANDLE == "crank":
    crank_parts.append(disc(KNOB_R - PEG_R - 1.5, 0, PEG_R, knob_top - 0.01, knob_top + PEG_H))
# flutes round the knob's edge, for grip
for i in range(16):
    a = 2 * math.pi * (i + 0.5) / 16
    crank_parts.append(disc((KNOB_R + 1) * math.cos(a), (KNOB_R + 1) * math.sin(a), 1.6, z_knob + 0.8, knob_top + 1, hole = True))
crank = group("union", crank_parts, "Crank")

# --- the cage: pins, posts and lid ----------------------------------------------------

# the lid: within reach of the pins and posts, inside its outside, clear of
# the dial ring's chamfer...
grow = group("union", [cone(Z_PIN - fr, Z_TOP + 1, up = False, x = fx, y = fy) for fx, fy, fr in feet()])
within = disc(0, 0, LID_R, Z_PIN - 10, Z_TOP)
clear_of_dial = group("union", [disc(0, 0, TIP_B - CLEAR, Z_PIN - 10, Z_TOP + 1), cone(DIAL_CUT + G, Z_TOP + 1, up = False)])
# ...and open, so the planets show: a hub round the shaft, an arm out to
# each pin and post, and a collar round each pin. The posts' arms go on
# between the planets to the lid's outside, over the dial's chamfer, which
# keeps the top of the cage centred.
spider = [disc(0, 0, SHAFT_R + CLEAR + 2.5, Z_PIN - 10, Z_TOP + 1)]
for fx, fy, fr in feet():
    d = math.sqrt(fx * fx + fy * fy)
    nx, ny = -fy / d * ARM / 2, fx / d * ARM / 2
    reach = 1 if fr == PIN_R else (LID_R + 1) / d
    ex, ey = fx * reach, fy * reach
    spider.append(cad.add_outline([[nx, ny], [ex + nx, ey + ny], [ex - nx, ey - ny], [-nx, -ny]], z0 = Z_PIN - 10, z1 = Z_TOP + 1))
    spider.append(disc(fx, fy, fr + (2.5 if fr == PIN_R else 0), Z_PIN - 10, Z_TOP + 1))
lid_parts = [group("intersect", [grow, within, clear_of_dial, group("union", spider)])]
# ...the planets' countersunk tops, and the shaft
for i in range(PLANETS):
    th = 2 * math.pi * i / PLANETS
    x, y = ORBIT * math.cos(th), ORBIT * math.sin(th)
    keep = cone(Z_PIN - PIN_R, Z_TOP + 1, up = False, x = x, y = y)
    cad.set_hole(True, ids = [keep])
    lid_parts.append(hole("union", [disc(x, y, P_TIP_B + CLEAR, Z_PIN - 10, Z_TOP + 1), keep]))
lid_parts.append(disc(0, 0, SHAFT_R + CLEAR, Z_PIN - 10, Z_TOP + 1, hole = True))
cage_parts = [group("union", lid_parts)]
for fx, fy, fr in feet():
    cage_parts.append(disc(fx, fy, fr, 0, Z_PIN + 0.01))
cage = group("union", cage_parts, "Cage")

# --- the planets ------------------------------------------------------------------

planets = []
for i in range(PLANETS):
    th = 2 * math.pi * i / PLANETS
    x, y = ORBIT * math.cos(th), ORBIT * math.sin(th)
    # bottom teeth: fit the sun (first tooth on +X) and the fixed ring
    low = place(gear("helicalgear", MODULE, PLANET, HT, -HELIX, {"bore": 0}), x, y, 0,
                th + math.pi + (SUN * th - math.pi) / PLANET)
    # top teeth: fit the dial ring, whose teeth are half a tooth past +X;
    # the same height as the dial ring's, so their herringbones line up
    rr, rp = pitch(M2, OUT_RING), pitch(M2, PLANET)
    turn = th + (rr * (math.pi / OUT_RING - th) - math.pi * M2 / 2) / rp
    high = place(gear("helicalgear", M2, PLANET, Z_TOP - HT + LAP, -HELIX, {"bore": 0}), x, y, HT - LAP, turn)
    # chamfered from the roots at HT, 45 degrees out
    keep = cone(HT - ROOT_P2, Z_TOP + 1, up = False, x = x, y = y)
    high = group("intersect", [high, keep])
    # the pin's hole, and the top countersunk G under the lid
    bore = disc(x, y, BORE, -1, Z_TOP + 1, hole = True)
    sink = cone(Z_PIN - G - PIN_R, Z_TOP + 1, hole = True, up = False, x = x, y = y)
    planets.append(group("union", [low, high, bore, sink], "Planet"))

reducer = cad.group("bundle", ids = [base, dial, crank, cage] + planets)
cad.set_name("Crank reducer", ids = [reducer])
b = cad.node(reducer)["bounds"]
print("crank reducer: %s:1, top module %s, %s mm across, %s mm tall" % (
    str(math.round(RATIO * 100) / 100), str(math.round(M2 * 1000) / 1000),
    str(math.round(b["size"][0] * 10) / 10), str(math.round(b["size"][2] * 10) / 10)))
