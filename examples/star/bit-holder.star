# bit-holder.star - a block that holds 1/4" hex screwdriver bits upright,
# each pocket labelled with its bit.
#
# The layout comes from a list: each row is a list of labels, one pocket
# each. The script works out the block's size from the list, engraves each
# label in front of its pocket, and makes the pocket once, copying it to
# every other spot.
#
# Across flats and across corners: a hexagon has two widths. 1/4" bits are
# 6.35 mm across the FLATS (what a spanner grips); across the CORNERS they
# are 2 / sqrt(3) = 1.155 times that, 7.33 mm. BIT is across flats, and the
# pocket is made a little bigger (FIT) so the bit drops in without forcing:
# 6.35 mm bits want about 6.6 mm. The polygon shape's width (X) is across
# corners and its depth (Y) across flats, so the script sets those from
# BIT.
#
# Prints as it stands, with no supports. Basic shapes only.
# Sizes are in mm.

ROWS = [                                   # front row first
    ["SL3", "SL4", "SL5", "SL6", "SL8"],
    ["H2", "H2.5", "H3", "H4", "H5", "H6"],
    ["T10", "T15", "T20", "T25", "T27", "T30"],
    ["PH0", "PH1", "PH2", "PH3", "PZ1", "PZ2", "PZ3"],
]

BIT = 6.35             # across flats (1/4")
FIT = 0.25             # added across flats, so bits drop in
DEPTH_IN = 12.0        # pocket depth (about half a 25 mm bit)
HEIGHT = 18.0          # block height
CHAMFER = 0.6          # a lead-in round the top of each pocket
POKE = 2.5             # a hole through the bottom to push stuck bits out (0: none)
SPACING = 4.0          # least material between pockets
LABEL_H = 3.2          # label height
LABEL_DEPTH = 0.6      # engraving depth
MARGIN = 5.0           # round the edge
RADIUS = 2.0           # rounding of the block's edges
COLOR = "#d0612a"

# --- sizes ----------------------------------------------------------------

FLATS = BIT + FIT                          # the pocket, across flats (Y)
CORNERS = FLATS * 2 / math.sqrt(3)         # ... and across corners (X)

cell_w = max(CORNERS + SPACING, 12.0)      # left to right
cell_d = CORNERS + SPACING + LABEL_H + 1.5 # front to back, with the label
cols = 0
for row in ROWS:
    cols = max(cols, len(row))
WIDTH = cols * cell_w + 2 * MARGIN - SPACING
DEPTH = len(ROWS) * cell_d + 2 * MARGIN - SPACING

# --- helpers --------------------------------------------------------------

def place(kind, lo, hi, hole = False, sides = 0):
    id = cad.add_shape(kind, hole = hole)
    if sides:
        cad.set_param(id, "sides", sides)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def pocket(x, y):
    """A pocket centred at (x, y): the hexagon, a chamfer round its mouth,
    and a poke hole through the bottom, as one hole group."""
    w, d = CORNERS, FLATS
    ids = [place("polygon", [x - w / 2, y - d / 2, HEIGHT - DEPTH_IN], [x + w / 2, y + d / 2, HEIGHT + 1], True, 6)]
    if CHAMFER > 0:
        # an upside-down cone: wider than the pocket at the top, narrowing to
        # its flats CHAMFER down
        c = w + 2 * CHAMFER
        cone = place("cone", [x - c / 2, y - c / 2, HEIGHT - CHAMFER], [x + c / 2, y + c / 2, HEIGHT + 0.01], True, 48)
        cad.set_param(cone, "top", d / 2)
        cad.mirror("z", ids = [cone])
        ids.append(cone)
    if POKE > 0:
        ids.append(place("cylinder", [x - POKE / 2, y - POKE / 2, -1], [x + POKE / 2, y + POKE / 2, HEIGHT], True, 24))
    g = cad.group("union", ids = ids)
    cad.set_hole(True, ids = [g])
    return g

def copy_of(id, dx, dy):
    """A copy of shape id, moved by (dx, dy)."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, ids = [new])
    return new

def label(word, x, y):
    """Engraved text centred at (x, y), at most a cell wide."""
    id = cad.add_shape("text", hole = True)
    cad.set_text(id, word)
    w, h, _ = cad.node(id)["bounds"]["size"]
    f = min(LABEL_H / h, (cell_w - 1.5) / w)
    w, h = w * f, h * f
    cad.set_bounds(min = [x - w / 2, y - h / 2, HEIGHT - LABEL_DEPTH], max = [x + w / 2, y + h / 2, HEIGHT + 1], ids = [id])
    return id

# --- the block ------------------------------------------------------------

block = place("box", [-WIDTH / 2, -DEPTH / 2, 0], [WIDTH / 2, DEPTH / 2, HEIGHT])
cad.set_param(block, "radius", RADIUS)
parts = [block]
first = None            # the pocket the others are copies of, and where it is
count = 0
for r, row in enumerate(ROWS):
    # the row's cells, centred left to right; the label in front of the pocket
    y_label = -DEPTH / 2 + MARGIN + r * cell_d + LABEL_H / 2
    y_pocket = y_label + LABEL_H / 2 + 1.5 + CORNERS / 2
    x_first = -(len(row) - 1) * cell_w / 2
    for c, name in enumerate(row):
        x = x_first + c * cell_w
        if first == None:
            first = (pocket(x, y_pocket), x, y_pocket)
            parts.append(first[0])
        else:
            parts.append(copy_of(first[0], x - first[1], y_pocket - first[2]))
        parts.append(label(name, x, y_label))
        count += 1

holder = cad.group("union", ids = parts)
cad.set_color(COLOR, ids = [holder])
cad.set_name("bit holder", ids = [holder])
cad.fit_view()
print("%d pockets; block %s x %s x %s mm" % (count, str(math.round(WIDTH)), str(math.round(DEPTH)), str(HEIGHT)))
