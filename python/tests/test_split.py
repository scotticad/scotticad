"""The Split tool: cutting shapes with a plane."""
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
    yield


def test_split_api(cad):
    box = cad.add_shape("box")  # 20 mm cube, z 0..20
    low, high = cad.split("z", 5, ids=[box])
    assert math.isclose(cad.node(low)["volume"], 20 * 20 * 5, rel_tol=1e-6)
    assert math.isclose(cad.node(high)["volume"], 20 * 20 * 15, rel_tol=1e-6)
    assert cad.selection()["ids"] == [low, high]
    # one undo step brings the cube back
    cad.undo()
    assert [n["id"] for n in cad.nodes()] == [box]
    # default: through the middle; keep one side; a gap
    (top,) = cad.split("x", keep="above", gap=6, ids=[box])
    b = cad.node(top)["bounds"]
    assert math.isclose(b["min"][0], 6, abs_tol=1e-6) and math.isclose(b["max"][0], 16, abs_tol=1e-6)
    with pytest.raises(ScottiCADError, match="doesn't cross"):
        cad.split("z", 50, ids=[top])


def test_split_from_a_script(cad):
    r = cad.run_script('b = cad.add_shape("cylinder")\nprint(len(cad.split("z", 8, ids=[b])))')
    assert r["error"] is None and r["output"] == "2"


def test_split_tool(cad):
    box = cad.add_shape("box")
    cad.select([box])
    cad.key("k")
    assert cad.ui_state()["mode"] == "split"
    ids = {c["id"] for c in cad.controls()}
    assert {"split-grip", "split-at", "split-axis-x", "split-keep-both", "split-gap", "split-apply"} <= ids
    # type the plane's height above the shape's base, a gap, then split
    cad.click_control("split-at")
    cad.type_text("4")
    cad.key("enter")
    cad.click_control("split-gap")
    cad.type_text("10")
    cad.key("enter")
    cad.click_control("split-apply")
    assert cad.ui_state()["mode"] == "none"
    halves = cad.selection()["ids"]
    assert len(halves) == 2
    lo, hi = (cad.node(i)["bounds"] for i in halves)
    assert math.isclose(lo["min"][2], 0, abs_tol=1e-6)        # the bottom half stays put
    assert math.isclose(lo["max"][2], 4, abs_tol=1e-6)
    assert math.isclose(hi["min"][2], 4 + 10, abs_tol=1e-6)   # the top half lifts by the gap


def test_split_tool_axis_keys_and_drag(cad):
    box = cad.add_shape("box")
    cad.select([box])
    cad.click_control("split")
    cad.key("x")                       # cut across X, through the middle
    cad.click_control("split-keep-below")
    cad.click_control("split-gap")     # the gap is remembered between uses
    cad.type_text("0")
    cad.key("enter")
    # drag the grip; the plane follows, snapped to the grid
    gx, gy = next((c["rect"][0] + c["rect"][2] / 2, c["rect"][1] + c["rect"][3] / 2)
                  for c in cad.controls() if c["id"] == "split-grip")
    cad.drag(gx, gy, gx + 40, gy, steps=8)
    cad.key("enter")
    (left,) = cad.selection()["ids"]
    b = cad.node(left)["bounds"]
    assert math.isclose(b["min"][0], -10, abs_tol=1e-6)
    assert b["max"][0] != pytest.approx(0) and -10 < b["max"][0] < 10
    assert math.isclose(b["max"][0], round(b["max"][0]), abs_tol=1e-6)  # on the 1 mm grid
