"""Regression tests: the console and shortcuts, section view, Alt-drag,
fields being edited, and a 200 % scale, each driven through the GUI or the
API the way a user would."""
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
    cad.set_section(on=False)
    cad.set_tool("none")
    cad.home_view()
    if cad.console()["open"]:
        cad.click_control("scripting")
    yield


def rect(cad, id):
    return next(c["rect"] for c in cad.controls() if c["id"] == id)


def centre(r):
    return r[0] + r[2] / 2, r[1] + r[3] / 2


def test_typing_in_the_console_doesnt_trigger_shortcuts(cad):
    box = cad.add_shape("box")
    cad.select([box])
    cad.click_control("scripting")
    assert cad.console()["focus"] == "input"
    # real key presses: H would make the box a hole, L opens Align,
    # D drops, K opens Split, Delete deletes
    for k in ["h", "l", "d", "k", "s", "delete"]:
        cad.key(k)
    assert cad.ui_state()["mode"] == "none"
    n = cad.node(box)
    assert not n["hole"]
    # the same from the transcript: typing moves to the prompt instead
    cad.click(*centre(rect(cad, "con-transcript")))
    assert cad.console()["focus"] == ""
    cad.key("h")
    assert cad.console()["focus"] == "input"
    assert not cad.node(box)["hole"]
    # Ctrl combinations still reach the app
    cad.key("ctrl+d")
    assert len(cad.nodes()) == 2


def test_non_ascii_code_doesnt_freeze(cad):
    cad.click_control("scripting")
    cad.type_text("x = 3 × 4  # “quoted” € 🎉")
    cad.controls()  # draws a frame, which must not hang
    assert cad.console()["input"].startswith("x = 3 ×")
    cad.click_control("con-tab-Editor")
    cad.click_control("con-new")
    cad.type_text("print(“hi”)")
    cad.controls()
    assert "“hi”" in cad.console()["script"]


def test_lay_flat_through_a_section_lays_the_face_seen(cad):
    box = cad.add_shape("box")
    cad.set_size(x=20, y=40, z=10, ids=[box])          # y -20..20, z 0..10
    cad.add_note(box, 6, 20, 2, "back face")            # on the +Y face
    cad.key("esc")                                      # close its text box
    cad.clear_selection()
    # cut away the front (camera) half: through the cut we see the inside
    # of the back (+Y) wall
    cad.set_section(axis="y", at=0, keep="above")
    cad.set_camera(target=[0, 0, 5], yaw=-90, pitch=4, dist=80)  # low, square on, close: the cut shows the back wall
    cad.key("shift+d")
    x, y = cad.project(-7, 20, 8)                       # the back wall, clear of the note pin and the section label
    cad.mouse_move(x, y)
    cad.click(x, y)
    (note,) = cad.notes()
    assert math.isclose(note["world"][2], 0, abs_tol=1e-6), note  # the back face is down


def test_cancelling_an_alt_drag_removes_the_duplicate(cad):
    box = cad.add_shape("box")
    cad.select([box])
    x, y = cad.project(0, 0, 10)
    cad.mouse_down(x, y, mods=["alt"])
    for i in range(1, 6):
        cad.mouse_move(x + 8 * i, y, mods=["alt"])
    cad.key("esc")
    cad.mouse_up(x + 40, y)
    assert [n["id"] for n in cad.nodes()] == [box]
    assert cad.node(box)["bounds"]["center"][0] == pytest.approx(0)
    assert not cad.redo()  # the duplicate can't be redone back


def test_marquee_ignores_what_section_view_hides(cad):
    front = cad.add_shape("box", x=-40, y=-30)
    back = cad.add_shape("box", x=40, y=30)
    cad.clear_selection()
    cad.fit_view()
    cad.set_section(axis="y", at=0, keep="above")       # hides the front box
    b = cad.node(front)["bounds"]
    pts = [cad.project(px, py, pz) for px in (b["min"][0], b["max"][0])
           for py in (b["min"][1], b["max"][1]) for pz in (b["min"][2], b["max"][2])]
    xs, ys = [p[0] for p in pts], [p[1] for p in pts]
    cad.drag(min(xs) - 5, min(ys) - 5, max(xs) + 5, max(ys) + 5)
    assert not cad.selection()["ids"]
    assert back  # (the back box is never inside that rectangle)


def test_unpinning_keeps_the_section_open(cad):
    box = cad.add_shape("box")
    cad.select([box])
    cad.key("shift+k")
    cad.click_control("section-pin")
    cad.clear_selection()
    cad.controls()
    cad.click_control("section-pin")                    # unpin, nothing selected
    cad.controls()
    assert "section" in cad.ui_state()
    assert cad.ui_state()["section"]["ids"] == [box]    # still cutting the box
    cad.select([box])                                    # a selection change closes it
    cad.controls()
    assert "section" not in cad.ui_state()


def test_a_tool_cancels_an_armed_tile(cad):
    cad.click_control("tile-box")                        # armed: next click places a box
    assert cad.ui_state()["placing"]
    cad.key("v")                                         # Visibility mode
    assert not cad.ui_state()["placing"]
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + vw / 2, vy + vh / 2)
    assert cad.nodes() == []


def test_clicking_a_field_being_edited_keeps_the_typing(cad):
    cad.click_control("search")
    cad.type_text("gea")
    cad.click_control("search-field")                    # click again to place the caret
    cad.type_text("r")
    assert cad.ui_state()["field"]["text"] == "gear"
    cad.key("esc")
    cad.key("esc")


def test_add_to_selection_adds(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.select([a])
    assert sorted(cad.add_to_selection(b)) == sorted([a, b])
    assert sorted(cad.add_to_selection(b)) == sorted([a, b])  # not a toggle
    cad.attach_construction_plane(a)
    assert cad.state().get("construction_plane")
    cad.attach_construction_plane(0)
    assert not cad.state().get("construction_plane")


def test_nan_filament_settings_are_refused(cad):
    cad.add_shape("box")
    before = cad.set_filament_settings()
    with pytest.raises(ScottiCADError):
        cad.set_filament_settings(infill="nan", price_per_kg="inf")
    assert cad.set_filament_settings() == before         # nothing changed
    e = cad.filament_estimate()                          # the connection still works
    assert e["grams"] > 0


def test_200_percent_scale_grows_the_atlas():
    # at 200% the glyphs and thumbnails outgrow a 2048 atlas; it must grow
    # once and the shape tiles must still be drawn
    with launch(headless=True, width=1400, height=860, scale=2) as c:
        c.add_shape("box")
        st = c.ui_state()
        assert st["scale"] == 2
        assert st["atlas"]["size"] == 4096
        assert st["atlas"]["generation"] >= 1
        assert any(x["id"].startswith("tile-") for x in c.controls())
