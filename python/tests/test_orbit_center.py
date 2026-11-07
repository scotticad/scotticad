"""The view button that sets the center of rotation."""
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
    cad.set_camera(ortho=False)
    cad.home_view()
    yield


def eye(c):
    """The camera position, from get_camera's target, angles and distance."""
    y, p = math.radians(c["yaw"]), math.radians(c["pitch"])
    d = [math.cos(p) * math.cos(y), math.cos(p) * math.sin(y), math.sin(p)]
    return [t + c["dist"] * k for t, k in zip(c["target"], d)]


def close(a, b, tol=1e-3):
    return all(math.isclose(x, y, abs_tol=tol) for x, y in zip(a, b))


def press_button(cad):
    cad.click_control("orbitcenter")
    assert cad.ui_state()["mode"] == "orbit_center"


def test_clicking_a_shape_turns_the_camera_to_face_it(cad):
    box = cad.add_shape("box")
    cad.clear_selection()
    top = cad.node(box)["bounds"]["center"]
    top[2] = cad.node(box)["bounds"]["max"][2]
    before = cad.camera()
    press_button(cad)
    cad.click(*cad.project(*top))
    assert cad.ui_state()["mode"] == "none"             # one click, then it's done
    after = cad.camera()
    assert close(after["target"], top, 0.05), after
    assert close(eye(after), eye(before)), (before, after)   # turned, not moved
    assert not cad.selection()["ids"]                    # the click selects nothing
    assert cad.node(box)["bounds"]["center"][0] == pytest.approx(0, abs=1e-9)


def test_clicking_the_plate_puts_the_center_on_it(cad):
    press_button(cad)
    before = cad.camera()
    cad.click(*cad.project(40, 30, 0))
    after = cad.camera()
    assert close(after["target"], [40, 30, 0], 0.05), after
    assert close(eye(after), eye(before))


def test_clicking_the_sky_keeps_the_depth(cad):
    cad.set_camera(pitch=5)                              # the top of the view is sky
    before = cad.camera()
    e = eye(before)
    fwd = [t - k for t, k in zip(before["target"], e)]
    n = math.sqrt(sum(v * v for v in fwd))
    press_button(cad)
    ctl = {c["id"]: c for c in cad.controls()}
    x, y, w, h = ctl["home"]["rect"]
    cad.click(x + 400, y + h / 2)
    after = cad.camera()
    assert after["target"][2] > 1, after                 # off the plate, in the air
    depth = sum((t - k) * f / n for t, k, f in zip(after["target"], e, fwd))
    assert depth == pytest.approx(before["dist"], rel=1e-6)
    assert close(eye(after), e)


def test_escape_and_a_second_press_cancel(cad):
    before = cad.camera()
    press_button(cad)
    cad.key("escape")
    assert cad.ui_state()["mode"] == "none"
    press_button(cad)
    cad.click_control("orbitcenter")
    assert cad.ui_state()["mode"] == "none"
    assert cad.camera() == before


def test_the_marker_shows_after_setting_and_can_be_turned_off(cad):
    assert cad.ui_state()["orbit_marker_setting"] is True
    press_button(cad)
    p = cad.project(40, 30, 0)
    cad.mouse_move(*p)
    assert close(cad.ui_state()["orbit_marker"], p, 1.5)   # where the click would put it
    cad.click(*p)
    st = cad.ui_state()
    vx, vy, vw, vh = st["viewport"]
    assert close(st["orbit_marker"], [vx + vw / 2, vy + vh / 2], 1.5)   # the new center, mid-view

    cad.click_control("settings")
    try:
        cad.click_control("set-orbit-marker")
        st = cad.ui_state()
        assert st["orbit_marker_setting"] is False and "orbit_marker" not in st
    finally:
        cad.click_control("set-close")
    try:
        press_button(cad)                      # the tool's own marker stays
        cad.mouse_move(*cad.project(40, 30, 0))
        assert "orbit_marker" in cad.ui_state()
        cad.click(*cad.project(40, 30, 0))
        assert "orbit_marker" not in cad.ui_state()
    finally:
        cad.click_control("settings")
        cad.click_control("set-orbit-marker")
        cad.click_control("set-close")
    assert cad.ui_state()["orbit_marker_setting"] is True


def test_orthographic_slides_across(cad):
    cad.set_camera(ortho=True)
    before = cad.camera()
    press_button(cad)
    cad.click(*cad.project(40, 30, 0))
    after = cad.camera()
    assert close(after["target"], [40, 30, 0], 0.05), after
    for k in ("yaw", "pitch", "dist"):                    # the same view, moved
        assert after[k] == pytest.approx(before[k]), k
