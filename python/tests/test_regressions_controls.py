"""Regression tests: locked shapes in the colour picker and toggles, the filament estimate's colours, controls under other
controls, fields that rewrote values they only showed, key chords, and
undo from elsewhere during a drag."""
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
    cad.set_units("mm")
    cad.set_tool("none")
    cad.home_view()
    if cad.ui_state()["popup"]:
        cad.key("esc")
    yield


def rect(cad, id):
    return next(c["rect"] for c in cad.controls() if c["id"] == id)


def test_multicolour_group_estimate_keeps_its_colours(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=30)
    cad.set_color("#ff0000", ids=[a])
    cad.set_color("#0000ff", ids=[b])
    g = cad.group("union", ids=[a, b])
    cad.set_multicolor(True, ids=[g])
    e = cad.filament_estimate(ids=[g])
    colours = {c["color"]: c["grams"] for c in e["by_color"]}
    assert set(colours) == {"#ff0000", "#0000ff"}
    assert math.isclose(colours["#ff0000"], colours["#0000ff"], rel_tol=1e-6)
    assert math.isclose(sum(colours.values()), e["grams"], rel_tol=1e-9)


def test_holes_dont_count_as_shapes_printed(cad):
    cad.add_shape("box")
    cad.add_shape("box", x=40, hole=True)
    cad.clear_selection()
    assert cad.filament_estimate()["shapes"] == 1


def test_colour_picker_and_transparent_skip_locked_shapes(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.set_color("#20c040", ids=[a])
    cad.set_color("#0000ff", ids=[b])
    cad.set_locked(True, ids=[a])
    cad.set_transparent(True, ids=[b])
    cad.select([a, b])
    cad.key("ctrl+t")  # the unlocked one is transparent: off
    assert not cad.node(b)["transparent"]
    cad.key("ctrl+t")
    assert cad.node(b)["transparent"]
    cad.click_control("insp-solid")
    cad.click_control("color-tab-Custom")
    cad.click_control("color-slider-red")
    c = cad.node(b)["color"]
    assert c[3:] == "00ff", c  # green and blue are B's own, not the locked A's
    assert cad.node(a)["color"] == "#20c040"


def test_a_covered_control_isnt_clicked(cad):
    b = cad.add_shape("box")
    cad.select([b])
    cad.key("f1")
    with pytest.raises(ScottiCADError):
        cad.click_control("delete")
    cad.key("esc")
    assert len(cad.nodes()) == 1


def test_undo_from_elsewhere_during_a_slider_drag(cad):
    b = cad.add_shape("box")
    cad.set_param(b, "radius", 2)
    cad.select([b])
    r = rect(cad, "slider-radius")
    y = r[1] + r[3] / 2
    cad.mouse_down(r[0] + r[2] * 0.3, y)
    cad.mouse_move(r[0] + r[2] * 0.5, y)
    dragged = cad.node(b)["params"]["radius"]
    assert dragged != 2
    assert cad.undo()  # the drag so far is a step of its own, undone
    assert cad.node(b)["params"]["radius"] == 2
    cad.mouse_move(r[0] + r[2] * 0.8, y)  # the drag is over: no change
    cad.mouse_up(r[0] + r[2] * 0.8, y)
    assert cad.node(b)["params"]["radius"] == 2
    assert cad.redo()
    assert cad.node(b)["params"]["radius"] == dragged


def test_tiles_below_the_list_cant_be_clicked(cad):
    # a short library: empty panel below its last row of tiles
    cad.click_control("library")
    cad.click_control("lib-Parametric")
    last = rect(cad, "tile-grid")
    x, y = last[0] + last[2] / 2, last[1] + last[3] + 60
    cad.click(x, y)
    assert not cad.ui_state()["placing"]
    cad.click_control("library")
    cad.click_control("lib-Basic Shapes")


def test_key_chords(cad):
    d = cad.camera()["dist"]
    cad.key("+")
    assert cad.camera()["dist"] < d
    b = cad.add_shape("box")
    cad.key(" ctrl+z ")
    assert all(n["id"] != b for n in cad.nodes())
    with pytest.raises(ScottiCADError):
        cad.key("a+b")


def test_api_types_are_checked(cad):
    b = cad.add_shape("box")
    with pytest.raises(ScottiCADError):
        cad.call("set_locked", on="false", ids=[b])
    with pytest.raises(ScottiCADError):
        cad.call("set_text", id=b, text=5)
    assert not cad.node(b)["locked"]


def test_a_note_takes_several_lines(cad):
    b = cad.add_shape("box")
    cad.click_control("tab-notes")
    cad.click(*cad.project(0, 0, 20))
    cad.type_text("one\ntwo")
    cad.key("enter")
    assert cad.notes()[0]["text"] == "one\ntwo"


def test_shown_values_arent_written_back(cad):
    cad.set_units("in")
    b = cad.add_shape("box")
    cad.set_param(b, "radius", 4)
    cad.select([b])
    cad.click_control("param-radius")  # shows 0.157"
    cad.key("enter")
    assert cad.node(b)["params"]["radius"] == 4
    cad.set_size(x=20.004)
    cad.click_control("param-length")
    cad.key("enter")
    assert math.isclose(cad.node(b)["bounds"]["size"][0], 20.004, abs_tol=1e-9)


def test_kept_protractor_outlasts_save_and_reselect(cad, tmp_path):
    b = cad.add_shape("box")
    cad.set_size(x=40)
    cad.select([b])
    hx, hy = cad.handle("rotate", 2)
    c = cad.project(0, 0, 0)
    cad.drag(hx, hy, c[0] + (c[0] - hx) * 0.2 + 60, c[1] - 40, steps=10)
    assert any(x["id"] == "angle" for x in cad.controls())
    cad.save(str(tmp_path / "kept.scotticad"))
    cad.select([b])  # the same selection again
    assert any(x["id"] == "angle" for x in cad.controls())
    cad.set_param(b, "radius", 2)  # a change to the design ends it
    assert not any(x["id"] == "angle" for x in cad.controls())


def test_clicking_from_label_to_label_keeps_the_first(cad):
    b = cad.add_shape("box")
    cad.select([b])
    hx, hy = cad.handle("corner", 2)
    cad.click(hx, hy)
    cad.click_control("dim-x")
    cad.key("ctrl+a")
    cad.type_text("40")
    cad.click_control("dim-y")  # commits X, opens Y
    cad.key("ctrl+a")
    cad.type_text("30")
    cad.key("enter")
    s = cad.node(b)["bounds"]["size"]
    assert math.isclose(s[0], 40, abs_tol=1e-6) and math.isclose(s[1], 30, abs_tol=1e-6)


def test_fit_on_shapes_that_make_nothing(cad):
    a = cad.add_shape("box")
    h = cad.add_shape("box", hole=True)
    cad.set_size(x=40, y=40, z=40, ids=[h])
    cad.set_position(x=0, y=0, z=-10, anchor="center", ids=[h])
    g = cad.group("union", ids=[a, h])
    cad.select([g])
    cad.place_ruler(-60, -60)
    cad.key("f")
    c = cad.camera()  # still finite
    assert all(math.isfinite(v) for v in c["target"]) and math.isfinite(c["dist"])
    cad.controls()  # drawing labels for nothing doesn't break the connection


def test_typed_ruler_distance_reads_back(cad):
    b = cad.add_shape("box", x=30, y=0)  # x 20..40
    cad.place_ruler(0, -50)
    cad.select([b])
    cad.click_control("ruler-x")
    cad.key("ctrl+a")
    cad.type_text("-10")
    cad.key("enter")
    bb = cad.node(b)["bounds"]
    assert math.isclose(bb["max"][0], -10, abs_tol=1e-6), bb  # its near edge at -10


def test_ruler_midpoints_hold_for_height(cad):
    def typ(cid, text):
        cad.select([b])
        cad.click_control(cid)
        cad.key("ctrl+a")
        cad.type_text(text)
        cad.key("enter")

    def z():
        bb = cad.node(b)["bounds"]
        return bb["min"][2], bb["max"][2]

    b = cad.add_shape("box", x=20, y=20, z=10)  # z 10..30
    # measuring to midpoints: the elevation is the middle's, and a typed
    # height keeps the middle where it is, as width and length do
    cad.place_ruler(0, 0, midpoint=True)
    typ("dim-elev", "30")
    assert z() == (20, 40)
    typ("dim-z", "40")
    assert z() == (10, 50)  # the middle stays at 30
    # measuring to endpoints: the bottom stays put
    cad.place_ruler(0, 0, midpoint=False)
    typ("dim-z", "20")
    assert z() == (10, 30)


def test_help_fits_a_short_window():
    with launch(headless=True, width=900, height=500) as c:
        c.key("f1")
        r = next(x["rect"] for x in c.controls() if x["id"] == "modal-close")
        assert r[1] >= 0
