"""The filament estimate, and the toolbar's second row on narrow windows."""
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
    cad.set_filament_settings(material="PLA", diameter=1.75, layer_height=0.2, line_width=0.42, walls=2,
                              top_layers=4, bottom_layers=3, infill=15, price_per_kg=20, calibration=0)
    yield


def test_estimate_of_a_cube(cad):
    cad.add_shape("box")  # 20 mm cube
    e = cad.filament_estimate()
    shell = 4 * 400 * 0.84 + 400 * 0.8 + 400 * 0.6
    vol = shell + (8000 - shell) * 0.15
    assert math.isclose(e["volume"], vol, rel_tol=1e-9)
    assert math.isclose(e["grams"], vol / 1000 * 1.24, rel_tol=1e-9)
    assert math.isclose(e["meters"], vol / (math.pi * 1.75 ** 2 / 4) / 1000, rel_tol=1e-9)
    assert math.isclose(e["cost"], e["grams"] / 1000 * 20, rel_tol=1e-9)
    # settings change the estimate; a material sets its density
    s = cad.set_filament_settings(infill=100, material="PETG")
    assert s["material"] == "PETG" and math.isclose(s["density"], 1.27)
    assert math.isclose(cad.filament_estimate()["grams"], 8000 / 1000 * 1.27, rel_tol=1e-9)


def test_holes_dont_print_and_colours_are_listed(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.set_color("#2f7fe6", ids=[b])
    hole = cad.add_shape("cylinder", hole=True)
    cad.set_size(x=10, y=10, z=40, ids=[hole])
    g = cad.group("union", ids=[a, hole])
    cad.clear_selection()  # nothing selected: the whole design
    e = cad.filament_estimate()
    assert e["shapes"] == 2 and len(e["by_color"]) == 2
    total = sum(c["grams"] for c in e["by_color"])
    assert math.isclose(total, e["grams"], rel_tol=1e-9)
    # holes don't print, but they add walls: at low infill a box with a
    # hole through it takes more than a plain one (as in a slicer), at
    # 100% it takes less
    with_hole = cad.filament_estimate(ids=[g])["grams"]
    plain = cad.filament_estimate(ids=[b])["grams"]
    assert with_hole > plain
    cad.set_filament_settings(infill=100)
    assert cad.filament_estimate(ids=[g])["grams"] < cad.filament_estimate(ids=[b])["grams"]
    # only the selection when something is selected
    cad.select([b])
    assert math.isclose(cad.filament_estimate()["grams"], cad.filament_estimate(ids=[b])["grams"], rel_tol=1e-9)


def test_panel(cad):
    box = cad.add_shape("box")
    cad.click_control("filament")
    assert cad.ui_state()["filament_panel"]
    ids = {c["id"] for c in cad.controls()}
    assert "fil-settings" in ids and "fil-infill" not in ids
    cad.click_control("fil-settings")
    ids = {c["id"] for c in cad.controls()}
    assert {"fil-material", "fil-infill", "fil-walls", "fil-price_per_kg"} <= ids
    cad.click_control("fil-infill")
    cad.key("ctrl+a")
    cad.type_text("40")
    cad.key("enter")
    assert cad.set_filament_settings()["infill"] == 40
    cad.click_control("fil-material")   # next preset after PLA
    assert cad.set_filament_settings()["material"] == "PETG"
    # it sits beside the properties panel, not over it
    cad.select([box])
    panel = rect(cad, "fil-close")
    insp = next(c["rect"] for c in cad.controls() if c["id"].startswith("insp-"))
    assert panel[0] + panel[2] < insp[0]
    # it stays open while the design is edited: clicking a shape, the
    # viewport, a handle
    cad.click(*cad.project(0, 0, 10))
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + 80, vy + vh - 40)
    assert cad.ui_state()["filament_panel"]
    # its ✕, the toolbar button and Esc close it
    cad.click_control("fil-close")
    assert not cad.ui_state()["filament_panel"]
    cad.click_control("filament")
    cad.key("esc")
    assert not cad.ui_state()["filament_panel"]


def rect(cad, id):
    return next(c["rect"] for c in cad.controls() if c["id"] == id)


def test_toolbar_on_narrow_windows():
    # wide: one toolbar row with everything
    with launch(headless=True, width=1400, height=700) as wide:
        assert rect(wide, "copy")[1] == rect(wide, "align")[1]
        assert wide.ui_state()["viewport"][1] == 48 + 46
    # narrow: the clipboard and undo buttons move up into the title bar,
    # after the design name; the toolbar stays one row
    with launch(headless=True, width=900, height=700) as narrow:
        copy, align, name = rect(narrow, "copy"), rect(narrow, "align"), rect(narrow, "docname")
        assert copy[1] < 48 <= align[1]
        assert copy[0] > name[0] + name[2]
        assert narrow.ui_state()["viewport"][1] == 48 + 46
    # very narrow: no room in the title bar either, so a toolbar row of
    # their own above the tools
    with launch(headless=True, width=560, height=600) as tiny:
        copy, align = rect(tiny, "copy"), rect(tiny, "align")
        assert 48 <= copy[1] < 48 + 46 <= align[1]
        assert tiny.ui_state()["viewport"][1] == 48 + 2 * 46
