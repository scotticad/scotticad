# cable-clips.star - cable clips, one for each cable on your desk, each
# labelled with its size.
#
# List your cables' diameters below (measure them with calipers) and run:
# the script lays out one clip per cable. Each clip is a tube with its top
# cut open a little narrower than the cable, so the cable snaps in, standing
# on a flat tab with a countersunk screw hole on one side and the size
# engraved on the other.
#
# The clips print as they stand, with no supports: the tube's lower half is
# filled out square to the tab (a round underside would overhang), and the
# snap opening keeps the arms' undersides steeper than 45 degrees.
#
# Basic shapes only: cylinders, boxes, a cone and text, and holes.
# Change the numbers below and run again. Sizes are in mm.

CABLES = [3.5, 5.0, 6.0, 8.0]   # cable diameters, one clip each
COPIES = 1                      # clips of each size

WALL = 2.0          # around the cable
CLEAR = 0.2         # added to the cable's diameter, for an easy fit
SNAP = 0.8          # opening, as a fraction of the cable's diameter
WIDTH = 10.0        # along the cable
TAB = 2.5           # tab thickness
SCREW = 3.5         # screw shank; set 0 for no hole (use an adhesive pad)
HEAD = 7.0          # countersink diameter at the top
EAR = 11.0          # tab length on the screw side
LABEL_DEPTH = 0.6   # engraving depth
GAP = 5.0           # between clips on the build plate

COLOR = "#4a6fa5"
SIDES = 64

# --- helpers --------------------------------------------------------------

def box(lo, hi, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def tube(cx, cy, cz, d, length, hole = False):
    """A cylinder of diameter d lying along Y, centred at (cx, cy, cz)."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.rotate("x", 90, ids = [id])
    cad.set_bounds(min = [cx - d / 2, cy - length / 2, cz - d / 2], max = [cx + d / 2, cy + length / 2, cz + d / 2], ids = [id])
    return id

def copy_of(id, dx = 0.0, dy = 0.0, dz = 0.0):
    """A copy of shape id, moved by (dx, dy, dz)."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, dz = dz, ids = [new])
    return new

def countersunk(x, y, z_top):
    """A screw hole down through the tab from z_top, with a 90-degree
    countersink for a flat-head screw, as one hole group."""
    shank = cad.add_shape("cylinder", hole = True)
    cad.set_param(shank, "sides", 32)
    cad.set_bounds(min = [x - SCREW / 2, y - SCREW / 2, -1], max = [x + SCREW / 2, y + SCREW / 2, z_top + 1], ids = [shank])
    sink_h = (HEAD - SCREW) / 2           # 90 degrees: as deep as it is wide
    sink = cad.add_shape("cone", hole = True)
    cad.set_param(sink, "sides", 32)
    # a cone narrows upward: make it upside down, wide end at the top
    cad.set_bounds(min = [x - HEAD / 2, y - HEAD / 2, z_top - sink_h], max = [x + HEAD / 2, y + HEAD / 2, z_top + 0.01], ids = [sink])
    cad.set_param(sink, "top", SCREW / 2)
    cad.mirror("z", ids = [sink])
    g = cad.group("union", ids = [shank, sink])
    cad.set_hole(True, ids = [g])
    return g

# The screw hole is the same in every clip: one is made here at the origin,
# each clip gets a copy moved into place, and this one is deleted at the end
# (the ones in clips can't be copied: they're inside the clips' groups).
SCREW_HOLE = countersunk(0, 0, TAB) if SCREW > 0 else None

def label(word, cx, cy, max_w, max_h):
    """Text engraved LABEL_DEPTH into the tab's top, centred at (cx, cy)."""
    id = cad.add_shape("text", hole = True)
    cad.set_text(id, word)
    w, h, _ = cad.node(id)["bounds"]["size"]
    f = min(max_w / w, max_h / h)
    w, h = w * f, h * f
    cad.set_bounds(min = [cx - w / 2, cy - h / 2, TAB - LABEL_DEPTH], max = [cx + w / 2, cy + h / 2, TAB + 1], ids = [id])
    return id

def size_text(d):
    s = str(d)
    return s[:-2] if s.endswith(".0") else s

# --- one clip -------------------------------------------------------------

def clip(d, y):
    """A clip for a cable of diameter d, its tube centred on x = 0 and its
    middle at y; returns its id."""
    r = (d + CLEAR) / 2               # the cable's channel
    od = 2 * (r + WALL)               # the tube outside
    cz = od / 2                       # the tube sits on the bed
    word = size_text(d)
    # the label's ear: long enough for the size, at least as long as the
    # screw's
    text_w = 2.2 * len(word) + 2
    ear_l = max(EAR, text_w + 4)
    x0, x1 = -od / 2 - ear_l, od / 2 + (EAR if SCREW > 0 else 3)
    parts = [
        box([x0, y - WIDTH / 2, 0], [x1, y + WIDTH / 2, TAB]),                 # the tab
        tube(0, y, cz, od, WIDTH),                                             # the tube
        box([-od / 2, y - WIDTH / 2, 0], [od / 2, y + WIDTH / 2, cz]),         # its lower half, squared
        tube(0, y, cz, 2 * r, WIDTH + 2, hole = True),                         # the channel
        box([-d * SNAP / 2, y - WIDTH / 2 - 1, cz], [d * SNAP / 2, y + WIDTH / 2 + 1, od + 1], hole = True),   # the opening
        label(word, -od / 2 - ear_l / 2, y, ear_l - 3, WIDTH * 0.6),
    ]
    if SCREW > 0:
        parts.append(copy_of(SCREW_HOLE, od / 2 + EAR / 2, y))
    c = cad.group("union", ids = parts)
    cad.set_name("clip " + word + " mm", ids = [c])
    return c

# --- the batch ------------------------------------------------------------

# one clip of each size made; any further copies of that size copied
clips = []
y = 0.0
for d in CABLES:
    first = clip(d, y)
    clips.append(first)
    for k in range(1, COPIES):
        clips.append(copy_of(first, dy = -k * (WIDTH + GAP)))
    y -= COPIES * (WIDTH + GAP)
if SCREW_HOLE:
    cad.delete(ids = [SCREW_HOLE])

cad.set_color(COLOR, ids = clips)
cad.select(clips)
cad.set_position(x = 0, y = 0, anchor = "center")
cad.fit_view()
print("%d clips for cables of %s mm" % (len(clips), ", ".join([size_text(d) for d in CABLES])))
