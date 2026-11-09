# wankel.star - a motorized Wankel rotary engine: a three-cornered rotor
# rolls round inside its oval housing, driven by a small TT gear motor
# (5 V, 59 rpm, single shaft) fixed to the back. Three printed parts.
#
# HOW IT WORKS. The motor turns the CRANK, whose round lobe sits off centre
# by E and carries the ROTOR round in a circle. A ring of teeth inside the
# rotor rolls on the fixed gear in the middle of the HOUSING, 21 teeth on
# 14, so the rotor turns once for every three turns of the crank, its three
# tips sweeping the housing's epitrochoid wall all the way round. With the
# 59 rpm motor the rotor turns at about 20 rpm. The rotor's flanks and its
# ring of teeth are both cut by the script: each is everything that stays
# CLEAR away from the housing's wall, or from the fixed gear, as the rotor
# moves.
#
# THE MOTOR fits flat against the back of the housing, its gearbox along
# the housing's long axis and its can sticking out at one end. Its shaft
# goes up through the middle, the small boss beside the shaft drops into a
# pocket so the motor can't turn, and the crank is pushed onto the shaft's
# flats. Like a TT motor's wheel, the crank can also be held by an M2 screw
# into the end of the shaft, through the hole in its face.
#
# PRINTING. No supports. Print the three parts as they are laid out: the
# housing on its back, the rotor and the crank face down. 0.2 mm layers;
# 0.12-0.16 mm gives smoother teeth. Set ASSEMBLED to True to see the
# engine put together, with a stand-in for the motor that isn't printed.
#
# ASSEMBLY. Drop the rotor into the housing, teeth first, over the fixed
# gear, with any one of its tips in an end of the oval. Push the crank's
# sleeve down through the rotor into the fixed gear. Then, from the back,
# turn the crank until the flats line up and press the motor's shaft in,
# its boss in the pocket.
#
# The housing is about 62 x 47 mm and 13 mm tall, with a notch under one
# end for the clip on the motor's can. Sizes are in mm.

ASSEMBLED = False
CRANK = 0.0           # degrees, the crank's turn when ASSEMBLED
SCALE = 1.15          # the whole engine, but not its gaps; below about 1.1 the crank is too short for the shaft
MODULE = 1.0 * SCALE
FIXED_TEETH = 14      # the fixed gear; the rotor's ring has 3/2 as many
K = 6.0               # the rotor's size: its tips are K * E from its centre
CLEAR = 0.4           # gap between moving parts (0.3-0.5 for most printers)
TIP_GAP = 0.3         # gap between each tooth's tip and the root it turns in
WALL = 2.4 * SCALE    # the housing's wall
PLATE = 2.0 * SCALE   # its back
GEAR_H = 4.3 * SCALE  # the fixed gear's height above the back
BORE_H = 4.0 * SCALE  # the rotor's bearing band, round the crank's lobe
FLANGE = 1.2 * SCALE  # the crank's cap over the rotor
SHAFT_FIT = 0.1       # added to the motor shaft's 5.4 mm (3.7 across its flats)
SHAFT_ROOM = 0.6      # spare depth in the crank's bore past the shaft's end
CAP = 1.2             # the crank's face over the bore
COLORS = {"Housing": "#8c96a0", "Rotor": "#e84118", "Crank": "#2f3640", "Motor": "#f1c40f"}

# the TT motor, from Bambu Lab's model (LA001) and a test print, from the shaft's axis
SHAFT_D = 5.4
SHAFT_FLATS = 3.7
SHAFT_LEN = 11.8      # above the gearbox's face (the model's 8.5 left the motor 2.6 mm short of the back)
COLLAR = [3.6, 0.5]   # radius and height of the collar round the shaft
BOSS = [11.15, 2.0, 1.0]    # the locating boss: distance from the shaft, radius (about 4 mm across), height
BOSS_ROOM = 0.45      # added to the boss's radius for its pocket (small holes print small)
CLIP = [29.2, 32.2, 4.0, 1.2]   # the clip on the motor's can: its ends along the motor, width, height above the face
GEARBOX = [-11.2, 25.2, 22.4, 18.8]   # the face's ends along the motor, its width, its thickness

C = CLEAR / 2
ALPHA = math.radians(20)

# --- sizes --------------------------------------------------------------------------

def pitch(z):
    return MODULE * z / 2

if FIXED_TEETH % 2 != 0:
    fail("FIXED_TEETH must be even, so the rotor's ring has a whole number of teeth")
RING_TEETH = FIXED_TEETH * 3 // 2
E = pitch(RING_TEETH) - pitch(FIXED_TEETH)         # the crank's throw
R = K * E                                          # the epitrochoid's generating radius
SLEEVE_R = SHAFT_D / 2 + SHAFT_FIT / 2 + 1.2       # the crank's sleeve round the motor shaft
BORE = SLEEVE_R + CLEAR                            # the fixed gear's bore
FIXED_TIP = pitch(FIXED_TEETH) + MODULE
RING_TIP = pitch(RING_TEETH) - MODULE              # the rotor's teeth point inward
FIXED_ROOT = RING_TIP - E - TIP_GAP
LOBE_R = E + SLEEVE_R + 0.05                       # the lobe takes in the whole sleeve
if FIXED_ROOT < BORE + 1.0:
    fail("the fixed gear would be too thin round the crank: raise FIXED_TEETH or MODULE")
if LOBE_R + CLEAR > RING_TIP - 0.5:
    fail("the crank's lobe would not fit inside the rotor's teeth: raise FIXED_TEETH")

Z_ROTOR = PLATE + 0.3                              # the rotor rests on the back in use
Z_GEAR = PLATE + GEAR_H                            # the fixed gear's top
Z_LOBE = Z_GEAR + 0.3                              # the crank's lobe, above it
Z_SPLIT = Z_GEAR + 0.5                             # the rotor's teeth below, its bearing above
Z_TOP = Z_SPLIT + BORE_H                           # the rotor's top
Z_WALL = Z_TOP + 0.3
Z_CRANK = Z_WALL + FLANGE
Z_SLEEVE = COLLAR[1]                               # pushed on until it meets the collar
Z_BORE = SHAFT_LEN + SHAFT_ROOM                    # the top of the crank's bore
if Z_BORE + CAP > Z_CRANK:
    fail("the motor's shaft would come through the crank: raise SCALE")

# --- outlines ---------------------------------------------------------------------

def polar(rho, a):
    return [rho * math.cos(a), rho * math.sin(a)]

def circle(r, x = 0, y = 0, n = 96):
    return [[x + r * math.cos(2 * math.pi * i / n), y + r * math.sin(2 * math.pi * i / n)] for i in range(n)]

def teeth(z, ra, rf, cut):
    """A gear's outline, counter-clockwise: z teeth with the first on +X, tips
    at ra, roots at rf, each flank an involute cut in by cut radians."""
    r = pitch(z)
    rb = r * math.cos(ALPHA)
    inva = math.tan(ALPHA) - ALPHA
    r0 = max(rb, rf)
    def inv(rho):
        if rho <= rb:
            return 0.0
        t = math.sqrt(rho * rho / (rb * rb) - 1)
        return t - math.atan(t)
    def half(rho):
        return math.pi / (2 * z) + inva - inv(rho) - cut
    out = []
    for k in range(z):
        c = 2 * math.pi * k / z
        if rf < rb:
            out.append(polar(rf, c - half(rb)))
        for i in range(11):
            rho = r0 + (ra - r0) * i / 10
            out.append(polar(rho, c - half(rho)))
        for i in range(1, 4):
            out.append(polar(ra, c - half(ra) + 2 * half(ra) * i / 4))
        for i in range(10, -1, -1):
            rho = r0 + (ra - r0) * i / 10
            out.append(polar(rho, c + half(rho)))
        if rf < rb:
            out.append(polar(rf, c + half(rb)))
        a0, a1 = c + half(r0), c + 2 * math.pi / z - half(r0)
        steps = max(3, int((a1 - a0) * rf / 0.5))
        for i in range(1, steps):
            out.append(polar(rf, a0 + (a1 - a0) * i / steps))
    return out

def densify(pts, step):
    out = []
    for i in range(len(pts)):
        p, q = pts[i], pts[(i + 1) % len(pts)]
        d = math.sqrt((q[0] - p[0]) * (q[0] - p[0]) + (q[1] - p[1]) * (q[1] - p[1]))
        n = max(1, int(d / step) + 1)
        for k in range(n):
            out.append([p[0] + (q[0] - p[0]) * k / n, p[1] + (q[1] - p[1]) * k / n])
    return out

def trochoid(t, off):
    """The housing's epitrochoid at t, moved off along its normal (outward)."""
    x = E * math.cos(3 * t) + R * math.cos(t)
    y = E * math.sin(3 * t) + R * math.sin(t)
    dx = -3 * E * math.sin(3 * t) - R * math.sin(t)
    dy = 3 * E * math.cos(3 * t) + R * math.cos(t)
    d = math.sqrt(dx * dx + dy * dy)
    return [x + dy / d * off, y - dx / d * off]

NT = 720
CAVITY = [trochoid(2 * math.pi * i / NT, 0) for i in range(NT)]
OUTSIDE = [trochoid(2 * math.pi * i / NT, WALL) for i in range(NT)]

# --- the rotor, cut by its motion ------------------------------------------------------
#
# Seen from the rotor, the housing and the fixed gear move: with the crank
# at th, the rotor's centre is at E (cos th, sin th) and it has turned th / 3.
# The rotor's outline is a radius at each of NB angles round its centre: its
# flanks as far out as they can go and stay CLEAR inside the wall, its ring
# of teeth as far in as they can go and stay CLEAR / 2 from the fixed gear's
# teeth (each point grown to a circle of CLEAR / 2; the fixed gear's own
# faces are cut back the other half, and its tips are lengthened to leave
# TIP_GAP at the roots). The rotor is the same on each third, so each point
# is binned a third of the way round, and one turn of the crank is enough.

NB = 1440
THIRD = NB // 3
STEPS = 360

def binned(pts, steps, inner):
    """Each third-of-a-turn bin's nearest (or, if inner, farthest) point of
    pts, seen from the rotor as the crank turns once."""
    out = [0.0 if inner else 1e9] * THIRD
    for s in range(steps):
        th = 2 * math.pi * s / steps
        cx, cy = E * math.cos(th), E * math.sin(th)
        ca, sa = math.cos(-th / 3), math.sin(-th / 3)
        for p in pts:
            u, v = p[0] - cx, p[1] - cy
            x, y = u * ca - v * sa, u * sa + v * ca
            rho = math.sqrt(x * x + y * y)
            j = int((math.atan2(y, x) / (2 * math.pi) * NB) % NB) % THIRD
            if inner:
                if rho > out[j]:
                    out[j] = rho
            elif rho < out[j]:
                out[j] = rho
    return out

flank = binned([trochoid(2 * math.pi * i / NT, -CLEAR) for i in range(NT)], STEPS, False)
cutter = []
for p in densify(teeth(FIXED_TEETH, FIXED_TIP + TIP_GAP - C, FIXED_ROOT, 0.0), 0.25):
    for k in range(8):
        a = 2 * math.pi * k / 8
        cutter.append([p[0] + C * math.cos(a), p[1] + C * math.sin(a)])
ring = binned([p for p in cutter if math.sqrt(p[0] * p[0] + p[1] * p[1]) > FIXED_ROOT + C], STEPS, True)

def rotor_outline(radii, floor = 0):
    return [polar(max(radii[j % THIRD], floor), 2 * math.pi * (j + 0.5) / NB) for j in range(NB)]

FLANK = rotor_outline(flank)
RING = rotor_outline(ring, RING_TIP)
if min(flank) < max(ring) + 1.2:
    fail("the rotor's flanks would be too thin outside its teeth: raise K")

# --- helpers ----------------------------------------------------------------------

def shape(kind, lo, hi, hole = False):
    id = cad.add_shape(kind, hole = hole)
    if kind in ("cylinder", "cone"):
        cad.set_param(id, "sides", 64)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def disc(x, y, r, z0, z1, hole = False):
    return shape("cylinder", [x - r, y - r, z0], [x + r, y + r, z1], hole)

def group(ids, name = "", mode = "union"):
    g = cad.group("union", ids = ids)
    if mode == "intersect":
        cad.set_group_mode("intersect", ids = [g])
    if name:
        cad.set_name(name, ids = [g])
        cad.set_color(COLORS[name.split(" ")[0]], ids = [g])
    return g

def hole(ids):
    """The shapes' intersection, used as a hole."""
    g = group(ids, mode = "intersect")
    cad.set_hole(True, ids = [g])
    return g

def bed(id):
    """Turn a part over and stand it on the bed."""
    cad.rotate("x", 180, ids = [id])
    cad.move(dz = -cad.node(id)["bounds"]["min"][2], ids = [id])

def turn_about(id, cx, cy, deg):
    """Turn a shape about the vertical axis through (cx, cy)."""
    b = cad.node(id)["bounds"]["center"]
    cad.rotate("z", deg, ids = [id])
    w = math.radians(deg)
    ux, uy = cx - b[0], cy - b[1]
    rx, ry = ux * math.cos(w) - uy * math.sin(w), ux * math.sin(w) + uy * math.cos(w)
    cad.move(dx = ux - rx, dy = uy - ry, ids = [id])

# --- the parts, assembled, with the crank at 0 ---------------------------------------

housing = group([
    cad.add_outline([OUTSIDE], z0 = 0, z1 = PLATE),
    cad.add_outline([OUTSIDE, CAVITY], z0 = PLATE - 0.01, z1 = Z_WALL),
    cad.add_outline([teeth(FIXED_TEETH, FIXED_TIP, FIXED_ROOT, C / (pitch(FIXED_TEETH) * math.cos(ALPHA)))], z0 = PLATE - 0.01, z1 = Z_GEAR),
    disc(0, 0, BORE, -1, Z_GEAR + 1, hole = True),
    disc(BOSS[0], 0, BOSS[1] + BOSS_ROOM, -1, BOSS[2] + 0.5, hole = True),
    # a notch under the end of the housing for the clip on the motor's can
    shape("box", [CLIP[0] - CLEAR, -CLIP[2] / 2 - CLEAR, -1], [CLIP[1] + 10, CLIP[2] / 2 + CLEAR, CLIP[3] + 0.3], hole = True),
], "Housing")

rotor = group([
    cad.add_outline([FLANK, RING], z0 = Z_ROTOR, z1 = Z_SPLIT + 0.01),
    cad.add_outline([FLANK, circle(LOBE_R + CLEAR, n = 128)], z0 = Z_SPLIT, z1 = Z_TOP),
], "Rotor")

# the motor shaft's bore: round, with two flats, and the M2 screw's hole above it
r_bore = SHAFT_D / 2 + SHAFT_FIT / 2
f_bore = SHAFT_FLATS / 2 + SHAFT_FIT / 2
crank = group([
    disc(0, 0, SLEEVE_R, Z_SLEEVE, Z_LOBE + 0.01),
    disc(E, 0, LOBE_R, Z_LOBE, Z_WALL + 0.01),
    disc(E, 0, LOBE_R + CLEAR + 1.6, Z_WALL, Z_CRANK),
    hole([disc(0, 0, r_bore, Z_SLEEVE - 1, Z_BORE),
          shape("box", [-f_bore, -r_bore - 1, Z_SLEEVE - 1], [f_bore, r_bore + 1, Z_BORE])]),
    disc(0, 0, 1.1, Z_BORE - 0.1, Z_CRANK + 1, hole = True),
], "Crank")

parts = [housing, rotor, crank]
if ASSEMBLED:
    turn_about(rotor, 0, 0, CRANK / 3)
    cad.move(dx = E * math.cos(math.radians(CRANK)), dy = E * math.sin(math.radians(CRANK)), ids = [rotor])
    turn_about(crank, 0, 0, CRANK)
    # a stand-in for the motor, under the housing's back
    gx0, gx1, gw, gt = GEARBOX
    parts.append(group([
        shape("box", [gx0, -gw / 2, -gt], [gx1, gw / 2, 0]),
        shape("box", [gx1, -gw / 2 + 1, -gt - 1.5], [gx1 + 28, gw / 2 - 1, -0.5]),
        disc(0, 0, COLLAR[0], -0.01, COLLAR[1]),
        group([disc(0, 0, SHAFT_D / 2, 0, SHAFT_LEN),
               shape("box", [-SHAFT_FLATS / 2, -SHAFT_D, 0], [SHAFT_FLATS / 2, SHAFT_D, SHAFT_LEN])], mode = "intersect"),
        disc(BOSS[0], 0, BOSS[1], -0.01, BOSS[2]),
        shape("box", [CLIP[0], -CLIP[2] / 2, -1], [CLIP[1], CLIP[2] / 2, CLIP[3]]),
    ], "Motor (not printed)"))
else:
    # laid out for printing: the rotor and the crank face down beside the housing
    hb = cad.node(housing)["bounds"]
    for id, side in ((rotor, 1), (crank, -1)):
        bed(id)
        b = cad.node(id)["bounds"]
        y = hb["max"][1] + 6 - b["min"][1] if side > 0 else hb["min"][1] - 6 - b["max"][1]
        cad.move(dx = -b["center"][0], dy = y, ids = [id])

b = cad.node(housing)["bounds"]
engine = cad.group("bundle", ids = parts)
cad.set_name("Wankel engine", ids = [engine])
print("wankel: E %s, rotor tips at %s, housing %s x %s x %s mm, flanks %s outside the teeth" % (
    str(E), str(math.round(max(flank) * 100) / 100), str(math.round(b["size"][0] * 10) / 10),
    str(math.round(b["size"][1] * 10) / 10), str(math.round(b["size"][2] * 10) / 10),
    str(math.round((min(flank) - max(ring)) * 100) / 100)))
