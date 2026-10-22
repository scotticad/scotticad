"""ScottiCAD's own look: the shape libraries, the Settings dialog (theme,
background colour picker, dropdowns), the Export dialog's format list,
hiding notes, the start screen and the Place on surface tool."""
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch() as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    yield
    for _ in range(3):
        cad.key("esc")


def ids(cad):
    return {c["id"] for c in cad.controls()}


def test_libraries(cad):
    assert "moreshapes" not in ids(cad)
    cad.click_control("library")
    libs = {i for i in ids(cad) if i.startswith("lib-")}
    assert "lib-Parametric" in libs
    assert "lib-Featured" not in libs and "lib-Text and Numbers" not in libs
    cad.click_control("lib-Parametric")
    assert {"tile-gear", "tile-thread", "tile-grid"} <= ids(cad)
    cad.click_control("library")
    cad.click_control("lib-Basic Shapes")
    # the three hole tiles stay, and text is a basic shape
    assert {"tile-box-hole", "tile-cylinder-hole", "tile-sphere-hole", "tile-text"} <= ids(cad)


def test_background_colour_picker(cad):
    before = cad.ui_state()["background"]
    depth = cad.state()["undo_depth"]
    cad.click_control("settings")
    cad.click_control("set-bg")
    assert {"color-wheel", "bg-close"} <= ids(cad)
    # a click in the saturation/brightness square sets the background
    wheel = next(c["rect"] for c in cad.controls() if c["id"] == "color-wheel")
    cx, cy = wheel[0] + wheel[2] / 2, wheel[1] + wheel[3] / 2
    cad.click(cx + 20, cy - 20)
    after = cad.ui_state()["background"]
    assert after != before
    # the background isn't the design: no undo step
    assert cad.state()["undo_depth"] == depth
    cad.click_control("bg-close")
    assert "color-wheel" not in ids(cad)
    cad.click_control("set-reset")  # back to the default background
    assert cad.ui_state()["background"] == before


def test_settings_title_and_theme_row(cad):
    cad.click_control("settings")
    assert {"set-theme", "set-bg"} <= ids(cad)
    themes = ("Blue", "Green", "Orange", "Purple", "Tan", "Dark")
    start = cad.ui_state()["theme"]
    assert start in themes
    # the dropdown opens a list of every theme
    cad.click_control("set-theme")
    assert {"set-theme-" + t for t in themes} <= ids(cad)
    try:
        cad.click_control("set-theme-Dark")
        st = cad.ui_state()
        assert st["theme"] == "Dark" and "set-theme-Blue" not in ids(cad)
        # the default background goes dark with it, and back
        assert st["background"] != "#ffffff"
        cad.click_control("set-theme")
        cad.click_control("set-theme-Blue")
        assert cad.ui_state()["background"] == "#ffffff"
    finally:
        cad.click_control("set-theme")
        cad.click_control("set-theme-" + start)


def test_dropdowns_open_a_list(cad):
    cad.click_control("settings")
    assert "set-units-Imperial" not in ids(cad)
    cad.click_control("set-units")
    assert {"set-units-Metric", "set-units-Imperial"} <= ids(cad)
    # Esc closes the list, not the dialog
    cad.key("esc")
    assert "set-units-Imperial" not in ids(cad) and "set-units" in ids(cad)
    # a click outside the list closes it and changes nothing
    cad.click_control("set-preset")
    preset = "set-preset-Prusa MK4 (250 × 210)"
    assert preset in ids(cad)
    units = next(c["rect"] for c in cad.controls() if c["id"] == "set-units")
    cad.click(units[0] + units[2] / 2, units[1] + units[3] / 2)  # under the list's catcher
    assert preset not in ids(cad) and "set-units-Metric" not in ids(cad)
    # choosing an option picks that one, not the next
    cad.click_control("set-preset")
    cad.click_control(preset)
    assert "set-preset" in ids(cad) and preset not in ids(cad)
    cad.key("esc")  # closes Settings, discarding the draft


def test_export_formats_listed(cad):
    cad.add_shape("box")
    cad.click_control("export")
    assert {"exp-3mf", "exp-stl", "exp-obj", "exp-glb", "exp-svg"} <= ids(cad)


def test_hide_notes(cad):
    box = cad.add_shape("box")
    cad.add_note(box, 0, 0, 20, "a note")
    marker = "note-%d-0" % box
    assert marker in ids(cad)
    # the visibility menu: no connectors item; Hide notes works
    cad.click_control("visibility")
    assert "m-conn" not in ids(cad) and "m-notes" in ids(cad)
    cad.click_control("m-notes")
    assert cad.ui_state()["notes_hidden"] and marker not in ids(cad)
    # the design keeps the note, and nothing was undoable
    assert len(cad.notes()) == 1
    cad.key("shift+n")
    assert not cad.ui_state()["notes_hidden"] and marker in ids(cad)
    # the Notes tool shows hidden notes again
    cad.key("shift+n")
    cad.set_tool("notes")
    assert not cad.ui_state()["notes_hidden"]
    cad.set_tool("none")


def test_start_screen(cad, tmp_path):
    # automated runs start without it; the File menu opens it
    assert cad.ui_state()["popup"] == ""
    cad.click_control("filemenu")
    cad.click_control("dm-start")
    assert cad.ui_state()["popup"] == "start"
    assert {"start-new", "start-open"} <= ids(cad)
    cad.key("esc")
    # saving and opening put designs at the top of the recent list
    a, b = str(tmp_path / "first.scotticad"), str(tmp_path / "second.scotticad")
    cad.add_shape("box")
    cad.save(a)
    cad.add_shape("cylinder", x=30)
    cad.save(b)
    recent = cad.ui_state()["recent"]
    assert [os.path.normcase(p) for p in recent[:2]] == [os.path.normcase(b), os.path.normcase(a)]
    # a recent design opens from the start screen
    cad.new_design()
    cad.click_control("filemenu")
    cad.click_control("dm-start")
    cad.click_control("start-recent-1")
    assert cad.ui_state()["popup"] == "" and len(cad.nodes()) == 1
    assert os.path.normcase(cad.ui_state()["recent"][0]) == os.path.normcase(a)
    # one that has gone leaves the list
    os.remove(b)
    cad.click_control("filemenu")
    cad.click_control("dm-start")
    cad.click_control("start-recent-1")
    assert all(os.path.normcase(p) != os.path.normcase(b) for p in cad.ui_state()["recent"])
    assert cad.ui_state()["popup"] == "start"
    cad.key("esc")
    # New design from it starts afresh
    cad.click_control("filemenu")
    cad.click_control("dm-start")
    cad.click_control("start-new")
    assert cad.ui_state()["popup"] == "" and len(cad.nodes()) == 0


def test_place_on_surface_tooltip_and_eye(cad):
    cad.add_shape("box")
    cad.click_control("cruise")  # Place on surface's control id
    assert cad.ui_state()["cruise"]
    assert cad.ui_state()["banner"].startswith("Place on surface")
    cad.click_control("cruise")
    assert not cad.ui_state()["cruise"]
