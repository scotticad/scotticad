# hook-and-bracket.star - a wall hook and a shelf bracket, each drawn as a
# side outline (cad.add_outline) and extruded to its width, with
# countersunk screw holes cut out as holes.
#
# Both parts lie on their sides, the way they should be printed. A printed
# part is weakest between its layers, so a hook printed standing up tends
# to snap where the arm leaves the plate. On its side, every layer holds
# the whole outline and the load runs along the layers instead of across
# them. No supports are needed.
#
# In the design, Y runs up the wall, X runs out from the wall, and Z is the
# part's width (its height on the print bed).
#
# Change the numbers below and run again. Sizes are in mm.

# --- screws -------------------------------------------------------------
# Countersunk wood screws. The defaults suit 4 mm (#8) screws: the hole
# lets the thread slide through, and the 90-degree countersink lets the
# head sit flush.
SCREW_D = 4.5        # clearance hole
HEAD_D = 8.5         # screw head diameter

# --- hook ---------------------------------------------------------------
HOOK_W = 20.0        # width of the hook
PLATE_T = 6.0        # thickness of the plate against the wall
PLATE_H = 80.0       # height of the plate
HOOK_GAP = 30.0      # inside width of the hook's U, plate to tip
HOOK_T = 7.0         # thickness of the curved arm
LIP_H = 35.0         # height of the tip, from the bottom of the hook
HOOK_SCREWS = [52.0, 70.0]   # screw heights; keep them above the tip so
                             # a screwdriver can reach them

# --- shelf bracket ------------------------------------------------------
BRACKET_W = 25.0     # width of the bracket
SHELF_D = 120.0      # length of the arm under the shelf (about 3/4 of
                     # the shelf's depth)
WALL_H = 140.0       # length of the leg against the wall
LEG_T = 6.0          # thickness of the wall leg and the shelf arm
STRUT_W = 10.0       # width of the diagonal strut
TAIL = 25.0          # wall leg below the strut, for the lower screw
ARM_END = 18.0       # arm beyond the strut, for the screw up into the shelf

CORNER_R = 3.0       # rounding on outside corners
WINDOW_R = 5.0       # rounding inside the bracket's window (sharp inside
                     # corners are where cracks start)
GAP = 15.0           # space between the two parts on the plate

HOOK_COLOR = "#2f7fc1"
BRACKET_COLOR = "#e0a030"

# --- outline helpers ----------------------------------------------------

def unit(dx, dy):
    k = math.sqrt(dx * dx + dy * dy)
    return (dx / k, dy / k)

def arc(cx, cy, r, a0, a1, n):
    """n+1 points from angle a0 to a1 (radians) on a circle."""
    return [(cx + r * math.cos(a0 + (a1 - a0) * i / n),
             cy + r * math.sin(a0 + (a1 - a0) * i / n)) for i in range(n + 1)]

def fillet(a, p, b, r, n = 8):
    """Corner p, between neighbours a and b, replaced by an arc of radius r
    that touches both edges. Works for inside and outside corners. r
    shrinks if the edges are too short for it."""
    u = unit(a[0] - p[0], a[1] - p[1])
    v = unit(b[0] - p[0], b[1] - p[1])
    half = math.acos(max(-1.0, min(1.0, u[0] * v[0] + u[1] * v[1]))) / 2
    if r <= 0 or half > math.pi / 2 - 0.01:       # no corner to round
        return [p]
    la = math.sqrt((a[0] - p[0]) * (a[0] - p[0]) + (a[1] - p[1]) * (a[1] - p[1]))
    lb = math.sqrt((b[0] - p[0]) * (b[0] - p[0]) + (b[1] - p[1]) * (b[1] - p[1]))
    t = min(r / math.tan(half), 0.45 * min(la, lb))  # tangent points' distance
    r = t * math.tan(half)
    bis = unit(u[0] + v[0], u[1] + v[1])
    c = (p[0] + bis[0] * r / math.sin(half), p[1] + bis[1] * r / math.sin(half))
    s = (p[0] + u[0] * t, p[1] + u[1] * t)
    e = (p[0] + v[0] * t, p[1] + v[1] * t)
    a0 = math.atan2(s[1] - c[1], s[0] - c[0])
    a1 = math.atan2(e[1] - c[1], e[0] - c[0])
    if a1 - a0 > math.pi:                         # the short way round
        a1 -= 2 * math.pi
    elif a0 - a1 > math.pi:
        a1 += 2 * math.pi
    return arc(c[0], c[1], r, a0, a1, n)

def round_all(pts, r):
    """Every corner of the closed outline pts rounded with radius r."""
    out = []
    n = len(pts)
    for i in range(n):
        out += fillet(pts[i - 1], pts[i], pts[(i + 1) % n], r)
    return out

# --- screw holes --------------------------------------------------------
# Each is a cylinder for the thread plus a cone for the head. The cone is
# 1 mm wider than the head and starts 1 mm proud of the face, so it meets
# the face at exactly the head's size without lying on top of it (holes
# whose faces sit exactly on a solid's faces can leave paper-thin skins).

def wall_screw(face_x, y, z):
    """A screw through a wall leg along X, its head on the face at x =
    face_x (the side away from the wall)."""
    shaft = cad.add_shape("cylinder", hole = True)
    cad.rotate("y", 90, ids = [shaft])
    cad.set_bounds(min = [-1, y - SCREW_D / 2, z - SCREW_D / 2],
                   max = [face_x + 1, y + SCREW_D / 2, z + SCREW_D / 2], ids = [shaft])
    r = HEAD_D / 2 + 1
    head = cad.add_shape("cone", hole = True)
    cad.rotate("y", -90, ids = [head])            # point towards the wall
    cad.set_bounds(min = [face_x + 1 - r, y - r, z - r],
                   max = [face_x + 1, y + r, z + r], ids = [head])
    g = cad.group("union", ids = [shaft, head])
    cad.set_hole(True, ids = [g])
    return g

def wall_screws(face_x, spots):
    """Wall screws at each (y, z) in spots: the first made, the rest copies
    of it moved into place."""
    y0, z0 = spots[0]
    first = wall_screw(face_x, y0, z0)
    out = [first]
    for y, z in spots[1:]:
        cad.select([first])
        new = cad.duplicate()[0]
        cad.move(dy = y - y0, dz = z - z0, ids = [new])
        out.append(new)
    return out

def shelf_screw(x, under_y, top_y, z):
    """A screw up through the shelf arm along Y, its head on the underside
    at y = under_y."""
    shaft = cad.add_shape("cylinder", hole = True)
    cad.rotate("x", 90, ids = [shaft])
    cad.set_bounds(min = [x - SCREW_D / 2, under_y - 1, z - SCREW_D / 2],
                   max = [x + SCREW_D / 2, top_y + 1, z + SCREW_D / 2], ids = [shaft])
    r = HEAD_D / 2 + 1
    head = cad.add_shape("cone", hole = True)
    cad.rotate("x", -90, ids = [head])            # point up into the arm
    cad.set_bounds(min = [x - r, under_y - 1, z - r],
                   max = [x + r, under_y - 1 + r, z + r], ids = [head])
    return [shaft, head]

def color(id, c):
    cad.set_color(c, ids = [id])
    return id

# --- hook ---------------------------------------------------------------
# One outline: the plate, then a U-shaped arm round to the tip. The arm's
# inside curve meets the plate's face smoothly, so there is no sharp
# inside corner to crack. The arm's outside curve is cut off where it
# would poke behind the plate, keeping the back flat against the wall.

if LIP_H < HOOK_GAP / 2 + HOOK_T or PLATE_H < LIP_H + HEAD_D:
    fail("the hook's tip must be above its curve, and the plate taller than the tip")

ri = HOOK_GAP / 2                    # inside radius of the U
ro = ri + HOOK_T                     # outside radius
cx, cy = PLATE_T + ri, ro            # the U's centre; its bottom is at y = 0
a0 = math.pi + math.acos(min(cx / ro, 1.0))   # where the outside meets x = 0
tip_x = cx + ri + HOOK_T / 2

top_right = (PLATE_T, PLATE_H)
top_left = (0.0, PLATE_H)
hook_pts = (arc(cx, cy, ro, a0, 1.5 * math.pi, 16) +          # under the U,
            arc(cx, cy, ro, 1.5 * math.pi, 2 * math.pi, 16)[1:] +  # through its lowest point

            arc(tip_x, LIP_H, HOOK_T / 2, 0, math.pi, 10) +    # round tip
            arc(cx, cy, ri, 2 * math.pi, math.pi, 32) +        # inside the U
            fillet((PLATE_T, cy), top_right, top_left, CORNER_R) +
            fillet(top_right, top_left, (0.0, 0.0), CORNER_R))
hook_body = cad.add_outline(hook_pts, z0 = 0, z1 = HOOK_W)

hook_holes = wall_screws(PLATE_T, [(y, HOOK_W / 2) for y in HOOK_SCREWS])
hook = color(cad.group("union", ids = [hook_body] + hook_holes), HOOK_COLOR)
cad.set_name("wall hook", ids = [hook])

# --- shelf bracket ------------------------------------------------------
# An L (wall leg and shelf arm) filled in as a triangle, with a triangular
# window cut out of it to leave a diagonal strut. The strut leaves the wall
# leg at TAIL and meets the arm ARM_END short of its end. The outline's
# second contour, inside the first, becomes the window.

under = WALL_H - LEG_T               # underside of the shelf arm
p0 = (LEG_T, TAIL)                   # strut's lower edge, at the wall leg...
p1 = (SHELF_D - ARM_END, under)      # ...and at the arm
outer = round_all([(0.0, 0.0), (LEG_T, 0.0), p0, p1, (SHELF_D, under),
                   (SHELF_D, WALL_H), (0.0, WALL_H)], CORNER_R)

# the window's sloping side is the strut's lower edge moved STRUT_W in
dx, dy = p1[0] - p0[0], p1[1] - p0[1]
L = math.sqrt(dx * dx + dy * dy)
win_low = TAIL + STRUT_W * L / dx    # where it meets the wall leg
win_far = p1[0] - STRUT_W * L / dy   # where it meets the arm
if win_low > under - 2 * WINDOW_R or win_far < LEG_T + 2 * WINDOW_R:
    fail("the strut is too wide for the bracket; make STRUT_W smaller or the bracket bigger")
window = round_all([(LEG_T, win_low), (win_far, under), (LEG_T, under)], WINDOW_R)

bracket_body = cad.add_outline([outer, window], z0 = 0, z1 = BRACKET_W)

# Two screws into the wall: the top one takes the load (the shelf's weight
# tries to pull the top of the bracket away from the wall, pivoting on the
# bottom), the lower one in the tail keeps it straight. The strut is in
# front of the top screw, so a driver-sized tunnel goes through it.
zc = BRACKET_W / 2
top_y = under - HEAD_D / 2 - 3
bracket_holes = wall_screws(LEG_T, [(top_y, zc), (TAIL / 2, zc)])
tunnel = cad.add_shape("cylinder", hole = True)
cad.rotate("y", 90, ids = [tunnel])
tr = HEAD_D / 2 + 1
cad.set_bounds(min = [LEG_T + 0.3, top_y - tr, zc - tr],
               max = [SHELF_D + 1, top_y + tr, zc + tr], ids = [tunnel])
# and one up into the shelf, through the end of the arm
bracket_holes += [tunnel] + shelf_screw(SHELF_D - ARM_END / 2, under, WALL_H, zc)

bracket = color(cad.group("union", ids = [bracket_body] + bracket_holes), BRACKET_COLOR)
cad.set_name("shelf bracket", ids = [bracket])

# --- layout -------------------------------------------------------------
# the bracket beside the hook, the pair centred on the build plate

cad.set_position(x = cad.node(hook)["bounds"]["max"][0] + GAP, y = 0, ids = [bracket])
cad.select([hook, bracket])
cad.set_position(x = 0, y = 0, anchor = "center")
cad.drop()
cad.fit_view()
print("hook", cad.node(hook)["bounds"]["size"], "bracket", cad.node(bracket)["bounds"]["size"])
