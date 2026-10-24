# benchy-loft.star - the 3DBenchy calibration boat rebuilt from ScottiCAD
# shapes, its hull made as lofts (cad.add_loft): smooth, with no
# stair-steps, in about 130 shapes and groups. (benchy-layered-slow.star is
# the earlier prototype that stacks 55 layers instead - about 1,060 shapes
# and much slower; it is kept only as an example of what not to do.)
#
# 3DBenchy is by Creative Tools AB and has been in the public domain (CC0)
# since 2025. The hull is lofted through outlines measured from the
# official STL (github.com/johan--/3DBenchy, Single-part/3DBenchy.stl);
# the other numbers were measured from it by slicing, and the ones the
# official spec sheet (3dbenchy.com/dimensions) names are exact:
#   overall 60.00 x 31.00 x 48.00, cargo box top 15.50 above the bottom
#   cargo box 12.00 x 10.81 outside, 8.00 x 7.00 inside, 9.00 deep
#   chimney 7.00 outside at the top, 3.00 hole, 11.00 deep
#   bridge roof 23.00 long, inclined 5.5 degrees
#   front window 10.50 x 9.50, rear window 9.00 (12.00 flange, 0.30 proud)
#   hawsepipes 4.00 (flange 0.30 proud)
#
# Not modelled: the 0.10 mm stern nameplate and its "#3DBenchy" lettering,
# the "CT3D.xyz" keel lettering, the front window's frame, fillets and
# rounded edges.
#
# Axes: X runs stern (-) to bow (+), Y is port/starboard, Z is up.

COLOR = "#e8562a"
SIDES = 64      # facets on cylinders

# --- helpers ------------------------------------------------------------

def box(lo, hi, hole = False):
    id = cad.add_shape("box", hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def cyl(cx, cy, d, z0, z1, hole = False):
    """A vertical cylinder of diameter d."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d / 2, cy - d / 2, z0], max = [cx + d / 2, cy + d / 2, z1], ids = [id])
    return id

def flared(cx, cy, d0, d1, z0, z1, hole = False):
    """A cone frustum that widens upward from d0 at z0 to d1 at z1 (a cone
    only narrows upward, so build it upside down and mirror it)."""
    id = cad.add_shape("cone", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d1 / 2, cy - d1 / 2, z0], max = [cx + d1 / 2, cy + d1 / 2, z1], ids = [id])
    cad.set_param(id, "top", d0 / 2)
    cad.mirror("z", ids = [id])
    return id

def tilted_cyl(cx, cz, d, length, lean, hole = False):
    """A cylinder on the centreline (y = 0) centred at (cx, cz), its axis
    leaning lean degrees from vertical toward -X."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [cx - d / 2, -d / 2, cz - length / 2], max = [cx + d / 2, d / 2, cz + length / 2], ids = [id])
    cad.rotate("y", -lean, ids = [id])
    return id

def level_cyl(c, d, length, heading, hole = False):
    """A horizontal cylinder centred at c whose axis points at heading
    degrees (0 = +X, 90 = +Y)."""
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.rotate("x", 90, ids = [id])      # axis along Y
    cad.set_bounds(min = [c[0] - d / 2, c[1] - length / 2, c[2] - d / 2], max = [c[0] + d / 2, c[1] + length / 2, c[2] + d / 2], ids = [id])
    cad.rotate("z", heading - 90, ids = [id])
    return id

SLAB = 80.0     # size of the boxes used as half-spaces

def half(p, q, plane, lo, hi, hole = False):
    """A slab filling the left of the line p->q: in the XY plane (spanning
    z lo..hi) or the XZ plane (spanning y lo..hi). Left of p->q is the
    side its direction turns toward going counter-clockwise."""
    dx, dv = q[0] - p[0], q[1] - p[1]
    n = math.sqrt(dx * dx + dv * dv)
    nx, nv = -dv / n, dx / n                       # left normal
    cx = (p[0] + q[0]) / 2 + nx * SLAB / 4
    cv = (p[1] + q[1]) / 2 + nv * SLAB / 4
    deg = math.degrees(math.atan2(dv, dx))
    L, T = SLAB * 2, SLAB / 2
    if plane == "xy":
        id = box([cx - L / 2, cv - T / 2, lo], [cx + L / 2, cv + T / 2, hi], hole)
        cad.rotate("z", deg, ids = [id])
    else:
        id = box([cx - L / 2, lo, cv - T / 2], [cx + L / 2, hi, cv + T / 2], hole)
        cad.rotate("y", -deg, ids = [id])
    return id

def half3(p, n, hole = False):
    """A slab filling the side of the plane through point p that its
    normal n points to (any direction): a box turned about Y, then Z."""
    k = math.sqrt(n[0] * n[0] + n[1] * n[1] + n[2] * n[2])
    nx, ny, nz = n[0] / k, n[1] / k, n[2] / k
    T, L = SLAB / 2, SLAB * 2
    c = [p[0] + nx * T / 2, p[1] + ny * T / 2, p[2] + nz * T / 2]
    id = box([c[0] - T / 2, c[1] - L / 2, c[2] - L / 2], [c[0] + T / 2, c[1] + L / 2, c[2] + L / 2], hole)
    cad.rotate("y", -math.degrees(math.asin(nz)), ids = [id])
    cad.rotate("z", math.degrees(math.atan2(ny, nx)), ids = [id])
    return id

def union(ids):
    return cad.group("union", ids = ids)

def intersect(ids):
    return cad.group("intersect", ids = ids)

def hole(id):
    cad.set_hole(True, ids = [id])
    return id

def mirrored(id):
    """A copy of shape id mirrored across the boat's centreline (y = 0):
    the starboard twin of a port-side part."""
    cad.select([id])
    new = cad.duplicate()[0]
    cad.mirror("y", ids = [new])                  # flips it about its own middle...
    cy = cad.node(new)["bounds"]["center"][1]
    cad.move(dy = -2 * cy, ids = [new])           # ...then across to the other side
    return new
def trapezoid(x0, h0, x1, h1, z0, z1):
    """Half-width h0 at x0 changing in a straight line to h1 at x1."""
    m = max(h0, h1) + 1
    return intersect([
        box([x0, -m, z0], [x1, m, z1]),
        half([x1, h1], [x0, h0], "xy", z0 - 1, z1 + 1),
        half([x0, -h0], [x1, -h1], "xy", z0 - 1, z1 + 1),
    ])
def cut_above(a, b):
    """A hole removing everything above the line a->b (x, z) between
    their x values."""
    return hole(intersect([
        box([a[0], -20, -5], [b[0], 20, 60]),
        half(a, b, "xz", -20, 20),
    ]))

def cut_below(a, b):
    """A hole removing everything below the line a->b (x, z) between
    their x values."""
    return hole(intersect([
        box([a[0], -20, -5], [b[0], 20, 60]),
        half(b, a, "xz", -20, 20),
    ]))
def hull_outline(xs, hs, x0, wa, w, xt, arc = 24):
    """A hull outline as points: a straight taper from the transom (xs,
    half-width hs) to x0 (half-width wa), then a pointed arch (half-width w
    at x0, tangent there) closing to the bow tip xt. Used for the wells."""
    R = ((xt - x0) * (xt - x0) + w * w) / (2 * w)
    top = math.asin((xt - x0) / R)
    port = []                                  # the arch on +Y, tip to x0
    for k in range(arc + 1):
        a = top * (1 - k / arc)
        port.append((x0 + R * math.sin(a), w - R + R * math.cos(a)))
    port[0] = (xt, 0.0)
    # (the taper ends a hair aft of x0, so it is a point of its own even
    # where wa == w and every outline keeps the same count)
    pts = port[:-1] + [(x0, w), (x0 - 0.01, wa), (xs, hs), (xs, -hs), (x0 - 0.01, -wa), (x0, -w)]
    pts += [(x, -y) for x, y in reversed(port[1:-1])]
    return pts

def axis_cyl(c, d, l0, l1, axis, hole = False):
    """A cylinder of diameter d along the direction axis through point c,
    from l0 to l1 along it."""
    k = math.sqrt(axis[0] * axis[0] + axis[1] * axis[1] + axis[2] * axis[2])
    ax = [a / k for a in axis]
    m = [c[i] + ax[i] * (l0 + l1) / 2 for i in range(3)]
    id = cad.add_shape("cylinder", hole = hole)
    cad.set_param(id, "sides", SIDES)
    cad.set_bounds(min = [m[0] - d / 2, m[1] - d / 2, m[2] - (l1 - l0) / 2], max = [m[0] + d / 2, m[1] + d / 2, m[2] + (l1 - l0) / 2], ids = [id])
    cad.rotate("y", 90 - math.degrees(math.asin(ax[2])), ids = [id])     # Z axis tipped toward +X
    cad.rotate("z", math.degrees(math.atan2(ax[1], ax[0])), ids = [id])
    return id

# --- hull ---------------------------------------------------------------

# The hull, measured from the official STL in cylindrical coordinates: at
# each of a few heights, the distance from the hull's axis (x = 0, y = 0)
# to the outer hull along each of a set of rays (more of them where the
# hull turns sharply). Between those heights the distances follow smooth
# curves (hull_at); between rays the loft's Smooth draws a curve through
# the points. Every outline has the same rays, so the loft joins each
# point to the same ray's point above it: no twisting, no creases.
#
# Features made separately were measured around: the keel hole, nameplate
# lettering and hawsepipes are filled in, and from the rubbing strake up
# the hull goes straight up just inside it (the strake covers it, and the
# sheer cuts remove what is above).
HULL_RAYS = [0, 3.75, 4.5, 15, 30, 45, 55.25, 55.75, 57.25, 58.5, 60, 60.5, 61.75, 62.75, 64, 65, 66.25, 67.25, 69.5, 70.5, 71.25, 75, 90, 105, 120, 135, 150, 158.75, 159.5, 160, 160.5, 161.25, 162.25, 162.5, 163.5, 165, 166.75, 167, 169.25, 169.5, 171.75, 172, 180]

HULL_KNOTS = [
    (0, [13.84, 13.56, 13.47, 12.26, 10.77, 9.62, 9.02, 9.00, 8.92, 8.86, 8.79, 8.77, 8.72, 8.68, 8.63, 8.59, 8.54, 8.51, 8.43, 8.40, 8.37, 8.27, 8.01, 8.17, 8.94, 10.69, 14.52, 19.15, 19.71, 20.11, 20.52, 21.18, 22.13, 22.38, 23.46, 24.88, 25.41, 25.44, 25.46, 25.44, 25.27, 25.26, 25.26]),
    (0.5, [14.44, 14.13, 14.04, 12.77, 11.22, 10.02, 9.40, 9.37, 9.29, 9.23, 9.16, 9.14, 9.08, 9.04, 8.99, 8.95, 8.90, 8.86, 8.78, 8.75, 8.73, 8.62, 8.36, 8.51, 9.31, 11.12, 15.06, 19.83, 20.40, 20.81, 21.23, 21.91, 22.88, 23.14, 24.24, 25.35, 25.74, 25.76, 25.69, 25.67, 25.50, 25.49, 25.49]),
    (1.1, [15.18, 14.85, 14.75, 13.41, 11.77, 10.51, 9.86, 9.84, 9.76, 9.69, 9.62, 9.60, 9.54, 9.49, 9.44, 9.40, 9.35, 9.31, 9.23, 9.19, 9.17, 9.06, 8.80, 8.95, 9.77, 11.64, 15.74, 20.66, 21.26, 21.67, 22.11, 22.80, 23.80, 24.07, 25.15, 25.87, 26.12, 26.13, 25.98, 25.96, 25.79, 25.77, 25.77]),
    (2, [16.28, 15.91, 15.80, 14.36, 12.61, 11.26, 10.57, 10.54, 10.45, 10.38, 10.31, 10.28, 10.22, 10.17, 10.11, 10.07, 10.02, 9.98, 9.89, 9.86, 9.83, 9.71, 9.45, 9.59, 10.46, 12.43, 16.73, 21.88, 22.50, 22.94, 23.39, 24.11, 25.15, 25.43, 26.14, 26.56, 26.64, 26.62, 26.41, 26.39, 26.22, 26.20, 26.20]),
    (2.9, [17.37, 16.96, 16.85, 15.30, 13.43, 11.99, 11.26, 11.23, 11.14, 11.07, 10.98, 10.96, 10.89, 10.84, 10.78, 10.73, 10.68, 10.64, 10.55, 10.51, 10.48, 10.36, 10.08, 10.23, 11.13, 13.20, 17.70, 23.07, 23.71, 24.16, 24.63, 25.38, 26.36, 26.51, 26.91, 27.15, 27.08, 27.06, 26.83, 26.81, 26.64, 26.62, 26.62]),
    (3, [17.49, 17.08, 16.96, 15.41, 13.52, 12.07, 11.34, 11.30, 11.21, 11.14, 11.06, 11.03, 10.97, 10.92, 10.85, 10.81, 10.75, 10.71, 10.62, 10.58, 10.55, 10.43, 10.15, 10.30, 11.20, 13.28, 17.81, 23.20, 23.84, 24.30, 24.77, 25.52, 26.47, 26.61, 26.99, 27.21, 27.13, 27.10, 26.88, 26.86, 26.68, 26.67, 26.41]),
    (4.1, [18.79, 18.34, 18.21, 16.53, 14.50, 12.95, 12.16, 12.13, 12.03, 11.96, 11.87, 11.84, 11.77, 11.71, 11.65, 11.60, 11.54, 11.49, 11.40, 11.36, 11.33, 11.20, 10.91, 11.05, 12.00, 14.19, 18.95, 24.58, 25.25, 25.72, 26.21, 26.95, 27.48, 27.56, 27.77, 27.82, 27.64, 27.61, 27.38, 27.36, 27.18, 27.16, 26.90]),
    (4.9, [19.71, 19.22, 19.09, 17.33, 15.20, 13.58, 12.75, 12.71, 12.61, 12.53, 12.44, 12.41, 12.33, 12.28, 12.21, 12.16, 12.09, 12.05, 11.95, 11.90, 11.87, 11.74, 11.43, 11.58, 12.55, 14.82, 19.74, 25.53, 26.22, 26.71, 27.21, 27.73, 28.08, 28.13, 28.25, 28.20, 27.99, 27.96, 27.73, 27.71, 27.53, 27.51, 27.24]),
    (5.1, [19.93, 19.44, 19.31, 17.52, 15.37, 13.73, 12.89, 12.86, 12.75, 12.67, 12.58, 12.55, 12.47, 12.42, 12.35, 12.29, 12.23, 12.18, 12.08, 12.04, 12.01, 11.87, 11.56, 11.71, 12.69, 14.98, 19.93, 25.77, 26.46, 26.95, 27.44, 27.89, 28.21, 28.26, 28.37, 28.29, 28.07, 28.04, 27.81, 27.79, 27.61, 27.59, 27.33]),
    (5.2, [20.05, 19.55, 19.42, 17.62, 15.46, 13.80, 12.96, 12.93, 12.82, 12.74, 12.65, 12.61, 12.54, 12.48, 12.41, 12.36, 12.30, 12.25, 12.15, 12.10, 12.07, 11.93, 11.63, 11.77, 12.76, 15.05, 20.03, 25.88, 26.58, 27.07, 27.54, 27.98, 28.28, 28.33, 28.42, 28.33, 28.23, 28.20, 27.97, 27.94, 27.76, 27.75, 27.48]),
    (5.3, [20.16, 19.66, 19.52, 17.72, 15.54, 13.88, 13.03, 13.00, 12.89, 12.81, 12.72, 12.68, 12.61, 12.55, 12.48, 12.43, 12.36, 12.32, 12.21, 12.17, 12.14, 12.00, 11.69, 11.83, 12.82, 15.13, 20.12, 26.00, 26.70, 27.19, 27.65, 28.06, 28.35, 28.39, 28.48, 28.38, 28.27, 28.24, 28.01, 27.99, 27.81, 27.79, 27.52]),
    (6, [20.93, 20.40, 20.26, 18.39, 16.12, 14.40, 13.52, 13.48, 13.37, 13.29, 13.19, 13.16, 13.08, 13.02, 12.95, 12.89, 12.82, 12.77, 12.67, 12.62, 12.59, 12.44, 12.12, 12.26, 13.28, 15.65, 20.77, 26.78, 27.49, 27.96, 28.28, 28.57, 28.78, 28.80, 28.85, 28.67, 28.57, 28.54, 28.31, 28.29, 28.10, 28.09, 27.78]),
    (7.5, [22.49, 21.92, 21.77, 19.74, 17.30, 15.44, 14.49, 14.45, 14.34, 14.24, 14.14, 14.10, 14.02, 13.95, 13.88, 13.82, 13.74, 13.69, 13.57, 13.52, 13.49, 13.33, 12.97, 13.11, 14.18, 16.67, 22.06, 28.33, 28.93, 29.16, 29.33, 29.48, 29.55, 29.55, 29.44, 29.22, 29.02, 28.99, 28.75, 28.73, 28.54, 28.53, 28.11]),
    (8.1, [23.08, 22.49, 22.33, 20.25, 17.75, 15.83, 14.85, 14.81, 14.69, 14.60, 14.49, 14.45, 14.36, 14.30, 14.22, 14.16, 14.08, 14.02, 13.90, 13.85, 13.82, 13.65, 13.28, 13.42, 14.50, 17.04, 22.52, 28.89, 29.35, 29.53, 29.67, 29.78, 29.81, 29.80, 29.67, 29.45, 29.23, 29.20, 28.96, 28.93, 28.75, 28.73, 28.45]),
    (9, [23.91, 23.30, 23.14, 20.98, 18.37, 16.38, 15.36, 15.32, 15.19, 15.09, 14.98, 14.94, 14.85, 14.78, 14.70, 14.63, 14.55, 14.49, 14.37, 14.31, 14.28, 14.10, 13.71, 13.84, 14.95, 17.56, 23.19, 29.60, 29.90, 30.03, 30.12, 30.18, 30.15, 30.12, 29.97, 29.75, 29.52, 29.49, 29.25, 29.22, 29.03, 29.02, 28.81]),
    (9.1, [24.00, 23.39, 23.22, 21.06, 18.44, 16.44, 15.41, 15.37, 15.25, 15.15, 15.03, 14.99, 14.90, 14.83, 14.75, 14.68, 14.60, 14.54, 14.42, 14.36, 14.32, 14.15, 13.75, 13.88, 15.00, 17.61, 23.26, 29.67, 29.96, 30.08, 30.17, 30.22, 30.19, 30.16, 30.00, 29.78, 29.55, 29.52, 29.28, 29.25, 29.07, 29.05, 28.85]),
    (10.1, [24.85, 24.22, 24.05, 21.80, 19.08, 16.99, 15.92, 15.87, 15.74, 15.64, 15.51, 15.47, 15.38, 15.31, 15.22, 15.15, 15.07, 15.00, 14.87, 14.81, 14.77, 14.59, 14.16, 14.28, 15.44, 18.08, 23.70, 30.08, 30.31, 30.42, 30.48, 30.52, 30.45, 30.41, 30.24, 30.01, 29.78, 29.75, 29.51, 29.49, 29.29, 29.28, 29.17]),
    (10.5, [25.17, 24.53, 24.36, 22.08, 19.31, 17.19, 16.10, 16.06, 15.92, 15.81, 15.69, 15.65, 15.55, 15.48, 15.39, 15.32, 15.23, 15.17, 15.03, 14.97, 14.93, 14.75, 14.30, 14.42, 15.58, 18.16, 23.76, 30.09, 30.34, 30.45, 30.52, 30.56, 30.50, 30.46, 30.29, 30.07, 29.84, 29.81, 29.56, 29.54, 29.35, 29.33, 29.24]),
    (10.8, [25.40, 24.75, 24.58, 22.28, 19.48, 17.34, 16.23, 16.19, 16.05, 15.94, 15.82, 15.78, 15.68, 15.60, 15.51, 15.44, 15.35, 15.29, 15.15, 15.09, 15.05, 14.86, 14.40, 14.57, 15.65, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.32, 30.09, 29.86, 29.83, 29.59, 29.56, 29.37, 29.35, 29.28]),
    (12, [26.26, 25.59, 25.41, 23.02, 20.11, 17.87, 16.71, 16.66, 16.51, 16.40, 16.27, 16.22, 16.12, 16.04, 15.94, 15.86, 15.77, 15.70, 15.56, 15.49, 15.45, 15.24, 14.75, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (12.8, [26.77, 26.09, 25.91, 23.47, 20.49, 18.18, 16.98, 16.93, 16.78, 16.66, 16.52, 16.48, 16.37, 16.28, 16.18, 16.10, 16.01, 15.94, 15.78, 15.74, 15.69, 15.48, 14.88, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (12.9, [26.83, 26.15, 25.97, 23.52, 20.53, 18.21, 17.01, 16.96, 16.81, 16.69, 16.55, 16.51, 16.40, 16.31, 16.21, 16.13, 16.04, 15.96, 15.82, 15.90, 15.71, 15.50, 14.89, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13, [26.89, 26.21, 26.03, 23.57, 20.57, 18.25, 17.04, 16.99, 16.84, 16.72, 16.58, 16.53, 16.42, 16.34, 16.24, 16.16, 16.06, 15.99, 15.89, 16.33, 15.74, 15.53, 14.90, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.1, [26.95, 26.27, 26.08, 23.63, 20.62, 18.28, 17.07, 17.02, 16.87, 16.75, 16.61, 16.56, 16.45, 16.37, 16.26, 16.18, 16.09, 16.02, 16.20, 16.34, 15.77, 15.55, 14.91, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.2, [27.01, 26.33, 26.14, 23.68, 20.66, 18.32, 17.10, 17.05, 16.90, 16.78, 16.64, 16.59, 16.48, 16.39, 16.29, 16.21, 16.11, 16.04, 16.45, 16.34, 15.79, 15.57, 14.91, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.3, [27.07, 26.38, 26.20, 23.73, 20.70, 18.35, 17.13, 17.08, 16.92, 16.80, 16.66, 16.62, 16.51, 16.42, 16.32, 16.24, 16.14, 16.07, 16.45, 16.34, 15.82, 15.59, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.4, [27.13, 26.44, 26.25, 23.78, 20.74, 18.38, 17.16, 17.11, 16.95, 16.83, 16.69, 16.64, 16.53, 16.45, 16.34, 16.26, 16.16, 16.10, 16.45, 16.34, 15.84, 15.61, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.6, [27.24, 26.55, 26.36, 23.88, 20.82, 18.45, 17.21, 17.16, 17.01, 16.88, 16.74, 16.70, 16.58, 16.50, 16.39, 16.31, 16.21, 16.24, 16.45, 16.34, 15.88, 15.65, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (13.8, [27.36, 26.66, 26.47, 23.97, 20.90, 18.51, 17.27, 17.21, 17.06, 16.93, 16.79, 16.74, 16.63, 16.54, 16.44, 16.36, 16.27, 16.63, 16.45, 16.34, 15.92, 15.68, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14, [27.46, 26.77, 26.58, 24.06, 20.98, 18.57, 17.32, 17.26, 17.11, 16.98, 16.84, 16.79, 16.68, 16.59, 16.48, 16.40, 16.39, 16.77, 16.45, 16.34, 15.96, 15.71, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14.2, [27.57, 26.87, 26.68, 24.16, 21.05, 18.63, 17.37, 17.31, 17.16, 17.03, 16.89, 16.84, 16.72, 16.63, 16.53, 16.44, 16.69, 16.71, 16.45, 16.34, 15.99, 15.74, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14.3, [27.62, 26.92, 26.73, 24.20, 21.09, 18.66, 17.39, 17.34, 17.18, 17.05, 16.91, 16.86, 16.74, 16.65, 16.55, 16.47, 16.81, 16.71, 16.45, 16.34, 16.00, 15.75, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14.5, [27.72, 27.02, 26.83, 24.29, 21.16, 18.72, 17.44, 17.38, 17.23, 17.10, 16.95, 16.90, 16.79, 16.70, 16.59, 16.55, 16.83, 16.71, 16.45, 16.34, 16.03, 15.77, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14.6, [27.78, 27.07, 26.88, 24.33, 21.19, 18.74, 17.46, 17.41, 17.25, 17.12, 16.97, 16.92, 16.81, 16.72, 16.61, 16.62, 16.83, 16.71, 16.45, 16.34, 16.04, 15.78, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (14.8, [27.88, 27.17, 26.97, 24.42, 21.26, 18.80, 17.51, 17.45, 17.29, 17.16, 17.01, 16.96, 16.85, 16.75, 16.65, 16.90, 16.83, 16.71, 16.45, 16.34, 16.06, 15.79, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (15, [27.97, 27.26, 27.07, 24.50, 21.33, 18.85, 17.55, 17.49, 17.33, 17.20, 17.05, 17.00, 16.88, 16.79, 16.73, 16.99, 16.83, 16.71, 16.45, 16.34, 16.07, 15.80, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (15.2, [28.07, 27.35, 27.16, 24.58, 21.39, 18.90, 17.59, 17.53, 17.37, 17.24, 17.09, 17.04, 16.92, 16.83, 16.95, 16.99, 16.83, 16.71, 16.45, 16.34, 16.09, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (15.4, [28.16, 27.44, 27.25, 24.66, 21.45, 18.94, 17.63, 17.57, 17.41, 17.27, 17.12, 17.07, 16.95, 16.86, 17.15, 16.99, 16.83, 16.71, 16.45, 16.34, 16.09, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (15.5, [28.20, 27.49, 27.29, 24.69, 21.48, 18.97, 17.65, 17.59, 17.42, 17.29, 17.14, 17.09, 16.97, 16.90, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (15.8, [28.34, 27.61, 27.42, 24.81, 21.57, 19.03, 17.70, 17.64, 17.48, 17.34, 17.19, 17.14, 17.01, 17.15, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (16.1, [28.46, 27.74, 27.54, 24.91, 21.65, 19.09, 17.75, 17.69, 17.52, 17.39, 17.23, 17.18, 17.12, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (16.3, [28.54, 27.82, 27.62, 24.98, 21.71, 19.13, 17.78, 17.72, 17.55, 17.42, 17.26, 17.21, 17.34, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (16.5, [28.62, 27.90, 27.70, 25.05, 21.76, 19.17, 17.81, 17.75, 17.58, 17.44, 17.28, 17.24, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (16.7, [28.70, 27.97, 27.77, 25.11, 21.81, 19.20, 17.84, 17.78, 17.61, 17.47, 17.31, 17.32, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (16.9, [28.78, 28.04, 27.84, 25.17, 21.85, 19.24, 17.86, 17.80, 17.63, 17.49, 17.38, 17.53, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (17.1, [28.85, 28.12, 27.92, 25.23, 21.90, 19.27, 17.89, 17.83, 17.65, 17.51, 17.54, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (17.3, [28.92, 28.18, 27.98, 25.29, 21.94, 19.30, 17.91, 17.85, 17.67, 17.53, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (17.5, [28.99, 28.25, 28.05, 25.35, 21.98, 19.33, 17.93, 17.87, 17.69, 17.57, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (17.7, [29.06, 28.32, 28.12, 25.40, 22.02, 19.36, 17.95, 17.89, 17.71, 17.65, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (18, [29.16, 28.41, 28.21, 25.48, 22.08, 19.39, 17.98, 17.92, 17.74, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (18.2, [29.22, 28.47, 28.27, 25.52, 22.11, 19.42, 17.99, 17.93, 17.79, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (18.4, [29.28, 28.53, 28.33, 25.57, 22.15, 19.44, 18.01, 17.95, 17.92, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (21, [29.91, 29.13, 28.91, 26.02, 22.43, 19.63, 18.21, 18.12, 18.11, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (24, [30.32, 29.51, 29.28, 26.23, 22.57, 19.70, 18.23, 18.13, 18.11, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (27, [30.54, 29.69, 29.44, 26.29, 22.59, 19.70, 18.23, 18.13, 18.11, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
    (27.6, [30.54, 29.71, 29.45, 26.29, 22.59, 19.70, 18.23, 18.13, 18.11, 17.93, 17.70, 17.63, 17.44, 17.30, 17.12, 16.99, 16.83, 16.71, 16.45, 16.34, 16.10, 15.81, 14.92, 14.76, 15.70, 18.19, 23.77, 30.09, 30.34, 30.45, 30.52, 30.57, 30.52, 30.49, 30.33, 30.11, 29.87, 29.84, 29.60, 29.57, 29.38, 29.36, 29.30]),
]

# the rubbing strake's face, port half, bow to stern (an outline: it is
# upright over its band)
STRAKE = [(30.64, 0.00), (30.61, 0.40), (30.51, 0.80), (30.26, 1.32), (29.56, 2.33), (28.76, 3.40), (27.75, 4.64), (26.90, 5.59), (25.56, 6.97), (23.90, 8.46), (22.23, 9.78), (21.55, 10.28), (20.18, 11.18), (18.82, 11.99), (16.82, 13.02), (14.77, 13.90), (12.70, 14.61), (10.80, 15.14), (9.68, 15.35), (8.05, 15.46), (6.26, 15.50), (4.96, 15.48), (3.56, 15.41), (2.29, 15.31), (0.99, 15.16), (-0.46, 14.95), (-7.25, 13.79), (-13.03, 12.92), (-19.99, 12.01), (-27.40, 11.21), (-27.82, 11.10), (-28.28, 10.85), (-28.51, 10.66), (-28.71, 10.45), (-28.99, 9.98), (-29.15, 9.47), (-29.18, 9.06), (-29.18, 3.71), (-29.35, 1.80), (-29.40, 0.00)]

# The data is the port half (rays 0 to 180 degrees); the hull is mirror-
# symmetric, so each outline is that half and its mirror image.
def both_sides(half):
    """values for rays -180..180 from those for 0..180."""
    return [v for a, v in reversed(zip(HULL_RAYS, half)) if a > 0 and a < 180] + half

RAYS = [-a for a in reversed(HULL_RAYS) if a > 0 and a < 180] + HULL_RAYS
RAY_COS = [math.cos(math.radians(a)) for a in RAYS]
RAY_SIN = [math.sin(math.radians(a)) for a in RAYS]
KNOT_Z = [z for z, rs in HULL_KNOTS]
KNOT_R = [both_sides(rs) for z, rs in HULL_KNOTS]

def slopes(i):
    """dr/dz at height knot i, each ray (Catmull-Rom on uneven spacing)."""
    a, b = max(i - 1, 0), min(i + 1, len(KNOT_Z) - 1)
    h = KNOT_Z[b] - KNOT_Z[a]
    return [(rb - ra) / h for ra, rb in zip(KNOT_R[a], KNOT_R[b])]

KNOT_M = [slopes(i) for i in range(len(KNOT_Z))]

def hull_at(z):
    """The distance along each ray at height z: a cubic between the height
    knots either side."""
    i = 0
    for j in range(len(KNOT_Z) - 1):     # the knot at or below z (not the last)
        if KNOT_Z[j] <= z:
            i = j
    h = KNOT_Z[i + 1] - KNOT_Z[i]
    t = (z - KNOT_Z[i]) / h
    a, b, c, d = 2 * t * t * t - 3 * t * t + 1, (t * t * t - 2 * t * t + t) * h, -2 * t * t * t + 3 * t * t, (t * t * t - t * t) * h
    return [a * r0 + b * m0 + c * r1 + d * m1 for r0, m0, r1, m1 in zip(KNOT_R[i], KNOT_M[i], KNOT_R[i + 1], KNOT_M[i + 1])]

def ring(rs, grow = 0.0):
    """An outline from the distances along the rays, grown outward by
    grow."""
    return [((r + grow) * c, (r + grow) * s) for r, c, s in zip(rs, RAY_COS, RAY_SIN)]

def smooth_loft(sections):
    id = cad.add_loft(sections)
    cad.set_param(id, "smooth", 1)
    cad.set_param(id, "steps", 6)
    return id

# outlines every half millimetre: the renderer shades each face flat, so a
# curved surface needs them close
LEVELS = [0.5 * i for i in range(56)] + [27.6]

DECK = 8.5          # the aft deck and cabin floor
BOW_DECK = 15.0     # the bow deck, before it ramps up
SIDE_WALL = 2.1     # hull side walls above the deck
TRANSOM_WALL = 2.0

hull_body = smooth_loft([(z, ring(hull_at(z))) for z in LEVELS])

# the sheer line (top of the hull sides), in straight runs from stern to bow
SHEER = [[-40, 10.72], [-18, 10.77], [-8, 11.75], [0, 13.26], [4.6, 14.6], [9.5, 21.1], [32, 28.1]]
RUNS = [(SHEER[i], SHEER[i + 1]) for i in range(len(SHEER) - 1)]
sheer = [cut_above(a, b) for a, b in RUNS]

# the rubbing strake: a band about 1.6 mm wide under the sheer, standing
# out from the hull (it sets the 31.00 overall width). Its face is upright
# over the band, so it is one measured outline (STRAKE) extruded, kept only
# between the sheer and a line parallel to it, 1.8 mm below measured square
# to the sheer (a little more than the strake's 1.6, to cover the hull
# bending up behind it). Where the sheer climbs steeply that is nearly
# 3 mm straight down.
STRAKE_W = 1.8

def offset_below(pts, d):
    """The polyline pts (x, z) moved d square to itself, downward, its
    runs meeting at their corners."""
    def shifted(a, b):
        dx, dz = b[0] - a[0], b[1] - a[1]
        n = math.sqrt(dx * dx + dz * dz)
        nx, nz = dz / n * d, -dx / n * d            # the downward normal
        return [a[0] + nx, a[1] + nz], [b[0] + nx, b[1] + nz]
    lines = [shifted(pts[i], pts[i + 1]) for i in range(len(pts) - 1)]
    out = [lines[0][0]]
    for (p, q), (r, s) in zip(lines[:-1], lines[1:]):
        # where the two shifted runs cross
        d1, d2 = [q[0] - p[0], q[1] - p[1]], [s[0] - r[0], s[1] - r[1]]
        den = d1[0] * d2[1] - d1[1] * d2[0]
        t = ((r[0] - p[0]) * d2[1] - (r[1] - p[1]) * d2[0]) / den
        out.append([p[0] + t * d1[0], p[1] + t * d1[1]])
    out.append(lines[-1][1])
    return out

LOW = offset_below(SHEER, STRAKE_W)
strake_outline = STRAKE + [(x, -y) for x, y in reversed(STRAKE[1:-1])]
strake = union([cad.add_outline(strake_outline, z0 = 8, z1 = 28.2)] +
               [cut_above(a, b) for a, b in RUNS] +
               [cut_below(LOW[i], LOW[i + 1]) for i in range(len(LOW) - 1)])

# the aft well: the hull shrunk by the wall thickness, from the deck up, so
# the walls follow the hull's flare (fitted rows: z0, z1, transom x,
# transom half-width, x0, aft half-width at x0, half-width at x0, bow tip
# x). Its front climbs from the deck at x = 6.4 to the bow deck at
# x = 10.3 (beside the cabin; the cabin's front fills the middle).
WELL_ROWS = [
    (8.5, 9.0, -28.60, 10.50, 1.75, 13.52, 13.62, 23.69),
    (9.0, 9.5, -29.16, 10.60, 2.00, 13.75, 13.85, 24.13),
    (10.0, 10.5, -29.17, 10.65, 2.00, 14.15, 14.25, 24.97),
    (11.5, 12.0, -29.17, 10.55, 1.50, 15.07, 15.07, 26.09),
    (12.0, 12.5, -29.17, 11.00, 1.75, 15.12, 15.12, 26.42),
    (13.0, 13.5, -29.17, 11.00, 2.25, 15.30, 15.30, 27.04),
    (14.5, 15.0, -29.17, 11.00, 3.25, 15.35, 15.35, 27.85),
]

def shrunk(r):
    z0, z1, xs, hs, x0, wa, w, xt = r
    return hull_outline(xs + TRANSOM_WALL, hs - SIDE_WALL, x0, wa - SIDE_WALL, w - SIDE_WALL, xt - SIDE_WALL)

def mid(r):
    return (r[0] + r[1]) / 2

aft_well = hole(intersect([
    cad.add_loft([(DECK, shrunk(WELL_ROWS[0]))] + [(mid(r), shrunk(r)) for r in WELL_ROWS[1:]] + [(45, shrunk(WELL_ROWS[-1]))]),
    half([6.4, DECK], [10.3, BOW_DECK], "xz", -20, 20),
]))

# the bow well: bulwarks about 2.3 mm, floor flat at 15 then ramping up to
# the bow at 0.3125 (to 20 at x = 27); its outline is a pointed arch cut
# straight across at x = 6
arch = hull_outline(6.0, 12.8, 6.0, 12.8, 12.8, 28.0)
bow_well = hole(intersect([
    cad.add_outline([p for p in arch if p[0] >= 6.0], z0 = BOW_DECK, z1 = 45),
    half([11, BOW_DECK - 0.45], [31, BOW_DECK - 0.45 + 20 * 0.3125], "xz", -20, 20),
]))

# hawsepipes: a 4.00 mm hole through the bulwark each side, along an axis
# turned 44.5 degrees out from the bow and 6 down, with a flange 0.30 proud
# of the hull out to 5.8 across: the hull grown by 0.3 inside a cylinder on
# that axis, so it follows the hull's curve. The port one is made, the
# starboard one is its mirror image.
HAWSE_C, HAWSE_AX = [25.17, 5.54, 20.72], [0.709, 0.697, -0.106]
FLANGE_LEVELS = [z for z in LEVELS if z >= 15.5 and z <= 26]
flange = intersect([
    smooth_loft([(z, ring(hull_at(z), 0.3)) for z in FLANGE_LEVELS]),
    axis_cyl(HAWSE_C, 5.8, 0, 5, HAWSE_AX),     # from mid-wall outward
])
pipe = axis_cyl(HAWSE_C, 4.0, -5, 5, HAWSE_AX, hole = True)
flanges = [flange, mirrored(flange)]
hawse = [pipe, mirrored(pipe)]

hull = union([hull_body, strake] + sheer + [aft_well, bow_well])
hull = union([hull] + flanges + hawse)    # the holes go through the flanges too

# --- stern: fishing rod holder ------------------------------------------

# a 2.0 mm bore in a 3.5 mm boss against the transom, leaning 16.2 degrees
# aft; its axis passes (-25.21, 0, 6.12)
ROD_LEAN = 16.2

def rod_x(z):
    return -25.21 - (z - 6.12) * math.tan(math.radians(ROD_LEAN))

rod_boss = tilted_cyl(rod_x(10.4), 10.4, 3.5, 4.8, ROD_LEAN)

# --- cargo box ----------------------------------------------------------

CARGO_X0 = -17.8    # 10.81 long, 12.00 wide
cargo = box([CARGO_X0, -6.0, DECK - 0.5], [CARGO_X0 + 10.81, 6.0, 15.5])

# --- cabin (bridge) -----------------------------------------------------

CAB_TOP = 38.2      # the walls run up into the roof
CEILING = 36.6
CAB_WALL = 2.0
SPLIT = 1.85        # mid-door: the rear block is narrower than the front
WINDOW_Z = 24.6     # bottom of the front window

# The cabin's faces, as planes: a point on each and its inward normal.
# Sides of the rear block: half-width 7.30 at x = -7, z = 20, widening
# 0.13 per mm forward and 0.027 per mm upward; the front block's sides are
# 8.38 at z = 20, widening upward the same. The rear face leans aft
# (x = -7.2 at z = 16, 0.04 per mm); the front leans forward (x = 11.9 at
# z = 20, 0.115 per mm).
def rear_side(s, inset):
    return half3([-7, s * (7.30 - inset), 20], [0.13, -s, 0.027])

def front_side(s, inset):
    return half3([4, s * (8.38 - inset), 20], [0, -s, 0.027])

def rear_face(inset):
    return half3([-7.2 + inset, 0, 16], [1, 0, 0.04])

def front_face(inset):
    return half3([11.9 - inset, 0, 20], [-1, 0, 0.115])

cabin_shell = union([
    intersect([box([-12, -12, DECK - 0.5], [SPLIT, 12, CAB_TOP]), rear_side(1, 0), rear_side(-1, 0), rear_face(0)]),
    intersect([box([SPLIT - 0.05, -12, DECK - 0.5], [16, 12, CAB_TOP]), front_side(1, 0), front_side(-1, 0), front_face(0)]),
    # inside, open from the floor to the ceiling
    hole(intersect([box([-12, -12, DECK], [SPLIT, 12, CEILING]), rear_side(1, CAB_WALL), rear_side(-1, CAB_WALL), rear_face(CAB_WALL)])),
    hole(intersect([box([SPLIT - 0.05, -12, DECK], [16, 12, CEILING]), front_side(1, CAB_WALL), front_side(-1, CAB_WALL), front_face(2.2)])),
    # front window 10.50 x 9.50
    box([8.0, -5.25, WINDOW_Z], [16, 5.25, WINDOW_Z + 9.5], hole = True),
    # rear window 9.00
    level_cyl([-6.5, 0, 27.3], 9.0, 6, 0, hole = True),
    # arched side doors, through both sides
    box([-2.3, -12, DECK], [6.0, 12, 27.85], hole = True),
    level_cyl([1.85, 0, 27.85], 8.3, 24, 90, hole = True),
])

# the front below the window: a block up to z = 20.5 whose back leans
# (x = 6.5 at the floor, 7.05 at z = 20); on it a half-round console
# (radius 5) with the ship's wheel on its back - a ring 10.0 across with a
# 7.0 hole, cut flat at z = 22.6, and a 2.5 hub, standing 0.5 proud. The
# gap between the console and the front wall drops to z = 20.5.
WHEEL_Z = 19.6
dash = union([
    intersect([box([6.0, -7, DECK], [14, 7, 20.5]), front_face(0), half3([6.5, 0, DECK], [1, 0, -0.0478])]),
    level_cyl([7.7, 0, WHEEL_Z], 10.0, 1.3, 0),                          # console
    union([
        level_cyl([6.8, 0, WHEEL_Z], 10.0, 0.6, 0),                      # wheel rim
        level_cyl([6.8, 0, WHEEL_Z], 7.0, 1.0, 0, hole = True),
        box([6.0, -6, 22.6], [7.5, 6, 26], hole = True),
    ]),
    level_cyl([6.8, 0, WHEEL_Z], 2.5, 0.6, 0),                           # hub
])

# rear window flange: 12.00 across, 0.30 proud of the rear wall
FLANGE_X = -7.2 - 0.04 * (27.3 - 16) - 0.25        # its outer face
rear_flange = union([
    level_cyl([FLANGE_X + 0.25, 0, 27.3], 12.0, 0.5, 0),
    level_cyl([FLANGE_X + 0.25, 0, 27.3], 9.0, 1.0, 0, hole = True),
])

cabin = union([cabin_shell, dash, rear_flange])

# --- roof ---------------------------------------------------------------

# one slab, 2.4 mm thick: its top and underside both slope at 5.5 degrees,
# it widens toward the front as the cabin does, and the cabin's walls run
# up into it
ROOF_X0, ROOF_X1 = -9.0, 14.0        # 23.00 long
ROOF_SLOPE = math.tan(math.radians(5.5))
ROOF_THICK = 2.4

def roof_top(x):
    return 38.1 + (x - ROOF_X0) * ROOF_SLOPE

A, B = ROOF_X0 - 5, ROOF_X1 + 5
roof = union([
    trapezoid(ROOF_X0, 8.7, ROOF_X1, 9.9, 30, 45),
    hole(half([A, roof_top(A)], [B, roof_top(B)], "xz", -20, 20)),                                # above the top
    hole(half([B, roof_top(B) - ROOF_THICK], [A, roof_top(A) - ROOF_THICK], "xz", -20, 20)),      # below the underside
])

# --- chimney ------------------------------------------------------------

CX = -4.0
chimney = union([
    cyl(CX, 0, 6.2, CAB_TOP - 0.4, 39.0),            # base, merging into the roof
    flared(CX, 0, 5.65, 6.0, 38.9, 44.0),            # tapered stack
    flared(CX, 0, 6.0, 7.0, 44.0, 46.0),             # flare to the top
    cyl(CX, 0, 7.0, 46.0, 48.0),                     # 7.00 top
])

# --- assemble -----------------------------------------------------------

boat = union([
    hull, rod_boss, cargo, cabin, roof, chimney,
    # holes through everything
    box([CARGO_X0 + 1.9, -4.0, 15.5 - 9.0], [CARGO_X0 + 8.9, 4.0, 16], hole = True),   # cargo box, 8.00 x 7.00, 9.00 deep
    cyl(CX, 0, 3.0, 48.0 - 11.0, 48.5, hole = True),                         # chimney, 11.00 deep
    tilted_cyl(rod_x(9.5), 9.5, 2.0, 10, ROD_LEAN, hole = True),             # rod holder
    level_cyl([-23.25, 0, 1.5], 2.0, 6.5, 0, hole = True),                   # hole in the keel
])
cad.set_color(COLOR, ids = [boat])
cad.set_name("3DBenchy (lofted)", ids = [boat])
cad.drop(ids = [boat])
cad.fit_view()
print("3DBenchy built: group", boat, "size", cad.selection()["bounds"]["size"])
