"""GUI paths driven with clicks, drags and keys the way a user would: the
mirror arrows, align dots, measure tool, visibility mode, Cruise, the
inspector buttons, the view controls and the notes and ruler tabs."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    if cad.ui_state()["cruise"]:
        cad.set_tool("cruise")
    cad.home_view()
    cad.set_camera(ortho=False)
    yield


def approx(a, b, tol=1e-6):
    return all(abs(x - y) <= tol for x, y in zip(a, b))


def bounds(cad, id):
    return cad.node(id)["bounds"]


def widget(cad, kind, **match):
    return next(w for w in cad.ui_state()[kind] if all(w[k] == v for k, v in match.items()))


def test_mirror_by_clicking_an_arrow(cad):
    a = cad.add_shape("box", x=-30)
    b = cad.add_shape("box", x=30)
    cad.set_size(x=10, ids=[b])
    cad.select([a, b])
    cad.click_control("mirror")
    assert cad.ui_state()["mode"] == "mirror"
    w = widget(cad, "mirror_arrows", axis="x")
    cad.click(w["x"], w["y"])
    # mirrored about the selection's centre (x = -2.5): the narrow box is
    # now on the left, the wide one on the right
    assert approx([bounds(cad, b)["min"][0], bounds(cad, b)["max"][0]], [-40, -30])
    assert approx([bounds(cad, a)["min"][0], bounds(cad, a)["max"][0]], [15, 35])
    assert cad.ui_state()["mode"] == "mirror"  # stays on for another click
    cad.undo()
    assert approx([bounds(cad, b)["min"][0], bounds(cad, b)["max"][0]], [25, 35])


def test_align_by_clicking_a_dot(cad):
    a = cad.add_shape("box", x=-30, y=-20)
    b = cad.add_shape("box", x=30, y=20)
    cad.set_size(z=40, ids=[b])
    cad.select([a, b])
    cad.key("l")
    assert cad.ui_state()["mode"] == "align"
    w = widget(cad, "align_dots", axis="y", where="min")
    cad.click(w["x"], w["y"])
    assert math.isclose(bounds(cad, a)["min"][1], bounds(cad, b)["min"][1], abs_tol=1e-6)
    # x untouched
    assert math.isclose(bounds(cad, a)["min"][0], -40, abs_tol=1e-6)
    w = widget(cad, "align_dots", axis="z", where="max")
    cad.click(w["x"], w["y"])
    assert math.isclose(bounds(cad, a)["max"][2], 40, abs_tol=1e-6)


def test_measure_points_and_type_a_distance(cad):
    b = cad.add_shape("box", x=30)
    cad.select([b])
    cad.click_control("measure")
    m = cad.ui_state()["measure"]
    assert m["stage"] == 0
    cad.click(*cad.project(-40, 0, 0))
    m = cad.ui_state()["measure"]
    assert m["stage"] == 1 and approx(m["start"], [-40, 0, 0], 1e-6)
    # the end on the box's -X face, level with the start point
    cad.click(*cad.project(20, 0, 0.5))
    m = cad.ui_state()["measure"]
    assert m["stage"] == 2 and math.isclose(m["distance"], math.hypot(60, 0.5), abs_tol=0.05)
    # typing a distance moves the shape the end point is on
    label = next(c for c in cad.controls() if c["id"] == "measure" and c["tip"] == "")
    cad.click(label["rect"][0] + label["rect"][2] / 2, label["rect"][1] + label["rect"][3] / 2)
    cad.key("ctrl+a")
    cad.type_text("80")
    cad.key("enter")
    m = cad.ui_state()["measure"]
    assert math.isclose(m["distance"], 80, abs_tol=1e-6)
    assert math.isclose(bounds(cad, b)["min"][0], 20 + 80 - math.hypot(60, 0.5), abs_tol=0.05)
    cad.key("esc")
    assert cad.ui_state()["mode"] == "none"


def test_visibility_menu_and_mode(cad):
    a = cad.add_shape("box", x=-30)
    b = cad.add_shape("box", x=30)
    cad.select([a])
    cad.click_control("visibility")
    assert cad.ui_state()["popup"] == "visibility"
    cad.click_control("m-hide")
    assert cad.node(a)["hidden"] and not cad.node(b)["hidden"]
    cad.click_control("visibility")
    cad.click_control("m-togglevis")
    assert cad.ui_state()["mode"] == "visibility"
    # in visibility mode a click toggles a shape, hidden or not
    cad.click(*cad.project(30, 0, 20))
    assert cad.node(b)["hidden"]
    cad.click(*cad.project(-30, 0, 20))
    assert not cad.node(a)["hidden"]
    cad.key("esc")
    cad.click_control("showall")
    assert not cad.node(b)["hidden"]


def test_cruise_drags_onto_a_surface(cad):
    base = cad.add_shape("box", x=-40)
    cad.set_size(x=60, y=60, z=10, ids=[base])
    small = cad.add_shape("box", x=40)
    cad.select([small])
    cad.click_control("cruise")
    assert cad.ui_state()["cruise"]
    # grab the front face away from the handles
    x0, y0 = cad.project(40, -10, 8)
    x1, y1 = cad.project(-40, 0, 10)
    cad.drag(x0, y0, x1, y1, steps=20)
    b = bounds(cad, small)
    # it rests on the base's top face
    assert math.isclose(b["min"][2], 10, abs_tol=1e-3)
    assert -70 < b["min"][0] and b["max"][0] < -10
    cad.click_control("cruise")
    assert not cad.ui_state()["cruise"]
    cad.undo()
    assert math.isclose(bounds(cad, small)["min"][2], 0, abs_tol=1e-9)


def test_inspector_buttons(cad):
    a = cad.add_shape("box")
    cad.select([a])
    cad.click_control("insp-hole")
    assert cad.node(a)["hole"]
    cad.click_control("insp-solid")  # opens the colour picker
    assert not cad.node(a)["hole"] and cad.ui_state()["popup"] == "color"
    cad.key("esc")
    cad.click_control("insp-lock")
    assert cad.node(a)["locked"]
    cad.click_control("insp-lock")
    assert not cad.node(a)["locked"]
    cad.click_control("insp-hide")
    assert cad.node(a)["hidden"] and not cad.state()["selection"]
    cad.click_control("showall")
    assert not cad.node(a)["hidden"]
    cad.select([a])
    cad.click_control("insp-collapse")
    assert not any(c["id"] == "insp-hole" for c in cad.controls())
    cad.click_control("insp-collapse")
    assert any(c["id"] == "insp-hole" for c in cad.controls())


def test_duplicate_and_drop_buttons(cad):
    a = cad.add_shape("box")
    cad.move(dz=15, ids=[a])
    cad.select([a])
    cad.click_control("duplicate")
    ids = [n["id"] for n in cad.nodes()]
    assert len(ids) == 2
    dup = cad.state()["selection"][0]
    assert dup != a
    cad.click_control("drop")
    assert math.isclose(bounds(cad, dup)["min"][2], 0, abs_tol=1e-9)
    assert math.isclose(bounds(cad, a)["min"][2], 15, abs_tol=1e-9)


def test_view_controls(cad):
    cad.add_shape("box")
    d0 = cad.camera()["dist"]
    cad.click_control("zoomin")
    assert cad.camera()["dist"] < d0
    cad.click_control("zoomout")
    cad.click_control("zoomout")
    assert cad.camera()["dist"] > d0
    cad.set_camera(yaw=10, pitch=60, target=[80, 80, 0], dist=600)
    cad.click_control("fit")
    c = cad.camera()
    assert c["dist"] < 600 and math.hypot(c["target"][0], c["target"][1]) < 20
    cad.set_camera(yaw=10, pitch=60)
    cad.click_control("home")
    home = cad.camera()
    assert (round(home["yaw"]), round(home["pitch"])) != (10, 60)
    # the view cube's middle (the top face at home) turns the view to look down
    vc = next(c for c in cad.controls() if c["id"] == "viewcube")["rect"]
    cad.click(vc[0] + vc[2] / 2, vc[1] + vc[3] / 2)
    assert cad.camera()["pitch"] != home["pitch"]
    ortho = cad.camera()["ortho"]
    cad.click_control("ortho")
    assert cad.camera()["ortho"] != ortho


def test_notes_and_ruler_tabs(cad):
    a = cad.add_shape("box")
    cad.click_control("tab-notes")
    assert cad.ui_state()["mode"] == "notes"
    cad.click(*cad.project(0, 0, 20))
    ns = cad.notes()
    assert len(ns) == 1 and ns[0]["id"] == a and math.isclose(ns[0]["world"][2], 20, abs_tol=1e-3)
    # Esc backs out a layer at a time: the text field, the note, the tool
    cad.key("esc")
    cad.key("esc")
    assert cad.ui_state()["mode"] == "notes" and not any(n["open"] for n in cad.notes())
    cad.key("esc")
    assert cad.ui_state()["mode"] == "none"
    cad.click_control("tab-ruler")
    assert cad.ui_state()["mode"] == "ruler"
    cad.click(*cad.project(-60, -60, 0))
    r = cad.state()["ruler"]
    assert r is not None
    assert cad.ui_state()["mode"] == "none"


def test_panel_toggle_and_library(cad):
    tiles = lambda: sum(1 for c in cad.controls() if c["id"].startswith("tile-"))
    assert tiles() > 0
    cad.click_control("paneltoggle")
    assert tiles() == 0
    cad.click_control("paneltoggle")
    assert tiles() > 0
    cad.click_control("library")
    assert cad.ui_state()["popup"] == "library"
    cad.click_control("lib-Mechanical")
    assert cad.ui_state()["popup"] == ""
    assert any(c["id"] == "tile-spurgear" for c in cad.controls())


def test_build_plate_clicks_in_ortho_view(cad):
    # an orthographic ray starts far behind the camera: the build plate
    # must still be hit, or nothing can be placed or dragged
    box = cad.add_shape("box")
    cad.set_camera(ortho=True)
    cad.select([box])
    x0, y0 = cad.project(0, -10, 10)
    x1, y1 = cad.project(30, -10, 10)
    cad.drag(x0, y0, x1, y1, steps=8)
    assert math.isclose(bounds(cad, box)["min"][0], 20, abs_tol=0.5)
    cad.click_control("tab-ruler")
    cad.click(*cad.project(-60, -60, 0))
    assert cad.state()["ruler"]["origin"]["X"] == -60
    cad.click_control("measure")
    cad.click(*cad.project(-40, 0, 0))
    assert cad.ui_state()["measure"]["stage"] == 1
    cad.key("esc")
