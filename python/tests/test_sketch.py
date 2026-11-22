"""Sketch shapes: made from a drawing by the API, and drawn in the sketch
workspace with the mouse."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


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


def square(x0, y0, x1, y1, **kw):
    return dict(points=[[x0, y0], [x1, y0], [x1, y1], [x0, y1]], closed=True, **kw)


def near(a, b, tol=1e-6):
    return abs(a - b) <= tol


class Board:
    """Screen positions of drawing millimeters, for a view set to center
    (cx, cy) at zoom px per mm."""

    def __init__(self, cad, cx=0.0, cy=0.0, zoom=8.0):
        cad.sketch_view(cx, cy, zoom)
        st = cad.sketch_state()
        self.bx, self.by, self.bw, self.bh = st["board"]
        # (as set, but a revolve's or coil's center line stays two thirds
        # across: its x is the board's)
        (self.cx, self.cy), self.z = st["center"], st["zoom"]

    def at(self, x, y):
        return self.bx + self.bw / 2 + (x - self.cx) * self.z, self.by + self.bh / 2 - (y - self.cy) * self.z


def test_add_and_set_sketch(cad):
    i = cad.add_sketch([square(0, 0, 20, 10), square(5, 2, 10, 8)], x=30)
    n = cad.node(i)
    assert n["kind"] == "sketch" and len(n["sketch"]) == 2
    b = n["bounds"]
    assert near(b["min"][0], 30) and near(b["max"][0], 50) and near(b["max"][2], 10)
    # the inner square joins the outer: no hole
    assert near(n["volume"], 200 * 10, 1e-6)
    # a line with a width, round ends, and curve points given by handles
    cad.set_sketch(i, [square(0, 0, 20, 10),
                       {"points": [{"p": [0, 20], "out": [5, 5], "kind": "smooth"}, [20, 20]], "stroke": 3, "cap": "round"}])
    n = cad.node(i)
    assert n["sketch"][1]["stroke"] == 3 and n["sketch"][1]["cap"] == 1
    # the old square stayed put: the line's round end reaches 1.5 past it
    assert n["bounds"]["min"][0] == pytest.approx(30 - 1.5, abs=0.01) and n["bounds"]["min"][1] == pytest.approx(0, abs=1e-6)
    r = cad.add_sketch([square(-10, 0, 0, 10)], kind="revolve")
    rn = cad.node(r)
    assert rn["volume"] == pytest.approx(math.pi * 100 * 10, rel=0.01)
    cad.set_param(r, "sweep", 180)
    assert cad.node(r)["volume"] == pytest.approx(math.pi * 100 * 10 / 2, rel=0.01)


@pytest.mark.parametrize("paths,kind", [
    ([], "sketch"),
    ([square(1, 0, 10, 10)], "revolve"),
    ([{"points": [[0, 0], [1, "x"]]}], "sketch"),
    ([{"points": [[0, 0], [1, 1]], "join": "wobbly"}], "sketch"),
    ([{"points": [[0, 0], [1, 1]], "stroke": -1}], "sketch"),
    ([square(0, 0, 1, 1)], "cube"),
])
def test_add_sketch_refuses(cad, paths, kind):
    with pytest.raises(ScottiCADError):
        cad.add_sketch(paths, kind=kind)
    assert cad.nodes() == []


def test_the_sketch_tools_have_their_own_row(cad):
    libs = ["Basic Shapes", "Parametric", "Hardware & Enclosures", "Mechanical", "Computers & Networking", "All Shapes"]
    for lib in libs:
        cad.click_control("library")
        cad.click_control("lib-" + lib)
        ctl = {}
        for c in cad.controls():
            if c["id"].startswith("tile-"):
                assert c["id"] not in ctl, (lib, c["id"])  # each tool once, none as a library tile
            ctl[c["id"]] = c["rect"]
        row = [ctl["tile-" + k] for k in ("sketch", "revolve", "coil")]
        # one row, under the tool tabs and above the library list
        assert len({r[1] for r in row}) == 1 and row[0][0] < row[1][0] < row[2][0], lib
        assert ctl["tab-notes"][1] + ctl["tab-notes"][3] <= row[0][1], lib
        assert row[0][1] + row[0][3] <= ctl["library"][1], lib
    cad.click_control("library")
    cad.click_control("lib-Basic Shapes")
    # dragged onto the plate, a tool opens the workspace for its kind
    x, y, w, h = rect = {c["id"]: c["rect"] for c in cad.controls()}["tile-revolve"]
    px, py = cad.project(0, 0, 0)
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(x + w / 2 - 40, y + h / 2 + 20)
    cad.mouse_move(px, py)
    cad.mouse_up(px, py)
    st = cad.sketch_state()
    assert st["open"] and st["kind"] == "revolve", rect
    cad.sketch_cancel()
    # clicked, then the plate clicked
    cad.click_control("tile-coil")
    assert cad.ui_state()["placing"]
    cad.click(px, py)
    st = cad.sketch_state()
    assert st["open"] and st["kind"] == "coil"
    cad.sketch_cancel()


def test_a_hole_path_from_the_api(cad):
    i = cad.add_sketch([square(0, 0, 20, 20), square(5, 5, 15, 15, hole=True)])
    n = cad.node(i)
    assert n["volume"] == pytest.approx((400 - 100) * 10, rel=1e-6)
    assert n["sketch"][1]["hole"] and not n["sketch"][0].get("hole", False)
    with pytest.raises(ScottiCADError):
        cad.add_sketch([square(0, 0, 20, 20, hole="yes")])


def test_hole_mirror_and_turn_buttons(cad):
    i = cad.add_sketch([square(0, 0, 20, 20), square(5, 5, 15, 15)])
    cad.edit_sketch(id=i)
    b = Board(cad)
    # the rulers take the board's left and bottom edges
    bx, by, bw, bh = cad.sketch_state()["board"]
    assert bx > 20 and by + bh <= 900 - 20  # (the window is 900 high)
    cad.click(*b.at(10, 15))  # the inner square's top
    assert [p for p, _ in cad.sketch_state()["selected"]] == [1, 1, 1, 1]
    cad.click_control("sk-hole")
    assert cad.sketch_state()["paths"][1]["hole"]
    cad.key("h")  # the key switches it back
    assert not cad.sketch_state()["paths"][1].get("hole", False)
    cad.key("h")
    # Mirror and turn: a quarter turn of the outer square about its middle
    # leaves it where it was; a mirror copy to the right doubles it
    cad.click(*b.at(0, 10))
    assert {p for p, _ in cad.sketch_state()["selected"]} == {0}
    cad.click_control("sk-xform")
    cad.click_control("sk-turn-left")
    assert sorted(q["p"] for q in cad.sketch_state()["paths"][0]["points"]) == [[0, 0], [0, 20], [20, 0], [20, 20]]
    cad.click_control("sk-mirror-right")
    st = cad.sketch_state()
    assert len(st["paths"]) == 3 and {p for p, _ in st["selected"]} == {2}
    assert sorted(q["p"] for q in st["paths"][2]["points"]) == [[20, 0], [20, 20], [40, 0], [40, 20]]
    # typed angle: turns the copy by it
    cad.click_control("sk-turn-by")
    cad.type_text("45")
    cad.key("enter")
    xs = [q["p"][0] for q in cad.sketch_state()["paths"][2]["points"]]
    assert min(xs) == pytest.approx(30 - 10 * math.sqrt(2)) and max(xs) == pytest.approx(30 + 10 * math.sqrt(2))
    cad.key("ctrl+z")  # (undo clears the selection; the panel shows again with one)
    cad.click(*b.at(40, 10))
    cad.click_control("sk-flip-x")  # a square flipped is itself
    assert sorted(q["p"] for q in cad.sketch_state()["paths"][2]["points"]) == [[20, 0], [20, 20], [40, 0], [40, 20]]
    cad.key("esc")  # closes the panel
    assert not any(c["id"] == "sk-flip-x" for c in cad.controls())
    i = cad.sketch_done()
    # two 20 mm squares side by side, a 10 mm hole in the first
    assert cad.node(i)["volume"] == pytest.approx((800 - 100) * 10, rel=1e-6)


def test_draw_an_outline_and_one_inside_it(cad):
    cad.edit_sketch(kind="sketch")
    st = cad.sketch_state()
    assert st["open"] and st["tool"] == "line"
    b = Board(cad)
    for p in [(-20, -10), (20, -10), (20, 10), (-20, 10), (-20, -10)]:
        cad.click(*b.at(*p))
    st = cad.sketch_state()
    assert len(st["paths"]) == 1 and st["paths"][0]["closed"] and st["drawing"] == -1
    assert st["tool"] == "select"  # closing goes on to the Edit tools
    # another inside it, with the smooth curve tool
    cad.sketch_tool("smooth")
    for p in [(-10, -5), (0, -5), (0, 5), (-10, 5), (-10, -5)]:
        cad.click(*b.at(*p))
    st = cad.sketch_state()
    assert [p["closed"] for p in st["paths"]] == [True, True]
    assert all(q["kind"] == 2 for q in st["paths"][1]["points"])
    i = cad.sketch_done()
    assert not cad.sketch_state()["open"]
    n = cad.node(i)
    assert n["kind"] == "sketch" and n["size"][0] == pytest.approx(40) and n["size"][2] == pytest.approx(10)
    # one shape inside another joins it: no hole
    assert n["volume"] == pytest.approx(800 * 10, rel=1e-6)


def test_bezier_and_mixed_tools(cad):
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    cad.sketch_tool("bezier")
    x0, y0 = b.at(-20, 0)
    cad.drag(x0, y0, x0, y0 - 80, steps=4)  # handles pulled up 10 mm
    st = cad.sketch_state()
    q = st["paths"][0]["points"][0]
    assert q["kind"] == 1 and q["out"] == [0, 10] and q["in"] == [0, -10]
    # carry on with straight lines (switching tools keeps the path)
    cad.sketch_tool("line")
    for p in [(20, 0), (20, -20), (-20, -20), (-20, 0)]:
        cad.click(*b.at(*p))
    st = cad.sketch_state()
    assert len(st["paths"]) == 1 and st["paths"][0]["closed"] and len(st["paths"][0]["points"]) == 4
    cad.sketch_done()
    n = cad.nodes()[0]
    assert n["bounds"]["max"][1] > 0.5  # the curve bulges above the straight top


def test_lines_join_and_take_a_width(cad):
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    for p in [(0, 0), (10, 0)]:
        cad.click(*b.at(*p))
    cad.key("enter")
    for p in [(10, 10), (0, 10)]:
        cad.click(*b.at(*p))
    cad.key("enter")
    assert len(cad.sketch_state()["paths"]) == 2
    # a line from the first's end to the second's end is a third line, as
    # lines drawn apart stay apart
    cad.click(*b.at(10, 0))
    cad.click(*b.at(10, 10))
    cad.key("enter")
    st = cad.sketch_state()
    assert len(st["paths"]) == 3 and all(len(p["points"]) == 2 for p in st["paths"])
    assert not any(p.get("closed", False) for p in st["paths"])
    cad.sketch_cancel()
    # a box drawn in one go, closed, as a 2 mm line with round corners
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    for p in [(0, 0), (10, 0), (10, 10), (0, 10)]:
        cad.click(*b.at(*p))
    cad.key("enter")
    cad.sketch_tool("select")
    cad.click(*b.at(5, 10))
    cad.click_control("sk-close")
    cad.click_control("sk-stroke")
    cad.click_control("sk-as-line")
    cad.click_control("sk-join-Round")
    p = cad.sketch_state()["paths"][0]
    assert p["closed"] and p["stroke"] == 2 and p["join"] == 1
    i = cad.sketch_done()
    n = cad.node(i)
    # a frame 2 mm wide, its outside corners rounded
    area = 12 * 12 - 8 * 8 - 4 * (1 - math.pi / 4)
    assert n["size"][0] == pytest.approx(12) and n["volume"] == pytest.approx(area * 10, rel=0.002)


def test_refine_undo_and_cancel(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10)])
    before = cad.node(i)
    cad.edit_sketch(i)
    b = Board(cad)
    cad.sketch_tool("refine")
    cad.click(*b.at(0, -10))  # a point on the bottom
    assert len(cad.sketch_state()["paths"][0]["points"]) == 5
    cad.sketch_tool("select")
    cad.drag(*b.at(0, -10), *b.at(0, -20), steps=4)  # pulled down 10
    assert [0, -20] in [q["p"] for q in cad.sketch_state()["paths"][0]["points"]]
    cad.key("ctrl+z")
    assert [0, -10] in [q["p"] for q in cad.sketch_state()["paths"][0]["points"]]
    cad.key("ctrl+y")
    cad.sketch_cancel()
    assert cad.node(i)["sketch"] == before["sketch"]
    # done with a change: one undo step
    cad.edit_sketch(i)
    b = Board(cad)
    cad.sketch_tool("refine")
    cad.click(*b.at(-10, 10))  # remove a corner: a triangle
    cad.sketch_done()
    assert len(cad.node(i)["sketch"][0]["points"]) == 3
    cad.undo()
    assert cad.node(i)["sketch"] == before["sketch"]


def test_double_click_opens_a_sketch(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10)])
    x, y = cad.project(0, 0, 10)
    cad.click(x, y, double=True)
    st = cad.sketch_state()
    assert st["open"] and st["id"] == i
    cad.sketch_cancel()
    # not a box
    cad.add_shape("box", x=60)
    x, y = cad.project(60, 0, 20)
    cad.click(x, y, double=True)
    assert not cad.sketch_state()["open"]


def test_done_refuses_nothing_to_build(cad):
    # (points can't go right of a revolve's center line, so an
    # empty board is the drawing with nothing to spin)
    cad.edit_sketch(kind="revolve")
    with pytest.raises(ScottiCADError):
        cad.sketch_done()
    assert cad.sketch_state()["open"]
    cad.sketch_cancel()
    assert cad.nodes() == []


def test_copy_paste_between_sketches(cad):
    a = cad.add_sketch([square(-10, 0, 0, 10)])
    r = cad.add_sketch([square(-20, 0, -15, 5)], kind="revolve")
    cad.edit_sketch(a)
    cad.key("ctrl+a")
    cad.key("ctrl+c")
    cad.sketch_cancel()
    cad.edit_sketch(r)
    cad.key("ctrl+v")
    assert len(cad.sketch_state()["paths"]) == 2
    cad.sketch_done()
    assert len(cad.node(r)["sketch"]) == 2


def test_screenshot_of_the_workspace(cad, tmp_path):
    cad.edit_sketch(kind="revolve")
    p = tmp_path / "ws.png"
    cad.screenshot(str(p))
    assert p.stat().st_size > 1000
    ids = {c["id"] for c in cad.controls()}
    # a new drawing opens in the Pen's tools
    assert {"sk-back", "sk-line", "sk-bezier", "sk-smooth", "sk-done", "sk-cancel", "sk-close", "sk-stroke",
            "sk-copy", "sk-paste", "sk-delete", "sk-undo", "sk-redo", "sk-dims", "sk-home", "sk-fit"} <= ids
    cad.key("esc")
    ids = {c["id"] for c in cad.controls()}
    assert {"sk-pen", "sk-edit"} <= ids and "sk-line" not in ids


def test_modes_and_keys(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10)])
    cad.edit_sketch(i)
    assert cad.sketch_state()["tool"] == "shape"
    for key, tool in [("p", "line"), ("3", "smooth"), ("esc", "shape"), ("p", "smooth"),
                      ("a", "select"), ("4", "refine"), ("esc", "shape"), ("a", "refine"), ("2", "bezier")]:
        cad.key(key)
        assert cad.sketch_state()["tool"] == tool, key


def test_select_shapes_and_type_sizes(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10), square(20, 0, 30, 5)])
    cad.edit_sketch(i)
    b = Board(cad)
    cad.click(*b.at(0, 0))  # inside the first: the whole path
    st = cad.sketch_state()
    assert sorted(map(tuple, st["selected"])) == [(0, 0), (0, 1), (0, 2), (0, 3)]
    # its width typed: stretched about its middle
    cad.click_control("sk-dim-w")
    cad.type_text("40")
    cad.key("enter")
    pts = [q["p"] for q in cad.sketch_state()["paths"][0]["points"]]
    assert pts[0] == [-20, -10] and pts[1] == [20, -10]
    # a stretch's length typed: both its ends move
    cad.key("a")
    cad.click(*b.at(20, -5))  # the right side, from (20, -10) to (20, 10)
    cad.click_control("sk-dim-len")
    cad.type_text("30")
    cad.key("enter")
    pts = [q["p"] for q in cad.sketch_state()["paths"][0]["points"]]
    assert pts[1] == [20, -15] and pts[2] == [20, 15]
    # Open path at the selected stretch: the other three sides stay
    cad.key("o")
    p = cad.sketch_state()["paths"][0]
    assert not p.get("closed", False) and [q["p"] for q in p["points"]][0] == [20, 15] and [q["p"] for q in p["points"]][-1] == [20, -15]


def test_type_a_length_while_drawing(cad):
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    cad.click(*b.at(0, 0))
    cad.mouse_move(*b.at(10, 0))
    cad.key("tab")
    cad.type_text("7.5")
    cad.key("enter")
    st = cad.sketch_state()
    assert [q["p"] for q in st["paths"][0]["points"]] == [[0, 0], [7.5, 0]]
    assert st["drawing"] == 0


def test_stroke_alignment_and_extrude_settings(cad):
    i = cad.add_sketch([square(0, 0, 20, 20, stroke=2, align="outside")])
    n = cad.node(i)
    assert n["sketch"][0]["align"] == 2
    assert n["size"][0] == pytest.approx(24) and n["volume"] == pytest.approx((24 * 24 - 20 * 20) * 10, rel=1e-6)
    j = cad.add_sketch([square(-10, -10, 10, 10)], x=60)
    cad.set_param(j, "topscale", 50)
    assert cad.node(j)["volume"] == pytest.approx(10 / 3 * (400 + 100 + 200), rel=1e-6)
    cad.set_param(j, "topscale", 100)
    cad.set_param(j, "twist", 90)
    assert cad.node(j)["volume"] == pytest.approx(4000, rel=0.01)


def test_smooth_points_and_handles(cad):
    # four smooth points on a square make a near-circle (each handle a
    # quarter of the way between its point's neighbours)
    i = cad.add_sketch([{"points": [{"p": [10, 0], "kind": "auto"}, {"p": [0, 10], "kind": "auto"},
                                    {"p": [-10, 0], "kind": "auto"}, {"p": [0, -10], "kind": "auto"}], "closed": True}])
    v = cad.node(i)["volume"] / 10
    assert math.pi * 100 * 0.96 < v < math.pi * 100
    # Make smooth gives linked, equal handles; dragging one back onto its
    # point and Split makes a corner of it
    cad.edit_sketch(i)
    b = Board(cad)
    cad.key("a")
    cad.click(*b.at(10, 0))
    cad.click_control("sk-make-smooth")
    q = cad.sketch_state()["paths"][0]["points"][0]
    assert q["kind"] == 1 and q["in"] == [-x for x in q["out"]]
    cad.click_control("sk-split")
    q = cad.sketch_state()["paths"][0]["points"][0]
    hx, hy = b.at(10 + q["out"][0], q["out"][1])
    cad.drag(hx, hy, *b.at(10, 0), steps=4)
    q = cad.sketch_state()["paths"][0]["points"][0]
    assert q.get("out", [0, 0]) == [0, 0]


def test_an_end_dropped_on_the_start_closes_the_path(cad):
    """An open path whose end is dragged onto its start becomes an outline
    (a filled area), the doubled point gone; and one whose ends are already
    together (as a design saved before could have) closes as its sketch
    opens, without a side of no length."""
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    for p in [(0, 0), (20, 0), (20, 10), (0, 10), (-5, 5)]:
        cad.click(*b.at(*p))
    cad.key("enter")
    assert not cad.sketch_state()["paths"][0].get("closed", False)
    cad.sketch_tool("select")
    cad.drag(*b.at(-5, 5), *b.at(0, 0), steps=6)
    p = cad.sketch_state()["paths"][0]
    assert p.get("closed") and [q["p"] for q in p["points"]] == [[0, 0], [20, 0], [20, 10], [0, 10]]
    i = cad.sketch_done()
    assert cad.node(i)["volume"] == pytest.approx(200 * 10)
    # ends already together, still open (a design saved before): closed as
    # its sketch opens, and Finish saves that
    j = cad.add_sketch([{"points": [[0, 0], [20, 0], [20, 10], [0, 10], [0, 0]]}])
    cad.edit_sketch(j)
    p = cad.sketch_state()["paths"][0]
    assert p.get("closed") and len(p["points"]) == 4
    cad.sketch_done()
    assert cad.node(j)["volume"] == pytest.approx(200 * 10)


def test_clear_drops_the_path_being_drawn(cad):
    i = cad.add_sketch([{"points": [[0, 0], [10, 0]]}])
    cad.edit_sketch(i)
    b = Board(cad)
    cad.key("p")
    cad.click(*b.at(10, 0))  # a new line from the first's end (they stay apart)
    cad.click(*b.at(10, 10))
    cad.click(*b.at(-20, -20))
    cad.click_control("sk-clear")
    st = cad.sketch_state()
    assert st["drawing"] == -1 and len(st["paths"]) == 1
    assert [q["p"] for q in st["paths"][0]["points"]] == [[0, 0], [10, 0]]
    # a new path, cleared; nothing else changes
    cad.click(*b.at(-20, -20))
    cad.click(*b.at(-10, -20))
    cad.click_control("sk-clear")
    assert len(cad.sketch_state()["paths"]) == 1
    # with nothing being drawn it does nothing
    cad.click_control("sk-clear")  # shown, but disabled
    assert len(cad.sketch_state()["paths"]) == 1 and cad.sketch_state()["can_undo"]


def test_coil(cad):
    # a 4 mm square wire at radius 8…12, 3 turns rising 6 a turn: a spring
    i = cad.add_sketch([square(-12, 0, -8, 4)], kind="coil")
    n = cad.node(i)
    assert n["kind"] == "coil" and n["size"][2] == pytest.approx(4 + 3 * 6)
    assert n["volume"] == pytest.approx(3 * 2 * math.pi * 16 * 10, rel=0.005)
    cad.set_param(i, "turns", 1)
    assert cad.node(i)["size"][2] == pytest.approx(4 + 6)
    # drawing it: the side right of the center line takes no points
    cad.edit_sketch(kind="coil")
    b = Board(cad)
    cad.click(*b.at(-10, 0))
    cad.click(*b.at(5, 0))  # past the center line: kept on it
    st = cad.sketch_state()
    assert [q["p"] for q in st["paths"][0]["points"]] == [[-10, 0], [0, 0]]
    cad.sketch_cancel()


def test_new_sketch_opens_at_the_home_scale(cad):
    # the board as it opens and after Home: a 10 mm square is
    # 273 px across, the small squares 1 mm, the origin in the middle
    cad.edit_sketch(kind="sketch")
    st = cad.sketch_state()
    assert st["zoom"] == pytest.approx(27.3) and st["center"] == [0, 0]
    cad.sketch_view(40, 40, 3)  # away from home...
    cad.click_control("sk-home")  # ...and back
    st = cad.sketch_state()
    assert st["zoom"] == pytest.approx(27.3) and st["center"] == [0, 0]
    cad.sketch_cancel()
    # a drawing that fits there opens there too; a big one opens fitted
    i = cad.add_sketch([square(0, 0, 10, 10)])
    cad.edit_sketch(i)
    assert cad.sketch_state()["zoom"] == pytest.approx(27.3)
    cad.sketch_cancel()
    j = cad.add_sketch([square(0, 0, 300, 200)])
    cad.edit_sketch(j)
    assert cad.sketch_state()["zoom"] < 27.3
    cad.sketch_cancel()


def test_refine_drags_the_new_point(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10)])
    cad.edit_sketch(i)
    b = Board(cad)
    cad.sketch_tool("refine")
    cad.drag(*b.at(0, -10), *b.at(0, -15), steps=4)
    assert [0, -15] in [q["p"] for q in cad.sketch_state()["paths"][0]["points"]]


def pts(cad, path=0):
    return [q["p"] for q in cad.sketch_state()["paths"][path]["points"]]


def type_into(cad, control, text):
    cad.click_control(control)
    cad.type_text(text)
    cad.key("enter")


def test_nothing_gets_right_of_the_center_line(cad):
    # by any route: a typed position, size or length, a nudge, or a paste
    ext = cad.add_sketch([square(5, 0, 15, 10)], x=80)
    cad.edit_sketch(ext)
    cad.key("ctrl+a")
    cad.key("ctrl+c")
    cad.sketch_cancel()
    for kind in ("revolve", "coil"):
        i = cad.add_sketch([square(-10, 0, -2, 8)], kind=kind)
        cad.edit_sketch(i)
        b = Board(cad)
        # a point's X typed past the line stops on it
        cad.key("a")
        cad.click(*b.at(-2, 0))
        type_into(cad, "sk-pos-0", "7")
        assert [0, 0] in pts(cad)
        cad.key("ctrl+z")
        # the width typed: the shape moves left rather than cross the line
        cad.key("esc")
        cad.click(*b.at(-6, 4))
        type_into(cad, "sk-dim-w", "30")
        xs = [p[0] for p in pts(cad)]
        assert max(xs) == 0 and min(xs) == pytest.approx(-30)
        # nudged right, it stays
        cad.key("right")
        assert max(p[0] for p in pts(cad)) == 0
        # pasted from an extruded sketch, where it was right of the line
        cad.key("ctrl+v")
        st = cad.sketch_state()
        assert len(st["paths"]) == 2
        xs = [p[0] for p in pts(cad, 1)]
        assert max(xs) == 0 and min(xs) == pytest.approx(-10)
        # a typed length while drawing
        cad.key("p")
        cad.click(*b.at(-40, 20))
        cad.mouse_move(*b.at(-30, 20))
        cad.key("tab")
        cad.type_text("100")
        cad.key("enter")
        assert pts(cad, 2) == [[-40, 20], [0, 20]]
        cad.sketch_cancel()


def test_typed_values_out_of_range_are_refused(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10)])
    cad.edit_sketch(i)
    b = Board(cad)
    before = pts(cad)
    cad.key("a")
    cad.click(*b.at(10, 10))
    type_into(cad, "sk-pos-0", "1e20")
    assert pts(cad) == before and not cad.sketch_state()["can_undo"]
    cad.key("f")  # (a drawing that far away would not fit back into view)
    b = Board(cad)
    cad.key("esc")
    cad.key("esc")
    cad.click(*b.at(0, 0))
    for text in ("0", "-5", "1e12", "wide"):
        type_into(cad, "sk-dim-w", text)
        assert pts(cad) == before, text
    cad.sketch_cancel()


def test_undo_while_drawing_carries_on(cad):
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    for p in [(0, 0), (10, 0), (10, 10)]:
        cad.click(*b.at(*p))
    cad.key("ctrl+z")
    st = cad.sketch_state()
    assert pts(cad) == [[0, 0], [10, 0]] and st["drawing"] == 0
    # the next click adds to the same path
    cad.click(*b.at(20, 0))
    assert pts(cad) == [[0, 0], [10, 0], [20, 0]]
    # back past the first point: nothing left, and nothing being drawn
    for _ in range(3):
        cad.key("ctrl+z")
    st = cad.sketch_state()
    assert st["paths"] == [] and st["drawing"] == -1 and not st["can_undo"]
    cad.key("ctrl+y")
    assert pts(cad) == [[0, 0]] and cad.sketch_state()["drawing"] == 0
    # O is for a finished path: while one is being drawn it does nothing
    cad.click(*b.at(10, 0))
    cad.click(*b.at(10, 10))
    cad.key("o")
    st = cad.sketch_state()
    assert not st["paths"][0].get("closed", False) and st["drawing"] == 0
    cad.sketch_cancel()


def test_a_click_on_a_handle_leaves_it(cad):
    i = cad.add_sketch([{"points": [{"p": [0, 0], "out": [3.3, 4.7], "in": [-3.3, -4.7], "kind": "smooth"}, [20, 0], [10, 15]],
                         "closed": True}])
    cad.edit_sketch(i)
    b = Board(cad)
    cad.key("a")
    cad.click(*b.at(0, 0))
    cad.click(*b.at(3.3, 4.7))  # off the grid: a drag would snap it
    st = cad.sketch_state()
    assert st["paths"][0]["points"][0]["out"] == [3.3, 4.7] and not st["can_undo"]
    cad.sketch_cancel()


def test_the_sketch_is_not_thrown_away_by_an_import(cad, tmp_path):
    # a file dropped on the window (or Import, New, Open) waits for the
    # sketch to be finished or cancelled, never replacing the drawing unasked
    box = cad.add_shape("box")
    stl = str(tmp_path / "box.stl")
    cad.export(stl)
    cad.delete([box])
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    cad.click(*b.at(0, 0))
    cad.click(*b.at(10, 0))
    cad.key("enter")
    cad.drop_files(stl)
    st = cad.sketch_state()
    assert st["open"] and pts(cad) == [[0, 0], [10, 0]]
    assert cad.nodes() == []  # nothing imported behind the board
    cad.sketch_cancel()
    cad.drop_files(stl)
    assert len(cad.nodes()) == 1


def test_box_select_cut_and_duplicate(cad):
    i = cad.add_sketch([square(-30, -10, -10, 10), square(10, -10, 30, 10)])
    cad.edit_sketch(i)
    b = Board(cad)
    sel = lambda: sorted(map(tuple, cad.sketch_state()["selected"] or []))  # noqa: E731
    # outside the modes, a box on empty space picks every shape it touches
    cad.drag(*b.at(-40, -20), *b.at(-25, -5), steps=4)  # round one corner of the first
    assert sel() == [(0, 0), (0, 1), (0, 2), (0, 3)]
    # with Shift it adds, and a box over nothing clears
    cad.drag(*b.at(5, -20), *b.at(15, -5), steps=4, mods=["shift"])
    assert len(sel()) == 8
    cad.drag(*b.at(-5, 30), *b.at(5, 40), steps=4)
    assert sel() == []
    # in Edit, a box picks the points inside it, and Shift-click takes one out
    cad.key("a")
    cad.drag(*b.at(-35, -15), *b.at(-5, -5), steps=4)
    assert sel() == [(0, 0), (0, 1)]
    cad.click(*b.at(-10, -10), mods=["shift"])
    assert sel() == [(0, 0)]
    # the arrows nudge by the grid's step, ten with Shift
    cad.key("left")
    cad.key("shift+up")
    assert pts(cad)[0] == [-31, 0]
    # Delete removes the point: a triangle is left
    cad.key("delete")
    assert len(pts(cad)) == 3
    # Ctrl+X takes the whole of a path with a point selected, to paste back
    cad.click(*b.at(30, 10))
    cad.key("ctrl+x")
    assert len(cad.sketch_state()["paths"]) == 1
    cad.key("ctrl+v")
    st = cad.sketch_state()
    assert len(st["paths"]) == 2 and pts(cad, 1) == [[10, -10], [30, -10], [30, 10], [10, 10]]
    # Ctrl+D leaves a copy in place, selected
    cad.key("ctrl+d")
    st = cad.sketch_state()
    assert len(st["paths"]) == 3 and pts(cad, 2) == pts(cad, 1)
    assert sorted({s[0] for s in st["selected"]}) == [2]
    # each of those was one undo step
    for want in (2, 1, 2, 2):  # the copy, the paste, the cut, the deleted point
        cad.key("ctrl+z")
        assert len(cad.sketch_state()["paths"]) == want
    assert len(pts(cad)) == 4
    cad.sketch_cancel()


def test_a_setting_clicked_again_is_no_undo_step(cad):
    i = cad.add_sketch([square(-10, -10, 10, 10, stroke=2, join="round")])
    cad.edit_sketch(i)
    b = Board(cad)
    cad.click(*b.at(10, 0))
    cad.click_control("sk-stroke")
    cad.click_control("sk-join-Round")  # as it is
    cad.click_control("sk-align-Center")
    assert not cad.sketch_state()["can_undo"]
    cad.click_control("sk-join-Bevel")
    st = cad.sketch_state()
    assert st["can_undo"] and st["paths"][0]["join"] == 2
    cad.key("ctrl+z")
    st = cad.sketch_state()
    assert not st["can_undo"] and st["paths"][0]["join"] == 1
    cad.sketch_cancel()


def _png_rows(data):
    import struct
    import zlib
    pos, idat, w, h, bpp = 8, b"", 0, 0, 4
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        chunk = data[pos + 8:pos + 8 + n]
        if kind == b"IHDR":
            w, h, depth, ctype = struct.unpack(">IIBB", chunk[:10])
            bpp = 3 if ctype == 2 else 4
        elif kind == b"IDAT":
            idat += chunk
        pos += 12 + n
    raw, stride, rows, prev = zlib.decompress(idat), w * bpp, [], bytearray(w * bpp)
    for y in range(h):
        f, line = raw[y * (stride + 1)], bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b, c = prev[i], prev[i - bpp] if i >= bpp else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + (a + b) // 2) & 255
            elif f == 4:
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                line[i] = (line[i] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append([tuple(line[x * bpp:x * bpp + 3]) for x in range(w)])
        prev = line
    return rows


def _preview_shape_pixels(cad):
    """How many pixels of the preview card's view (right of its buttons)
    are strongly colored, as the shape is and the card and its text aren't;
    None when there is no card."""
    import base64
    import time
    card = [c for c in cad.controls() if c["id"] == "sk-preview"]
    if not card:
        return None
    for _ in range(30):  # (built off the main thread)
        cad.ui_state()
        time.sleep(0.05)
    shot = cad.screenshot()
    rows, dpi = _png_rows(base64.b64decode(shot["png"])), shot["dpi"]
    x, y, w, h = card[0]["rect"]
    n = 0
    for py in range(int((y + 10) * dpi), int((y + h - 10) * dpi), 3):
        for px in range(int((x + 70) * dpi), int((x + w - 10) * dpi), 3):
            if max(rows[py][px]) - min(rows[py][px]) > 50:
                n += 1
    return n


def test_spun_boards_keep_the_center_line_two_thirds_across(cad):
    """A revolve's or coil's center line stays two thirds across the board
    (the right third, not spun, is blanked and holds the preview) however
    the view is set, zoomed or fitted; an extruded sketch's board moves
    freely."""
    for kind in ["revolve", "coil"]:
        cad.edit_sketch(kind=kind)
        for view in [None, (50, 10, 8), (-200, 0, 2.5)]:
            if view:
                cad.sketch_view(*view)
            st = cad.sketch_state()
            bx, by, bw, bh = st["board"]
            axis = bx + bw / 2 + (0 - st["center"][0]) * st["zoom"]
            assert axis == pytest.approx(bx + bw * 2 / 3), (kind, view)
        card = [c for c in cad.controls() if c["id"] == "sk-preview"][0]["rect"]
        assert card[0] >= bx + bw * 2 / 3, "the preview is over the drawing side"
        cad.key("+")  # zoom: still there
        st = cad.sketch_state()
        assert bx + bw / 2 - st["center"][0] * st["zoom"] == pytest.approx(bx + bw * 2 / 3)
        cad.sketch_cancel()
    cad.edit_sketch(kind="sketch")
    cad.sketch_view(50, 10, 8)
    assert cad.sketch_state()["center"] == [50, 10]
    cad.sketch_cancel()


def test_revolve_and_coil_show_a_live_preview(cad):
    # (an extruded sketch has none)
    profile = [square(-12, 0, -4, 10)]
    for kind in ["revolve", "coil"]:
        i = cad.add_sketch(profile, kind=kind)
        cad.edit_sketch(id=i)
        ids = {c["id"] for c in cad.controls()}
        assert {"sk-preview", "sk-preview-reset", "sk-preview-in", "sk-preview-out"} <= ids, kind
        n = _preview_shape_pixels(cad)
        assert n > 50, f"{kind}: the preview shows nothing ({n} colored pixels)"
        # dragging in it turns the view, and leaves the drawing as it was
        before = cad.sketch_state()["paths"]
        r = [c for c in cad.controls() if c["id"] == "sk-preview"][0]["rect"]
        cx, cy = r[0] + r[2] * 0.7, r[1] + r[3] / 2
        cad.drag(cx, cy, cx + 40, cy + 20, steps=4)
        assert cad.sketch_state()["paths"] == before
        cad.click_control("sk-preview-reset")
        cad.sketch_cancel()
    # an empty revolve board says what to do; an extruded sketch has no card
    cad.edit_sketch(kind="revolve")
    assert _preview_shape_pixels(cad) < 5
    cad.sketch_cancel()
    cad.edit_sketch(kind="sketch")
    assert _preview_shape_pixels(cad) is None


def test_a_picture_to_trace(cad, picture):
    """A picture under the drawing: loaded from the Picture button, scaled
    from two points a known distance apart, levelled, kept with the sketch
    (shown again when it is edited), and never part of the shape."""
    cad.edit_sketch(kind="sketch")
    b = Board(cad)
    cad.answer_dialogs(("open", picture))
    cad.click_control("sk-image")
    st = cad.sketch_state()
    im = st["image"]
    assert st["image_panel"] and im is not None
    assert im["pixels"] == [300, 600]
    assert near(im["height"], im["width"] * im["pixels"][1] / im["pixels"][0], 1e-9)
    # set scale: two points 20 mm apart (as placed) are really 50 mm
    cad.click_control("sk-img-scale")
    cad.click(*b.at(-10, 0))
    cad.click(*b.at(10, 0))
    assert cad.sketch_state()["image_mode"] == "scale"
    cad.click_control("sk-img-dist")
    cad.key("ctrl+a")
    cad.type_text("50")
    cad.key("enter")
    im2 = cad.sketch_state()["image"]
    assert near(im2["width"], im["width"] * 2.5, 1e-6)
    # the first point stayed put: the center moved away from it 2.5 times as far
    assert near(im2["x"], -10 + (im["x"] + 10) * 2.5, 1e-6) and near(im2["y"], im["y"] * 2.5, 1e-6)
    # level: two points that should be level turn it, about their middle
    cad.click_control("sk-img-level")
    cad.click(*b.at(-10, 0))
    cad.click(*b.at(10, 2))
    assert near(cad.sketch_state()["image"]["angle"], -math.degrees(math.atan2(2, 20)), 1e-6)
    # undo takes the turn back
    cad.key("ctrl+z")
    assert near(cad.sketch_state()["image"]["angle"], 0, 1e-9)
    # a tool picked goes back to drawing: the board draws, not drags the picture
    cad.sketch_tool("line")
    assert not cad.sketch_state()["image_panel"]
    for p in [(0, 0), (10, 0), (10, 10), (0, 10), (0, 0)]:
        cad.click(*b.at(*p))
    i = cad.sketch_done()
    n = cad.node(i)
    assert n["volume"] == pytest.approx(1000, rel=1e-9)  # the picture adds nothing
    # it comes back with the drawing, and can be taken away
    cad.edit_sketch(i)
    assert cad.sketch_state()["image"]["width"] == pytest.approx(im2["width"])
    cad.sketch_image(opacity=0.3, x=1, y=2)
    st = cad.sketch_state()["image"]
    assert st["opacity"] == pytest.approx(0.3) and [st["x"], st["y"]] == [1, 2]
    with pytest.raises(ScottiCADError):
        cad.sketch_image(opacity=2)
    cad.sketch_image_remove()
    assert cad.sketch_state()["image"] is None
    cad.sketch_done()  # a picture taken away is a change: saved
    cad.edit_sketch(i)
    assert cad.sketch_state()["image"] is None
    cad.sketch_cancel()
    # undo of that brings the picture back to the shape
    cad.undo()
    cad.edit_sketch(i)
    assert cad.sketch_state()["image"] is not None
    cad.sketch_cancel()


def test_the_sketch_board_shows_the_other_shapes(cad):
    """The extruded sketch's board shows the design's other
    shapes where they are, seen from above, in a translucent tan; a
    revolve's board (a slice through its axis) doesn't."""
    import base64
    cad.add_shape("box", x=30, y=0)  # 20 mm: 20 to 40 across

    def pixel(b, x, y):
        shot = cad.screenshot()
        rows, dpi = _png_rows(base64.b64decode(shot["png"])), shot["dpi"]
        sx, sy = b.at(x, y)
        return rows[int(sy * dpi)][int(sx * dpi)]

    def tan(c):
        return c[0] - c[2] > 25  # warm: red well above blue

    cad.edit_sketch(kind="sketch")
    b = Board(cad, zoom=6)
    on, off = pixel(b, 30, 5), pixel(b, -30, 5)
    assert on != off and tan(on), (on, off)
    cad.sketch_cancel()
    # a sketch being changed sees them in its own frame: drawn at x = 30,
    # the box is round its origin
    i = cad.add_sketch([square(0, 0, 5, 5)], x=30)
    cad.edit_sketch(i)
    b = Board(cad, zoom=6)
    assert pixel(b, -5, -5) != pixel(b, -30, -30)
    cad.sketch_cancel()
    cad.edit_sketch(kind="revolve")
    b = Board(cad, zoom=6)
    shot = cad.screenshot()
    rows, dpi = _png_rows(base64.b64decode(shot["png"])), shot["dpi"]
    for x in range(-50, -2, 3):
        for y in range(2, 40, 3):
            sx, sy = b.at(x + 0.4, y + 0.4)
            assert not tan(rows[int(sy * dpi)][int(sx * dpi)]), (x, y)
