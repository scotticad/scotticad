"""Bent tubes: a round tube made from a bend table, placed like any shape
and its table set from Python and from scripts."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1200, height=800) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    yield


def close(a, b, tol=1e-6):
    return all(abs(x - y) <= tol for x, y in zip(a, b))


def test_placed_tube_is_an_l_on_the_plate(cad):
    i = cad.add_shape("benttube")
    n = cad.node(i)
    assert n["bends"] == [{"length": 20, "angle": 90}, {"length": 20, "angle": 0}]
    assert abs(n["bounds"]["min"][2]) < 1e-6
    assert abs(n["bounds"]["size"][2] - 10) < 1e-6  # lying flat, its diameter tall
    assert n["volume"] > 0


def test_set_bends_keeps_the_start(cad):
    i = cad.add_shape("benttube", x=10, y=5)
    start = cad.node(i)["tube"]["start"]
    cad.set_bends(i, [(30, 90), (30, 90), (30,)])
    t = cad.node(i)["tube"]
    assert close(t["start"], start)
    # a U: back where it began along x, 2 bend radii plus 30 across
    r = cad.node(i)["params"].get("bend", 15)
    assert close(t["end"], [start[0], start[1] + 30 + 2 * r, start[2]], 1e-6)
    assert close(t["end_dir"], [-1, 0, 0])
    # dicts work too, and a turn takes the tube out of the plate's plane
    cad.set_bends(i, [{"length": 10, "angle": 90, "turn": 90}, {"length": 10}])
    assert cad.node(i)["tube"]["end"][2] > start[2] + 10
    assert cad.undo()
    assert len(cad.node(i)["bends"]) == 3


def test_bad_tables_are_refused(cad):
    i = cad.add_shape("benttube")
    before = cad.node(i)["bends"]
    for bad in ([], [(-5, 0)], [(10, 200)], "no"):
        with pytest.raises(ScottiCADError):
            cad.set_bends(i, bad)
    assert cad.node(i)["bends"] == before
    b = cad.add_shape("box")
    with pytest.raises(ScottiCADError):
        cad.set_bends(b, [(10,)])


def typein(cad, id, text):
    cad.click_control(id)
    cad.key("ctrl+a")
    cad.type_text(text)
    cad.key("enter")


def test_bend_table_in_the_inspector(cad):
    t = cad.add_shape("benttube")
    cad.select([t])
    ids = {c["id"] for c in cad.controls()}
    assert {"bend-add", "bend-info", "bend-0-0", "bend-0-1", "bend-0-2", "bend-1-0", "bend-del-0", "bend-del-1"} <= ids
    # typing a length, an angle and a turn: each one undo step
    depth = cad.state()["undo_depth"]
    typein(cad, "bend-0-0", "35")
    typein(cad, "bend-0-1", "-45")
    typein(cad, "bend-0-2", "90")
    assert cad.node(t)["bends"][0] == {"length": 35, "angle": -45, "turn": 90}
    assert cad.state()["undo_depth"] == depth + 3
    # what the table can't hold is refused, with the reason in a toast
    typein(cad, "bend-0-1", "500")
    assert cad.node(t)["bends"][0]["angle"] == -45 and "angle" in cad.ui_state()["toast"]
    typein(cad, "bend-0-0", "abc")
    assert cad.node(t)["bends"][0]["length"] == 35 and "not a number" in cad.ui_state()["toast"]
    assert cad.state()["undo_depth"] == depth + 3
    # in inches, lengths are typed in inches
    cad.set_units("in")
    typein(cad, "bend-0-0", "2")
    assert math.isclose(cad.node(t)["bends"][0]["length"], 50.8)
    cad.set_units("mm")
    # adding a row gives the last one a bend to go round, and a run after it
    cad.click_control("bend-add")
    b = cad.node(t)["bends"]
    assert len(b) == 3 and b[1]["angle"] == 90 and b[2] == {"length": b[1]["length"], "angle": 0}
    # deleting rows, down to the last, which stays
    cad.click_control("bend-del-1")
    cad.click_control("bend-del-0")
    assert len(cad.node(t)["bends"]) == 1
    cad.click_control("bend-del-0")
    assert len(cad.node(t)["bends"]) == 1 and "at least one row" in cad.ui_state()["toast"]
    # undo takes each change back in turn
    cad.undo()
    assert len(cad.node(t)["bends"]) == 2


def test_bend_markers_on_the_tube(cad):
    t = cad.add_shape("benttube")
    cad.set_bends(t, [(30, 90), (30, -90, 90), (30,)])
    box = cad.add_shape("box", x=80)
    cad.select([t])
    marks = [c for c in cad.controls() if c["id"].startswith("bend-mark-")]
    assert len(marks) == 3 and marks[0]["tip"] == "Row 1 of the bend table"
    # a marker's click is the marker's: the tube stays selected
    cad.click_control("bend-mark-1")
    assert cad.state()["selection"] == [t]
    # only the one selected tube has them
    cad.select([t, box])
    assert not [c for c in cad.controls() if c["id"].startswith("bend-mark-")]
    cad.select([box])
    assert not [c for c in cad.controls() if c["id"].startswith("bend")]


def test_tubes_group_split_and_export(cad, tmp_path):
    t = cad.add_shape("benttube")
    cad.set_param(t, "wall", 2)
    cad.set_bends(t, [(30, 90), (30, 90), (30,)])
    solid = cad.node(t)["volume"]
    hole = cad.add_shape("cylinder", hole=True, x=15)
    cad.set_size(x=4, y=4, z=30, ids=[hole])
    g = cad.group("union", ids=[t, hole])
    assert 0 < cad.node(g)["volume"] < solid
    assert cad.print_check(ids=[g])["parts"][0]["status"] == "ok"
    for ext in ("stl", "3mf", "obj", "glb"):
        p = tmp_path / ("tube." + ext)
        cad.export(str(p))
        assert p.stat().st_size > 100
    # a copy keeps its table, and a tube splits like any shape
    cad.new_design()
    t = cad.add_shape("benttube")
    whole = cad.node(t)["volume"]
    cad.select([t])
    (copy,) = cad.duplicate()
    assert cad.node(copy)["bends"] == cad.node(t)["bends"]
    halves = cad.split("x", ids=[t])
    assert len(halves) == 2
    assert math.isclose(sum(cad.node(h)["volume"] for h in halves), whole, rel_tol=1e-6)


def test_tube_from_a_script_and_saved(cad, tmp_path):
    r = cad.run_script(
        'i = cad.add_shape("benttube")\n'
        'cad.set_param(i, "diameter", 6)\n'
        'cad.set_param(i, "wall", 1)\n'
        'cad.set_bends(i, [[25, 45], [25, -45], [25]])\n'
        'print(len(cad.node(i)["bends"]))\n')
    assert r["error"] is None, r["error"]
    assert r["output"].strip() == "3"
    p = str(tmp_path / "tube.scotticad")
    cad.save(p)
    cad.new_design()
    cad.open(p)
    n = cad.nodes()[0]
    assert len(n["bends"]) == 3
    # hollow: less than a solid rod of the same path would be
    path = 75 + 2 * 15 * math.radians(45)
    assert 0 < n["volume"] < math.pi * 3 * 3 * path
