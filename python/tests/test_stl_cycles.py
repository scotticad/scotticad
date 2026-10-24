"""Export, import, mutate, export again, several times over: a solid goes
out as an STL (or OBJ), comes into a fresh design, is cut and added to
with booleans, an imported SVG extrusion, mirrors and an intersection, and
goes out again. The shape is tracked exactly in a grid of 1 mm cells
(everything lies on integer coordinates, which float32 files keep exactly),
so the volume is known after every step, and every file is checked by an
independent reader."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402
from test_stl import check_stl  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


def cells_of(lo, hi):
    return {(x, y, z) for x in range(lo[0], hi[0]) for y in range(lo[1], hi[1]) for z in range(lo[2], hi[2])}


def extent(cells):
    lo = [min(c[k] for c in cells) for k in range(3)]
    hi = [max(c[k] for c in cells) + 1 for k in range(3)]
    return lo, hi


def near(a, b):
    return math.isclose(a, b, rel_tol=1e-9, abs_tol=1e-6)


def box(cad, lo, hi, hole=False):
    i = cad.add_shape("box", hole=hole)
    cad.set_bounds(lo, hi, ids=[i])
    return i


def test_export_import_mutate_cycles(cad, tmp_path):
    cad.new_design()
    cells = cells_of((0, 0, 0), (12, 12, 12))
    box(cad, [0, 0, 0], [12, 12, 12])
    for cycle in range(6):
        ext = "stl" if cycle % 2 == 0 else "obj"
        path = str(tmp_path / f"cycle-{cycle}.{ext}")
        cad.export(path)
        if ext == "stl":
            check_stl(path, len(cells), f"cycle {cycle} export")

        # into a fresh design, standing on the build plate with its bottom
        # centre at a chosen point: a whole-number move of every cell
        lo, hi = extent(cells)
        w, d = hi[0] - lo[0], hi[1] - lo[1]
        tx, ty = 3 * cycle - 5, 7 - 2 * cycle  # the new minimum corner
        cad.new_design()
        i = cad.import_file(path, x=tx + w / 2, y=ty + d / 2)
        b = cad.node(i)["bounds"]
        assert b["min"] == [tx, ty, 0] and b["max"] == [tx + w, ty + d, hi[2] - lo[2]], (cycle, b)
        cells = {(x + tx - lo[0], y + ty - lo[1], z - lo[2]) for x, y, z in cells}
        assert near(cad.node(i)["volume"], len(cells)), cycle

        # mutate: a box added over one corner, a hole through the middle
        lo, hi = extent(cells)
        add_lo = [hi[0] - 4, hi[1] - 4, 2]
        add_hi = [hi[0] + 5, hi[1] + 3, hi[2] + 2]
        added = box(cad, add_lo, add_hi)
        hole_lo = [lo[0] + 3, lo[1] + 3, -2]
        hole_hi = [lo[0] + 6, lo[1] + 7, hi[2] + 4]
        hole = box(cad, hole_lo, hole_hi, hole=True)
        members = [i, added, hole]
        cells |= cells_of(add_lo, add_hi)
        if cycle % 3 == 1:
            # another import joins in: an SVG rectangle extruded, standing
            # on the build plate at a chosen point
            svg = tmp_path / f"strip-{cycle}.svg"
            svg.write_text('<svg width="20mm" height="4mm" viewBox="0 0 20 4"><rect width="20" height="4"/></svg>')
            sx, sy = lo[0] - 6, lo[1] + 1  # overlapping the solid's side
            s = cad.import_file(str(svg), x=sx + 10, y=sy + 2, height=5)
            sb = cad.node(s)["bounds"]
            assert sb["min"] == [sx, sy, 0] and sb["max"] == [sx + 20, sy + 4, 5], sb
            members.append(s)
            cells |= cells_of((sx, sy, 0), (sx + 20, sy + 4, 5))
        cells -= cells_of(hole_lo, hole_hi)  # a union's holes cut every solid in it, the strip too
        g = cad.group("union", ids=members)
        assert near(cad.node(g)["volume"], len(cells)), f"cycle {cycle} after the union"

        if cycle % 2 == 1:
            # flipped left to right about its own centre (a whole number
            # twice over, so the cells stay on the grid)
            gb = cad.node(g)["bounds"]
            twice = gb["min"][0] + gb["max"][0]
            assert twice == int(twice)
            cad.mirror("x", ids=[g])
            cells = {(int(twice) - 1 - x, y, z) for x, y, z in cells}
            assert near(cad.node(g)["volume"], len(cells)), f"cycle {cycle} after the mirror"
        if cycle == 4:
            # clipped to a block: an intersection on top of everything
            lo, hi = extent(cells)
            clip_lo = [lo[0] + 2, lo[1], 1]
            clip_hi = [hi[0] - 3, hi[1] + 9, hi[2] - 1]
            clip = box(cad, clip_lo, clip_hi)
            g = cad.group("intersect", ids=[g, clip])
            cells &= cells_of(clip_lo, clip_hi)
            assert near(cad.node(g)["volume"], len(cells)), "after the clip"
        assert cells, "the solid vanished"

    # the last result once more as an STL, read back as the same solid
    path = str(tmp_path / "final.stl")
    cad.export(path)
    check_stl(path, len(cells), "final")
    cad.new_design()
    i = cad.import_file(path)
    assert near(cad.node(i)["volume"], len(cells))
    # and after all that a file that saves and reopens as itself
    design = str(tmp_path / "final.scotticad")
    cad.save(design)
    cad.new_design()
    cad.open(design)
    assert near(sum(n["volume"] for n in cad.nodes()), len(cells))
