# vault-case.star - a Raspberry Pi case themed after Fallout's Vault-Tec,
# in blue and yellow: the Vault-Tec cog badge set into the lid, with vents
# between its teeth; a riveted nameplate on the back with the lettering
# and the winged emblem, and a latch either side, as on a Vault-Tec
# toolbox; and ribbed corners. One script for three boards: set BOARD.
# The Zero's case is too small for lettering that prints (see MIN_TEXT):
# its lid has a bigger emblem instead, and its nameplate is plain.
#
# HOW IT IS MADE. Two printed parts and four screws. The BASE is a plate
# with a low rim and four standoffs. The SHELL is everything above the
# board: its ports are notches in the bottom edge of its walls (and dips
# in the base's rim), so it drops straight down over the sockets. Four
# M2.5 screws go up through the base and the board's own holes into posts
# in the shell, holding the board and closing the case.
#
# THE YELLOW is a third solid: the badge, inlaid flush into the lid's top,
# and the lettering, emblem and latches on the back. Print the shell with
# a second colour for it (the YELLOW part), or set INLAY = False for one
# colour, the badge then engraved and the rest raised in the shell's own.
# SIDE_FLUSH sinks the nameplate's lettering flush into it instead of
# raising it: inlaid in yellow, or engraved in one colour.
#
# PRINTING. No supports, 0.2 mm layers. The shell prints upside down, its
# lid (and the badge) on the bed; the base as it stands.
#
# ASSEMBLY. Set the board on the base's standoffs, lower the shell over it,
# turn it all over and drive four M2.5 x 10 mm screws (pan or button head)
# up through the base.
#
# Board sizes are from Raspberry Pi's mechanical drawings (the Pi 4's and
# the Zero's port heights less certain). Sizes are in mm.

BOARD = "pi5"         # "pi5", "pi4" or "zero2w"
ASSEMBLED = True      # True: the case closed, with an outline of the board (not printed);
                      # False: the parts as they print
LABEL = "VAULT-TEC"   # the lettering on the lid's badge and the nameplate ("" for none)
SIDE_FLUSH = False    # True: the nameplate's lettering sunk flush into it (inlaid in yellow, or engraved
                      # in one colour), so it prints with no overhang; False: raised TRIM_OUT off it
MIN_TEXT = 4.5       # lettering that would be shorter is left off: its strokes would be too thin
                      # for a 0.4 mm nozzle (the Zero's case; its lid gets a bigger emblem instead)
INLAY = True          # the yellow as a second colour; False: one colour, the badge engraved
GILLS = True          # slots up the end wall by the microSD card, for air
FAN_GRILLE = True     # Pi 5: arc slots through the badge's disc, under the emblem, over the fan
FRONT_GILLS = True    # Pi 5: slots in the front wall above the plugs, by the heatsink's fins
FEET = 8.5            # hollows under the base for stick-on feet this wide; 0 for none

WALL = 2.0            # the walls' thickness
FLOOR = 2.4           # the base plate's thickness
ROOF = 1.8            # the lid's thickness
INLAY_T = 0.6         # how deep the door is set into it
EDGE = 1.2            # the bevel round the lid's edge and the base's bottom edge
STANDOFF = 3.0        # gap under the board
GAP = 0.6             # between the board's edge and the walls
TAP = 2.2             # hole an M2.5 screw cuts its own thread in
PASS = 3.2            # hole an M2.5 screw passes through (small holes print small)
HEAD_D, HEAD_H = 5.6, 2.0   # pocket for the screw's head under the base
POST_R = 2.7          # the shell's posts
TAP_DEPTH = 9.0       # how far up the posts the screw holes go
COG_TEETH = 9         # teeth on the badge's cog; the gaps between them are vents
PLATE_OUT = 1.0       # how far the nameplate stands off the back wall
TRIM_OUT = 0.8        # and the lettering and emblem off the plate
SIDE_SINK = 0.8       # how deep they are set into it when SIDE_FLUSH
LATCH_W = 9.0         # the latches either side of it
CORNER_L = 7.0        # the ribs reach this far along the walls from each corner
RIB_OUT, RIB_H, RIB_PITCH = 1.0, 1.0, 2.0   # the ribs: how far out, how tall, how far apart
SHELL_COLOR = "#1e4fa0"   # vault blue
DOOR_COLOR = "#f6c90e"    # vault yellow
BASE_COLOR = "#2f3640"
BOARD_COLOR = "#2e8b57"
SIDES = 48

# --- the boards ----------------------------------------------------------------
# x, y from the corner by the power socket; heights from the board's top face.
# PORTS are openings: (name, side, middle along the side, width, bottom,
# top); "front" is the y = 0 edge, "right" the x = BW edge, "left" x = 0.
# PARTS are boxes on the board for the outline: (x0, y0, x1, y1, height).

BOARDS = {
    "pi5": {
        "name": "Raspberry Pi 5", "size": (85.0, 56.0), "headroom": 16.6,
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 52.5), (61.5, 52.5)],
        "sd": (22.0, 34.0),
        "ports": [
            ("USB-C power", "front", 11.2, 13.0, -1.8, 5.2),
            ("micro HDMI 0", "front", 25.8, 12.0, -2.4, 5.6),
            ("micro HDMI 1", "front", 39.2, 12.0, -2.4, 5.6),
            ("Ethernet", "right", 10.2, 16.8, -0.3, 14.0),
            ("USB 3", "right", 29.1, 15.4, -0.3, 16.4),
            ("USB 2", "right", 47.0, 15.4, -0.3, 16.4),
            ("power button and LED", "left", 16.0, 9.2, -0.3, 5.5),
            ("PCIe ribbon", "left", 30.5, 15.0, -0.3, 2.5),
        ],
        "parts": [
            ("Ethernet", 65.5, 2.2, 88.0, 18.2, 13.5),
            ("USB 3", 69.5, 22.4, 88.0, 35.8, 16.0),
            ("USB 2", 69.5, 40.3, 88.0, 53.7, 16.0),
            ("USB-C power", 6.7, -1.0, 15.7, 6.5, 3.2),
            ("micro HDMI 0", 22.3, -1.0, 29.3, 6.5, 3.4),
            ("micro HDMI 1", 35.7, -1.0, 42.7, 6.5, 3.4),
            ("GPIO header", 7.1, 47.5, 57.9, 52.5, 8.5),
            ("power button", -0.45, 16.6, 3.0, 20.2, 4.1),
            ("Active Cooler", 6.6, 7.0, 64.0, 49.4, 13.1),
        ],
    },
    "pi4": {
        "name": "Raspberry Pi 4 B", "size": (85.0, 56.0), "headroom": 16.6,
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 52.5), (61.5, 52.5)],
        "sd": (22.0, 34.0),
        "ports": [
            ("USB-C power", "front", 11.2, 13.0, -1.8, 5.2),
            ("micro HDMI 0", "front", 26.0, 12.0, -2.4, 5.6),
            ("micro HDMI 1", "front", 39.5, 12.0, -2.4, 5.6),
            ("audio", "front", 53.5, 8.5, -0.8, 7.2),
            ("USB 2", "right", 9.0, 15.4, -0.3, 16.4),
            ("USB 3", "right", 27.0, 15.4, -0.3, 16.4),
            ("Ethernet", "right", 45.8, 16.8, -0.3, 14.0),
        ],
        "parts": [
            ("USB 2", 69.5, 2.3, 88.0, 15.7, 16.0),
            ("USB 3", 69.5, 20.3, 88.0, 33.7, 16.0),
            ("Ethernet", 65.5, 37.8, 88.0, 53.8, 13.5),
            ("USB-C power", 6.7, -1.0, 15.7, 6.5, 3.3),
            ("micro HDMI 0", 22.5, -1.0, 29.5, 6.5, 3.0),
            ("micro HDMI 1", 36.0, -1.0, 43.0, 6.5, 3.0),
            ("audio", 50.5, -2.0, 56.5, 12.5, 6.0),
            ("GPIO header", 7.1, 47.5, 57.9, 52.5, 8.5),
        ],
    },
    "zero2w": {
        "name": "Raspberry Pi Zero 2 W", "size": (65.0, 30.0), "headroom": 10.0,
        "holes": [(3.5, 3.5), (61.5, 3.5), (3.5, 26.5), (61.5, 26.5)],
        "sd": (9.0, 20.0),
        "ports": [
            ("mini HDMI", "front", 12.4, 13.5, -1.8, 5.6),
            ("micro USB data", "front", 41.4, 10.0, -1.6, 4.8),
            ("micro USB power", "front", 54.0, 10.0, -1.6, 4.8),
            ("camera ribbon", "right", 15.0, 17.0, -0.3, 2.0),
        ],
        "parts": [
            ("mini HDMI", 6.8, -0.5, 18.0, 7.0, 3.5),
            ("micro USB data", 37.4, -1.0, 45.4, 5.0, 3.0),
            ("micro USB power", 50.0, -1.0, 58.0, 5.0, 3.0),
            ("GPIO header", 7.1, 24.0, 57.9, 29.0, 8.5),
        ],
    },
}
if BOARD not in BOARDS:
    fail('BOARD must be "pi5", "pi4" or "zero2w"')
B = BOARDS[BOARD]
BW, BH = B["size"]
BT = 1.4
BOARD_R = 3.0
HOLES = B["holes"]
SD = B["sd"]
PORTS = B["ports"]
HEADROOM = B["headroom"]

# --- sizes --------------------------------------------------------------------

ZB = FLOOR + STANDOFF                 # the board's underside
ZT = ZB + BT                          # the board's top face
RIM_TOP = ZT - 0.35                   # the base's rim stops just under the top face
SHELL_Z0 = ZT - 0.2                   # the shell's walls start here (a hairline gap)
CEIL = ZT + HEADROOM                  # the lid's underside
ZTOP = CEIL + ROOF
X0, Y0, X1, Y1 = -GAP - WALL, -GAP - WALL, BW + GAP + WALL, BH + GAP + WALL
R_OUT = BOARD_R + GAP + WALL
R_IN = BOARD_R + GAP
DX, DY = (X0 + X1) / 2, (Y0 + Y1) / 2                       # the door's middle
DOOR_R = min(X1 - X0, Y1 - Y0) / 2 - 4.0
for hx, hy in HOLES:
    # the door's vents mustn't open over a post
    d = math.sqrt((hx - DX) * (hx - DX) + (hy - DY) * (hy - DY))
    if d < DOOR_R + POST_R + 0.5 and d > DOOR_R * 0.3:
        fail("the door would cross a post: lower DOOR_R")
if ROOF < INLAY_T + 1.0:
    fail("the lid is too thin for the door's inlay: raise ROOF")

# --- helpers ------------------------------------------------------------------

def box(x0, y0, z0, x1, y1, z1, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = [x0, y0, z0], max = [x1, y1, z1], ids = [id])
    return id

def cyl(x, y, r, z0, z1, hole = False):
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [x - r, y - r, z0], max = [x + r, y + r, z1], ids = [id])
    return id

def rounded(x0, y0, x1, y1, r, n = 10):
    """A rectangle's outline with rounded corners, counter-clockwise."""
    pts = []
    for cx, cy, a0 in ((x1 - r, y1 - r, 0), (x0 + r, y1 - r, 90), (x0 + r, y0 + r, 180), (x1 - r, y0 + r, 270)):
        for i in range(n + 1):
            a = math.radians(a0 + 90.0 * i / n)
            pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    return pts

def circle(x, y, r, n = 96):
    return [(x + r * math.cos(2 * math.pi * i / n), y + r * math.sin(2 * math.pi * i / n)) for i in range(n)]

def ring(r0, r1, z0, z1, hole = False):
    return cad.add_outline([circle(DX, DY, r1), circle(DX, DY, r0)], z0 = z0, z1 = z1, hole = hole)

def named(ids, name, color):
    g = cad.group("union", ids = ids)
    cad.set_name(name, ids = [g])
    cad.set_color(color, ids = [g])
    return g

def turn_about(id, cx, cy, deg):
    b = cad.node(id)["bounds"]["center"]
    cad.rotate("z", deg, ids = [id])
    w = math.radians(deg)
    ux, uy = cx - b[0], cy - b[1]
    rx, ry = ux * math.cos(w) - uy * math.sin(w), ux * math.sin(w) + uy * math.cos(w)
    cad.move(dx = ux - rx, dy = uy - ry, ids = [id])

OUTER = rounded(X0, Y0, X1, Y1, R_OUT)
OUTER_IN = rounded(X0 + EDGE, Y0 + EDGE, X1 - EDGE, Y1 - EDGE, R_OUT - EDGE)   # inside the edges' 45 degree bevels
INNER = rounded(X0 + WALL, Y0 + WALL, X1 - WALL, Y1 - WALL, R_IN)

def port_holes(z_from, z_to):
    """A hole through the wall for the part of each port's opening between
    heights z_from and z_to."""
    out = []
    for name, side, mid, width, lo, hi in PORTS:
        z0, z1 = max(ZT + lo, z_from), min(ZT + hi, z_to)
        if z1 <= z0:
            continue
        if side == "front":
            out.append(box(mid - width / 2, Y0 - 1, z0, mid + width / 2, Y0 + WALL + 0.5, z1, hole = True))
        elif side == "right":
            out.append(box(X1 - WALL - 0.5, mid - width / 2, z0, X1 + 1, mid + width / 2, z1, hole = True))
        else:
            out.append(box(X0 - 1, mid - width / 2, z0, X0 + WALL + 0.5, mid + width / 2, z1, hole = True))
    return out

# --- the emblem and the lettering ------------------------------------------------------
#
# Drawn flat, centred on (cx, cy), from z0 to z1: on the lid they are used
# as they are, on the back wall they are stood up against it.

def emblem(cx, cy, s, z0, z1):
    """The winged emblem: a ring round a solid centre, with two bars as
    wings each side, the upper one longer, both pointed at their outer
    ends. s is the ring's outer radius."""
    parts = [cad.add_outline([circle(cx, cy, s, 48), circle(cx, cy, s * 0.64, 48)], z0 = z0, z1 = z1),
             cyl(cx, cy, s * 0.34, z0, z1)]
    for side in (-1, 1):
        for ya, yb, length in ((0.08, 0.38, 2.5), (-0.42, -0.12, 1.7)):
            x0 = s * 0.82
            x1 = s * (0.82 + length)
            tip = (yb - ya) * s                    # the outer end pointed at 45 degrees
            pts = [(x0, ya * s), (x1 - tip, ya * s), (x1, yb * s), (x0, yb * s)]
            parts.append(cad.add_outline([(cx + side * px, cy + py) for px, py in (pts if side > 0 else pts[::-1])], z0 = z0, z1 = z1))
    return cad.group("union", ids = parts)

def lettering(text, cx, cy, h, z0, z1):
    t = cad.add_shape("text")
    cad.set_text(t, text)
    w = h * 0.62 * len(text)
    cad.set_bounds(min = [cx - w / 2, cy - h / 2, z0], max = [cx + w / 2, cy + h / 2, z1], ids = [t])
    return t

def text_h(text, w, h):
    """The lettering's height, to fit w by h."""
    return min(h, w / (0.62 * max(len(text), 1)))

def to_wall(id, cx, cz, y_face):
    """Stand a part drawn flat (reading along +x, its thickness up) against
    the back wall, reading from behind, its back on y = y_face."""
    cad.rotate("x", 90, ids = [id])
    cad.rotate("z", 180, ids = [id])
    b = cad.node(id)["bounds"]
    cad.move(dx = cx - b["center"][0], dy = y_face - b["min"][1], dz = cz - b["center"][2], ids = [id])
    return id

# --- the badge on the lid ----------------------------------------------------------------
#
# A yellow cog round a blue disc, with the lettering and the emblem in
# yellow on the disc. The gaps between the cog's teeth are vents through
# the lid. In two colours the yellow is inlaid INLAY_T deep, flush with the
# lid; in one colour it is engraved.

R = DOOR_R
R_BODY = R * 0.84                     # the cog's body, inside its teeth
R_DISC = R * 0.70                     # the blue disc inside its ring
PITCH = 2 * math.pi / COG_TEETH

def badge(z0, z1):
    """The badge's yellow, from z0 to z1."""
    parts = [ring(R_DISC, R_BODY, z0, z1)]
    tw = 2 * math.pi * R / COG_TEETH * 0.46
    for k in range(COG_TEETH):
        t = box(DX + R_BODY - 0.5, DY - tw / 2, z0, DX + R, DY + tw / 2, z1)
        turn_about(t, DX, DY, math.degrees(PITCH * k) + 90)
        parts.append(t)
    # the lettering over the emblem, turned to read from the nameplate's side
    h = text_h(LABEL, R_DISC * 1.55, R_DISC * 0.36)
    if LABEL and h >= MIN_TEXT:
        face = [emblem(DX, DY - R_DISC * 0.32, R_DISC * 0.19, z0, z1)]
    else:
        # no room for lettering that prints: the emblem alone, as big as
        # its wings fit in the disc
        face = [emblem(DX, DY, R_DISC * 0.27, z0, z1)]
    if LABEL and h >= MIN_TEXT:
        face.append(lettering(LABEL, DX, DY + R_DISC * 0.30, h, z0, z1))
    for id in face:
        turn_about(id, DX, DY, 180)
    return cad.group("union", ids = parts + face)

GRILLE_W, GRILLE_PITCH = 1.5, 2.5     # the grille's arcs: their width, and centre to centre

def grille(z0, z1):
    """Arc slots through the blue disc, centred on it, in the part under
    the emblem (the side toward the nameplate): there the Pi 5's fan draws
    its air. They keep 1 mm from the emblem (its ring and wings) and from
    the yellow ring round the disc. As holes."""
    s = R_DISC * 0.19                         # the emblem's ring, as in badge()
    ey = R_DISC * 0.32                        # the emblem's middle, from the disc's
    r_lo = ey + s + 1.0 + GRILLE_W / 2        # clear of the emblem's ring
    r_hi = R_DISC - 1.0 - GRILLE_W / 2        # and of the yellow ring
    y_lim = ey + 0.42 * s + 1.0               # past the wings' outer edge
    out = []
    r = r_lo
    for _ in range(10):
        if r > r_hi:
            break
        ri, ro = r - GRILLE_W / 2, r + GRILLE_W / 2
        if ri > y_lim + 0.5:
            a = math.asin(y_lim / ri)         # where the arc's inner edge meets y_lim
            n = 24
            outer, inner = [], []
            for i in range(n + 1):
                t = a + (math.pi - 2 * a) * i / n
                outer.append((DX + ro * math.cos(t), DY + ro * math.sin(t)))
                inner.append((DX + ri * math.cos(t), DY + ri * math.sin(t)))
            out.append(cad.add_outline(outer + inner[::-1], z0 = z0, z1 = z1, hole = True))
        r += GRILLE_PITCH
    return out

def vents(z0, z1):
    """A slot in the gap between each pair of the cog's teeth, as holes."""
    out = []
    gw = 2 * math.pi * R_BODY / COG_TEETH * 0.30
    for k in range(COG_TEETH):
        v = box(DX + R_BODY + 0.6, DY - gw / 2, z0, DX + R - 0.3, DY + gw / 2, z1, hole = True)
        turn_about(v, DX, DY, math.degrees(PITCH * (k + 0.5)) + 90)
        out.append(v)
    return out

# --- the nameplate, the latches and the ribs ------------------------------------------------
#
# On the back wall, which has no ports: a raised plate with a grooved
# border and a rivet in each corner, the lettering and the emblem on it in
# yellow, and a yellow latch either side. The case's four corners are
# ribbed, each rib's top sloping at 45 degrees (it prints upside down).

ZW0, ZW1 = SHELL_Z0 + 1.0, ZTOP - EDGE - 1.0      # the walls' usable height
PH = ZW1 - ZW0 - 1.0                               # the plate's height
PZ = (ZW0 + ZW1) / 2                               # its middle
STRAIGHT = (X1 - X0) - 2 * R_OUT                   # the back wall's straight run
PW = min(STRAIGHT - 2 * (LATCH_W + 3.0) - 2 * CORNER_L + 2 * R_OUT, (X1 - X0) * 0.6)
LATCH_AT = PW / 2 + 2.0 + LATCH_W / 2              # the latches' middles, from DX
if PW < 25:
    fail("no room for the nameplate on the back wall")

def wall_block(x0, x1, z0, z1, y0, y1):
    """A block on the back wall from y0 out to y1, its top sloping back to
    y0 at 45 degrees: the shell prints upside down, so that top prints
    first, and grows out of the wall instead of hanging in the air."""
    def rect(y):
        return [(x0, y0), (x1, y0), (x1, y), (x0, y)]
    out = min(y1 - y0, z1 - z0 - 0.05)               # a block shorter than it stands out slopes from its bottom
    return cad.add_loft([(z0, rect(y1)), (z1 - out, rect(y1)), (z1, rect(y1 - out + 0.02))])

def nameplate():
    """The plate itself, part of the shell, with its groove and rivets."""
    parts = [wall_block(DX - PW / 2, DX + PW / 2, PZ - PH / 2, PZ + PH / 2, Y1 - 0.01, Y1 + PLATE_OUT)]
    inset = 1.2
    frame = cad.group("union", ids = [
        box(DX - PW / 2 + inset, Y1 + PLATE_OUT - 0.4, PZ - PH / 2 + inset, DX + PW / 2 - inset, Y1 + PLATE_OUT + 1, PZ + PH / 2 - inset),
        box(DX - PW / 2 + inset + 0.7, Y1 + PLATE_OUT - 1, PZ - PH / 2 + inset + 0.7, DX + PW / 2 - inset - 0.7, Y1 + PLATE_OUT + 2, PZ + PH / 2 - inset - 0.7, hole = True)])
    cad.set_hole(True, ids = [frame])
    parts.append(frame)
    for sx in (-1, 1):
        for sz in (-1, 1):
            # each rivet runs from the wall's face through the plate and
            # stands 0.5 proud of it: the top two are on the plate's sloped
            # top edge, which a rivet only on the plate's face would float
            # in front of
            rv = cad.add_shape("cylinder")
            cad.set_param(rv, "sides", 16)
            cad.set_bounds(min = [-0.6, -0.6, 0], max = [0.6, 0.6, PLATE_OUT + 0.5], ids = [rv])
            cad.rotate("x", -90, ids = [rv])
            b = cad.node(rv)["bounds"]
            cad.move(dx = DX + sx * (PW / 2 - 0.6) - b["center"][0], dy = Y1 - 0.01 - b["min"][1],
                     dz = PZ + sz * (PH / 2 - 0.6) - b["center"][2], ids = [rv])
            parts.append(rv)
    return cad.group("union", ids = parts)

def plate_marks(y0, y1):
    """The lettering and emblem on the nameplate, from y0 out to y1."""
    inner_w, inner_h = PW - 5.0, PH - 4.0
    s = min(inner_h * 0.18, inner_w / 9.0)        # the emblem's ring's radius
    big = s >= 2.0                                 # too small to print, it is left off
    h = text_h(LABEL, inner_w, inner_h * (0.42 if big else 0.8))
    parts = []
    if LABEL and h >= MIN_TEXT:
        parts.append(to_wall(lettering(LABEL, 0, 0, h, 0, y1 - y0), DX, PZ + (inner_h * 0.22 if big else 0), y0))
    if big:
        parts.append(to_wall(emblem(0, 0, s, 0, y1 - y0), DX, PZ - inner_h * 0.24, y0))
    return parts

def plate_trim():
    """The yellow on the plate (its lettering, unless that is sunk flush),
    and the two latches."""
    face = Y1 + PLATE_OUT
    parts = [] if SIDE_FLUSH else plate_marks(face, face + TRIM_OUT)
    # each latch: a plate on the wall, the lever on it and the hinge's bar below
    for sx in (-1, 1):
        x = DX + sx * LATCH_AT
        lh = PH * 0.9
        parts.append(wall_block(x - LATCH_W / 2, x + LATCH_W / 2, PZ - lh / 2, PZ + lh / 2, Y1, Y1 + 0.8))
        parts.append(wall_block(x - LATCH_W * 0.32, x + LATCH_W * 0.32, PZ - lh * 0.12, PZ + lh * 0.42, Y1 + 0.5, Y1 + 1.8))
        parts.append(wall_block(x - LATCH_W * 0.42, x + LATCH_W * 0.42, PZ - lh * 0.40, PZ - lh * 0.22, Y1 + 0.5, Y1 + 2.0))
    return cad.group("union", ids = parts)

def corner_ribs():
    """Ribs round the case's four corners."""
    zones = cad.group("union", ids = [box(cx - CORNER_L, cy - CORNER_L, ZW0 - 1, cx + CORNER_L, cy + CORNER_L, ZW1 + 1)
                                      for cx in (X0, X1) for cy in (Y0, Y1)])
    ribs = []
    z = ZW0
    for _ in range(40):
        if z + RIB_H > ZW1:
            break
        o = RIB_OUT
        ribs.append(cad.add_loft([(z, rounded(X0 - o, Y0 - o, X1 + o, Y1 + o, R_OUT + o)),
                                  (z + RIB_H, rounded(X0, Y0, X1, Y1, R_OUT))]))
        z += RIB_PITCH
    ribs.append(cad.add_outline(rounded(X0 + 0.5, Y0 + 0.5, X1 - 0.5, Y1 - 0.5, R_OUT - 0.5), z0 = ZW0 - 2, z1 = ZW1 + 2, hole = True))
    g = cad.group("union", ids = ribs)
    return group_intersect([g, zones])

def group_intersect(ids):
    g = cad.group("union", ids = ids)
    cad.set_group_mode("intersect", ids = [g])
    return g

# --- the base -------------------------------------------------------------------

def make_base():
    parts = [cad.add_loft([(0, OUTER_IN), (EDGE, OUTER), (FLOOR, OUTER)]),
             cad.add_outline([OUTER, INNER], z0 = FLOOR - 0.01, z1 = RIM_TOP)]
    for hx, hy in HOLES:
        parts.append(cyl(hx, hy, 3.1, FLOOR - 0.01, ZB))
        parts.append(cyl(hx, hy, PASS / 2, -1, ZB + 1, hole = True))
        parts.append(cyl(hx, hy, HEAD_D / 2, -1, HEAD_H, hole = True))
    if FEET > 0:
        for fx, fy in ((X0 + 10, Y0 + 9), (X1 - 10, Y0 + 9), (X0 + 10, Y1 - 9), (X1 - 10, Y1 - 9)):
            parts.append(cyl(fx, fy, FEET / 2, -1, 0.6, hole = True))
    parts += port_holes(FLOOR + 0.6, RIM_TOP + 1)
    # the microSD card: a notch in the rim, and a scoop in the plate under
    # it for a fingernail
    parts.append(box(X0 - 1, SD[0] - 1.5, ZB - 2.2, X0 + WALL + 0.5, SD[1] + 1.5, RIM_TOP + 1, hole = True))
    parts.append(box(X0 - 1, SD[0] + 1, -1, 2.5, SD[1] - 1, ZB - 2.2 + 0.01, hole = True))
    return named(parts, "Base", BASE_COLOR)

# --- the shell --------------------------------------------------------------------

def make_shell():
    cuts = [cad.add_outline(INNER, z0 = SHELL_Z0 - 1, z1 = CEIL, hole = True)]
    body = cad.group("union", ids = [cad.add_loft([(SHELL_Z0, OUTER), (ZTOP - EDGE, OUTER), (ZTOP, OUTER_IN)]),
                                     corner_ribs(), nameplate()] + cuts)
    parts = [body]
    if not INLAY:
        parts.append(plate_trim())
    for hx, hy in HOLES:
        parts.append(cad.group("union", ids = [
            cyl(hx, hy, POST_R, ZT, CEIL + 0.01),
            cyl(hx, hy, TAP / 2, ZT - 1, ZT + TAP_DEPTH, hole = True),
        ]))
    parts += port_holes(SHELL_Z0 - 1, ZTOP)
    # the badge: its yellow taken out of the lid's top for the inlay (or
    # engraved), and its vents through the lid
    b = badge(ZTOP - INLAY_T, ZTOP + 1)
    cad.set_hole(True, ids = [b])
    parts.append(b)
    # the nameplate's lettering sunk flush: its pocket (inlaid, or left
    # engraved in one colour)
    if SIDE_FLUSH:
        face = Y1 + PLATE_OUT
        marks = plate_marks(face - SIDE_SINK, face + 1)
        cad.set_hole(True, ids = marks)
        parts += marks
    parts += vents(CEIL - 1, ZTOP + 1)
    if FAN_GRILLE and BOARD == "pi5":
        parts += grille(CEIL - 1, ZTOP + 1)
    # slots up the end wall above the microSD card. On the Pi 5 the Active
    # Cooler's blower pushes its air sideways out through the heatsink's
    # fins, toward this wall and the front one
    if GILLS:
        n = int((BH - 12.0) / 4.0)
        for i in range(n):
            y = BH / 2 + (i - (n - 1) / 2.0) * 4.0
            if y + 0.8 > BH + GAP - CORNER_L + 1.0 or y - 0.8 < -GAP + CORNER_L - WALL - 1.0:
                continue                              # clear of the corners' ribs
            z0 = ZT + 6.0
            if BOARD == "pi5" and y + 0.8 > 11.4 - 1.5 and y - 0.8 < 20.6 + 1.5:
                z0 = ZT + 8.0                         # over the power button's window: 2.5 mm of wall above it
            parts.append(box(X0 - 1.5, y - 0.8, z0, X0 + WALL + 0.5, y + 0.8, CEIL - 2.5, hole = True))
    # slots along the front wall over the plugs (Pi 5): out beside the fins,
    # and in beside the fan, which draws from the air over it
    if FRONT_GILLS and BOARD == "pi5":
        lo, hi = X0 + CORNER_L + 3.6, X1 - CORNER_L - 3.6      # clear of the corners' ribs
        n = int((hi - lo) / 4.0) + 1
        for i in range(n):
            x = (lo + hi) / 2 + (i - (n - 1) / 2.0) * 4.0
            parts.append(box(x - 0.8, Y0 - 1.5, ZT + 8.0, x + 0.8, Y0 + WALL + 0.5, CEIL - 2.5, hole = True))
    return named(parts, "Shell", SHELL_COLOR)

def make_yellow():
    parts = [badge(ZTOP - INLAY_T, ZTOP - 0.001), plate_trim()]
    if SIDE_FLUSH:
        face = Y1 + PLATE_OUT
        parts += plate_marks(face - SIDE_SINK, face - 0.001)
    return named(parts, "Yellow", DOOR_COLOR)

# --- the board's outline (not printed) ------------------------------------------------

def make_board():
    shapes = [cad.add_outline(rounded(0, 0, BW, BH, BOARD_R), z0 = ZB, z1 = ZT)]
    for name, x0, y0, x1, y1, h in B["parts"]:
        shapes.append(box(x0, y0, ZT, x1, y1, ZT + h))
    shapes.append(box(-2.5, SD[0], ZB - 1.5, 12.0, SD[1], ZB))
    return named(shapes, B["name"] + " (not printed)", BOARD_COLOR)

# --- build ----------------------------------------------------------------------------

# Each half is one item: ungrouping the case gives the base, the top (the
# shell with its yellow, as a bundle so each keeps its colour) and the
# board's outline; ungrouping the top gives the shell and the yellow.
base = make_base()
top = make_shell()
if INLAY:
    top = cad.group("bundle", ids = [top, make_yellow()])
    cad.set_name("Top", ids = [top])
if ASSEMBLED:
    case = cad.group("bundle", ids = [base, top, make_board()])
    cad.set_name("Vault case for " + B["name"], ids = [case])
else:
    # as printed: the top lid down, beside the base
    cad.rotate("x", 180, ids = [top])
    cad.set_position(x = X0, y = Y1 + 8, z = 0, ids = [top])
    case = cad.group("bundle", ids = [base, top])
    cad.set_name("Vault case for " + B["name"] + ", as printed", ids = [case])
print("vault case for %s: %s x %s x %s mm; badge %s mm across, nameplate %s x %s" % (
    B["name"], str(math.round((X1 - X0) * 10) / 10), str(math.round((Y1 - Y0) * 10) / 10),
    str(math.round(ZTOP * 10) / 10), str(math.round(2 * DOOR_R * 10) / 10),
    str(math.round(PW * 10) / 10), str(math.round(PH * 10) / 10)))
