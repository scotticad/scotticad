"""The API methods not exercised elsewhere: each is called and its effect
checked through the design or editor state."""
import json
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
    yield


def approx(a, b, tol=1e-6):
    return all(abs(x - y) <= tol for x, y in zip(a, b))


def bounds(cad, id):
    return cad.node(id)["bounds"]


def test_name_is_the_file_name_and_get_document(cad, tmp_path):
    box = cad.add_shape("box")
    assert cad.state()["name"] == "Unsaved new design"
    cad.save(str(tmp_path / "Bracket v2.scotticad"))
    assert cad.state()["name"] == "Bracket v2"
    tip = next(c["tip"] for c in cad.controls() if c["id"] == "docname")
    assert tip.endswith("Bracket v2.scotticad")  # the full path on hover
    d = cad.get_document()
    assert [n["id"] for n in d["nodes"]] == [box]
    json.dumps(d)  # plain JSON
    assert "rename" not in {m["name"] for m in cad.describe()}
    cad.new_design()
    assert cad.state()["name"] == "Unsaved new design"


def test_shape_kinds_and_fonts(cad):
    kinds = {k["id"]: k for k in cad.shape_kinds()}
    for id in ["box", "cylinder", "sphere", "text", "thread"]:
        assert id in kinds
    pitch = next(p for p in kinds["thread"]["params"] if p["key"] == "pitch")
    assert pitch["min"] < pitch["default"] < pitch["max"]
    fonts = cad.fonts()
    assert "Multilanguage" in fonts
    t = cad.add_shape("text")
    assert cad.node(t)["font"] in fonts


def test_notes_add_edit_delete(cad):
    box = cad.add_shape("box")
    i = cad.add_note(box, 0, 0, 20, "first")
    assert [(n["id"], n["index"], n["text"]) for n in cad.notes()] == [(box, i, "first")]
    assert approx(cad.notes()[0]["world"], [0, 0, 20])
    cad.set_note_text(box, i, "second")
    assert cad.notes()[0]["text"] == "second"
    # the note moves with its shape
    cad.move(dx=10, ids=[box])
    assert approx(cad.notes()[0]["world"], [10, 0, 20])
    cad.delete_note(box, i)
    assert cad.notes() == []
    with pytest.raises(ScottiCADError):
        cad.delete_note(box, 5)
    with pytest.raises(ScottiCADError):
        cad.add_note(9999, 0, 0, 0)


def test_set_position_and_bounds(cad):
    box = cad.add_shape("box")
    cad.set_position(x=5, y=6, z=7)
    assert approx(bounds(cad, box)["min"], [5, 6, 7])
    cad.set_position(x=0, anchor="center")  # y and z unchanged
    b = bounds(cad, box)
    assert approx(b["min"], [-10, 6, 7])
    cad.set_bounds([0, 0, 0], [40, 10, 5], ids=[box])
    b = bounds(cad, box)
    assert approx(b["min"], [0, 0, 0]) and approx(b["max"], [40, 10, 5])


def test_nudge_follows_the_snap_grid(cad):
    box = cad.add_shape("box")
    start = bounds(cad, box)["min"]
    cad.set_snap(2.5)
    assert cad.state()["settings"]["snap"] == 2.5
    cad.nudge("x", 2)
    cad.nudge("z", -1)
    assert approx(bounds(cad, box)["min"], [start[0] + 5, start[1], start[2] - 2.5])
    cad.set_snap(0)  # off: arrows still move 1 mm
    cad.nudge("y")
    assert approx(bounds(cad, box)["min"], [start[0] + 5, start[1] + 1, start[2] - 2.5])
    with pytest.raises(ScottiCADError):
        cad.nudge("w")


def test_appearance_setters(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("cylinder", x=30)
    cad.set_hole(True, ids=[b])
    assert cad.node(b)["hole"] and not cad.node(a)["hole"]
    cad.set_hole(False, ids=[b])
    assert not cad.node(b)["hole"]
    cad.set_transparent(True, ids=[a])
    assert cad.node(a)["transparent"]
    cad.set_transparent(False, ids=[a])
    assert not cad.node(a)["transparent"]
    cad.set_name("Base", ids=[a])
    assert cad.node(a)["name"] == "Base"
    g = cad.group("union", ids=[a, b])
    cad.set_multicolor(True, ids=[g])
    assert cad.node(g)["multicolor"]
    cad.set_multicolor(False, ids=[g])
    assert not cad.node(g)["multicolor"]


def test_toggle_hidden(cad):
    box = cad.add_shape("box")
    cad.toggle_hidden(box)
    assert cad.node(box)["hidden"]
    cad.toggle_hidden(box)
    assert not cad.node(box)["hidden"]


def test_build_plate_size_and_settings(cad):
    cad.set_build_plate(300, 150)
    s = cad.state()["settings"]
    assert (s["workplaneW"], s["workplaneL"]) == (300, 150)
    with pytest.raises(ScottiCADError):
        cad.set_build_plate(-1, 100)
    cad.set_settings(show_grid=False, show_shadows=False)
    s = cad.state()["settings"]
    assert not s["showGrid"] and not s["showShadows"]
    cad.set_settings(show_grid=True)
    s = cad.state()["settings"]
    assert s["showGrid"] and not s["showShadows"]
    cad.undo()
    assert not cad.state()["settings"]["showGrid"]


def test_ruler_remove(cad):
    cad.place_ruler(10, 20)
    assert "ruler" in cad.state()
    cad.remove_ruler()
    assert "ruler" not in cad.state()


def test_toggle_construction_plane(cad):
    box = cad.add_shape("box")
    cad.rotate("x", 30, ids=[box])
    cad.select([box])
    cad.toggle_construction_plane()
    assert "construction_plane" in cad.state()
    cad.toggle_construction_plane()
    assert "construction_plane" not in cad.state()
    cad.select([])
    cad.toggle_construction_plane()  # nothing selected: stays off
    assert "construction_plane" not in cad.state()


def test_camera_and_wheel(cad):
    cad.home_view()
    c = cad.camera()
    assert set(c) >= {"target", "yaw", "pitch", "dist", "ortho"}
    cad.set_camera(yaw=30, pitch=20, dist=150)
    c = cad.camera()
    assert math.isclose(c["yaw"], 30) and math.isclose(c["pitch"], 20) and math.isclose(c["dist"], 150)
    vp = cad.ui_state()["viewport"]
    cad.wheel(vp[0] + vp[2] / 2, vp[1] + vp[3] / 2, 2)
    assert cad.camera()["dist"] < 150
    cad.wheel(vp[0] + vp[2] / 2, vp[1] + vp[3] / 2, -4)
    assert cad.camera()["dist"] > 150
