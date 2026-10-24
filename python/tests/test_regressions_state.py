"""Regression tests: state left over from a replaced design, the API's
argument checks, locked shapes, and edits that read stale values."""
import math
import os
import socket
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
    yield


def size(cad, id):
    return cad.node(id)["bounds"]["size"]


def test_new_design_drops_an_open_field_and_its_edit(cad):
    cad.add_shape("box")
    t = cad.add_shape("text", x=40)
    cad.select([t])
    cad.click_control("param-text")
    assert cad.ui_state().get("field", {}).get("id") == "param-text"
    cad.new_design()
    st = cad.state()
    assert st["node_count"] == 0 and not st["can_undo"]
    assert "field" not in cad.ui_state()
    assert not cad.undo()  # the old design doesn't come back
    assert cad.state()["node_count"] == 0


def test_a_size_field_doesnt_commit_into_a_new_design(cad):
    b = cad.add_shape("box")
    cad.select([b])
    cad.click_control("param-length")
    cad.new_design()
    nb = cad.add_shape("box")  # the same id as the old box
    assert nb == b
    cad.type_text("77")
    cad.key("enter")
    assert math.isclose(size(cad, nb)[0], 20, abs_tol=1e-9)


def test_a_pinned_section_ends_with_its_design(cad):
    a = cad.add_shape("box")
    cad.select([a])
    cad.set_section(pin=True)
    cad.new_design()
    assert "section" not in cad.ui_state()


def test_undo_commits_the_field_being_typed_in(cad):
    t = cad.add_shape("text")
    cad.select([t])
    cad.click_control("param-text")
    cad.key("ctrl+a")
    cad.type_text("AB")
    cad.undo()  # the typing is committed as a step, then undone
    assert "field" not in cad.ui_state()
    assert cad.node(t)["text"] == "TEXT"
    cad.redo()
    assert cad.node(t)["text"] == "AB"


def test_api_refuses_bad_arguments(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.select([a])
    with pytest.raises(ScottiCADError):
        cad.call("delete", ids="2")  # not ids: not "the selection" either
    assert {n["id"] for n in cad.nodes()} == {a, b}
    with pytest.raises(ScottiCADError):
        cad.call("delete", ids=["x"])
    with pytest.raises(ScottiCADError):
        cad.call("set_param", id=a, key="radius")  # no value
    with pytest.raises(ScottiCADError):
        cad.call("set_bounds", min=["a", 0, 0], max=[1, 1, 1])
    with pytest.raises(ScottiCADError):
        cad.set_section(keep="sideways")
    assert "section" not in cad.ui_state()  # refused before turning on
    cad.clear_selection()
    with pytest.raises(ScottiCADError):
        cad.set_tool("align")  # nothing to align


def test_api_hangs_up_on_http(cad):
    host, port = cad._sock.getpeername()[:2]
    snap = cad.state()["settings"]["snap"]
    s = socket.create_connection((host, port), timeout=5)
    body = '{"jsonrpc":"2.0","id":1,"method":"set_snap","params":{"mm":7.5}}'
    s.sendall(("POST / HTTP/1.1\r\nHost: x\r\nContent-Type: text/plain\r\n\r\n" + body + "\n").encode())
    s.recv(4096)  # the parse error
    try:
        while s.recv(4096):  # then the connection closes
            pass
    except OSError:
        pass
    s.close()
    assert cad.state()["settings"]["snap"] == snap  # the request never ran


def test_locked_shapes_keep_their_parameters(cad):
    b = cad.add_shape("box")
    cad.select([b])
    cad.set_locked(True)
    with pytest.raises(ScottiCADError):
        cad.set_param(b, "length", 50)
    cad.click_control("param-length-up")
    assert math.isclose(size(cad, b)[0], 20, abs_tol=1e-9)
    # and Alt-drag or Ctrl+D make no copy of them
    cad.key("ctrl+d")
    assert len(cad.nodes()) == 1


def test_size_stepper_past_a_metre(cad):
    cad.set_build_plate(3000, 3000)
    b = cad.add_shape("box")
    cad.select([b])
    cad.set_size(x=1500)
    cad.click_control("param-length-up")
    assert math.isclose(size(cad, b)[0], 1501, abs_tol=1e-6)


def test_settings_dialog_keeps_changes_made_elsewhere(cad):
    cad.click_control("settings")
    cad.set_snap(5)
    cad.set_settings(show_grid=False)
    cad.click_control("set-close")
    s = cad.state()["settings"]
    assert s["snap"] == 5 and not s["showGrid"]


def test_tab_to_the_next_size_keeps_the_last(cad):
    b = cad.add_shape("box")
    cad.select([b])
    hx, hy = cad.handle("corner", 2)
    cad.click(hx, hy)  # the corner's sizes stay up for typing
    cad.click_control("dim-x")
    cad.key("ctrl+a")
    cad.type_text("35")
    cad.key("tab")
    assert cad.ui_state()["field"]["id"] == "dim-y"
    cad.key("enter")  # Y unchanged
    s = size(cad, b)
    assert math.isclose(s[0], 35, abs_tol=1e-6) and math.isclose(s[1], 20, abs_tol=1e-6)


def test_a_typed_angle_after_another_edit_does_nothing_stale(cad):
    b = cad.add_shape("box")
    cad.set_size(x=40)
    cad.select([b])
    hx, hy = cad.handle("rotate", 2)
    c = cad.project(0, 0, 0)
    cad.drag(hx, hy, c[0] + (c[0] - hx) * 0.2 + 60, c[1] - 40, steps=10)
    assert any(x["id"] == "angle" for x in cad.controls())
    cad.undo()  # the rotation's protractor goes with it
    assert not any(x["id"] == "angle" for x in cad.controls())
    assert math.isclose(size(cad, b)[0], 40, abs_tol=1e-6)


def test_length_sliders_follow_the_units(cad):
    cad.set_units("in")
    try:
        b = cad.add_shape("box")
        cad.set_param(b, "radius", 4)
        cad.select([b])
        cad.click_control("param-radius")  # the knob: type a value
        assert cad.ui_state()["field"]["text"] == "0.157"
        cad.key("ctrl+a")
        cad.type_text("1/8")
        cad.key("enter")
        assert math.isclose(cad.node(b)["params"]["radius"], 3.175, abs_tol=1e-9)
        cad.click_control("param-radius")
        cad.key("ctrl+a")
        cad.type_text("2mm")  # a suffix still works
        cad.key("enter")
        assert math.isclose(cad.node(b)["params"]["radius"], 2, abs_tol=1e-9)
        # dragging steps in round fractions of an inch (0.005" for this one)
        track = next(c["rect"] for c in cad.controls() if c["id"] == "slider-radius")
        y = track[1] + track[3] / 2
        cad.drag(track[0] + 15, y, track[0] + track[2] * 0.37, y, steps=6)
        steps = cad.node(b)["params"]["radius"] / 25.4 / 0.005
        assert abs(steps - round(steps)) < 1e-6
        # counts stay counts
        cad.click_control("param-steps")
        assert cad.ui_state()["field"]["text"].isdigit()
        cad.key("esc")
        kinds = {k["id"]: k for k in cad.shape_kinds()}
        thread = {p["key"]: p for p in kinds["thread"]["params"]}
        assert thread["pitch"]["metric"] and not thread["pitch"]["length"]
        assert {p["key"]: p for p in kinds["box"]["params"]}["radius"]["length"]
    finally:
        cad.set_units("mm")


def test_minimum_window_size_fits_the_toolbar(cad):
    st = cad.ui_state()
    assert st["min_window"][0] >= 700
