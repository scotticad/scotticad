"""Paths not covered elsewhere: a drag losing the mouse, resizing with
Shift and Alt, orbit and pan, the keyboard editing shortcuts, and a few
API edge cases."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    cad.home_view()
    cad.set_camera(ortho=False)
    if cad.ui_state()["popup"]:
        cad.key("esc")
    yield


def size(cad, id):
    return cad.node(id)["bounds"]["size"]


def near(a, b, tol=1e-6):
    return all(math.isclose(x, y, abs_tol=tol) for x, y in zip(a, b))


def test_losing_the_mouse_mid_drag(cad):
    b = cad.add_shape("box")
    cad.select([b])
    depth = cad.state()["undo_depth"]
    # a handle drag, interrupted: the shape goes back as it was, no undo step
    hx, hy = cad.handle("edge", 1)
    tx, ty = cad.project(40, 0, 0)
    cad.mouse_down(hx, hy)
    cad.mouse_move(tx, ty)
    assert size(cad, b)[0] > 20
    cad.lose_capture()
    assert near(size(cad, b), [20, 20, 20]) and cad.state()["undo_depth"] == depth
    # the next click selects as usual
    cad.clear_selection()
    cad.click(*cad.project(0, 0, 20))
    assert cad.state()["selection"] == [b]
    # a slider drag, interrupted: finished where it is, as one undo step
    r = next(c["rect"] for c in cad.controls() if c["id"] == "slider-radius")
    y = r[1] + r[3] / 2
    cad.mouse_down(r[0] + r[2] * 0.3, y)
    cad.mouse_move(r[0] + r[2] * 0.5, y)
    cad.lose_capture()
    v = cad.node(b)["params"]["radius"]
    assert v > 0 and cad.state()["undo_depth"] == depth + 1
    cad.mouse_move(r[0] + r[2] * 0.9, y)  # the drag is over
    assert cad.node(b)["params"]["radius"] == v


def test_resize_with_shift_and_alt(cad):
    b = cad.add_shape("box")
    cad.set_size(x=20, y=10, z=30)
    cad.select([b])
    # Shift: all three sizes scale alike, the opposite corner stays
    before = cad.node(b)["bounds"]
    hx, hy = cad.handle("corner", 2)  # +X +Y
    tx, ty = cad.project(before["max"][0] + 20, before["max"][1], 0)
    cad.drag(hx, hy, tx, ty, steps=8, mods=["shift"])
    after = cad.node(b)["bounds"]
    f = after["size"][0] / before["size"][0]
    assert f > 1.5
    assert near(after["size"], [20 * f, 10 * f, 30 * f], 1e-6)
    assert near(after["min"], before["min"], 1e-6)
    # Alt: the centre stays
    cad.undo()
    c = cad.node(b)["bounds"]["center"]
    hx, hy = cad.handle("edge", 1)  # +X
    tx, ty = cad.project(c[0] + 25, c[1], 0)
    cad.drag(hx, hy, tx, ty, steps=8, mods=["alt"])
    after = cad.node(b)["bounds"]
    assert near(after["center"], c, 1e-6) and after["size"][0] > 40
    # Shift on the top handle: X and Y follow, the base stays
    cad.undo()
    hx, hy = cad.handle("top", 0)
    cad.drag(hx, hy, hx, hy - 60, steps=8, mods=["shift"])
    after = cad.node(b)["bounds"]
    f = after["size"][2] / 30
    assert f > 1.2 and near(after["size"], [20 * f, 10 * f, 30 * f], 1e-6) and math.isclose(after["min"][2], 0, abs_tol=1e-6)


def test_orbit_and_pan(cad):
    cad.add_shape("box")
    vp = cad.ui_state()["viewport"]
    cx, cy = vp[0] + vp[2] / 2, vp[1] + vp[3] / 2
    c0 = cad.camera()
    cad.drag(cx, cy, cx + 150, cy + 60, steps=10, button="right")
    c1 = cad.camera()
    assert c1["yaw"] != c0["yaw"] and c1["pitch"] != c0["pitch"] and c1["target"] == c0["target"]
    cad.drag(cx, cy, cx, cy - 3000, steps=20, button="right")  # far past the pole
    assert abs(cad.camera()["pitch"]) < 90
    cad.drag(cx, cy, cx + 100, cy, steps=10, button="middle")
    c2 = cad.camera()
    assert c2["target"] != c1["target"] and math.isfinite(c2["dist"])
    cad.set_camera(ortho=True)
    cad.drag(cx, cy, cx - 100, cy + 50, steps=10, button="middle")
    assert all(math.isfinite(v) for v in cad.camera()["target"])
    # a right press while a size field is open commits it first
    cad.set_camera(ortho=False)
    b = cad.nodes()[0]["id"]
    cad.select([b])
    cad.click_control("param-length")
    cad.key("ctrl+a")
    cad.type_text("33")
    cad.mouse_down(cx, cy, button="right")
    cad.mouse_up(cx, cy, button="right")
    assert math.isclose(size(cad, b)[0], 33, abs_tol=1e-6)


def test_editing_shortcuts(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.set_locked(True, ids=[a])
    cad.select([a, b])
    cad.key("ctrl+x")  # cuts the unlocked one only
    assert [n["id"] for n in cad.nodes()] == [a]
    cad.key("ctrl+shift+v")  # back where it was
    n = cad.nodes()[-1]
    assert math.isclose(n["bounds"]["center"][0], 40, abs_tol=1e-6)
    cad.key("ctrl+v")  # offset by half its size
    m = cad.nodes()[-1]
    assert m["bounds"]["center"][0] > 40
    cad.select([n["id"]])
    cad.key("ctrl+h")
    assert cad.node(n["id"])["hidden"]
    cad.key("ctrl+shift+h")
    assert not cad.node(n["id"])["hidden"]
    cad.select([n["id"], m["id"]])
    cad.key("ctrl+g")
    assert cad.nodes()[-1]["kind"] == "group"
    cad.key("ctrl+shift+g")
    assert all(x["kind"] != "group" for x in cad.nodes())
    cad.select([b for b in [x["id"] for x in cad.nodes()] if b != a][:2])
    cad.key("ctrl+b")
    assert cad.nodes()[-1]["group_mode"] == "bundle"
    cad.key("ctrl+l")
    assert cad.nodes()[-1]["locked"]
    cad.key("ctrl+l")
    assert not cad.nodes()[-1]["locked"]
    cad.key("ctrl+t")
    assert cad.nodes()[-1]["transparent"]


def test_set_size_edge_cases(cad):
    b = cad.add_shape("box")
    cad.set_size(x=10, y=20, z=30, ids=[b])
    with pytest.raises(ScottiCADError):
        cad.set_size(x=0, ids=[b])
    assert near(size(cad, b), [10, 20, 30])
    cad.set_size(x=20, uniform=True, ids=[b])
    assert near(size(cad, b), [20, 40, 60])
    with pytest.raises(ScottiCADError):
        cad.set_bounds([10, 0, 0], [0, 10, 10], ids=[b])  # min above max
    # a selection whose shapes combine to nothing has no size to set
    h = cad.add_shape("box", hole=True)
    cad.set_bounds([-50, -50, -50], [50, 50, 100], ids=[h])  # covers the box entirely
    g = cad.group("union", ids=[b, h])
    with pytest.raises(ScottiCADError):
        cad.set_size(x=5, ids=[g])
    with pytest.raises(ScottiCADError):
        cad.set_position(x=5, ids=[g])
    cad.select([g])
    cad.key("l")  # align and mirror have nowhere to put their widgets
    cad.ui_state()
    cad.key("m")
    cad.ui_state()
    cad.key("k")  # nor split
    assert cad.ui_state()["mode"] == "none"


def test_console_collapses_repeats(cad):
    cad.run_script("for i in range(5):\n    print('same')\nprint('other')")
    lines = [l for l in cad.console()["lines"] if l["kind"] == "print"]
    assert [(l["text"], l.get("count", 1)) for l in lines][-2:] == [("same", 5), ("other", 1)]


def test_api_undo_during_a_canvas_drag(cad):
    """An undo from the API while a shape is being dragged ends the drag
    first: the drag must not go on moving shapes no longer in the design,
    and Esc on an Alt-drag must not then undo an unrelated earlier step."""
    a = cad.add_shape("box")
    b = cad.add_shape("box")
    cad.set_position(x=60, ids=[b])
    bc = cad.node(b)["bounds"]["center"]
    cad.clear_selection()  # no handles in the way: the press is on the box
    depth = cad.state()["undo_depth"]
    sx, sy = cad.project(0, 0, 20)
    tx, ty = cad.project(30, 0, 20)
    cad.mouse_down(sx, sy, mods=["alt"])  # Alt-drag: duplicates first
    cad.mouse_move((sx + tx) / 2, (sy + ty) / 2, mods=["alt"])
    cad.mouse_move(tx, ty, mods=["alt"])
    assert len(cad.nodes()) == 3
    cad.undo()  # undoes the drag's move
    cad.undo()  # and the duplicate
    cad.key("esc")
    cad.mouse_up(tx, ty, mods=["alt"])
    # the earlier steps are intact: b still where it was put
    assert near(cad.node(b)["bounds"]["center"], bc)
    assert cad.state()["undo_depth"] == depth
    assert sorted(n["id"] for n in cad.nodes()) == sorted([a, b])
    # the drag is over: moving the mouse changes nothing
    before = {n["id"]: n["bounds"]["center"] for n in cad.nodes()}
    cad.mouse_move(sx, sy)
    assert {n["id"]: n["bounds"]["center"] for n in cad.nodes()} == before


def test_note_wraps_long_words(cad):
    """A note's box wraps at spaces, and breaks a word too long for a line
    anywhere; it stays at most 440 px wide and grows downward in lines."""
    b = cad.add_shape("box")
    cad.click_control("tab-notes")
    px, py = cad.project(0, 0, 20)

    def box_top(text):
        cad.click(px, py)
        cad.type_text(text)
        ctl = {c["id"]: c["rect"] for c in cad.controls()}
        cad.key("enter")
        return ctl["note-delete"], ctl.get("note-text")

    short, _ = box_top("hi")
    cad.key("esc")
    for n in cad.notes():
        cad.delete_note(n["id"], n["index"])
    word = "x" * 300 + " caf\u00e9 \U0001F600 " + "y" * 120
    long, _ = box_top(word)
    assert cad.notes()[-1]["text"] == word
    # at most 440 wide: the delete button, at the box's right edge, is
    # within 220 px of the pin either side
    assert abs(long[0] + long[2] / 2 - px) < 230, long
    # several lines: the box's top (where the delete button sits) is well
    # above where a one-line note's is
    assert long[1] < short[1] - 3 * 14, (short, long)
