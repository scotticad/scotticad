# spool-holder.star - a freestanding filament spool holder that sits on the
# table beside any printer.
#
# Two A-frame uprights stand either side of the spool, joined at their feet
# by two braces whose tabs fit slots in the uprights; the braces' shoulders
# set the spacing, so tightening the axle's nut squares the frame up. The
# axle runs through both uprights - supported at both ends - and the spool
# rides on two cone-shaped hubs, one in each side of its centre hole, each
# turning on a 608 skateboard bearing (22 mm outside, 8 mm bore, 7 mm
# thick, pressed into a pocket in the hub). The cones fit any centre hole
# from 30 to 56 mm.
#
# The axle and nut are printed too, using the ISO Thread shape: the axle is
# a hex head, a smooth shank for the bearings and an M8 thread at the end.
# The library's hex nut has a plain hole, so the nut here is a hex block
# with a thread cut out of it as a hole - the same thread made a little
# bigger (NUT_CLEAR) so it turns on the axle's. A steel M8 bolt and nut of
# the length printed here fit the same way.
#
# Assembly, along the axle: head | upright | spacer | hub (cone in) |
# spool | hub (cone in) | spacer | upright | nut.
#
# Every part prints flat with no supports. They are laid out on a 200 mm
# build plate - the uprights nested head to foot, the small parts beside
# them - and whatever doesn't fit is put just past the plate's edge, to
# print in a second go. Sizes are in mm.

SPOOL_D = 200.0        # largest spool diameter (sets the axle's height)
SPOOL_W = 70.0         # spool width, between its outer faces
CLEAR = 10.0           # spool above the table

UPRIGHT_T = 8.0        # upright thickness
BASE_W = 120.0         # width of an upright's foot
TOP_W = 26.0           # ... and of its rounded top
BRACE_H = 15.0         # brace height (it stands on the table)
BRACE_T = 8.0          # ... and thickness
TAB_H = 9.0            # the tabs at its ends, through the uprights' slots
FIT = 0.3              # clearance in the slots

BEARING_D = 22.1       # 608 outside diameter, for a press fit
BEARING_T = 7.0
AXLE_D = 8.0           # 608 bore
HUB_TIP = 30.0         # cone diameter at the tip...
HUB_BASE = 58.0        # ... and at the flange end
HUB_LEN = 16.0         # cone length
FLANGE = 3.0           # flange thickness (it covers the spool's face)
FLANGE_D = 64.0

THREAD_PITCH = 1.25    # M8 coarse
NUT_AF = 13.0          # nut across flats
NUT_H = 7.0
NUT_CLEAR = 0.5        # added to the nut's thread so it turns on the axle's
HEAD_AF = 13.0
HEAD_H = 5.5
SPACER_T = 2.0         # spacers between the hubs and the uprights
SPACER_D = 14.0

BED = 200.0            # build plate size, for the layout
GAP = 5.0              # between parts on the plate

FRAME_COLOR = "#3a6ea5"
HUB_COLOR = "#e0a030"
AXLE_COLOR = "#8a8f96"
SIDES = 64

# --- sizes ----------------------------------------------------------------

AXLE_Y = CLEAR + SPOOL_D / 2                              # axle above the table
INNER = SPOOL_W + 2 * FLANGE + 2 * SPACER_T + 2           # between the uprights
BRACE_X = BASE_W / 2 - 20                                 # braces' distance from the middle
SHANK = 2 * UPRIGHT_T + INNER - 1                         # smooth part of the axle
THREAD_LEN = NUT_H + 4

# --- helpers --------------------------------------------------------------

def place(kind, lo, hi, hole = False, sides = 0):
    id = cad.add_shape(kind, hole = hole)
    if sides:
        cad.set_param(id, "sides", sides)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def cyl(x, y, d, z0, z1, hole = False, sides = SIDES):
    return place("cylinder", [x - d / 2, y - d / 2, z0], [x + d / 2, y + d / 2, z1], hole, sides)

def cone(x, y, d0, d1, z0, z1):
    """A cone frustum from diameter d0 at z0 to d1 at z1 (either way up)."""
    big = max(d0, d1)
    id = place("cone", [x - big / 2, y - big / 2, z0], [x + big / 2, y + big / 2, z1], False, SIDES)
    cad.set_param(id, "top", min(d0, d1) / 2)
    if d1 > d0:                      # a cone narrows upward: turn it over
        cad.mirror("z", ids = [id])
    return id

def hexagon(x, y, af, z0, z1):
    """A hexagonal prism af across flats."""
    w = af * 2 / math.sqrt(3)
    return place("polygon", [x - w / 2, y - af / 2, z0], [x + w / 2, y + af / 2, z1], False, 6)

def thread(x, y, d, z0, z1, hole = False):
    id = place("thread", [x - d / 2, y - d / 2, z0], [x + d / 2, y + d / 2, z1], hole)
    cad.set_param(id, "pitch", THREAD_PITCH)
    return id

def named(id, name, color):
    cad.set_name(name, ids = [id])
    cad.set_color(color, ids = [id])
    return id

def copy_of(id, dx, dy):
    cad.select([id])
    new = cad.duplicate()[0]
    cad.move(dx = dx, dy = dy, ids = [new])
    return new

# --- uprights ---------------------------------------------------------------
# lying on the bed as they print: X across the foot, Y up the upright (its
# foot at y = 0, standing on the table), Z its thickness

def half_w(y):
    """The upright's half-width at height y."""
    return BASE_W / 2 - (BASE_W - TOP_W) / 2 * y / AXLE_Y

def upright():
    wy0 = 20.0                       # a window, to save plastic
    wy1 = AXLE_Y - 32
    wx = half_w(wy0) - 14
    body = cad.add_outline([[(-BASE_W / 2, 0), (BASE_W / 2, 0), (TOP_W / 2, AXLE_Y), (-TOP_W / 2, AXLE_Y)],
                            [(-wx, wy0), (0, wy1), (wx, wy0)]], z0 = 0, z1 = UPRIGHT_T)
    parts = [body, cyl(0, AXLE_Y, TOP_W, 0, UPRIGHT_T),
             cyl(0, AXLE_Y, AXLE_D + 0.4, -1, UPRIGHT_T + 1, hole = True)]
    # the braces' slots, near the foot
    slot = place("box", [-BRACE_X - (BRACE_T + FIT) / 2, (BRACE_H - TAB_H - FIT) / 2, -1],
                 [-BRACE_X + (BRACE_T + FIT) / 2, (BRACE_H + TAB_H + FIT) / 2, UPRIGHT_T + 1], True)
    parts += [slot, copy_of(slot, 2 * BRACE_X, 0)]
    return named(cad.group("union", ids = parts), "upright", FRAME_COLOR)

# --- the parts ----------------------------------------------------------------

up_a = upright()
up_b = copy_of(up_a, 0, 0)

def brace():
    """Lying flat: X its length, Y its height in use, Z its thickness."""
    tab = UPRIGHT_T + 2                  # through an upright, 2 mm proud
    y0 = (BRACE_H - TAB_H) / 2
    return named(cad.group("union", ids = [
        place("box", [0, 0, 0], [INNER, BRACE_H, BRACE_T]),
        place("box", [-tab, y0, 0], [0.01, y0 + TAB_H, BRACE_T]),
        place("box", [INNER - 0.01, y0, 0], [INNER + tab, y0 + TAB_H, BRACE_T]),
    ]), "brace", FRAME_COLOR)

brace_a = brace()
brace_b = copy_of(brace_a, 0, 0)

def hub():
    """Flange down; the bearing's pocket opens on the bed face (its ceiling
    is a short bridge), and the hole beyond it clears the axle so only the
    bearing touches it."""
    return named(cad.group("union", ids = [
        cyl(0, 0, FLANGE_D, 0, FLANGE),
        cone(0, 0, HUB_BASE, HUB_TIP, FLANGE - 0.01, FLANGE + HUB_LEN),
        cyl(0, 0, BEARING_D, -1, BEARING_T, hole = True),
        cyl(0, 0, AXLE_D + 2, -1, FLANGE + HUB_LEN + 1, hole = True),
    ]), "hub", HUB_COLOR)

hub_a = hub()
hub_b = copy_of(hub_a, 0, 0)

axle = named(cad.group("union", ids = [                  # standing on its head
    hexagon(0, 0, HEAD_AF, 0, HEAD_H),
    cyl(0, 0, AXLE_D - 0.1, HEAD_H - 0.01, HEAD_H + SHANK),
    thread(0, 0, AXLE_D, HEAD_H + SHANK - 0.01, HEAD_H + SHANK + THREAD_LEN),
]), "axle", AXLE_COLOR)

nut = named(cad.group("union", ids = [
    hexagon(0, 0, NUT_AF, 0, NUT_H),
    thread(0, 0, AXLE_D + NUT_CLEAR, -1, NUT_H + 1, hole = True),
]), "nut", AXLE_COLOR)

spacer_a = named(cad.group("union", ids = [cyl(0, 0, SPACER_D, 0, SPACER_T),
                                           cyl(0, 0, AXLE_D + 0.4, -1, SPACER_T + 1, hole = True)]), "spacer", AXLE_COLOR)
spacer_b = copy_of(spacer_a, 0, 0)

# --- layout -------------------------------------------------------------------
# The uprights nest head to foot: the second turned round, slid beside the
# first so their sloping edges run parallel GAP apart. Then the other parts
# go in rows in the space left on the plate, largest first; what doesn't
# fit goes in a row past the plate's right edge.

def size(id):
    return cad.node(id)["bounds"]["size"]

def put(id, x0, y0):
    """Move id so its bounds' lower left corner is at (x0, y0)."""
    lo = cad.node(id)["bounds"]["min"]
    cad.move(dx = x0 - lo[0], dy = y0 - lo[1], ids = [id])

# The first's right edge runs from its foot's corner (BASE_W / 2 out) in
# by `slope` per mm up; the turned one's left edge does the same from the
# top of its round end (TOP_W / 2 out, TOP_W / 2 down from its top). Their
# middles are `offset` apart for the edges to be GAP apart.
slope = (BASE_W - TOP_W) / 2 / AXLE_Y                      # sideways per mm up
offset = BASE_W / 2 + TOP_W / 2 - slope * TOP_W / 2 + GAP * math.sqrt(1 + slope * slope)
cad.rotate("z", 180, ids = [up_b])
up_h = size(up_a)[1]
left = -(BASE_W + offset) / 2                              # the pair, centred across
put(up_a, left, -BED / 2)
put(up_b, left + offset, -BED / 2)

# rows above the uprights; the plate ends at y = BED / 2
rest = [hub_a, hub_b, axle, brace_a, brace_b, nut, spacer_a, spacer_b]
x, y, row_h = -BED / 2, -BED / 2 + up_h + GAP, 0.0
on_plate, off_plate = [], []
over_x, over_y = BED / 2 + 20, -BED / 2
for id in rest:
    w, h, _ = size(id)
    if x + w <= BED / 2 and y + h <= BED / 2:              # in this row
        put(id, x, y)
    elif -BED / 2 + w <= BED / 2 and y + row_h + GAP + h <= BED / 2:   # in a new row
        x, y, row_h = -BED / 2, y + row_h + GAP, 0.0
        put(id, x, y)
    else:                                       # past the plate's edge (and on
        put(id, over_x, over_y)                 # to the next part: a smaller
        over_y += h + GAP                       # one may still fit)
        off_plate.append(cad.node(id)["label"])
        continue
    x += w + GAP
    row_h = max(row_h, h)
    on_plate.append(cad.node(id)["label"])

cad.clear_selection()
cad.fit_view()
print("stand %s mm tall, axle %s mm above the table, uprights %s mm apart; axle %s mm long" % (
    str(math.round(AXLE_Y + TOP_W / 2)), str(AXLE_Y), str(INNER), str(math.round(HEAD_H + SHANK + THREAD_LEN))))
print("on the plate: uprights, " + ", ".join(on_plate))
print("for a second print: " + (", ".join(off_plate) if off_plate else "nothing"))
