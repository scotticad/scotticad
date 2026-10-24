# coasters.star - a set of coasters and a holder for them, made from basic
# shapes: cylinders (or hexagons), boxes, text, holes and groups.
#
# Each coaster is a disc with a shallow recess on top; a word stands up in
# the recess, its top flush with the rim. Print it in one colour, then swap
# filament at the height this script prints (the recess floor), and the
# lettering and rim come out in the second colour.
#
# The holder is a box with the coaster's shape cut out of it, a little
# larger, so the stack drops in; a slot in the front and a hole in the
# floor let you push the coasters out. Everything prints flat with no
# supports.
#
# Change the numbers below and run again. Sizes are in mm.

DIAMETER = 95.0         # across the coaster (corner to corner for a hexagon)
THICK = 5.0             # coaster thickness
SHAPE = "round"         # "round" or "hex"
RECESS = 1.2            # depth of the recess on top
RIM = 4.0               # width of the rim round the recess
NAMES = ["Tea", "Coffee", "Cocoa", "Juice"]    # one coaster per name
FONT = None             # None for the default; cad.fonts() lists the others
TEXT_W = 0.6            # lettering at most this fraction of the diameter wide
TEXT_H = 0.28           # ... and this fraction tall

CLEAR = 0.8             # gap round the coasters in the holder
WALL = 3.0              # holder walls
BASE = 3.0              # holder floor
SLOT = 0.4              # front slot width, as a fraction of the diameter
PUSH_HOLE = 0.4         # floor hole, as a fraction of the diameter

COASTER_COLOR = "#2f6f8f"
TEXT_COLOR = "#f2e6c9"
HOLDER_COLOR = "#8a5a3c"
GAP = 8.0               # between the parts on the build plate

# --- helpers --------------------------------------------------------------

HEX = math.sqrt(3) / 2  # a hexagon's width across flats, per corner-to-corner

def disc(cx, cy, d, z0, z1, hole = False):
    """The coaster's shape (a cylinder, or a hexagon d corner to corner),
    centred at (cx, cy) from z0 to z1."""
    if SHAPE == "hex":
        id = cad.add_shape("polygon", hole = hole)
        cad.set_param(id, "sides", 6)
        w, h = d, d * HEX
    else:
        id = cad.add_shape("cylinder", hole = hole)
        cad.set_param(id, "sides", 64)
        w, h = d, d
    cad.set_bounds(min = [cx - w / 2, cy - h / 2, z0], max = [cx + w / 2, cy + h / 2, z1], ids = [id])
    return id

def box(lo, hi, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def label(word, cx, cy, z0, z1, max_w, max_h):
    """Text centred at (cx, cy), as large as fits max_w x max_h, from z0
    to z1."""
    id = cad.add_shape("text")
    cad.set_text(id, word, font = FONT)
    w, h, _ = cad.node(id)["bounds"]["size"]
    f = min(max_w / w, max_h / h)
    w, h = w * f, h * f
    cad.set_bounds(min = [cx - w / 2, cy - h / 2, z0], max = [cx + w / 2, cy + h / 2, z1], ids = [id])
    return id

# --- coasters -------------------------------------------------------------

depth = DIAMETER * HEX if SHAPE == "hex" else DIAMETER     # front to back
inner = DIAMETER - 2 * RIM / (HEX if SHAPE == "hex" else 1)  # recess, corner to corner
floor = THICK - RECESS                                       # the recess floor

# the plate (the disc with its recess) is the same for every coaster: make
# one, and copy it for the rest; only the words differ
first_plate = cad.group("union", ids = [disc(0, 0, DIAMETER, 0, THICK),
                                        disc(0, 0, inner, floor, THICK + 1, hole = True)])
cad.set_color(COASTER_COLOR, ids = [first_plate])

def spot(i):
    """Where coaster i goes: two to a row."""
    return (i % 2) * (DIAMETER + GAP), -(i // 2) * (depth + GAP)

# all the copies first (once the first plate is grouped with its words it
# can't be copied on its own)
plates = [first_plate]
for i in range(1, len(NAMES)):
    cx, cy = spot(i)
    cad.select([first_plate])
    plate = cad.duplicate()[0]
    cad.move(dx = cx, dy = cy, ids = [plate])
    plates.append(plate)

coasters = []
for i, word in enumerate(NAMES):
    cx, cy = spot(i)
    plate = plates[i]
    text = label(word, cx, cy, floor - 0.2, THICK, inner * TEXT_W / 0.9, inner * TEXT_H)
    cad.set_color(TEXT_COLOR, ids = [text])
    c = cad.group("union", ids = [plate, text])
    cad.set_multicolor(True, ids = [c])
    cad.set_name("coaster: " + word, ids = [c])
    coasters.append(c)

# --- holder ---------------------------------------------------------------

n = len(NAMES)
cav = DIAMETER + 2 * CLEAR / (HEX if SHAPE == "hex" else 1)   # cavity, corner to corner
cav_depth = depth + 2 * CLEAR
HW, HD = cav + 2 * WALL, cav_depth + 2 * WALL
HH = BASE + n * THICK - THICK / 2          # the top coaster stands proud, to grab
hx = 2 * (DIAMETER + GAP) + GAP + HW / 2   # beside the coasters
hy = 0.0
holder = cad.group("union", ids = [
    box([hx - HW / 2, hy - HD / 2, 0], [hx + HW / 2, hy + HD / 2, HH]),
    disc(hx, hy, cav, BASE, HH + 1, hole = True),
    # a slot through the front wall, down to the floor
    box([hx - DIAMETER * SLOT / 2, hy - HD / 2 - 1, BASE], [hx + DIAMETER * SLOT / 2, hy, HH + 1], hole = True),
    # a hole in the floor to push the stack up
    disc(hx, hy, DIAMETER * PUSH_HOLE, -1, BASE + 1, hole = True),
])
cad.set_color(HOLDER_COLOR, ids = [holder])
cad.set_name("holder", ids = [holder])

# --- finish ---------------------------------------------------------------

cad.select(coasters + [holder])
cad.set_position(x = 0, y = 0, anchor = "center")
cad.drop()
cad.fit_view()
def mm(x):
    return str(math.round(x * 100) / 100)

print("%d coasters, %s x %s x %s mm each; holder %s x %s x %s mm" % (
    n, mm(DIAMETER), mm(depth), mm(THICK), mm(HW), mm(HD), mm(HH)))
print("for two colours: swap filament at z = %s mm (the recess floor)" % mm(floor))
