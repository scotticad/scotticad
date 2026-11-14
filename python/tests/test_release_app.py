"""Properties panel controls: a setting's dropdown list closing when its
shape is no longer shown, the bend table's steppers, and the shape
catalogue's lists."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1500, height=950) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    while cad.ui_state()["popup"]:
        cad.key("esc")
    yield


def ids(cad):
    return {c["id"] for c in cad.controls()}


def bearing_option(cad):
    kinds = {k["id"]: k for k in cad.shape_kinds()}
    p = next(p for p in kinds["bearingseat"]["params"] if p["key"] == "bearing")
    return "param-bearing-" + p["options"][0]


def test_dropdown_list_closes_when_its_shape_goes(cad):
    opt = bearing_option(cad)
    s = cad.add_shape("bearingseat", hole=True)
    box = cad.add_shape("box", x=60)
    cad.select([s])
    cad.click_control("param-bearing")
    assert opt in ids(cad)
    # another shape selected: its list goes, and doesn't come back by itself
    cad.select([box])
    assert opt not in ids(cad)
    cad.select([s])
    assert opt not in ids(cad)
    # nor after the inspector was folded away with the list open
    cad.click_control("param-bearing")
    cad.clear_selection()
    cad.select([s])
    assert opt not in ids(cad)


def test_shape_kinds_params_are_lists(cad):
    """A shape with no settings has an empty list of them, not null."""
    kinds = cad.shape_kinds()
    assert all(isinstance(k["params"], list) for k in kinds)
    assert any(k["params"] == [] for k in kinds)


def test_bend_table_steppers(cad):
    t = cad.add_shape("benttube")
    cad.select([t])
    before = cad.node(t)["bends"][0]
    depth = cad.state()["undo_depth"]
    cad.click_control("bend-0-0-up")  # length: 1 mm
    cad.click_control("bend-0-1-dn")  # angle: 5 degrees
    cad.click_control("bend-0-2-up")  # turn: 15 degrees
    b = cad.node(t)["bends"][0]
    assert math.isclose(b["length"], before["length"] + 1)
    assert math.isclose(b["angle"], before["angle"] - 5)
    assert math.isclose(b["turn"], 15)
    assert cad.state()["undo_depth"] == depth + 3
    # in inches a length steps by 1/16"
    cad.set_units("in")
    cad.click_control("bend-0-0-dn")
    assert math.isclose(cad.node(t)["bends"][0]["length"], b["length"] - 25.4 / 16)
    cad.set_units("mm")
    # a length never steps below 0
    cad.set_bends(t, [(0.5, 90), (20,)])
    cad.click_control("bend-0-0-dn")
    assert cad.node(t)["bends"][0]["length"] == 0
