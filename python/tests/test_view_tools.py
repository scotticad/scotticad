"""Section view and Lay flat."""
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
    cad.set_section(on=False)
    cad.home_view()
    yield


def centre_on_screen(cad, id):
    b = cad.node(id)["bounds"]
    return cad.project(*b["center"])


def test_section_view_hides_and_cannot_be_clicked(cad):
    front = cad.add_shape("box", x=-40, y=-30)
    back = cad.add_shape("box", x=40, y=30)
    cad.clear_selection()
    cad.fit_view()  # both boxes in view
    fx, fy = centre_on_screen(cad, front)
    # without the section, clicking the front box selects it
    cad.click(fx, fy)
    assert cad.selection()["ids"] == [front]
    cad.clear_selection()
    # section across Y at 0 showing the back (+Y) side: the front box is gone
    cad.set_section(axis="y", at=0, keep="above")
    sec = cad.ui_state()["section"]
    assert (sec["axis"], sec["at"], sec["keep"], sec["ids"]) == ("y", 0, "above", None)  # nothing selected: cuts all
    cad.click(fx, fy)
    assert not cad.selection()["ids"]
    # the back box still can be clicked
    cad.click(*centre_on_screen(cad, back))
    assert cad.selection()["ids"] == [back]
    # the design is unchanged: both boxes are whole
    assert len(cad.nodes()) == 2 and all(math.isclose(n["volume"], 8000, rel_tol=1e-6) for n in cad.nodes())


def test_section_view_controls(cad):
    cad.add_shape("box")
    cad.click_control("section")
    sec = cad.ui_state()["section"]
    assert sec["axis"] == "y"
    # the default shows the side away from the camera (which looks from -Y)
    assert sec["keep"] == "above"
    ids = {c["id"] for c in cad.controls()}
    assert {"section-grip", "section-at", "section-axis-x", "section-flip", "section-close"} <= ids
    cad.click_control("section-flip")
    assert cad.ui_state()["section"]["keep"] == "below"
    cad.click_control("section-axis-z")
    assert cad.ui_state()["section"]["axis"] == "z"
    cad.click_control("section-at")
    cad.type_text("5")
    cad.key("enter")
    assert math.isclose(cad.ui_state()["section"]["at"], 5, abs_tol=1e-9)
    cad.key("shift+k")
    assert "section" not in cad.ui_state()


def test_section_follows_the_selection(cad):
    a = cad.add_shape("box", x=-40)
    b = cad.add_shape("cylinder", x=40, y=20)
    cad.clear_selection()
    cad.fit_view()
    cad.select([a])
    cad.key("shift+k")
    sec = cad.ui_state()["section"]
    assert sec["ids"] == [a] and math.isclose(sec["at"], 0, abs_tol=1e-9)  # through a's middle
    # switching axis stays on a
    cad.click_control("section-axis-x")
    assert math.isclose(cad.ui_state()["section"]["at"], -40, abs_tol=1e-9)
    # b isn't cut, so it can be clicked; selecting it closes the section,
    # as selecting another shape closes the other tools
    cad.click(*centre_on_screen(cad, b))
    cad.controls()  # a frame
    assert "section" not in cad.ui_state()
    assert cad.selection()["ids"] == [b]
    # clicking off closes it
    cad.select([a])
    cad.key("shift+k")
    assert "section" in cad.ui_state()
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + 60, vy + vh - 40)
    cad.controls()
    assert "section" not in cad.ui_state()
    # so does Esc
    cad.select([a])
    cad.key("shift+k")
    assert "section" in cad.ui_state()
    cad.key("esc")
    assert "section" not in cad.ui_state()


def test_pinned_section_stays_open(cad):
    a = cad.add_shape("box", x=-40)
    b = cad.add_shape("cylinder", x=40, y=20)
    cad.clear_selection()
    cad.fit_view()
    cad.select([a])
    cad.key("shift+k")
    cad.click_control("section-pin")
    assert cad.ui_state()["section"]["pinned"]
    # selecting and moving another shape keeps it open, still cutting a
    cad.click(*centre_on_screen(cad, b))
    cad.move(dx=5)
    cad.controls()
    sec = cad.ui_state()["section"]
    assert sec["ids"] == [a] and cad.selection()["ids"] == [b]
    # clicking off keeps it open too; Esc closes it
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + 60, vy + vh - 40)
    cad.controls()
    assert "section" in cad.ui_state()
    cad.key("esc")
    assert "section" not in cad.ui_state()
    # it opens unpinned next time
    cad.select([a])
    cad.key("shift+k")
    assert not cad.ui_state()["section"]["pinned"]


def test_lay_flat_api(cad):
    w = cad.add_shape("wedge")
    size0 = cad.node(w)["bounds"]["size"]
    # any face works; the one facing straight up turns it upside down
    cad.lay_flat([0, 0, 1], ids=[w])
    b = cad.node(w)["bounds"]
    assert math.isclose(b["min"][2], 0, abs_tol=1e-6)
    assert math.isclose(b["size"][2], size0[2], abs_tol=1e-6)


def test_lay_flat_tool_clicks_a_face(cad):
    box = cad.add_shape("box")
    cad.rotate("x", 30, ids=[box])
    cad.drop(ids=[box])
    assert cad.node(box)["bounds"]["size"][2] > 25        # tilted
    # the middle of the (tilted) top face
    c = cad.node(box)["bounds"]["center"]
    top = [c[0], c[1] - 10 * math.sin(math.radians(30)), c[2] + 10 * math.cos(math.radians(30))]
    cad.clear_selection()
    cad.key("shift+d")
    assert cad.ui_state()["mode"] == "layflat"
    x, y = cad.project(*top)
    cad.mouse_move(x, y)
    cad.click(x, y)
    assert cad.ui_state()["mode"] == "none"
    b = cad.node(box)["bounds"]
    assert all(math.isclose(s, 20, abs_tol=1e-6) for s in b["size"]), b   # square again
    assert math.isclose(b["min"][2], 0, abs_tol=1e-6)
    assert cad.selection()["ids"] == [box]
    cad.undo()
    assert cad.node(box)["bounds"]["size"][2] > 25        # one undo step
