# board-mount.star - a mounting plate with standoffs for a Raspberry Pi,
# Pico, Arduino or your own board, with an outline of the board itself
# sitting on it, so you can see how it lines up inside a case.
#
# List the boards you want in MAKE - by default a Pi 4, a Pi 5 and a Pi
# Zero, side by side - picking from BOARDS (or add your own: its size, its
# mounting holes, and boxes for its tall parts), and run. Each plate gets a
# standoff under each of its board's holes, sized for heat-set inserts or
# for screws to cut their own thread, and holes to fix the plate down:
# plain screw holes, or a VESA 75 or 100 pattern to hang it behind a
# monitor.
#
# THE BOARD OUTLINE. The board - its circuit board and ports, as simple
# boxes - is a hole shape, bundled with the plate (a bundle keeps its
# parts separate, so here it cuts nothing). On its own it shows as a
# see-through ghost of the board, moves with the plate, and is left out of
# exports, so you can line the mount up inside a case by eye. Group the
# bundle with a case to merge them and the outline does useful work: it
# carves room for the board out of the case, and the ports, which stick
# out past the board's edges as real ones do, cut openings through the
# case's walls where the connectors are.
#
# Board sizes are approximate (the tall parts especially): check them
# against your board's mechanical drawing before cutting a case to fit.
#
# Prints flat, with no supports. Basic shapes only. Sizes are in mm.

MAKE = ["pi4", "pi5", "zero"]   # mounts to make, side by side: keys of BOARDS
STANDOFF_H = 6.0         # gap under the board
PLATE = 3.0              # plate thickness
FIXING = "tap"           # "tap": the screw cuts its own thread in a pilot hole
                         # (an M2.5 machine or self-tapping screw for a Pi);
                         # "insert": a hole for a brass heat-set insert
MOUNT = "screws"         # "screws", "vesa75", "vesa100" or "none"
MARGIN = 5.0             # plate beyond the standoffs
SHOW_BOARD = True        # the board outline
GAP = 6.0                # between plates on the build plate

# Each board: its size, thickness and screw; its mounting holes (x, y from
# the board's lower left corner); and its tall parts as boxes (x0, y0, x1,
# y1, bottom, top - heights from the board's top face; below 0 is
# underneath). Ports stick out past the board's edges as they do.
BOARDS = {
    "pi4": {
        "name": "Raspberry Pi 4 B", "size": (85.0, 56.0), "thick": 1.4, "screw": "M2.5",
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 52.5), (61.5, 52.5)],
        "parts": [
            ("USB 2", 69.5, 2.3, 87.3, 15.7, 0, 16.0),
            ("USB 3", 69.5, 20.3, 87.3, 33.7, 0, 16.0),
            ("Ethernet", 65.5, 37.8, 87.3, 53.8, 0, 13.5),
            ("USB-C power", 6.7, -1.0, 15.7, 6.5, 0, 3.3),
            ("micro HDMI 0", 22.5, -1.0, 29.5, 6.5, 0, 3.0),
            ("micro HDMI 1", 36.0, -1.0, 43.0, 6.5, 0, 3.0),
            ("audio", 50.5, -2.0, 56.5, 12.5, 0, 6.0),
            ("GPIO header", 7.1, 47.5, 57.9, 52.5, 0, 8.5),
            ("microSD", -2.5, 22.0, 12.0, 34.0, -1.5, 0),
        ],
    },
    "pi5": {
        "name": "Raspberry Pi 5", "size": (85.0, 56.0), "thick": 1.4, "screw": "M2.5",
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 52.5), (61.5, 52.5)],
        "parts": [
            ("Ethernet", 65.5, 2.2, 87.3, 18.2, 0, 13.5),
            ("USB 3", 69.5, 22.4, 87.3, 35.8, 0, 16.0),
            ("USB 2", 69.5, 40.3, 87.3, 53.7, 0, 16.0),
            ("USB-C power", 6.7, -1.0, 15.7, 6.5, 0, 3.3),
            ("micro HDMI 0", 22.3, -1.0, 29.3, 6.5, 0, 3.0),
            ("micro HDMI 1", 35.7, -1.0, 42.7, 6.5, 0, 3.0),
            ("GPIO header", 7.1, 47.5, 57.9, 52.5, 0, 8.5),
            ("microSD", -2.5, 22.0, 12.0, 34.0, -1.5, 0),
        ],
    },
    "zero": {
        "name": "Raspberry Pi Zero 2 W", "size": (65.0, 30.0), "thick": 1.4, "screw": "M2.5",
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 26.5), (61.5, 26.5)],
        "parts": [
            ("mini HDMI", 6.8, -0.5, 18.0, 7.0, 0, 3.5),
            ("micro USB data", 37.4, -1.0, 45.4, 5.0, 0, 3.0),
            ("micro USB power", 50.0, -1.0, 58.0, 5.0, 0, 3.0),
            ("GPIO header", 7.1, 24.0, 57.9, 29.0, 0, 8.5),
            ("microSD", -1.5, 9.0, 12.0, 20.0, -1.3, 0),
        ],
    },
    "pico": {
        "name": "Raspberry Pi Pico", "size": (51.0, 21.0), "thick": 1.0, "screw": "M2",
        "holes": [(2.0, 4.8), (49.0, 4.8), (2.0, 16.2), (49.0, 16.2)],
        "parts": [
            ("micro USB", -1.3, 6.5, 4.5, 14.5, 0, 2.8),
            ("RP2040", 20.0, 7.0, 27.0, 14.0, 0, 1.0),
        ],
    },
    "uno": {
        "name": "Arduino Uno R3", "size": (68.6, 53.3), "thick": 1.6, "screw": "M3",
        "holes": [(14.0, 2.5), (15.3, 50.7), (66.1, 7.6), (66.1, 35.5)],
        "parts": [
            ("USB-B", -6.4, 32.0, 9.9, 44.0, 0, 11.0),
            ("power jack", -1.8, 3.3, 12.4, 12.3, 0, 11.0),
            ("digital header", 18.0, 49.5, 65.0, 52.0, 0, 8.5),
            ("power and analog header", 27.0, 1.3, 65.0, 3.8, 0, 8.5),
            ("ATmega328", 31.0, 20.0, 64.0, 29.0, 0, 4.0),
        ],
    },
}

# hole sizes for each screw: heat-set insert, and pilot hole for a
# self-tapping screw
SCREWS = {"M2": (3.2, 1.7), "M2.5": (3.6, 2.2), "M3": (4.2, 2.6)}
INSERT_DEPTH = 4.5
FIX_SCREW, FIX_HEAD = 3.5, 7.0       # plate fixing screws: shank and countersunk head
VESA_HOLE = 4.5                      # M4 clearance

PLATE_COLOR = "#30343b"
BOARD_COLOR = "#2e8b57"
SIDES = 48

# --- helpers --------------------------------------------------------------

def place(kind, lo, hi, hole = False, sides = 0):
    id = cad.add_shape(kind, hole = hole)
    if sides:
        cad.set_param(id, "sides", sides)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def cyl(x, y, d, z0, z1, hole = False):
    return place("cylinder", [x - d / 2, y - d / 2, z0], [x + d / 2, y + d / 2, z1], hole, SIDES)

def copy_of(id, dx, dy):
    """A copy of shape id, moved by (dx, dy)."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, ids = [new])
    return new

def copies(id, at, spots):
    """id (which stands at spot at) and copies of it at the other spots."""
    out = [id]
    for x, y in spots:
        if (x, y) != at:
            out.append(copy_of(id, x - at[0], y - at[1]))
    return out

def countersunk(x, y, top):
    """A screw hole down through the plate, countersunk at the top."""
    shank = cyl(x, y, FIX_SCREW, -1, top + 1, hole = True)
    sink_h = (FIX_HEAD - FIX_SCREW) / 2
    cone = place("cone", [x - FIX_HEAD / 2, y - FIX_HEAD / 2, top - sink_h], [x + FIX_HEAD / 2, y + FIX_HEAD / 2, top + 0.01], True, SIDES)
    cad.set_param(cone, "top", FIX_SCREW / 2)
    cad.mirror("z", ids = [cone])
    g = cad.group("union", ids = [shank, cone])
    cad.set_hole(True, ids = [g])
    return g

# --- one board's mount ------------------------------------------------------

def make_mount(key):
    """The mount for board key, with its outline if SHOW_BOARD; returns the
    top-level shape, the plate's footprint [x0, y0, x1, y1] (what gets
    printed) and a line describing it."""
    b = BOARDS[key]
    insert_d, tap_d = SCREWS[b["screw"]]
    standoff_hole = insert_d if FIXING == "insert" else tap_d
    standoff_d = insert_d + 2.6
    bw, bh = b["size"]
    board_z = PLATE + STANDOFF_H               # the board's underside

    # the board centred on the plate, which reaches past the standoffs (and
    # the VESA pattern, if any). Centred on the board, not on its holes: a
    # Pi 4 or 5 has its holes bunched at one end, away from the USB and
    # Ethernet ports, so the plate reaches as far past the port end
    xs = [x for x, _ in b["holes"]]
    ys = [y for _, y in b["holes"]]
    cx, cy = bw / 2, bh / 2                    # the board's middle
    half_w = max([abs(x - cx) for x in xs]) + standoff_d / 2 + MARGIN
    half_h = max([abs(y - cy) for y in ys]) + standoff_d / 2 + MARGIN
    vesa = {"vesa75": 75.0, "vesa100": 100.0}.get(MOUNT, 0)
    if vesa:
        half_w = max(half_w, vesa / 2 + VESA_HOLE / 2 + MARGIN)
        half_h = max(half_h, vesa / 2 + VESA_HOLE / 2 + MARGIN)
    elif MOUNT == "screws":
        # room at the ends for the fixing screws, beyond the standoffs and
        # past the board's ends, so a screwdriver reaches them with the
        # board in place
        half_w = max(half_w + FIX_HEAD + 2, bw / 2 + 1 + FIX_HEAD + MARGIN)
    plate = place("box", [cx - half_w, cy - half_h, 0], [cx + half_w, cy + half_h, PLATE])
    cad.set_param(plate, "radius", 1.5)
    parts = [plate]

    # a standoff: a post with a hole down its middle - made once, copied to
    # every other mounting hole
    x0, y0 = b["holes"][0]
    post = cyl(x0, y0, standoff_d, PLATE - 0.5, board_z)
    bore = cyl(x0, y0, standoff_hole, board_z - (INSERT_DEPTH if FIXING == "insert" else STANDOFF_H + PLATE - 1), board_z + 1, hole = True)
    standoff = cad.group("union", ids = [post, bore])
    parts += copies(standoff, (x0, y0), b["holes"])

    # fixing holes: through the plate, clear of the board's footprint
    if vesa:
        h = vesa / 2
        first = cyl(cx - h, cy - h, VESA_HOLE, -1, PLATE + 1, hole = True)
        parts += copies(first, (cx - h, cy - h), [(cx - h, cy - h), (cx + h, cy - h), (cx - h, cy + h), (cx + h, cy + h)])
    elif MOUNT == "screws":
        ex = half_w - MARGIN - FIX_HEAD / 2
        first = countersunk(cx - ex, cy - half_h / 2, PLATE)
        parts += copies(first, (cx - ex, cy - half_h / 2),
                        [(cx - ex, cy - half_h / 2), (cx - ex, cy + half_h / 2), (cx + ex, cy - half_h / 2), (cx + ex, cy + half_h / 2)])

    mount = cad.group("union", ids = parts)
    cad.set_color(PLATE_COLOR, ids = [mount])
    cad.set_name(b["name"] + " mount", ids = [mount])

    # --- the board outline -----------------------------------------------------

    result = mount
    if SHOW_BOARD:
        top = board_z + b["thick"]
        shapes = [place("box", [0, 0, board_z], [bw, bh, top])]
        for name, px0, py0, px1, py1, pz0, pz1 in b["parts"]:
            shapes.append(place("box", [px0, py0, top + pz0], [px1, py1, top + pz1 if pz1 > 0 else top]))
        board = cad.group("union", ids = shapes)
        cad.set_hole(True, ids = [board])
        cad.set_name(b["name"] + " outline (cuts room for the board)", ids = [board])
        result = cad.group("bundle", ids = [mount, board])
        cad.set_name(b["name"] + " mount, with board outline", ids = [result])
    desc = "%s: plate %s x %s mm, %d standoffs %s mm tall for %s %s" % (
        b["name"], str(math.round(2 * half_w)), str(math.round(2 * half_h)), len(b["holes"]), str(STANDOFF_H),
        b["screw"], "heat-set inserts" if FIXING == "insert" else "self-tapping screws")
    plate_rect = [cx - half_w, cy - half_h, cx + half_w, cy + half_h]
    return result, plate_rect, desc

# --- all of them, packed onto the build plate --------------------------------
# Only the plates print (the board outlines are holes), so the packing goes
# by the plates' footprints; the outlines may hang over a neighbour or the
# edge, which does no harm. Shelf packing: the deepest plates first, each
# placed at the end of the current shelf if it fits across the plate, or
# else starting a new shelf behind it. A plate that fits across turned 90
# degrees but not straight is turned.

settings = cad.get_document()["settings"]
BED_W, BED_D = settings["workplaneW"], settings["workplaneL"]

items = []
for key in MAKE:
    top, rect, desc = make_mount(key)
    items.append({"top": top, "rect": rect, "w": rect[2] - rect[0], "d": rect[3] - rect[1]})
    print(desc)

def place_shelves(items, width):
    """Positions (x, y of each plate's lower left corner), the shelves'
    total depth and widest row, packing the items in order."""
    spots, x, y, shelf_d, widest = [], 0.0, 0.0, 0.0, 0.0
    for it in items:
        if x > 0 and x + it["w"] > width:
            y += shelf_d + GAP
            x, shelf_d = 0.0, 0.0
        spots.append((x, y))
        x += it["w"] + GAP
        shelf_d = max(shelf_d, it["d"])
        widest = max(widest, x - GAP)
    return spots, y + shelf_d, widest

for it in items:
    if it["w"] > BED_W and it["d"] <= BED_W:          # only fits turned
        # rotate turns about the shape's middle; turn the footprint with it
        c = cad.node(it["top"])["bounds"]["center"]
        x0, y0, x1, y1 = it["rect"]
        cad.rotate("z", 90, ids = [it["top"]])
        it["rect"] = [c[0] - (y1 - c[1]), c[1] + (x0 - c[0]), c[0] - (y0 - c[1]), c[1] + (x1 - c[0])]
        it["w"], it["d"] = it["d"], it["w"]
items = sorted(items, key = lambda it: -it["d"])
spots, depth, widest = place_shelves(items, BED_W)

# the first plate of a row that has others beside it is turned round, so
# its ports (on the boards' +X ends) face out, away from its neighbour
for i, (it, (x, y)) in enumerate(zip(items, spots)):
    shared = len([1 for _, yy in spots if yy == y]) > 1
    if x == 0 and shared:
        c = cad.node(it["top"])["bounds"]["center"]
        x0, y0, x1, y1 = it["rect"]
        cad.rotate("z", 180, ids = [it["top"]])
        it["rect"] = [2 * c[0] - x1, 2 * c[1] - y1, 2 * c[0] - x0, 2 * c[1] - y0]

# move each so its plate's lower left corner lands on its spot, the whole
# layout centred on the bed
ox, oy = -widest / 2, -depth / 2
for it, (x, y) in zip(items, spots):
    cad.move(dx = ox + x - it["rect"][0], dy = oy + y - it["rect"][1], ids = [it["top"]])

made = [it["top"] for it in items]
cad.select(made)
cad.drop()
cad.clear_selection()
cad.fit_view()
fits = widest <= BED_W and depth <= BED_D
print("plates packed into %s x %s mm on the %s x %s mm build plate%s" % (
    str(math.round(widest)), str(math.round(depth)), str(BED_W), str(BED_D),
    "" if fits else " - too big: print them in more than one go"))
