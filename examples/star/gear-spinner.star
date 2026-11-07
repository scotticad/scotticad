# gear-spinner.star - a planetary gear fidget spinner that prints in one
# piece, already assembled: no supports, no bearing, no glue.
#
# HOW IT WORKS. It is ScottiCAD's Planetary Gear Set shape, dressed up to be
# held and spun: a SUN gear in the middle, PLANET gears round it, and a RING
# gear round them, all printed in place with a gap (CLEAR) between their
# teeth. Pinch the sun between thumb and finger and flick the ring: the
# planets roll round the sun, each spinning on its own axis, and the ring
# turns on them. Hold the ring instead and turn the sun to watch the planets
# go round the other way.
#
# The teeth are herringbone (a V of two helices), so nothing can slide out
# up or down: the gears hold each other together. That is what lets the set
# print in place and work straight off the plate. The ring has SUN + 2 x
# PLANET teeth; the planets go where their teeth fit both the sun's and the
# ring's (evenly spaced here, because sun + ring = 60 teeth divides by 5).
#
# The script adds a raised boss with a finger dish on top of the sun, so a
# finger on it doesn't rub the turning parts; a dot on each planet, to show
# it spinning; and finger grips round the ring's edge.
#
# PRINTING. Print it as it comes, flat on the bed: 0.2 mm layers, 3 walls,
# 15% infill, no brim. Turn on your slicer's elephant's-foot compensation
# (or lower the first layer's squish): a first layer squashed outward is
# what welds print-in-place gears together. When it's done, hold the sun and
# work the ring back and forth until it turns freely. If the gears are
# fused, raise CLEAR by 0.1 mm; if they rattle, lower it. If the sun can be
# pulled far enough off centre for a planet to skip a tooth, lower TIP_GAP.
# Each part has its own colour (COLORS) for printers that change filament
# by part.
#
# About 77 mm across; fits any bed. Sizes are in mm.

MODULE = 1.5          # tooth size: the pitch circle's diameter per tooth
SUN = 18              # teeth on the sun
PLANET = 12           # teeth on each planet
PLANETS = 5           # how many planets
THICK = 10.0          # the gears' thickness
HELIX = 30            # the herringbone's angle, in degrees
CLEAR = 0.4           # gap between meshing teeth (0.3-0.5 for most printers)
TIP_GAP = 0.3         # gap between each tooth's tip and the root it turns in: how far
                      # the sun can be pulled toward a planet, so the smaller, the
                      # better centred it stays (0.25-0.4)
RIM = 5.0             # the ring's wall outside its tooth roots
COLORS = {"Ring Gear": "#2f3640", "Sun Gear": "#e1b12c", "Planet Gear": "#c8ccd0"}

BOSS_H = 1.2          # the sun's boss above the gears
DISH = 1.0            # the finger dish's depth
DOT = 1.2             # the planets' dots: radius, and depth below their tops
GRIPS = 24            # finger grips round the ring
GRIP_R = 2.5          # each grip's radius
GRIP_DEPTH = 1.2
SIDES = 48

RING_TEETH = SUN + 2 * PLANET
R_SUN = MODULE * SUN / 2                       # pitch radii
R_PLANET = MODULE * PLANET / 2
ORBIT = R_SUN + R_PLANET                       # the planets' axes from the middle
SUN_ROOT = R_SUN - MODULE - TIP_GAP            # the bottoms of the teeth
PLANET_ROOT = R_PLANET - MODULE - TIP_GAP
RING_OUT = MODULE * RING_TEETH / 2 + MODULE + TIP_GAP + RIM

# --- helpers ----------------------------------------------------------------

def shape(kind, lo, hi, hole = False):
    id = cad.add_shape(kind, hole = hole)
    if kind == "cylinder":
        cad.set_param(id, "sides", SIDES)
    elif kind == "sphere":
        cad.set_param(id, "steps", SIDES)
    cad.set_bounds(min = lo, max = hi, ids = [id])
    return id

def disc(x, y, r, z0, z1, hole = False):
    return shape("cylinder", [x - r, y - r, z0], [x + r, y + r, z1], hole)

def dress(id, extras):
    """Group a part with what it gains and loses, keeping its name and colour."""
    name = cad.node(id)["name"]
    g = cad.group("union", ids = [id] + extras)
    cad.set_name(name, ids = [g])
    cad.set_color(COLORS[name], ids = [g])
    return g

# --- the gears ----------------------------------------------------------------

set_id = cad.add_shape("planetary", x = 0, y = 0, z = 0)
for key, value in (("module", MODULE), ("sun", SUN), ("planet", PLANET), ("planets", PLANETS),
                   ("thickness", THICK), ("helix", HELIX), ("herringbone", 1),
                   ("clearance", CLEAR), ("tipgap", TIP_GAP), ("rim", RIM), ("bore", 0)):
    cad.set_param(set_id, key, value)
parts = cad.ungroup(ids = [set_id])   # the ring, the sun, then the planets
ring, sun, planets = parts[0], parts[1], parts[2:]

# where the planets went: at whole multiples of 360 / (sun + ring teeth)
# degrees, as near evenly spaced as that allows (exactly, here)
slots = SUN + RING_TEETH
angles = [2 * math.pi * math.round(i * slots / len(planets)) / slots for i in range(len(planets))]

# --- the sun: a boss with a finger dish ----------------------------------------

boss_r = SUN_ROOT - 1.5
boss = disc(0, 0, boss_r, THICK - 0.01, THICK + BOSS_H)
dish_r = boss_r - 1.5
dish = shape("sphere", [-dish_r, -dish_r, THICK + BOSS_H - DISH], [dish_r, dish_r, THICK + BOSS_H + DISH], hole = True)
sun = dress(sun, [boss, dish])

# --- the planets: a dot each, toward the rim ----------------------------------

dotted = []
for i in range(len(planets)):
    off = ORBIT + PLANET_ROOT / 2   # half-way out from the planet's axis
    x, y = off * math.cos(angles[i]), off * math.sin(angles[i])
    dot = disc(x, y, DOT, THICK - DOT, THICK + 1, hole = True)
    dotted.append(dress(planets[i], [dot]))

# --- the ring: finger grips round its edge ----------------------------------

grips = []
for i in range(GRIPS):
    a = 2 * math.pi * (i + 0.5) / GRIPS
    c = RING_OUT + GRIP_R - GRIP_DEPTH
    grips.append(disc(c * math.cos(a), c * math.sin(a), GRIP_R, -1, THICK + 1, hole = True))
ring = dress(ring, grips)

spinner = cad.group("bundle", ids = [ring, sun] + dotted)
cad.set_name("Gear spinner", ids = [spinner])
b = cad.node(spinner)["bounds"]
print("gear spinner: %d planets, ring %d teeth, %s mm across" %
      (len(planets), RING_TEETH, str(math.round((b["max"][0] - b["min"][0]) * 10) / 10)))
