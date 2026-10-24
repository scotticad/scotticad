# organizer.star - a desk organizer for pens, markers, brushes and
# screwdrivers: a block with rows of holes, a different size for each row,
# and a shallow tray at the front for erasers and paper clips.
#
# Made from basic shapes only - rounded boxes, and cylinders set as holes -
# with a loop placing the holes: the dozens of holes would take dozens of
# clicks by hand. Each row works out how many holes fit across.
#
# The rows step up toward the back, so taller tools behind don't hide the
# ones in front. Holes stop short of the bottom so nothing falls through.
# It prints as it stands, with no supports.
#
# Change the numbers below and run again. Sizes are in mm.

WIDTH = 170.0          # left to right
ROWS = [               # front to back: (what for, hole diameter)
    ("pencils", 9.0),
    ("pens", 12.0),
    ("markers", 18.0),
    ("brushes and screwdrivers", 14.0),
]
GAP = 4.0              # least material between holes
MARGIN = 6.0           # least material at the sides and between rows
FRONT_H = 45.0         # height of the first row of holes
STEP = 10.0            # each row stands this much taller than the one before
BOTTOM = 2.0           # holes stop this far above the bottom
RADIUS = 2.5           # rounding of the block's edges

TRAY_DEPTH = 32.0      # front to back
TRAY_H = 16.0          # height of the tray section
TRAY_FLOOR = 4.0       # tray floor thickness
TRAY_ROUND = 6.0       # rounding inside the tray

COLOR = "#3d7a5a"
SIDES = 48             # facets on the holes

# --- helpers --------------------------------------------------------------

def box(lo, hi, radius = 0.0, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    if radius > 0:
        cad.set_param(id, "radius", radius)
    return id

def drill(x, y, d, z0, z1):
    id = cad.add_shape("cylinder", hole = True)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [x - d / 2, y - d / 2, z0], max = [x + d / 2, y + d / 2, z1], ids = [id])
    return id

def copy_of(id, dx, dy = 0.0, dz = 0.0):
    """A copy of shape id, moved by (dx, dy, dz)."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, dz = dz, ids = [new])
    return new

# --- layout ---------------------------------------------------------------

# each row is as deep as its holes, plus a full margin of wall in front of
# them and half a margin behind (before the next, taller step; the last
# row gets a full margin at the back); the tray is in front
depths = [d + 1.5 * MARGIN for _, d in ROWS]
total = TRAY_DEPTH + MARGIN / 2
for dp in depths:            # (Starlark has no sum())
    total += dp
x0, x1 = -WIDTH / 2, WIDTH / 2
front = -total / 2
back = total / 2

parts = []

# the block: one rounded box per step, each running from its own front to
# the back, so the sides stay flat and only the steps show
y = front
parts.append(box([x0, y, 0], [x1, back, TRAY_H], RADIUS))
y += TRAY_DEPTH
row_y = []
for i, (_, d) in enumerate(ROWS):
    h = FRONT_H + STEP * i
    parts.append(box([x0, y, 0], [x1, back, h], RADIUS))
    # a full margin of wall in front of the holes, so the rounded top edge
    # of the step doesn't eat into them
    row_y.append((y + MARGIN + d / 2, h))
    y += depths[i]

# the tray: a rounded pocket in the front section
parts.append(box([x0 + MARGIN, front + MARGIN, TRAY_FLOOR],
                 [x1 - MARGIN, front + TRAY_DEPTH - MARGIN / 2, TRAY_H + 10], TRAY_ROUND, hole = True))

# the holes: for each row, as many as fit across, spread evenly - the
# first one made, the rest copies of it
counts = []
for (name, d), (cy, h) in zip(ROWS, row_y):
    span = WIDTH - 2 * MARGIN - d             # between the outer holes' centres
    n = max(1, int(span / (d + GAP)) + 1)
    pitch = span / (n - 1) if n > 1 else 0
    first = drill(x0 + MARGIN + d / 2 if n > 1 else 0.0, cy, d, BOTTOM, h + 1)
    parts.append(first)
    for k in range(1, n):
        parts.append(copy_of(first, k * pitch))
    counts.append("%d %s (%s mm)" % (n, name, str(d)))

org = cad.group("union", ids = parts)
cad.set_color(COLOR, ids = [org])
cad.set_name("organizer", ids = [org])
cad.drop(ids = [org])
cad.fit_view()
size = cad.node(org)["bounds"]["size"]
print("organizer %s x %s x %s mm: %s" % (str(math.round(size[0])), str(math.round(size[1])), str(math.round(size[2])), ", ".join(counts)))
