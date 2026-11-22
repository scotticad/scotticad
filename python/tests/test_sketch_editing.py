"""The sketch workspace, edited as a person would with the mouse and keys,
for all three kinds of sketch (extruded, revolve, coil). Each test checks
the shape that Finish builds (its volume, size or bounds), not only the
workspace's state: a path that looks right on the board can still build
wrong (an end dropped on its start must close the path into a filled area,
not leave it open as a thin line).

Drawings sit left of x = 0 so the same one works for every kind: an
extruded sketch is its area times 10 mm; a revolve spins it round x = 0
(Pappus: 2 pi |centroid x| area); a coil does that three turns over."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402

KINDS = ["sketch", "revolve", "coil"]
SPUN = ["revolve", "coil"]
COIL_TURNS = 3
SQ = [(-20, 0), (-10, 0), (-10, 10), (-20, 10)]  # 10 x 10, centroid x = -15


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=900) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    if cad.sketch_state()["open"]:
        cad.sketch_cancel()
    cad.new_design()
    yield


class Board:
    """Screen positions of drawing millimeters, for a view set to center
    (cx, cy) at zoom px per mm (read back: a revolve's or coil's center line
    stays two thirds across)."""

    def __init__(self, cad, cx=-10.0, cy=5.0, zoom=8.0):
        cad.sketch_view(cx, cy, zoom)
        st = cad.sketch_state()
        self.bx, self.by, self.bw, self.bh = st["board"]
        (self.cx, self.cy), self.z = st["center"], st["zoom"]

    def at(self, x, y):
        return self.bx + self.bw / 2 + (x - self.cx) * self.z, self.by + self.bh / 2 - (y - self.cy) * self.z


# --- helpers ------------------------------------------------------------------


def poly(pts, **kw):
    return dict(points=[list(p) for p in pts], closed=True, **kw)


def line(pts, **kw):
    return dict(points=[list(p) for p in pts], **kw)


def rect(x0, y0, x1, y1):
    return [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]


def area_centroid(pts):
    a = cx = 0.0
    for (x0, y0), (x1, y1) in zip(pts, pts[1:] + pts[:1]):
        c = x0 * y1 - x1 * y0
        a += c
        cx += (x0 + x1) * c
    a /= 2
    return abs(a), cx / (6 * a)


def spun(kind, area, cx):
    """The volume a region of area with centroid x = cx builds as kind."""
    if kind == "sketch":
        return area * 10
    v = 2 * math.pi * abs(cx) * area
    return v * COIL_TURNS if kind == "coil" else v


def expect(kind, pts):
    return spun(kind, *area_centroid(list(pts)))


def tol(kind):
    # a revolve is cut into 96 sides; a coil a few less finely, and kept a
    # hair off its center line
    return {"sketch": 1e-6, "revolve": 0.005, "coil": 0.015}[kind]


def new(cad, kind, **view):
    cad.edit_sketch(kind=kind)
    return Board(cad, **view)


def edit(cad, i, **view):
    cad.edit_sketch(i)
    return Board(cad, **view)


def draw(cad, b, pts, mods=()):
    for p in pts:
        cad.click(*b.at(*p), mods=list(mods))


def drag(cad, b, p0, p1, mods=()):
    cad.drag(*b.at(*p0), *b.at(*p1), steps=6, mods=list(mods))


def paths(cad):
    return cad.sketch_state()["paths"]


def pts(cad, path=0):
    return [tuple(q["p"]) for q in paths(cad)[path]["points"]]


def closed(cad, path=0):
    return paths(cad)[path].get("closed", False)


def done(cad):
    i = cad.sketch_done()
    assert not cad.sketch_state()["open"]
    return cad.node(i)


def reference(cad, drawing, kind):
    """The shape the API builds from drawing (paths as add_sketch takes, or
    as sketch_state gives them), for comparing with one drawn by hand."""
    i = cad.add_sketch(drawing, kind=kind)
    n = cad.node(i)
    cad.delete([i])
    return n


def same_shape(n, r, rel=1e-6):
    assert n["volume"] == pytest.approx(r["volume"], rel=rel)
    for k in ("min", "max"):
        assert n["bounds"][k] == pytest.approx(r["bounds"][k], abs=1e-6), k


def type_into(cad, control, text):
    cad.click_control(control)
    cad.key("ctrl+a")
    cad.type_text(text)
    cad.key("enter")


def ids(cad):
    return {c["id"] for c in cad.controls()}


# --- ending and closing paths ----------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_click_the_first_point_closes_and_builds_an_area(cad, kind):
    b = new(cad, kind)
    draw(cad, b, SQ + [SQ[0]])
    st = cad.sketch_state()
    assert len(st["paths"]) == 1 and closed(cad) and pts(cad) == SQ
    assert st["drawing"] == -1 and st["tool"] == "select"
    n = done(cad)
    assert n["kind"] == kind
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))


L_LINE = [(-20, 0), (-10, 0), (-10, 10)]


@pytest.mark.parametrize("how", ["enter", "check", "last", "esc", "tool"])
@pytest.mark.parametrize("kind", KINDS)
def test_ending_a_path_open_builds_a_line(cad, kind, how):
    b = new(cad, kind)
    draw(cad, b, L_LINE)
    if how == "enter":
        cad.key("enter")
    elif how == "check":
        assert "sk-finish-open" in ids(cad)
        cad.click_control("sk-finish-open")
    elif how == "last":
        cad.click(*b.at(*L_LINE[-1]))
    elif how == "esc":
        cad.key("esc")
    else:
        cad.sketch_tool("select")
    st = cad.sketch_state()
    assert st["drawing"] == -1 and len(st["paths"]) == 1
    assert not closed(cad) and pts(cad) == L_LINE
    n = done(cad)
    same_shape(n, reference(cad, [line(L_LINE)], kind))
    if kind == "sketch":
        # two 10 mm legs 2 mm wide, flat ends, a sharp (mitered) corner
        assert n["volume"] == pytest.approx(40 * 10)
    # a line, far less than the area the closed triangle would be
    assert n["volume"] < 0.9 * expect(kind, L_LINE)


# an open path whose end is dragged onto its start must
# close (a filled area), not stay a 2 mm line
EXTRA = (-20, 5)  # the open path's end, on its left side


@pytest.mark.parametrize("which", ["end", "start"])
@pytest.mark.parametrize("tool", ["line", "bezier"])
@pytest.mark.parametrize("kind", KINDS)
def test_dragging_an_end_onto_the_start_closes_the_path(cad, kind, tool, which):
    b = new(cad, kind)
    cad.sketch_tool(tool)
    draw(cad, b, SQ[:2])
    if tool == "bezier":
        # a curve point: press on the corner and pull its handles up 3 mm
        drag(cad, b, SQ[2], (SQ[2][0], SQ[2][1] + 3))
    else:
        draw(cad, b, [SQ[2]])
    draw(cad, b, [SQ[3], EXTRA])
    cad.key("enter")
    assert not closed(cad) and len(pts(cad)) == 5
    curve = paths(cad)[0]["points"][2]
    cad.sketch_tool("select")
    if which == "end":
        drag(cad, b, EXTRA, SQ[0])
        want = SQ
    else:
        drag(cad, b, SQ[0], EXTRA)
        want = [EXTRA] + SQ[1:]
    assert closed(cad), "an end dropped on the path's other end leaves it open"
    assert pts(cad) == want, "the doubled point should go"
    if tool == "bezier":
        assert paths(cad)[0]["points"][2] == curve  # the curve point kept its handles
    # one undo step: back to the open path, and forward to the closed one
    st_paths = paths(cad)
    cad.key("ctrl+z")
    assert not closed(cad) and len(pts(cad)) == 5
    cad.key("ctrl+y")
    assert paths(cad) == st_paths
    n = done(cad)
    if tool == "line":
        assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))
    else:
        same_shape(n, reference(cad, st_paths, kind))
        # the handle bows the top out: a little more than the straight one
        assert expect(kind, want) * 1.0 < n["volume"] < expect(kind, want) * 1.3


@pytest.mark.parametrize("kind", KINDS)
def test_a_line_ended_on_its_own_start_with_the_bezier_tool_closes(cad, kind):
    # drawing with the Bezier tool and pressing (and pulling) on the first
    # point closes too, the first point getting the handles
    b = new(cad, kind)
    cad.sketch_tool("bezier")
    draw(cad, b, SQ)
    drag(cad, b, SQ[0], (SQ[0][0] + 2, SQ[0][1] - 2))
    assert closed(cad) and pts(cad) == SQ
    q0 = paths(cad)[0]["points"][0]
    assert q0["kind"] == 1
    assert q0["out"] == pytest.approx([2, -2], abs=1 / b.z) and q0["in"] == pytest.approx([-2, 2], abs=1 / b.z)
    st_paths = paths(cad)
    n = done(cad)
    same_shape(n, reference(cad, st_paths, kind))
    assert n["volume"] > 0.8 * expect(kind, SQ)


@pytest.mark.parametrize("kind", KINDS)
def test_an_end_dropped_on_another_paths_end_stays_apart(cad, kind):
    a = [(-30, 0), (-20, 0), (-20, 10)]
    b_line = [(-10, 10), (-15, 15)]
    b = new(cad, kind)
    draw(cad, b, a)
    cad.key("enter")
    draw(cad, b, b_line)
    cad.key("enter")
    cad.sketch_tool("select")
    drag(cad, b, b_line[1], a[2])  # its end onto the other's end
    drag(cad, b, b_line[0], a[0])  # and its start onto the other's start
    st = cad.sketch_state()
    assert len(st["paths"]) == 2 and not closed(cad, 0) and not closed(cad, 1)
    assert pts(cad, 0) == a and pts(cad, 1) == [a[0], a[2]]
    n = done(cad)
    same_shape(n, reference(cad, [line(a), line([a[0], a[2]])], kind))
    # not the triangle the three points would close
    assert n["volume"] != pytest.approx(expect(kind, a), rel=0.05)


@pytest.mark.parametrize("kind", KINDS)
def test_dragging_the_end_of_a_three_point_path_onto_its_start(cad, kind):
    # (with the ends together that is a line there and back, not an area:
    # it stays a line, and builds)
    b = new(cad, kind)
    draw(cad, b, [(-20, 0), (-10, 0), (-10, 10)])
    cad.key("enter")
    cad.sketch_tool("select")
    drag(cad, b, (-10, 10), (-20, 0))
    st_paths = paths(cad)
    assert len(st_paths) == 1
    n = done(cad)
    same_shape(n, reference(cad, st_paths, kind))
    assert n["volume"] > 0


@pytest.mark.parametrize("kind", KINDS)
def test_close_and_open_path(cad, kind):
    b = new(cad, kind)
    draw(cad, b, SQ)
    cad.key("enter")
    assert not closed(cad)
    cad.sketch_tool("select")
    cad.click(*b.at(-15, 10))  # a side: its two ends
    cad.key("o")
    assert closed(cad) and pts(cad) == SQ
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))
    i = n["id"]
    # opened at the selected side: that side is the one left out
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 5))  # the right side, (-10, 0) to (-10, 10)
    assert sorted(map(tuple, cad.sketch_state()["selected"])) == [(0, 1), (0, 2)]
    cad.click_control("sk-close")  # (the button, as O)
    assert not closed(cad) and pts(cad) == [SQ[2], SQ[3], SQ[0], SQ[1]]
    n = done(cad)
    same_shape(n, reference(cad, [line([SQ[2], SQ[3], SQ[0], SQ[1]])], kind))
    # closed again, then opened with the whole path selected: between its
    # last point and its first
    b = edit(cad, i)
    cad.click(*b.at(-15, 0))  # outside the modes: the whole path
    cad.key("o")
    assert closed(cad)
    cad.key("o")
    assert not closed(cad) and pts(cad) == [SQ[2], SQ[3], SQ[0], SQ[1]]
    cad.key("o")
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_close_a_path_whose_ends_already_meet(cad, kind):
    # (a design saved before ends that meet were closed: the path closes as
    # its sketch is opened, without a side of no length, and Finish saves it)
    i = cad.add_sketch([line(SQ + [SQ[0]])], kind=kind)
    assert cad.node(i)["volume"] < 0.9 * expect(kind, SQ)  # a line round it
    edit(cad, i)
    assert closed(cad) and pts(cad) == SQ, "no side of no length"
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))


# --- moving ------------------------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_move_a_point_a_side_a_path_and_a_box(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.sketch_tool("select")
    drag(cad, b, (-10, 10), (-5, 12))  # a point
    assert pts(cad) == [(-20, 0), (-10, 0), (-5, 12), (-20, 10)]
    drag(cad, b, (-15, 0), (-15, -3))  # the bottom side
    assert pts(cad) == [(-20, -3), (-10, -3), (-5, 12), (-20, 10)]
    cad.key("esc")
    cad.key("esc")  # out of Edit, nothing selected
    drag(cad, b, (-15, 5), (-20, 7))  # a drag inside moves the whole path
    assert pts(cad) == [(-25, -1), (-15, -1), (-10, 14), (-25, 12)]
    cad.sketch_tool("select")
    drag(cad, b, (-30, 11), (-5, 16))  # a box round the top two
    assert sorted(map(tuple, cad.sketch_state()["selected"])) == [(0, 2), (0, 3)]
    drag(cad, b, (-10, 14), (-10, 16))
    want = [(-25, -1), (-15, -1), (-10, 16), (-25, 14)]
    assert pts(cad) == want and closed(cad)
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_out_of_edit_a_drag_inside_moves_the_whole_path(cad, kind):
    """Outside both modes, a drag inside a path moves the whole shape, even
    when some of its points were left selected in Edit."""
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-15, 0))  # the bottom side: points 0 and 1
    cad.key("esc")  # out of Edit, the side still selected
    assert cad.sketch_state()["tool"] == "shape"
    drag(cad, b, (-15, 5), (-20, 7))
    want = [(x - 5, y + 2) for x, y in SQ]
    assert pts(cad) == want, "only the points left selected moved"
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_arrow_keys_nudge(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.click(*b.at(-15, 5))  # the whole path
    cad.key("left")
    cad.key("shift+up")
    assert pts(cad) == [(x - 1, y + 10) for x, y in SQ]
    cad.key("shift+left")
    cad.key("down")
    want = [(x - 11, y + 9) for x, y in SQ]
    assert pts(cad) == want
    # one point
    cad.key("esc")  # (nothing selected)
    cad.sketch_tool("select")
    cad.click(*b.at(*want[2]))
    cad.key("shift+right")
    want[2] = (want[2][0] + 10, want[2][1])
    assert pts(cad) == want
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


NEAR_AXIS = rect(-10, 0, -2, 8)


@pytest.mark.parametrize("kind", SPUN)
def test_no_move_takes_points_right_of_the_center_line(cad, kind):
    i = cad.add_sketch([poly(NEAR_AXIS)], kind=kind)
    b = edit(cad, i)
    cad.sketch_tool("select")
    # a point dragged past the line stops on it
    drag(cad, b, (-2, 8), (5, 8))
    assert pts(cad) == [(-10, 0), (-2, 0), (0, 8), (-10, 8)]
    cad.key("ctrl+z")
    assert pts(cad) == NEAR_AXIS
    # a side dragged past it: both ends stop on it
    drag(cad, b, (-2, 4), (8, 6))
    assert pts(cad) == [(-10, 0), (0, 2), (0, 10), (-10, 8)]
    cad.key("ctrl+z")
    # a box round the right side, dragged by one of its points
    drag(cad, b, (-4, -2), (4, 10))
    assert sorted(map(tuple, cad.sketch_state()["selected"])) == [(0, 1), (0, 2)]
    drag(cad, b, (-2, 0), (9, 0))
    assert pts(cad) == [(-10, 0), (0, 0), (0, 8), (-10, 8)]
    cad.key("ctrl+z")
    # the whole path, outside the modes: it goes no further than the line
    cad.key("esc")
    drag(cad, b, (-6, 4), (6, 4))
    assert pts(cad) == rect(-8, 0, 0, 8)
    cad.key("ctrl+z")
    # a point added with Refine and dragged past the line
    cad.sketch_tool("refine")
    drag(cad, b, (-6, 8), (7, 9))
    assert pts(cad) == NEAR_AXIS[:3] + [(0, 9)] + NEAR_AXIS[3:]
    cad.key("ctrl+z")
    assert pts(cad) == NEAR_AXIS
    cad.key("esc")
    # nudged, ten at a time
    cad.click(*b.at(-6, 4))
    cad.key("shift+right")
    assert pts(cad) == rect(-8, 0, 0, 8)
    cad.key("shift+right")
    assert pts(cad) == rect(-8, 0, 0, 8)
    assert max(p[0] for p in pts(cad)) == 0
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, rect(-8, 0, 0, 8)), rel=2 * tol(kind))
    assert n["size"][0] == pytest.approx(16, abs=0.05)


# --- deleting points -----------------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_delete_points_of_a_closed_path(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-20, 10))
    cad.key("delete")
    tri = SQ[:3]
    assert closed(cad) and pts(cad) == tri
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, tri), rel=tol(kind))
    # down to two points: it opens, a line
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 10))
    cad.key("backspace")
    assert not closed(cad) and pts(cad) == SQ[:2]
    n = done(cad)
    same_shape(n, reference(cad, [line(SQ[:2])], kind))
    if kind == "sketch":
        assert n["volume"] == pytest.approx(10 * 2 * 10)
    # down to one: the path goes, and there is nothing to build
    before = cad.node(i)
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 0))
    cad.click_control("sk-delete")
    assert paths(cad) == []
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    assert cad.sketch_state()["open"]
    cad.sketch_cancel()
    assert cad.node(i)["sketch"] == before["sketch"] and cad.node(i)["volume"] == before["volume"]


@pytest.mark.parametrize("kind", KINDS)
def test_delete_points_of_an_open_path(cad, kind):
    b = new(cad, kind)
    draw(cad, b, L_LINE)
    cad.key("enter")
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 0))  # the corner: a straight line from end to end
    cad.key("delete")
    assert pts(cad) == [L_LINE[0], L_LINE[2]] and not closed(cad)
    n = done(cad)
    same_shape(n, reference(cad, [line([L_LINE[0], L_LINE[2]])], kind))
    b = edit(cad, n["id"])
    cad.sketch_tool("select")
    cad.click(*b.at(*L_LINE[2]))
    cad.key("delete")
    assert paths(cad) == []
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    cad.sketch_cancel()


@pytest.mark.parametrize("kind", KINDS)
def test_delete_several_points_at_once(cad, kind):
    pent = [(-20, 0), (-10, 0), (-10, 10), (-15, 15), (-20, 10)]
    i = cad.add_sketch([poly(pent), poly(rect(-40, 0, -30, 5))], kind=kind)
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-15, 15))
    cad.click(*b.at(-40, 5), mods=["shift"])
    cad.key("delete")
    assert pts(cad, 0) == [(-20, 0), (-10, 0), (-10, 10), (-20, 10)]
    assert pts(cad, 1) == [(-40, 0), (-30, 0), (-30, 5)] and closed(cad, 1)
    n = done(cad)
    want = expect(kind, SQ) + expect(kind, [(-40, 0), (-30, 0), (-30, 5)])
    assert n["volume"] == pytest.approx(want, rel=tol(kind))


# --- the add and remove points tool -----------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_refine_adds_and_removes_points(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    v0 = cad.node(i)["volume"]
    b = edit(cad, i)
    cad.sketch_tool("refine")
    cad.click(*b.at(-15, 0))  # on a straight side: the shape doesn't change
    assert pts(cad) == [(-20, 0), (-15, 0), (-10, 0), (-10, 10), (-20, 10)]
    n = done(cad)
    assert n["volume"] == pytest.approx(v0, rel=1e-9) and len(n["sketch"][0]["points"]) == 5
    # added and dragged out before letting go
    b = edit(cad, i)
    cad.key("4")
    drag(cad, b, (-15, 10), (-15, 14))
    want = [(-20, 0), (-15, 0), (-10, 0), (-10, 10), (-15, 14), (-20, 10)]
    assert pts(cad) == want
    # a point clicked goes
    cad.click(*b.at(-15, 0))
    cad.click(*b.at(-10, 0))
    want = [(-20, 0), (-10, 10), (-15, 14), (-20, 10)]
    assert pts(cad) == want
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_refine_on_a_curve_keeps_its_shape(cad, kind):
    circle = [{"p": [x, y], "kind": "auto"} for x, y in [(-10, 15), (-15, 20), (-20, 15), (-15, 10)]]
    i = cad.add_sketch([{"points": circle, "closed": True}], kind=kind)
    n0 = cad.node(i)
    b = edit(cad, i)
    cad.sketch_tool("refine")
    # on the curve between (-10, 15) and (-15, 20), a little in from the
    # straight chord's middle
    x, y = -12.5 + 1.0, 17.5 + 1.0
    cad.click(*b.at(x, y), mods=["alt"])
    assert len(pts(cad)) == 5, "no point was added on the curve"
    n = done(cad)
    assert n["volume"] == pytest.approx(n0["volume"], rel=0.01)
    for k in ("min", "max"):
        assert n["bounds"][k] == pytest.approx(n0["bounds"][k], abs=0.15)


# --- curves --------------------------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_point_kinds_and_handles(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    square_v = cad.node(i)["volume"]
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 10))
    cad.click_control("sk-make-smooth")
    q = paths(cad)[0]["points"][2]
    assert q["kind"] == 1 and q["in"] == [-v for v in q["out"]] and q["out"] != [0, 0]
    smooth = paths(cad)
    n = done(cad)
    same_shape(n, reference(cad, smooth, kind))
    assert n["volume"] != pytest.approx(square_v, rel=1e-4)
    assert 0.7 * square_v < n["volume"] < 1.3 * square_v
    # Make corner: back to the square, exactly
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 10))
    cad.click_control("sk-sharp")
    q = paths(cad)[0]["points"][2]
    assert q.get("kind", 0) == 0 and q.get("in", [0, 0]) == [0, 0] and q.get("out", [0, 0]) == [0, 0]
    n = done(cad)
    assert n["volume"] == pytest.approx(square_v, rel=1e-9)
    # smooth, then split: a handle dragged moves on its own
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 10))
    cad.click_control("sk-make-smooth")
    cad.click_control("sk-split")
    q = paths(cad)[0]["points"][2]
    assert q.get("kind", 0) == 0 and q["out"] != [0, 0]
    out, inn = q["out"], q["in"]
    hx, hy = -10 + out[0], 10 + out[1]
    drag(cad, b, (hx, hy), (-14, 15))
    q = paths(cad)[0]["points"][2]
    assert q["out"] == [-4, 5] and q["in"] == inn, "a split handle moved the other"
    split = paths(cad)
    n = done(cad)
    same_shape(n, reference(cad, split, kind))
    assert 0.7 * square_v < n["volume"] < 1.5 * square_v
    # a smooth point's handle dragged: the other mirrors it
    b = edit(cad, i)
    cad.sketch_tool("select")
    cad.click(*b.at(-10, 10))
    cad.click_control("sk-make-smooth")
    q = paths(cad)[0]["points"][2]
    drag(cad, b, (-10 + q["out"][0], 10 + q["out"][1]), (-13, 13))
    q = paths(cad)[0]["points"][2]
    assert q["out"] == [-3, 3] and q["in"] == [3, -3]
    # dragged back onto its point, both handles go: a sharp corner
    drag(cad, b, (-13, 13), (-10, 10))
    q = paths(cad)[0]["points"][2]
    assert q.get("out", [0, 0]) == [0, 0]
    n = done(cad)
    assert 0.7 * square_v < n["volume"] < 1.3 * square_v


@pytest.mark.parametrize("kind", KINDS)
def test_the_smooth_curve_tool_draws_a_round(cad, kind):
    b = new(cad, kind)
    cad.key("3")
    ring = [(-10, 15), (-15, 20), (-20, 15), (-15, 10)]
    draw(cad, b, ring + [ring[0]])
    assert closed(cad) and all(q["kind"] == 2 for q in paths(cad)[0]["points"])
    n = done(cad)
    lo, hi = spun(kind, math.pi * 25 * 0.96, -15), spun(kind, math.pi * 25, -15)
    assert lo < n["volume"] < hi * (1 + tol(kind))
    assert n["size"][0] == pytest.approx(10 if kind == "sketch" else 40, abs=0.1)


# --- strokes -------------------------------------------------------------------

FRAME = rect(-30, 0, -10, 10)  # 20 x 10, centroid x = -20


@pytest.mark.parametrize("kind", KINDS)
def test_stroke_settings_on_a_closed_path(cad, kind):
    b = new(cad, kind)
    draw(cad, b, FRAME + [FRAME[0]])
    i = done(cad)["id"]
    v = lambda: cad.node(i)["volume"]  # noqa: E731

    def setting(*controls):
        edit(cad, i)
        b2 = Board(cad)
        cad.click(*b2.at(-20, 5))  # outside the modes: the whole path
        cad.click_control("sk-stroke")
        for c in controls:
            if isinstance(c, tuple):
                type_into(cad, *c)
            else:
                cad.click_control(c)
        cad.sketch_done()

    cases = [
        (("sk-as-line",), 2 * 2 * (20 + 10)),  # a 2 mm frame on the path
        (("sk-align-Inside",), 200 - 16 * 6),
        (("sk-align-Outside",), 24 * 14 - 200),
        (("sk-align-Center", "sk-join-Round"), 120 - 4 * (1 - math.pi / 4)),
        (("sk-join-Sharp", ("sk-width", "3")), 2 * 3 * 30),
        (("sk-align-Inside", ("sk-width", "1")), 200 - 18 * 8),
        (("sk-as-line",), 200),  # stroke off: the area again
    ]
    for controls, area in cases:
        setting(*controls)
        assert v() == pytest.approx(spun(kind, area, -20), rel=max(tol(kind), 0.002)), controls
    # a bevel cuts the corners: less than sharp
    setting("sk-as-line", "sk-align-Center", "sk-join-Bevel")
    p = cad.node(i)["sketch"][0]
    assert p["join"] == 2 and p["stroke"] == 2
    same_shape(cad.node(i), reference(cad, [poly(FRAME, stroke=2, join="bevel")], kind))
    assert v() == pytest.approx(spun(kind, 120 - 4 * (math.sqrt(2) - 1) ** 2, -20), rel=max(tol(kind), 0.002))


@pytest.mark.parametrize("cap,extra", [("Flat", 0), ("Round", math.pi), ("Square", 4)])
@pytest.mark.parametrize("kind", KINDS)
def test_line_ends(cad, kind, cap, extra):
    b = new(cad, kind)
    draw(cad, b, [(-30, 5), (-10, 5)])
    cad.key("enter")
    cad.key("esc")  # out of the Pen: a click picks
    cad.click(*b.at(-20, 5))
    cad.click_control("sk-stroke")
    assert "sk-as-line" not in ids(cad)  # an open path is always a line
    cad.click_control("sk-cap-" + cap)
    n = done(cad)
    assert n["volume"] == pytest.approx(spun(kind, 20 * 2 + extra, -20), rel=max(tol(kind), 0.002))


@pytest.mark.parametrize("join", ["Sharp", "Round", "Bevel"])
def test_joins_on_an_open_line(cad, join):
    b = new(cad, "sketch")
    draw(cad, b, L_LINE)
    cad.key("enter")
    cad.key("esc")
    cad.click(*b.at(-15, 0))
    cad.click_control("sk-stroke")
    cad.click_control("sk-join-" + join)
    type_into(cad, "sk-width", "4")
    st_paths = paths(cad)
    n = done(cad)
    same_shape(n, reference(cad, st_paths, "sketch"))
    # two legs 10 x 4 overlapping in a 2 x 2 square, plus the corner
    # (a bevel cuts the mitered corner square off half the width out from
    # the corner, along the bisector)
    h = 2
    want = {"Sharp": 80, "Round": 76 + math.pi, "Bevel": 80 - (h * math.sqrt(2) - h) ** 2}[join]
    assert n["volume"] == pytest.approx(want * 10, rel=0.002)


# --- typed sizes and placing points -----------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_typed_sizes_and_positions(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.click(*b.at(-15, 5))  # the whole shape
    type_into(cad, "sk-dim-w", "20")  # about its middle
    assert pts(cad) == [(-25, 0), (-5, 0), (-5, 10), (-25, 10)]
    type_into(cad, "sk-dim-h", "16")
    assert pts(cad) == [(-25, -3), (-5, -3), (-5, 13), (-25, 13)]
    cad.sketch_tool("select")
    cad.click(*b.at(-15, -3))  # the bottom side
    type_into(cad, "sk-dim-len", "10")  # its ends move together evenly
    assert pts(cad)[:2] == [(-20, -3), (-10, -3)]
    cad.click(*b.at(-25, 13))
    type_into(cad, "sk-pos-0", "-30")
    type_into(cad, "sk-pos-1", "20")
    want = [(-20, -3), (-10, -3), (-5, 13), (-30, 20)]
    assert pts(cad) == want
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_tab_types_the_next_length(cad, kind):
    b = new(cad, kind)
    cad.click(*b.at(-30, 0))
    # (toward the pointer as it snaps to the grid)
    for toward, length in [((-10, 0), "15"), ((-15, 10), "7.5")]:
        cad.mouse_move(*b.at(*toward))
        cad.key("tab")
        cad.type_text(length)
        cad.key("enter")
    want = [(-30, 0), (-15, 0), (-15, 7.5)]
    assert pts(cad) == want and cad.sketch_state()["drawing"] == 0
    cad.click(*b.at(-30, 0))
    assert closed(cad)
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_shift_draws_at_15_degree_steps_and_alt_off_the_grid(cad, kind):
    b = new(cad, kind)
    cad.click(*b.at(-30, 0))
    a = math.radians(41)  # toward 45 degrees, 10 mm away
    cad.click(*b.at(-30 + 10 * math.cos(a), 10 * math.sin(a)), mods=["shift"])
    p1 = pts(cad)[1]
    assert p1 == pytest.approx((-30 + 10 / math.sqrt(2), 10 / math.sqrt(2)), abs=1e-6)
    # off the grid with Alt; on it without
    cad.click(*b.at(-12.4, 3.3), mods=["alt"])
    p2 = pts(cad)[2]
    assert p2 == pytest.approx((-12.4, 3.3), abs=1 / b.z) and p2[0] != round(p2[0])
    cad.click(*b.at(-14.4, -2.3))
    assert pts(cad)[3] == (-14, -2)
    cad.click(*b.at(-30, 0))
    assert closed(cad)
    got = pts(cad)
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, got), rel=tol(kind))


# --- the clipboard ----------------------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_copy_paste_cut_and_duplicate(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    b = edit(cad, i)
    cad.click(*b.at(-15, 5))
    cad.key("ctrl+c")
    cad.key("ctrl+v")  # in place, selected
    st = cad.sketch_state()
    assert len(st["paths"]) == 2 and pts(cad, 1) == SQ
    assert sorted({s[0] for s in st["selected"]}) == [1]
    cad.key("shift+left")
    cad.key("shift+left")  # moved off the first
    moved = [(x - 20, y) for x, y in SQ]
    assert pts(cad, 1) == moved and pts(cad, 0) == SQ
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ) + expect(kind, moved), rel=tol(kind))
    # cut takes it away; pasted, it is back where it was
    b = edit(cad, i)
    cad.click(*b.at(-35, 5))
    cad.key("ctrl+x")
    assert len(paths(cad)) == 1 and pts(cad) == SQ
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))
    b = edit(cad, i)
    cad.key("ctrl+v")
    assert pts(cad, 1) == moved
    # Ctrl+D: a copy in place, selected, nudged away
    cad.key("ctrl+d")
    assert len(paths(cad)) == 3 and pts(cad, 2) == moved
    cad.key("shift+up")
    cad.key("shift+up")
    up = [(x, y + 20) for x, y in moved]
    assert pts(cad, 2) == up and pts(cad, 1) == moved
    n = done(cad)
    want = expect(kind, SQ) + expect(kind, moved) + expect(kind, up)
    assert n["volume"] == pytest.approx(want, rel=tol(kind))
    # pasted onto itself, a copy changes nothing: areas join
    b = edit(cad, i)
    cad.key("ctrl+a")
    cad.key("ctrl+d")
    assert len(paths(cad)) == 6
    n = done(cad)
    assert n["volume"] == pytest.approx(want, rel=tol(kind))


def test_paste_between_a_sketch_and_a_revolve(cad):
    right = rect(5, 0, 15, 10)
    ext = cad.add_sketch([poly(right), poly(SQ)], x=100)
    rev = cad.add_sketch([poly(rect(-40, 0, -35, 5))], kind="revolve")
    rev_v = cad.node(rev)["volume"]
    cad.edit_sketch(ext)
    cad.key("ctrl+a")
    cad.key("ctrl+c")
    cad.sketch_cancel()
    cad.edit_sketch(rev)
    cad.key("ctrl+v")
    # pasted together, shifted left until none is right of the line
    assert pts(cad, 1) == rect(-10, 0, 0, 10) and pts(cad, 2) == rect(-35, 0, -25, 10)
    n = done(cad)
    want = rev_v + expect("revolve", rect(-10, 0, 0, 10)) + expect("revolve", rect(-35, 0, -25, 10))
    assert n["volume"] == pytest.approx(want, rel=0.005)
    # and back: the revolve's paths go into an extruded sketch where they were
    cad.edit_sketch(rev)
    cad.key("ctrl+a")
    cad.key("ctrl+c")
    cad.sketch_cancel()
    cad.edit_sketch(ext)
    cad.key("ctrl+v")
    assert pts(cad, 2) == rect(-40, 0, -35, 5) and pts(cad, 3) == rect(-10, 0, 0, 10)
    n = done(cad)
    # SQ and the -10..0 square overlap in nothing but an edge; the -35..-25
    # one overlaps the -40..-35 by an edge too
    area = 100 + 100 + 25 + 100 + 100
    assert n["volume"] == pytest.approx(area * 10, rel=1e-6)


# --- undo and redo -----------------------------------------------------------------


def _sel_point(p):
    def f(cad, b):
        cad.sketch_tool("select")
        cad.click(*b.at(*p))
    return f


def _sel_path(cad, b):
    cad.sketch_tool("shape")
    cad.click(*b.at(-15, 5))


def _stroke(*controls):
    def f(cad, b):
        _sel_path(cad, b)
        cad.click_control("sk-stroke")
        for c in controls:
            if isinstance(c, tuple):
                type_into(cad, *c)
            else:
                cad.click_control(c)
    return f


EDITS = {
    "drag point": (_sel_point((-10, 10)), lambda cad, b: drag(cad, b, (-10, 10), (-6, 13))),
    "drag side": (lambda cad, b: cad.sketch_tool("select"), lambda cad, b: drag(cad, b, (-15, 0), (-14, -4))),
    "drag path": (lambda cad, b: None, lambda cad, b: drag(cad, b, (-15, 5), (-25, 8))),
    "nudge": (_sel_path, lambda cad, b: cad.key("shift+left")),
    "delete": (_sel_point((-20, 10)), lambda cad, b: cad.key("delete")),
    "refine add": (lambda cad, b: cad.sketch_tool("refine"), lambda cad, b: cad.click(*b.at(-15, 0))),
    "refine remove": (lambda cad, b: cad.sketch_tool("refine"), lambda cad, b: cad.click(*b.at(-10, 0))),
    "make smooth": (_sel_point((-10, 10)), lambda cad, b: cad.click_control("sk-make-smooth")),
    "open": (_sel_path, lambda cad, b: cad.key("o")),
    "stroke on": (_stroke(), lambda cad, b: cad.click_control("sk-as-line")),
    "width": (_stroke("sk-as-line"), lambda cad, b: type_into(cad, "sk-width", "3")),
    "align": (_stroke("sk-as-line"), lambda cad, b: cad.click_control("sk-align-Outside")),
    "join": (_stroke("sk-as-line"), lambda cad, b: cad.click_control("sk-join-Round")),
    "typed width": (_sel_path, lambda cad, b: type_into(cad, "sk-dim-w", "14")),
    "typed length": (lambda cad, b: (cad.sketch_tool("select"), cad.click(*b.at(-15, 0))),
                     lambda cad, b: type_into(cad, "sk-dim-len", "6")),
    "typed x": (_sel_point((-20, 10)), lambda cad, b: type_into(cad, "sk-pos-0", "-24")),
    "paste": (lambda cad, b: (_sel_path(cad, b), cad.key("ctrl+c")), lambda cad, b: cad.key("ctrl+v")),
    "cut": (_sel_path, lambda cad, b: cad.key("ctrl+x")),
    "duplicate": (_sel_path, lambda cad, b: cad.key("ctrl+d")),
}


@pytest.mark.parametrize("name", list(EDITS))
@pytest.mark.parametrize("kind", KINDS)
def test_undo_and_redo_restore_each_edit_exactly(cad, kind, name):
    setup, act = EDITS[name]
    i = cad.add_sketch([poly(SQ)], kind=kind)
    n0 = cad.node(i)
    b = edit(cad, i)
    setup(cad, b)
    before = paths(cad)
    act(cad, b)
    after = paths(cad)
    assert after != before, "the edit changed nothing"
    cad.key("ctrl+z")
    assert paths(cad) == before
    cad.key("ctrl+y")
    assert paths(cad) == after
    cad.click_control("sk-undo")  # the buttons, as the keys
    assert paths(cad) == before
    cad.click_control("sk-redo")
    assert paths(cad) == after
    if not after:  # (cut: nothing left to build)
        with pytest.raises(ScottiCADError):
            cad.sketch_done()
        cad.key("ctrl+z")
        assert paths(cad) == before
        cad.sketch_cancel()
        return
    n = done(cad)
    # the whole change is one step in the design's undo
    cad.undo()
    m = cad.node(i)
    assert m["sketch"] == n0["sketch"] and m["volume"] == pytest.approx(n0["volume"], rel=1e-12)
    cad.redo()
    assert cad.node(i)["sketch"] == n["sketch"]
    same_shape(n, reference(cad, after, kind))


# --- tools, Clear and Esc -------------------------------------------------------------


@pytest.mark.parametrize("kind", KINDS)
def test_switching_tools_mid_path(cad, kind):
    b = new(cad, kind)
    draw(cad, b, [(-30, 0), (-10, 0)])
    cad.key("2")
    drag(cad, b, (-10, 10), (-10, 13))
    cad.sketch_tool("smooth")
    draw(cad, b, [(-20, 14)])
    cad.key("1")
    draw(cad, b, [(-30, 10), (-30, 0)])
    st = cad.sketch_state()
    assert len(st["paths"]) == 1 and closed(cad)
    assert [q.get("kind", 0) for q in st["paths"][0]["points"]] == [0, 0, 1, 2, 0]
    st_paths = st["paths"]
    n = done(cad)
    same_shape(n, reference(cad, st_paths, kind))
    corners = [(-30, 0), (-10, 0), (-10, 10), (-20, 14), (-30, 10)]
    assert 0.9 * expect(kind, corners) < n["volume"] < 1.3 * expect(kind, corners)


@pytest.mark.parametrize("kind", KINDS)
def test_clear_drops_only_the_path_being_drawn(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    v0 = cad.node(i)["volume"]
    b = edit(cad, i)
    cad.key("p")
    draw(cad, b, [(-40, 0), (-30, 0), (-30, 20)])
    cad.click_control("sk-clear")
    st = cad.sketch_state()
    assert st["drawing"] == -1 and len(st["paths"]) == 1 and pts(cad) == SQ
    # still drawing: a new path
    draw(cad, b, [(-40, 0), (-35, 0), (-35, 5), (-40, 0)])
    assert len(paths(cad)) == 2 and closed(cad, 1)
    n = done(cad)
    assert n["volume"] == pytest.approx(v0 + expect(kind, [(-40, 0), (-35, 0), (-35, 5)]), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_the_esc_ladder(cad, kind):
    b = new(cad, kind)
    draw(cad, b, [(-30, 0), (-10, 0)])
    cad.key("esc")  # ends the line, still in the Pen
    st = cad.sketch_state()
    assert st["drawing"] == -1 and st["tool"] == "line" and not closed(cad)
    # a click now starts another path, not more of that one
    draw(cad, b, [(-30, 10), (-20, 10)])
    cad.key("esc")
    assert len(paths(cad)) == 2
    cad.key("esc")  # out of the Pen
    assert cad.sketch_state()["tool"] == "shape"
    cad.click(*b.at(-20, 0))
    cad.click_control("sk-stroke")
    assert "sk-width" in ids(cad)
    cad.key("esc")  # the stroke settings close first, the selection stays
    assert "sk-width" not in ids(cad) and cad.sketch_state()["selected"]
    cad.key("esc")
    assert cad.sketch_state()["selected"] == []
    n = done(cad)
    same_shape(n, reference(cad, [line([(-30, 0), (-10, 0)]), line([(-30, 10), (-20, 10)])], kind))


# --- changing a placed shape; Cancel; nothing to build ------------------------------


def test_a_moved_and_turned_sketch_keeps_its_place(cad):
    i = cad.add_sketch([poly(rect(0, 0, 20, 10))])
    cad.move(dx=30, dy=5, ids=[i])
    cad.rotate("z", 90, ids=[i])
    b0 = cad.node(i)["bounds"]
    # a point added on a side: a change, but the shape is the same
    b = edit(cad, i, cx=10, cy=5)
    cad.sketch_tool("refine")
    cad.click(*b.at(10, 0))
    n = done(cad)
    assert len(n["sketch"][0]["points"]) == 5
    for k in ("min", "max"):
        assert n["bounds"][k] == pytest.approx(b0[k], abs=1e-6), k
    # a corner pulled 5 mm further along the drawing's x: only the side of
    # the bounds it is on moves (the drawing's x is the world's y, turned)
    b = edit(cad, i, cx=10, cy=5)
    cad.sketch_tool("select")
    drag(cad, b, (20, 10), (25, 10))
    n = done(cad)
    b1 = n["bounds"]
    changed = [(k, a) for k in ("min", "max") for a in range(3) if abs(b1[k][a] - b0[k][a]) > 1e-6]
    assert changed in ([("max", 1)], [("min", 1)]), changed
    assert abs(b1[changed[0][0]][1] - b0[changed[0][0]][1]) == pytest.approx(5)


def test_a_resized_sketch_keeps_its_size(cad):
    i = cad.add_sketch([poly(rect(0, 0, 20, 10))])
    cad.set_size(x=40, z=20, ids=[i])
    b0 = cad.node(i)["bounds"]
    assert b0["max"][0] - b0["min"][0] == pytest.approx(40)
    b = edit(cad, i, cx=10, cy=5)
    cad.sketch_tool("select")
    drag(cad, b, (20, 10), (25, 10))  # the top right corner 5 mm right
    n = done(cad)
    b1 = n["bounds"]
    # stretched twice as wide still: 10 mm more, the rest where it was
    assert b1["max"][0] == pytest.approx(b0["max"][0] + 10)
    assert b1["min"][0] == pytest.approx(b0["min"][0])
    assert b1["min"][1] == pytest.approx(b0["min"][1]) and b1["max"][1] == pytest.approx(b0["max"][1])
    assert b1["max"][2] == pytest.approx(20)
    # area 2 x (200 + 25), 20 tall
    assert n["volume"] == pytest.approx(2 * 225 * 20, rel=1e-6)


@pytest.mark.parametrize("kind", SPUN)
def test_a_moved_spun_sketch_keeps_its_place(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    cad.move(dx=50, dy=-20, dz=5, ids=[i])
    c0 = cad.node(i)["bounds"]
    b = edit(cad, i)
    cad.sketch_tool("select")
    drag(cad, b, (-20, 10), (-25, 12))  # out and up
    want = [(-20, 0), (-10, 0), (-10, 10), (-25, 12)]
    n = done(cad)
    c1 = n["bounds"]
    for a in range(2):  # its axis stayed put
        assert (c1["min"][a] + c1["max"][a]) / 2 == pytest.approx((c0["min"][a] + c0["max"][a]) / 2, abs=0.05)
    assert c1["min"][2] == pytest.approx(c0["min"][2], abs=1e-6)
    assert n["volume"] == pytest.approx(expect(kind, want), rel=tol(kind))


@pytest.mark.parametrize("kind", KINDS)
def test_cancel_leaves_the_design_as_it_was(cad, kind):
    i = cad.add_sketch([poly(SQ)], kind=kind)
    cad.move(dx=7, ids=[i])
    n0 = cad.node(i)
    b = edit(cad, i)
    cad.sketch_tool("select")
    drag(cad, b, (-10, 10), (-3, 14))
    cad.key("delete")
    cad.key("p")
    draw(cad, b, [(-40, 0), (-30, 0)])
    cad.click_control("sk-cancel")
    assert not cad.sketch_state()["open"]
    n = cad.node(i)
    assert n["sketch"] == n0["sketch"] and n["bounds"] == n0["bounds"] and n["volume"] == n0["volume"]
    assert len(cad.nodes()) == 1
    # a new drawing cancelled adds nothing
    b = new(cad, kind)
    draw(cad, b, SQ + [SQ[0]])
    cad.sketch_cancel()
    assert len(cad.nodes()) == 1


@pytest.mark.parametrize("kind", KINDS)
def test_finish_with_nothing_to_build_stays_open(cad, kind):
    b = new(cad, kind)
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    assert cad.sketch_state()["open"]
    # a single point is no path
    draw(cad, b, [(-20, 0)])
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    assert cad.sketch_state()["open"]
    cad.sketch_cancel()
    assert cad.nodes() == []


@pytest.mark.parametrize("kind", SPUN)
def test_finish_on_a_drawing_right_of_the_center_line_stays_open(cad, kind):
    # clicked right of the line, every point lands on it: an outline with
    # nothing left of the line to spin
    b = new(cad, kind)
    draw(cad, b, [(5, 0), (5, 10), (8, 5)])
    assert [p[0] for p in pts(cad)] == [0, 0, 0]
    cad.click(*b.at(0, 0))
    assert closed(cad)
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    st = cad.sketch_state()
    assert st["open"] and len(st["paths"]) == 1
    # once there is something left of it, Finish builds
    cad.key("p")
    draw(cad, b, SQ + [SQ[0]])
    n = done(cad)
    assert n["volume"] == pytest.approx(expect(kind, SQ), rel=tol(kind))


# --- the picture to trace ------------------------------------------------------------


@pytest.mark.parametrize("kind", ["sketch", "revolve"])
def test_drawing_over_a_picture_builds_the_same_shape(cad, kind, picture):
    # without a picture
    b = new(cad, kind)
    draw(cad, b, SQ + [SQ[0]])
    plain = done(cad)
    cad.new_design()
    # with one: loaded, moved, scaled, levelled, turned and faded first
    b = new(cad, kind)
    cad.answer_dialogs(("open", picture))
    cad.click_control("sk-image")
    st = cad.sketch_state()
    assert st["image_panel"] and st["image"] is not None
    im0 = st["image"]
    # a drag on the board moves the picture, and draws nothing
    drag(cad, b, (-20, 0), (-15, 2))
    im = cad.sketch_state()["image"]
    assert (im["x"], im["y"]) == pytest.approx((im0["x"] + 5, im0["y"] + 2), abs=1 / b.z)
    assert paths(cad) == []
    cad.click_control("sk-img-scale")
    draw(cad, b, [(-30, 0), (-10, 0)])
    type_into(cad, "sk-img-dist", "40")
    assert cad.sketch_state()["image"]["width"] == pytest.approx(im["width"] * 2, rel=1e-6)
    cad.click_control("sk-img-level")
    draw(cad, b, [(-30, 0), (-10, 1)])
    assert cad.sketch_state()["image"]["angle"] == pytest.approx(-math.degrees(math.atan2(1, 20)), abs=1e-6)
    cad.click_control("sk-img-turn")
    type_into(cad, "sk-img-opacity", "35")
    assert cad.sketch_state()["image"]["opacity"] == pytest.approx(0.35)
    assert paths(cad) == []  # none of those clicks drew
    cad.click_control("sk-img-done")
    assert not cad.sketch_state()["image_panel"]
    draw(cad, b, SQ + [SQ[0]])
    assert closed(cad) and pts(cad) == SQ
    traced = done(cad)
    same_shape(traced, plain)
    # kept with the sketch, and still nothing of the shape
    edit(cad, traced["id"])
    assert cad.sketch_state()["image"]["opacity"] == pytest.approx(0.35)
    cad.sketch_cancel()


def test_picture_panel_esc_and_tools(cad, picture):
    b = new(cad, "sketch")
    cad.answer_dialogs(("open", picture))
    cad.click_control("sk-image")
    cad.click_control("sk-img-scale")
    cad.click(*b.at(-10, 0))
    cad.key("esc")  # the scale part done goes first
    st = cad.sketch_state()
    assert st["image_panel"] and st["image_mode"] == "move"
    cad.key("esc")
    assert not cad.sketch_state()["image_panel"]
    # Esc took nothing of the drawing tool away: a click draws
    assert cad.sketch_state()["tool"] == "line"
    draw(cad, b, SQ + [SQ[0]])
    n = done(cad)
    assert n["volume"] == pytest.approx(1000)
