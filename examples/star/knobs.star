# knobs.star - replacement knobs for stoves, radios, amps, potentiometers
# and cabinets: three styles in a row, each with a different bore.
#
#   fluted   - a round knob with grip grooves round it, and a D-shaft bore
#              (6 mm round with one flat)
#   skirted  - a round knob with fine ridges, standing on a flared skirt
#              with tick marks, and a bore for a knurled shaft with a slot
#              (a rib in the bore fits the slot)
#   octagon  - an eight-sided knob with a plain round bore and a set-screw
#              hole
#
# Every knob has a pointer line on top. The grooves, ridges and ticks are
# placed by loops over angles. Measure your shaft and set the bore numbers
# below; swap the styles' bores around, or add styles, in STYLES.
#
# They print upright with no supports: the bore opens at the bottom (its
# closed end is a short bridge), the grip runs straight up, the skirt
# narrows upward and only the top edge is rounded.
#
# Basic shapes only. Sizes are in mm.

SHAFT = 6.0             # shaft diameter, all bore types
FIT = 0.2               # added to the shaft for the bore (a snug push fit)
D_FLAT = 4.5            # D-shaft: across the flat
KNURL_SLOT = 1.0        # knurled shaft: the slot's width (the rib is 0.1 less)
KNURL_SLOT_DEPTH = 6.0  # ... and how deep it goes
SET_SCREW = 2.8         # set-screw hole (2.8 to tap or self-tap an M3)
BORE_DEPTH = 12.0       # how far the shaft goes in
POINTER_W = 1.2         # pointer groove width
POINTER_D = 0.8         # ... and depth
ROUND = 1.5             # rounding of the top edge
GAP = 12.0              # between knobs on the build plate

STYLES = [
    {"name": "fluted", "body": "round", "d": 26.0, "h": 18.0, "grooves": 12, "groove_d": 4.0,
     "skirt": False, "bore": "d"},
    {"name": "skirted", "body": "round", "d": 22.0, "h": 16.0, "grooves": 30, "groove_d": 1.6,
     "skirt": True, "bore": "knurled"},
    {"name": "octagon", "body": "octagon", "d": 26.0, "h": 15.0, "grooves": 0, "groove_d": 0,
     "skirt": False, "bore": "round"},
]
SKIRT_D = 36.0          # skirt diameter at the bottom
SKIRT_H = 5.0           # skirt height
TICKS = 11              # tick marks round the skirt, over 300 degrees
COLORS = ["#2b2b2b", "#8c2f2f", "#c9a227"]
SIDES = 64

# --- helpers --------------------------------------------------------------

def place(kind, lo, hi, hole = False, sides = 0):
    id = cad.add_shape(kind, hole = hole)
    if sides:
        cad.set_param(id, "sides", sides)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def cyl(cx, cy, d, z0, z1, hole = False, sides = SIDES):
    return place("cylinder", [cx - d / 2, cy - d / 2, z0], [cx + d / 2, cy + d / 2, z1], hole, sides)

def box(lo, hi, hole = False):
    return place("box", lo, hi, hole)

def radial(cx, cy, r, angle, length, width, z0, z1, hole = False):
    """A box pointing out from (cx, cy), its middle at radius r and angle
    (degrees), length along the radius."""
    id = box([-length / 2, -width / 2, z0], [length / 2, width / 2, z1], hole)
    cad.rotate("z", angle, ids = [id])
    a = math.radians(angle)
    cad.set_position(x = cx + r * math.cos(a), y = cy + r * math.sin(a), anchor = "center", ids = [id])
    return id

def copy_of(id):
    """A copy of shape id, in the same place."""
    cad.select([id])
    return cad.duplicate()[0]

def around(id, cx, cy, r, from_angle, angle):
    """A copy of a shape lying at from_angle (degrees) on a circle of
    radius r about (cx, cy), turned and moved to lie at angle instead."""
    new = copy_of(id)
    cad.rotate("z", angle - from_angle, ids = [new])
    a = math.radians(angle)
    cad.set_position(x = cx + r * math.cos(a), y = cy + r * math.sin(a), anchor = "center", ids = [new])
    return new

def body(style, cx, cy, z0):
    """The knob's body from z0 to the top, its top edge rounded: a plain
    piece below and a bevelled one on top whose rounded lower edge stays
    hidden inside it, so the bottom edge stays square for the bed."""
    d, top = style["d"], z0 + style["h"]
    if style["body"] == "octagon":
        def piece(a, b, bevel):
            id = cad.add_shape("polygon")
            cad.set_param(id, "sides", 8)
            if bevel:
                cad.set_param(id, "bevel", bevel)
            w, h, _ = cad.node(id)["bounds"]["size"]
            f = d / max(w, h)
            cad.set_bounds(min = [cx - w * f / 2, cy - h * f / 2, a], max = [cx + w * f / 2, cy + h * f / 2, b], ids = [id])
            return id
    else:
        def piece(a, b, bevel):
            id = cyl(cx, cy, d, a, b)
            if bevel:
                cad.set_param(id, "bevel", bevel)
            return id
    return [piece(z0, top - ROUND, 0), piece(top - 3 * ROUND, top, ROUND)]

def bore(kind, cx, cy, z_top):
    """The bore's holes (and any solid parts that go inside it, added after
    the holes), from below the bottom up to z_top."""
    d = SHAFT + FIT
    if kind == "d":
        # round with one flat: a cylinder cut off by a box, as one hole
        flat_x = cx - d / 2 + D_FLAT + FIT / 2
        g = cad.group("intersect", ids = [cyl(cx, cy, d, -1, z_top),
                                          box([cx - d, cy - d, -2], [flat_x, cy + d, z_top + 1])])
        cad.set_hole(True, ids = [g])
        return [g], []
    if kind == "knurled":
        # round, with a rib across the top of the bore to fit the shaft's slot
        rib = box([cx - d / 2 - 0.1, cy - (KNURL_SLOT - 0.1) / 2, z_top - KNURL_SLOT_DEPTH],
                  [cx + d / 2 + 0.1, cy + (KNURL_SLOT - 0.1) / 2, z_top + 0.1])
        return [cyl(cx, cy, d, -1, z_top, hole = True)], [rib]
    # plain round, with a set-screw hole in from the side (-X) halfway up
    screw = cad.add_shape("cylinder", hole = True)
    cad.set_param(screw, "sides", 24)
    cad.rotate("y", 90, ids = [screw])
    z = z_top / 2
    cad.set_bounds(min = [cx - 30, cy - SET_SCREW / 2, z - SET_SCREW / 2], max = [cx, cy + SET_SCREW / 2, z + SET_SCREW / 2], ids = [screw])
    return [cyl(cx, cy, d, -1, z_top, hole = True), screw], []

# --- one knob -------------------------------------------------------------

def knob(style, cx):
    cy = 0.0
    z0 = SKIRT_H - 1 if style["skirt"] else 0.0       # the body stands on the skirt
    top = z0 + style["h"]
    r = style["d"] / 2
    parts = body(style, cx, cy, z0)

    if style["skirt"]:
        # a flared skirt: a short rim, then a cone narrowing up into the body
        rim = cyl(cx, cy, SKIRT_D, 0, 1.2)
        cone = place("cone", [cx - SKIRT_D / 2, cy - SKIRT_D / 2, 1.2], [cx + SKIRT_D / 2, cy + SKIRT_D / 2, SKIRT_H + 2], sides = SIDES)
        cad.set_param(cone, "top", r - 1)
        parts += [rim, cone]

    holes = []
    # grip grooves round the body, above the skirt: one, and copies of it
    # moved round the circle (a groove is round, so it needn't turn)
    n = style["grooves"]
    if n > 0:
        gd = style["groove_d"]
        gr = r + gd * 0.15
        first = cyl(cx + gr, cy, gd, z0 + (SKIRT_H if style["skirt"] else -1), top + 1, hole = True, sides = 16)
        holes.append(first)
        for k in range(1, n):
            a = 2 * math.pi * k / n
            holes.append(copy_of(first))
            cad.move(dx = gr * (math.cos(a) - 1), dy = gr * math.sin(a), ids = [holes[-1]])

    # tick marks round the skirt, over 300 degrees, the middle one under the
    # pointer: a long one and a short one made, the rest copies of them
    # turned into place
    if style["skirt"]:
        angles = [-150 + 300 * k / (TICKS - 1) for k in range(TICKS)]
        ends = [0, TICKS - 1, (TICKS - 1) // 2]          # the long ones
        long_r, short_r = SKIRT_D / 2 - 2.5, SKIRT_D / 2 - 1.7
        long_tick = radial(cx, cy, long_r, angles[0], 5, 0.9, 0.6, SKIRT_H + 2, hole = True)
        short_tick = radial(cx, cy, short_r, angles[1], 3.4, 0.9, 0.6, SKIRT_H + 2, hole = True)
        holes += [long_tick, short_tick]
        for k in range(2, TICKS):
            if k in ends:
                holes.append(around(long_tick, cx, cy, long_r, angles[0], angles[k]))
            else:
                holes.append(around(short_tick, cx, cy, short_r, angles[1], angles[k]))

    # the pointer: a groove across the top from the middle to the edge (+X)
    holes.append(box([cx + 1.5, cy - POINTER_W / 2, top - POINTER_D], [cx + r + 2, cy + POINTER_W / 2, top + 1], hole = True))

    bore_holes, inserts = bore(style["bore"], cx, cy, BORE_DEPTH)
    k = cad.group("union", ids = parts + holes + bore_holes)
    if inserts:
        k = cad.group("union", ids = [k] + inserts)     # (holes would cut these too)
    cad.set_name(style["name"] + " knob", ids = [k])
    return k

# --- the row --------------------------------------------------------------

knobs = []
x = 0.0
for i, style in enumerate(STYLES):
    w = SKIRT_D if style["skirt"] else style["d"]
    if i > 0:
        x += w / 2
    k = knob(style, x)
    cad.set_color(COLORS[i % len(COLORS)], ids = [k])
    knobs.append(k)
    x += w / 2 + GAP

cad.select(knobs)
cad.set_position(x = 0, y = 0, anchor = "center")
cad.fit_view()
print(", ".join(["%s (%s bore)" % (s["name"], s["bore"]) for s in STYLES]))
