"""The GUI at other interface scales (150 %, 200 %) and at the smallest
window it allows: the same click-and-drag tests the GUI has at 100 %,
run again with everything laid out differently, plus a check that every
control stays inside the window."""
import inspect
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402

import test_api as ta  # noqa: E402
import test_gaps as tg  # noqa: E402
import test_gui_paths as gp  # noqa: E402
import test_view_tools as tv  # noqa: E402

CASES = [
    gp.test_mirror_by_clicking_an_arrow, gp.test_align_by_clicking_a_dot, gp.test_measure_points_and_type_a_distance,
    gp.test_visibility_menu_and_mode, gp.test_cruise_drags_onto_a_surface, gp.test_inspector_buttons,
    gp.test_duplicate_and_drop_buttons, gp.test_view_controls, gp.test_notes_and_ruler_tabs,
    gp.test_panel_toggle_and_library, gp.test_build_plate_clicks_in_ortho_view,
    tg.test_losing_the_mouse_mid_drag, tg.test_resize_with_shift_and_alt, tg.test_orbit_and_pan, tg.test_editing_shortcuts,
    tv.test_section_view_controls, tv.test_lay_flat_tool_clicks_a_face,
    ta.test_gui_drag_tile_and_shortcuts,
]


def min_window():
    with launch(headless=True) as c:
        return c.ui_state()["min_window"]


@pytest.fixture(scope="module", params=["150%", "200%", "smallest"])
def cad(request):
    if request.param == "smallest":
        w, h = min_window()
        with launch(headless=True, width=w, height=h) as c:
            assert c.ui_state()["window"] == [w, h]
            yield c
    else:
        scale = float(request.param.rstrip("%")) / 100
        with launch(headless=True, width=1400, height=860, scale=scale) as c:
            assert c.ui_state()["scale"] == scale
            yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    if cad.ui_state()["cruise"]:
        cad.set_tool("cruise")
    if cad.ui_state()["popup"]:
        cad.key("esc")
    if cad.console()["open"]:
        cad.click_control("scripting")
    cad.set_section(False)
    cad.home_view()
    cad.set_camera(ortho=False)
    if not any(c["id"] == "tile-box" for c in cad.controls()):  # another library was left showing
        cad.click_control("library")
        cad.click_control("lib-Basic Shapes")
    yield


def test_controls_fit_the_window(cad):
    w, h = cad.ui_state()["window"]
    cad.add_shape("box")
    cad.select_all()
    for c in cad.controls():
        x, y, cw, ch = c["rect"]
        assert x >= -1 and y >= -1 and x + cw <= w + 1 and y + ch <= h + 1, (c["id"], c["rect"], (w, h))
    # the toolbar's tools are all there to click, none under another
    ids = {c["id"] for c in cad.controls()}
    for tool in ("union", "bundle", "ungroup", "align", "mirror", "split", "section", "undo", "redo", "export", "filemenu"):
        assert tool in ids, tool
    for tool in ("align", "mirror"):
        cad.click_control(tool)
        assert cad.ui_state()["mode"] == tool
        cad.key("esc")


def test_inspector_scrolls_in_a_short_window(cad):
    st = cad.ui_state()
    if st["window"] != st["min_window"]:
        pytest.skip("only the smallest window is too short for the inspector")
    w, h = st["window"]
    i = cad.add_shape("extrudedbox")  # ten sliders: far taller than the window
    cad.select([i])
    insp = next(c for c in cad.controls() if c["id"] == "inspector")
    x, y, cw, ch = insp["rect"]
    assert y + ch <= h, insp
    controls = lambda: {c["id"]: c["rect"] for c in cad.controls()}
    assert "slider-length" in controls() and "slider-panels" not in controls()  # the last one is out of view
    for _ in range(40):
        cad.wheel(x + cw / 2, y + ch / 2, -3)
    after = controls()
    assert "slider-panels" in after, "wheeling didn't bring the last slider into view"
    for c, r in after.items():
        if c.startswith("slider-"):
            assert r[1] >= y and r[1] + r[3] <= y + ch + 1, (c, r)
    # the slider that scrolled into view works
    r = after["slider-panels"]
    cad.click(r[0] + r[2] * 0.1, r[1] + r[3] / 2)
    assert cad.node(i)["params"]["panels"] == 0
    for _ in range(40):
        cad.wheel(x + cw / 2, y + ch / 2, 3)
    assert "slider-length" in controls() and "slider-panels" not in controls()


# at the smallest window the inspector covers the right half of the
# viewport, where these tests put their drags and clicks
SKIP_SMALLEST = {gp.test_cruise_drags_onto_a_surface, gp.test_build_plate_clicks_in_ortho_view}


@pytest.mark.parametrize("case", CASES, ids=lambda f: f.__name__)
def test_gui_case(cad, case, tmp_path):
    st = cad.ui_state()
    if case in SKIP_SMALLEST and st["window"] == st["min_window"]:
        pytest.skip("the inspector covers the viewport's right half at the smallest window")
    kw = {}
    if "tmp_path" in inspect.signature(case).parameters:
        kw["tmp_path"] = tmp_path
    case(cad, **kw)
