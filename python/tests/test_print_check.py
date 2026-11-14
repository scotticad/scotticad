"""The print check: the API method, and its section of the filament panel."""
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
    yield


def test_a_box_on_the_plate_is_ready(cad):
    b = cad.add_shape("box")
    r = cad.print_check()
    assert not r["pending"]
    assert r["parts"] == [{"id": b, "name": "Box", "status": "ok", "findings": [], "overhang_area": 0}]


def test_export_dialog_mentions_warnings(cad):
    ok = cad.add_shape("box")
    up = cad.add_shape("box", x=40, z=5)
    ids = lambda: {c["id"] for c in cad.controls()}
    # everything: one warning, and Show opens the print check
    cad.clear_selection()
    cad.click_control("export")
    assert cad.ui_state()["popup"] == "export"
    assert cad.ui_state()["export_warnings"] == 1 and "exp-check" in ids()
    cad.click_control("exp-check")
    st = cad.ui_state()
    assert st["popup"] == "" and st["filament_panel"]
    cad.click_control("filament")
    # the selection only: the box on the plate has none
    cad.select([ok])
    cad.click_control("export")
    assert cad.ui_state()["export_warnings"] == 0 and "exp-check" not in ids()
    cad.key("esc")
    # dropped, nothing to say
    cad.drop(ids=[up])
    cad.clear_selection()
    cad.click_control("export")
    assert cad.ui_state()["export_warnings"] == 0 and "exp-check" not in ids()
    cad.key("esc")
    assert "export_warnings" not in cad.ui_state()  # only in the dialog


def test_overhangs_are_information(cad):
    post = cad.add_shape("box")                    # 20 mm
    slab = cad.add_shape("box", z=20)
    cad.set_param(slab, "length", 40)
    cad.set_param(slab, "width", 40)
    cad.set_param(slab, "height", 2)
    g = cad.group(ids=[post, slab])
    p = cad.print_check()["parts"][0]
    assert p["id"] == g and p["status"] == "ok"
    assert p["overhang_area"] == pytest.approx(40 * 40 - 20 * 20)
    cad.clear_selection()
    cad.click_control("filament")
    lines = cad.ui_state()["print_check"]
    if lines == ["Checking…"]:
        lines = cad.ui_state()["print_check"]
    assert lines == ["Ready to print", "12.0 cm² of overhangs (may need support)"]
    cad.key("esc")


def test_a_floating_box_is_a_warning(cad):
    ok = cad.add_shape("box")
    up = cad.add_shape("box", x=40, z=5)
    cad.clear_selection()  # nothing selected: the whole design
    parts = cad.print_check()["parts"]
    assert [p["id"] for p in parts] == [up, ok]  # the worst first
    f = parts[0]["findings"]
    assert len(f) == 1 and f[0]["kind"] == "floating" and f[0]["severity"] == "warning"
    assert f[0]["height"] == pytest.approx(5) and "Press D" in f[0]["tip"]
    # ids check just those
    assert [p["id"] for p in cad.print_check(ids=[ok])["parts"]] == [ok]
    # dropped, it's fine
    cad.drop(ids=[up])
    assert all(p["status"] == "ok" for p in cad.print_check()["parts"])


def test_panel_lists_findings_and_selects_the_part(cad):
    cad.add_shape("box")
    up = cad.add_shape("box", x=40, z=5)
    cad.clear_selection()
    cad.click_control("filament")
    ids = {c["id"] for c in cad.controls()}
    assert "check-0" in ids
    cad.click_control("check-0")
    assert cad.state()["selection"] == [up]
    cad.key("esc")
