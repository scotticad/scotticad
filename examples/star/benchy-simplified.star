# benchy-simplified.star - a tugboat in the spirit of the classic 3D-printer torture
# test, built only from ScottiCAD primitives (boxes, cylinders, tubes).
#
# Run it from the Scripting console's Editor tab (or cad.run_script from
# Python). Everything ends up in one group, so a single Ctrl+Z removes it.
#
# Axes: X runs stern (-) to bow (+), Y is port/starboard, Z is up. Sizes
# are in mm; the boat is about 60 x 31 x 48, like the original.

LENGTH = 60.0      # stern to bow tip at the gunwale
BEAM = 31.0        # widest point
GUNWALE = 15.0     # height of the hull sides
DECK = 8.0         # height of the cargo deck inside the hull
WALL = 2.0         # hull and cabin wall thickness
COLOR = "#e8562a"

STERN = -LENGTH / 2
JOIN = 4.0         # where the straight sides give way to the curved bow

def block(kind, lo, hi, hole = False):
    """Adds a catalogue shape and stretches it to fill the box [lo, hi]."""
    id = cad.add_shape(kind, hole = hole)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def union(ids):
    return cad.group("union", ids = ids)

def hull_slice(z0, z1, stern, tip, half_w):
    """One horizontal layer of hull: a box for the straight aft section and
    the front half of a stretched cylinder for the bow."""
    aft = block("box", [stern, -half_w, z0], [JOIN, half_w, z1])
    bow = block("cylinder", [2 * JOIN - tip, -half_w, z0], [tip, half_w, z1])
    return [aft, bow]

def hull_shape(z0, z1, inset, steps, flare):
    """A stack of slices from z0 to z1 that widen going up (flare = 0
    gives straight sides). inset shrinks every slice, for the hollow."""
    parts = []
    for i in range(steps):
        t = (i + 1) / steps
        a = z0 + (z1 - z0) * i / steps
        b = z0 + (z1 - z0) * t
        k = 1 - flare * (1 - math.sqrt(t))   # 1 at the top, smaller below
        half_w = BEAM / 2 * k - inset
        tip = LENGTH / 2 - 8 * (1 - t) * flare - inset   # raked bow
        stern = STERN + 3 * (1 - t) * flare + inset      # raked transom
        parts += hull_slice(a, b, stern, tip, half_w)
    return union(parts)

# --- hull ---------------------------------------------------------------

outer = hull_shape(0, GUNWALE, 0, 12, 0.35)

# the raised bow bulwark: the bow's top slice continued upward, cut off
# forward of the cabin
bulwark = union([
    block("cylinder", [2 * JOIN - LENGTH / 2, -BEAM / 2, GUNWALE - 0.01], [LENGTH / 2, BEAM / 2, GUNWALE + 4]),
    block("box", [-LENGTH, -BEAM, GUNWALE - 1], [10, BEAM, GUNWALE + 5], hole = True),
])

# hollow out everything above the deck
well = hull_shape(DECK, GUNWALE + 10, WALL, 1, 0)
cad.set_hole(True, ids = [well])

# hawse holes through both sides of the bow
hawse = cad.add_shape("cylinder", hole = True)
cad.rotate("x", 90, ids = [hawse])
cad.set_bounds(min = [18, -BEAM, 14], max = [22, BEAM, 18], ids = [hawse])

hull = union([outer, bulwark, well, hawse])

# --- cabin --------------------------------------------------------------

CAB_X0, CAB_X1 = -14.0, 8.0
CAB_W = 24.0
CAB_TOP = 37.0

cabin_parts = [
    block("box", [CAB_X0, -CAB_W / 2, DECK], [CAB_X1, CAB_W / 2, CAB_TOP]),
    # hollow inside, open at the top (the roof covers it)
    block("box", [CAB_X0 + WALL, -CAB_W / 2 + WALL, DECK - 1], [CAB_X1 - WALL, CAB_W / 2 - WALL, CAB_TOP + 1], hole = True),
    # front window
    block("box", [CAB_X1 - 3, -8, 23], [CAB_X1 + 1, 8, 32], hole = True),
    # side windows (one hole cuts through both walls)
    block("box", [-10, -CAB_W, 23], [3, CAB_W, 32], hole = True),
    # rear door
    block("box", [CAB_X0 - 1, -5, DECK], [CAB_X0 + 3, 5, 31], hole = True),
]
for id in cabin_parts[2:]:
    cad.set_param(id, "radius", 1.5)
cabin = union(cabin_parts)

# --- roof and chimney ---------------------------------------------------

roof = block("box", [CAB_X0 - 4, -BEAM / 2 + 1, CAB_TOP - 1], [CAB_X1 + 3, BEAM / 2 - 1, CAB_TOP + 1.5])
cad.set_param(roof, "radius", 0.8)
cad.rotate("y", -4, ids = [roof])     # slopes down toward the stern

chimney = union([
    block("tube", [-11, -3.5, CAB_TOP], [-4, 3.5, CAB_TOP + 11]),
    block("tube", [-11.75, -4.25, CAB_TOP + 9.5], [-3.25, 4.25, CAB_TOP + 11]),   # lip
])

# --- deck fittings ------------------------------------------------------

rod_holder = block("tube", [STERN + 4, 6, DECK], [STERN + 8, 10, DECK + 7])
bollard = union([
    block("cylinder", [16, -2, DECK], [20, 2, GUNWALE + 2]),
    block("cylinder", [15, -3, GUNWALE + 2], [21, 3, GUNWALE + 3]),
])

# --- finish -------------------------------------------------------------

boat = union([hull, cabin, roof, chimney, rod_holder, bollard])
cad.set_color(COLOR, ids = [boat])
cad.set_name("benchy", ids = [boat])
cad.drop(ids = [boat])
cad.fit_view()
print("benchy built: group", boat, "size", cad.selection()["bounds"])
