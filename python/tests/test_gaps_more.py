"""More paths not covered elsewhere: redo and nudge keys, the tool keys, the toolbar's buttons, the
snap menu, the File menu, the Settings and Import dialogs, typing keys in
a field, the lift handle, and a few API edge cases."""
import math
import os
import struct
import subprocess
import sys
import time

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, default_exe, launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1500, height=950) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    cad.set_units("mm")
    cad.set_snap(1)
    cad.home_view()
    cad.set_camera(ortho=False)
    while cad.ui_state()["popup"]:
        cad.key("esc")
    yield


def ids(cad):
    return {c["id"] for c in cad.controls()}


def rect(cad, id):
    return next(c["rect"] for c in cad.controls() if c["id"] == id)


def near(a, b, tol=1e-6):
    return all(math.isclose(x, y, abs_tol=tol) for x, y in zip(a, b))


def typein(cad, id, text):
    cad.click_control(id)
    cad.key("ctrl+a")
    cad.type_text(text)
    cad.key("enter")


# --- keys -------------------------------------------------------------------

def test_redo_keys(cad):
    cad.add_shape("box")
    cad.add_shape("box", x=40)
    cad.key("ctrl+z")
    cad.key("ctrl+z")
    assert cad.nodes() == []
    cad.key("ctrl+y")
    assert len(cad.nodes()) == 1
    cad.key("ctrl+shift+z")
    assert len(cad.nodes()) == 2
    cad.key("ctrl+y")  # nothing left to redo
    assert len(cad.nodes()) == 2


def test_ctrl_arrows_move_up_and_down(cad):
    b = cad.add_shape("box")
    cad.select([b])
    z = lambda: cad.node(b)["bounds"]["min"][2]
    cad.key("ctrl+up")
    assert math.isclose(z(), 1)
    cad.key("ctrl+shift+up")
    assert math.isclose(z(), 11)
    cad.key("ctrl+down")
    assert math.isclose(z(), 10)
    cad.key("ctrl+shift+down")
    assert math.isclose(z(), 0)
    # a finer snap grid gives finer steps
    cad.set_snap(0.5)
    cad.key("ctrl+up")
    assert math.isclose(z(), 0.5)


def test_arrow_keys_follow_the_view(cad):
    b = cad.add_shape("box")
    cad.select([b])
    c0 = cad.node(b)["bounds"]["center"]
    cad.key("right")
    cad.key("up")
    c1 = cad.node(b)["bounds"]["center"]
    # one snap step along each of X and Y, whichever way the view faces
    assert sorted(abs(c1[i] - c0[i]) for i in range(3)) == [0, 1, 1]
    cad.key("left")
    cad.key("down")
    assert near(cad.node(b)["bounds"]["center"], c0)
    cad.key("shift+right")
    c2 = cad.node(b)["bounds"]["center"]
    assert sorted(abs(c2[i] - c0[i]) for i in range(3)) == [0, 0, 10]
    # with nothing selected the arrows do nothing
    cad.clear_selection()
    cad.key("right")
    assert near(cad.node(b)["bounds"]["center"], c2)


def test_tool_keys(cad):
    b = cad.add_shape("box")
    cad.select([b])
    for key, mode in (("r", "ruler"), ("w", "construction_plane"), ("n", "notes"), ("l", "align"), ("m", "mirror"),
                      ("v", "visibility"), ("k", "split"), ("shift+d", "layflat")):
        cad.select([b])
        cad.key(key)
        assert cad.ui_state()["mode"] == mode, key
        cad.key(key)  # the same key again leaves the tool
        assert cad.ui_state()["mode"] == "none", key
    cad.key("c")
    assert cad.ui_state()["cruise"]
    cad.key("esc")
    assert not cad.ui_state()["cruise"]
    cad.key("f1")
    assert cad.ui_state()["popup"] == "help"
    cad.key("esc")
    cad.type_text("?")
    assert cad.ui_state()["popup"] == "help"
    cad.key("esc")
    assert cad.ui_state()["popup"] == ""


def test_esc_backs_out_one_thing_at_a_time(cad):
    b = cad.add_shape("box")
    # a tile armed for placing: Esc disarms it and keeps the selection
    cad.select([b])
    cad.click_control("tile-cylinder")
    assert cad.ui_state()["placing"]
    cad.key("esc")
    assert not cad.ui_state()["placing"] and cad.state()["selection"] == [b]
    # a construction plane on a shape: Esc takes it off before it clears the selection
    cad.clear_selection()
    cad.attach_construction_plane(b)
    assert "construction_plane" in cad.state()
    cad.select([b])
    cad.key("esc")
    assert "construction_plane" not in cad.state() and cad.state()["selection"] == [b]
    cad.key("esc")
    assert cad.state()["selection"] == [] and cad.selection()["ids"] == []  # empty lists, not null


def test_s_makes_a_hole_solid_again(cad):
    b = cad.add_shape("box")
    colour = cad.node(b)["color"]
    cad.select([b])
    cad.key("h")
    assert cad.node(b)["hole"]
    cad.key("s")
    n = cad.node(b)
    assert not n["hole"] and n["color"] == colour
    # a shape placed as a hole has no colour of its own: it gets its kind's
    h = cad.add_shape("cylinder", hole=True, x=40)
    cad.select([h])
    cad.key("s")
    n = cad.node(h)
    assert not n["hole"] and n["color"] == cad.node(cad.add_shape("cylinder", x=80))["color"]


# --- the toolbar's buttons ------------------------------------------------------

def test_toolbar_buttons(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=10)
    cad.select([a, b])
    cad.click_control("union")
    (g,) = cad.nodes()
    assert g["kind"] == "group" and g["group_mode"] == "union"
    cad.click_control("ungroup")
    assert len(cad.nodes()) == 2
    cad.select([n["id"] for n in cad.nodes()])
    cad.click_control("bundle")
    (g,) = cad.nodes()
    assert g["group_mode"] == "bundle"
    cad.click_control("undo")
    assert len(cad.nodes()) == 2
    cad.click_control("redo")
    assert len(cad.nodes()) == 1
    cad.select([g["id"]])
    cad.click_control("copy")
    cad.click_control("paste")
    assert len(cad.nodes()) == 2
    cad.click_control("duplicate")
    assert len(cad.nodes()) == 3
    cad.click_control("delete")
    assert len(cad.nodes()) == 2
    cad.click_control("layflat")
    assert cad.ui_state()["mode"] == "layflat"
    cad.click_control("layflat")
    assert cad.ui_state()["mode"] == "none"
    cad.clear_selection()
    cad.click_control("drop")  # nothing selected: nothing happens
    assert len(cad.nodes()) == 2


# --- menus -------------------------------------------------------------------

def test_snap_menu(cad):
    cad.click_control("snap")
    assert cad.ui_state()["popup"] == "snap"
    assert {"snap-Off", "snap-0.1 mm", "snap-0.5 mm", "snap-1.0 mm", "snap-10.0 mm"} <= ids(cad)
    cad.click_control("snap-0.5 mm")
    assert cad.ui_state()["popup"] == "" and cad.state()["settings"]["snap"] == 0.5
    # the snap is what a nudge moves by
    b = cad.add_shape("box")
    cad.select([b])
    cad.key("ctrl+up")
    assert math.isclose(cad.node(b)["bounds"]["min"][2], 0.5)
    cad.click_control("snap")
    cad.click_control("snap-Off")
    assert cad.state()["settings"]["snap"] == 0
    # Esc closes the menu, changing nothing
    cad.click_control("snap")
    cad.key("esc")
    assert cad.ui_state()["popup"] == "" and cad.state()["settings"]["snap"] == 0
    # in inches the menu offers fractions of an inch
    cad.set_units("in")
    cad.click_control("snap")
    inch = sorted(i for i in ids(cad) if i.startswith("snap-"))
    assert inch and not any(i.endswith(" mm") for i in inch), inch
    cad.click_control(next(i for i in inch if i != "snap-Off"))
    assert cad.state()["settings"]["snap"] > 0


def test_file_menu_items(cad):
    cad.key("alt+f")
    assert cad.ui_state()["popup"] == "design_menu"
    cad.click_control("dm-import")
    assert cad.ui_state()["popup"] == "import"
    cad.key("esc")
    cad.click_control("filemenu")
    cad.click_control("dm-export")
    assert cad.ui_state()["popup"] == "export"
    cad.key("esc")
    cad.click_control("filemenu")
    cad.click_control("dm-help")
    assert cad.ui_state()["popup"] == "help"
    cad.key("esc")
    cad.click_control("filemenu")
    cad.click_control("dm-about")
    assert cad.ui_state()["popup"] == "about"
    cad.key("esc")
    # Alt+F again closes the menu
    cad.key("alt+f")
    cad.key("alt+f")
    assert cad.ui_state()["popup"] == ""


def test_exit_from_the_file_menu_asks_about_changes():
    with launch(headless=True) as c:
        c.add_shape("box")
        # Cancel: still running, the design still there
        c.answer_dialogs(("message", "cancel"))
        c.click_control("filemenu")
        c.click_control("dm-exit")
        assert len(c.nodes()) == 1
        # No: it ends without saving
        c.answer_dialogs(("message", "no"))
        c.click_control("filemenu")
        try:
            c.click_control("dm-exit")
        except (ConnectionError, OSError, ScottiCADError):
            pass
        assert c.process.wait(timeout=10) == 0
        c.process = None  # gone: nothing to quit


# --- dialogs -----------------------------------------------------------------

def test_settings_dialog(cad):
    cad.click_control("settings")
    assert cad.ui_state()["popup"] == "settings"
    s0 = cad.state()["settings"]
    depth = cad.state()["undo_depth"]
    # the plate's size, typed and stepped; nonsense is ignored
    typein(cad, "set-Width", "250")
    typein(cad, "set-Length", "210")
    cad.click_control("set-Width-up")
    typein(cad, "set-Length", "-5")
    typein(cad, "set-Length", "wide")
    cad.click_control("set-grid")
    cad.click_control("set-shadows")
    # the zoom speed slider drags
    x, y, w, h = rect(cad, "set-zoom")
    cad.drag(x + w * 0.2, y + h / 2, x + w * 0.9, y + h / 2)
    # nothing changes in the design until the dialog is closed
    assert cad.state()["settings"] == s0
    cad.click_control("set-close")
    assert cad.ui_state()["popup"] == ""
    s = cad.state()["settings"]
    assert (s["workplaneW"], s["workplaneL"]) == (260, 210)
    assert s["showGrid"] != s0["showGrid"] and s["showShadows"] != s0["showShadows"]
    # the whole visit is one undo step
    assert cad.state()["undo_depth"] == depth + 1
    cad.undo()
    assert cad.state()["settings"] == s0
    # units: inches, with a snap in inches; and back
    cad.click_control("settings")
    cad.click_control("set-units")
    cad.click_control("set-units-Imperial")
    cad.click_control("set-close")
    s = cad.state()["settings"]
    assert s["units"] == "in" and math.isclose(s["snap"], 25.4 / 16)
    cad.click_control("settings")
    cad.click_control("set-Width-up")  # an inch at a time now
    cad.click_control("set-units")
    cad.click_control("set-units-Metric")
    cad.click_control("set-close")
    s = cad.state()["settings"]
    assert s["units"] == "mm" and s["snap"] == 1 and math.isclose(s["workplaneW"], 225.4)
    # a printer's plate from the presets, then Reset
    cad.click_control("settings")
    cad.click_control("set-preset")
    preset = sorted(i for i in ids(cad) if i.startswith("set-preset-"))[0]
    cad.click_control(preset)
    cad.click_control("set-close")
    assert cad.state()["settings"]["workplaneW"] != 225.4
    cad.click_control("settings")
    cad.click_control("set-reset")
    cad.click_control("set-close")
    s = cad.state()["settings"]
    assert (s["workplaneW"], s["workplaneL"], s["showGrid"], s["showShadows"]) == (200, 200, True, True)
    # Esc closes it without applying
    cad.click_control("settings")
    typein(cad, "set-Width", "300")
    cad.key("esc")
    assert cad.ui_state()["popup"] == "" and cad.state()["settings"]["workplaneW"] == 200
    # and the next visit starts from the design's settings, not the abandoned ones
    cad.click_control("settings")
    cad.click_control("set-close")
    assert cad.state()["settings"]["workplaneW"] == 200


def test_import_dialog_fields(cad, tmp_path):
    stl = tmp_path / "part.stl"
    b = cad.add_shape("box")
    cad.export(str(stl))
    cad.new_design()
    cad.click_control("import")
    # nothing chosen yet
    cad.click_control("imp-go")
    assert cad.ui_state()["popup"] == "import" and "Choose a file" in cad.ui_state()["toast"]
    cad.answer_dialogs(("open", str(stl)))
    cad.click_control("imp-choose")
    typein(cad, "imp-scale", "200")
    cad.click_control("imp-go")
    assert cad.ui_state()["popup"] == ""
    (n,) = cad.nodes()
    assert near(n["bounds"]["size"], [40, 40, 40], 1e-4)
    # Cancel imports nothing
    cad.click_control("import")
    cad.answer_dialogs(("open", str(stl)))
    cad.click_control("imp-choose")
    cad.click_control("imp-cancel")
    assert cad.ui_state()["popup"] == "" and len(cad.nodes()) == 1
    # the scale's arrows, and nonsense typed
    cad.click_control("import")
    cad.answer_dialogs(("open", str(stl)))
    cad.click_control("imp-choose")
    typein(cad, "imp-scale", "nonsense")
    typein(cad, "imp-scale", "100")
    cad.click_control("imp-scale-up")
    cad.click_control("imp-go")
    assert near(cad.nodes()[-1]["bounds"]["size"], [22, 22, 22], 1e-4)


# --- fields and handles ---------------------------------------------------------

def test_editing_keys_in_a_field(cad):
    b = cad.add_shape("box")
    cad.select([b])
    field = lambda: cad.ui_state()["field"]["text"]
    cad.click_control("param-length")
    assert field() == "20.00"
    cad.type_text("20")     # the text was all selected: typing replaces it
    cad.key("ctrl+a")
    cad.key("end")          # all selected again: End puts the caret after it
    cad.type_text("5")
    assert field() == "205"
    cad.key("home")
    cad.key("delete")
    assert field() == "05"
    cad.key("right")
    cad.key("left")
    cad.type_text("1")
    assert field() == "105"
    cad.key("end")
    cad.key("left")
    cad.key("backspace")
    assert field() == "15"
    cad.key("ctrl+a")
    cad.key("delete")       # everything selected: Delete clears it
    assert field() == ""
    cad.type_text("32")
    cad.key("enter")
    assert math.isclose(cad.node(b)["bounds"]["size"][0], 32)
    # Esc leaves a field without changing anything
    cad.click_control("param-length")
    cad.type_text("99")
    cad.key("esc")
    assert "field" not in cad.ui_state() and math.isclose(cad.node(b)["bounds"]["size"][0], 32)


def system_clipboard():
    """The system clipboard's text on Windows, read without changing it;
    None elsewhere."""
    if sys.platform != "win32":
        return None
    import ctypes
    from ctypes import wintypes
    u32, k32 = ctypes.windll.user32, ctypes.windll.kernel32
    u32.GetClipboardData.restype = wintypes.HANDLE
    k32.GlobalLock.restype = ctypes.c_void_p
    k32.GlobalLock.argtypes = [wintypes.HANDLE]
    k32.GlobalUnlock.argtypes = [wintypes.HANDLE]
    for _ in range(20):  # another program may have it open
        if u32.OpenClipboard(None):
            break
        time.sleep(0.05)
    else:
        pytest.skip("the system clipboard stayed busy")
    try:
        h = u32.GetClipboardData(13)  # CF_UNICODETEXT
        if not h:
            return ""
        p = k32.GlobalLock(h)
        try:
            return ctypes.wstring_at(p) if p else ""
        finally:
            k32.GlobalUnlock(h)
    finally:
        u32.CloseClipboard()


def test_copy_and_paste_stay_off_the_system_clipboard(cad):
    """A headless ScottiCAD keeps its own clipboard, so running the tests
    doesn't replace what the user has copied; copy and paste still work
    inside it."""
    before = system_clipboard()
    b = cad.add_shape("box")
    cad.select([b])
    cad.click_control("param-length")
    cad.key("ctrl+a")
    cad.key("ctrl+c")
    cad.key("esc")
    cad.click_control("param-width")
    cad.key("ctrl+a")
    cad.key("ctrl+v")
    assert cad.ui_state()["field"]["text"] == "20.00"
    cad.key("esc")
    cad.key("ctrl+c")       # the shape itself
    cad.key("ctrl+v")
    assert len(cad.nodes()) == 2
    assert system_clipboard() == before


def test_lift_handle_raises_the_shape(cad):
    b = cad.add_shape("box")
    cad.select([b])
    before = cad.node(b)["bounds"]
    depth = cad.state()["undo_depth"]
    lift = next(h for h in cad.handles() if h["kind"] == "lift")
    cad.drag(lift["x"], lift["y"], lift["x"], lift["y"] - 80, steps=8)
    after = cad.node(b)["bounds"]
    z = after["min"][2]
    assert z > 1 and math.isclose(z, round(z))  # raised, by whole steps of the snap grid
    assert near(after["size"], before["size"]) and near(after["min"][:2], before["min"][:2])
    assert cad.state()["undo_depth"] == depth + 1
    # and back down, below the plate
    lift = next(h for h in cad.handles() if h["kind"] == "lift")
    cad.drag(lift["x"], lift["y"], lift["x"], lift["y"] + 200, steps=8)
    assert cad.node(b)["bounds"]["min"][2] < 0
    cad.undo()
    cad.undo()
    assert near(cad.node(b)["bounds"]["min"], before["min"])


def turned(cad, b, degrees, far, mods=None):
    """Drags b's Z rotation handle degrees round its centre, the pointer at
    far times the handle's distance, seen from above; returns the angle the
    protractor then shows."""
    cad.set_camera(yaw=-90, pitch=89.9, ortho=True)
    cad.select([b])
    hz = next(h for h in cad.handles() if h["kind"] == "rotate" and h["index"] == 2)
    cx, cy = cad.project(*cad.node(b)["bounds"]["center"])
    r = math.hypot(hz["x"] - cx, hz["y"] - cy)
    a0 = math.atan2(hz["y"] - cy, hz["x"] - cx)
    cad.mouse_move(hz["x"], hz["y"])
    cad.mouse_down(hz["x"], hz["y"])
    steps = max(6, abs(degrees) // 10)
    for i in range(1, steps + 1):
        a = a0 - math.radians(degrees) * i / steps  # screen y runs down: this is anticlockwise
        x, y = cx + r * far * math.cos(a), cy + r * far * math.sin(a)
        cad.mouse_move(x, y, mods=mods or [])
        cad.controls()  # a frame, with the protractor up
    cad.mouse_up(x, y)
    cad.click_control("angle")
    shown = float(cad.ui_state()["field"]["text"].rstrip("°"))
    cad.key("esc")
    return shown


def test_rotation_steps(cad):
    b = cad.add_shape("box")
    cad.set_size(x=40, y=10, ids=[b])
    size = lambda: cad.node(b)["bounds"]["size"]
    box = lambda deg: [40 * abs(math.cos(math.radians(deg))) + 10 * abs(math.sin(math.radians(deg))),
                       40 * abs(math.sin(math.radians(deg))) + 10 * abs(math.cos(math.radians(deg))), 20]
    # inside the protractor: steps of 22.5 degrees. The protractor counts
    # clockwise (seen from above) as positive, so an anticlockwise drag
    # shows a negative angle
    assert turned(cad, b, 37, 0.8) == -45
    assert near(size(), box(45), 1e-6)
    cad.undo()
    # out past its band: single degrees
    assert turned(cad, b, 37, 3.0) == -37
    assert near(size(), box(37), 1e-6)
    cad.undo()
    assert turned(cad, b, -100, 3.0) == 100
    cad.undo()
    # Shift: steps of 45 degrees, wherever the pointer is
    assert turned(cad, b, 50, 3.0, mods=["shift"]) == -45
    cad.undo()
    assert turned(cad, b, 50, 0.8, mods=["shift"]) == -45
    cad.undo()
    # a sweep past half a turn keeps counting
    assert turned(cad, b, 200, 0.8) == -202.5
    assert near(size(), box(202.5), 1e-6)
    cad.undo()
    assert near(size(), [40, 10, 20])
    # the shape follows the pointer: an anticlockwise drag is what
    # cad.rotate calls a positive angle about z
    far = cad.add_shape("box", x=60)
    cad.select([b, far])
    c0 = cad.node(far)["bounds"]["center"]
    cad.rotate("z", 90, ids=[b, far])
    by_api = cad.node(far)["bounds"]["center"]
    cad.undo()
    assert near(cad.node(far)["bounds"]["center"], c0)
    g = cad.group("bundle", ids=[b, far])
    assert turned(cad, g, 90, 3.0) == -90
    cad.ungroup(ids=[g])
    assert near(cad.nodes()[-1]["bounds"]["center"], by_api, 1e-6)
    # an angle typed into the protractor after the drag replaces it
    cad.new_design()
    b = cad.add_shape("box")
    cad.set_size(x=40, y=10, ids=[b])
    turned(cad, b, 37, 3.0)
    depth = cad.state()["undo_depth"]
    typein(cad, "angle", "-90")
    assert near(size(), [10, 40, 20], 1e-6) and cad.state()["undo_depth"] == depth + 1  # a step of its own
    typein(cad, "angle", "sideways")
    assert "not an angle" in cad.ui_state()["toast"] and near(size(), [10, 40, 20], 1e-6)


def test_view_cube_faces(cad):
    x, y, w, h = rect(cad, "viewcube")
    for yaw, pitch, want in ((12, 12, (0, 0)), (102, 12, (90, 0)), (192, 12, (180, 0)), (-78, 12, (-90, 0)),
                             (20, 70, (20, 90)), (20, -70, (20, -90))):
        cad.set_camera(yaw=yaw, pitch=pitch)
        cad.click(x + w / 2, y + h / 2)  # the face in the middle: the view squares up to it
        c = cad.camera()
        assert math.isclose(c["yaw"], want[0], abs_tol=1e-6) and math.isclose(c["pitch"], want[1], abs_tol=0.1), (yaw, pitch, c)
    # a drag on the cube turns the view
    cad.home_view()
    before = cad.camera()
    cad.drag(x + w / 2, y + h / 2, x + w / 2 + 20, y + h / 2 + 5, steps=4)
    after = cad.camera()
    assert not math.isclose(after["yaw"], before["yaw"], abs_tol=1) and after["dist"] == before["dist"]


def test_dragging_a_tile_from_the_panel(cad):
    x, y, w, h = rect(cad, "tile-cylinder")
    tx, ty = cad.project(30, -20, 0)
    cad.mouse_move(x + w / 2, y + h / 2)
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(x + w / 2 - 40, y + h / 2)
    cad.mouse_move(tx, ty)
    assert cad.ui_state()["placing"] and cad.nodes() == []  # a ghost follows the pointer; nothing is added yet
    cad.controls()
    cad.mouse_up(tx, ty)
    (n,) = cad.nodes()
    assert n["kind"] == "cylinder" and cad.state()["selection"] == [n["id"]]
    c = n["bounds"]["center"]
    assert abs(c[0] - 30) <= 0.5 and abs(c[1] + 20) <= 0.5 and near(n["bounds"]["min"][2:], [0])
    assert not cad.ui_state()["placing"] and cad.state()["undo_depth"] == 1
    # dropped back on the panel, nothing is added
    cad.mouse_move(x + w / 2, y + h / 2)
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(tx, ty)
    px, py, pw, ph = rect(cad, "tile-sphere")
    cad.mouse_move(px + pw / 2, py + ph / 2)
    cad.mouse_up(px + pw / 2, py + ph / 2)
    assert len(cad.nodes()) == 1 and not cad.ui_state()["placing"]
    # Esc during the drag lets go of the shape
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(tx + 60, ty)
    cad.key("esc")
    cad.mouse_up(tx + 60, ty)
    assert len(cad.nodes()) == 1 and not cad.ui_state()["placing"]
    # so does losing the mouse (another window taking it)
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(tx + 60, ty)
    assert cad.ui_state()["placing"]
    cad.lose_capture()
    assert len(cad.nodes()) == 1 and not cad.ui_state()["placing"]
    cad.mouse_move(tx + 30, ty)  # the drag is over: nothing follows the pointer
    assert not cad.ui_state()["placing"]
    # dragged onto a shape, the new one lands on top of it
    bx, by = cad.project(30, -20, 20)
    cad.mouse_move(x + w / 2, y + h / 2)
    cad.mouse_down(x + w / 2, y + h / 2)
    cad.mouse_move(bx, by - 5)
    cad.mouse_move(bx, by)
    cad.mouse_up(bx, by)
    assert len(cad.nodes()) == 2
    assert math.isclose(cad.nodes()[1]["bounds"]["min"][2], 20, abs_tol=1e-6)


def test_tools_preview_under_the_pointer(cad):
    b = cad.add_shape("box")
    frames = lambda: cad.ui_state()["frames"]
    # the ruler: its outline follows the pointer until a click places it
    cad.set_tool("ruler")
    px, py = cad.project(-40, 30, 0)
    cad.mouse_move(px - 30, py)
    cad.mouse_move(px, py)
    cad.controls()
    assert "ruler" not in cad.state()
    cad.click(px, py)
    origin = cad.state()["ruler"]["origin"]
    assert near([origin["X"], origin["Y"], origin["Z"]], [-40, 30, 0])
    # the construction plane tool: the face under the pointer lights up
    cad.set_tool("construction_plane")
    sx, sy = cad.project(0, 0, 20)
    cad.mouse_move(sx + 3, sy)
    cad.mouse_move(sx, sy)
    cad.controls()
    assert cad.ui_state()["hover_node"] == b and "construction_plane" not in cad.state()
    cad.click(sx, sy)
    assert "construction_plane" in cad.state()
    cad.attach_construction_plane(0)
    cad.remove_ruler()
    # a marquee: drawn while it is dragged, selecting what it covers
    c = cad.add_shape("box", x=40)
    cad.clear_selection()
    x0, y0 = cad.project(-25, -25, 0)
    x1, y1 = cad.project(75, 25, 0)
    cad.mouse_down(x0 - 20, y0 + 30)
    cad.mouse_move(x1 + 20, y1 - 60)
    f = frames()
    cad.controls()
    assert frames() > f
    cad.mouse_up(x1 + 20, y1 - 60)
    assert sorted(cad.state()["selection"]) == [b, c]


def test_custom_colour_picker(cad):
    b = cad.add_shape("box")
    cad.select([b])
    cad.click_control("insp-solid")
    assert cad.ui_state()["popup"] == "color"
    cad.click_control("color-tab-Custom")
    assert {"color-wheel", "color-hex", "color-copy", "color-slider-red", "color-num-green", "color-mode-HSB"} <= ids(cad)
    colour = lambda: cad.node(b)["color"]
    # a code typed, with or without its #
    typein(cad, "color-hex", "#3366CC")
    assert colour() == "#3366cc"
    typein(cad, "color-hex", "00ff00")
    assert colour() == "#00ff00"
    typein(cad, "color-hex", "nope")
    assert colour() == "#00ff00" and "not a color" in cad.ui_state()["toast"]
    # a channel typed: red up to full makes yellow; too much is held to the most
    typein(cad, "color-num-red", "255")
    assert colour() == "#ffff00"
    typein(cad, "color-num-blue", "999")
    assert colour() == "#ffffff"
    typein(cad, "color-num-blue", "x")
    assert colour() == "#ffffff" and "not a number" in cad.ui_state()["toast"]
    # a slider drag is one undo step
    typein(cad, "color-hex", "ff0000")
    depth = cad.state()["undo_depth"]
    x, y, w, h = rect(cad, "color-slider-green")
    cad.drag(x + 12, y + h / 2, x + w - 1, y + h / 2, steps=6)
    assert colour() == "#ffff00" and cad.state()["undo_depth"] == depth + 1
    # hue, saturation and brightness
    cad.click_control("color-mode-HSB")
    assert {"color-slider-hue", "color-slider-saturation", "color-slider-brightness"} <= ids(cad)
    typein(cad, "color-num-hue", "240")
    assert colour() == "#0000ff"
    typein(cad, "color-num-saturation", "0")
    assert colour() == "#ffffff"
    typein(cad, "color-num-brightness", "0")
    assert colour() == "#000000"
    x, y, w, h = rect(cad, "color-slider-brightness")
    cad.drag(x + 12, y + h / 2, x + w - 1, y + h / 2, steps=4)
    x, y, w, h = rect(cad, "color-slider-saturation")
    cad.drag(x + 12, y + h / 2, x + w - 1, y + h / 2, steps=4)
    assert colour() == "#0000ff"  # the hue was kept through white and black
    x, y, w, h = rect(cad, "color-slider-hue")
    cad.drag(x + w - 12, y + h / 2, x + 1, y + h / 2, steps=4)
    assert colour() == "#ff0000"
    # the wheel: the ring picks the hue (green is a third of the way round) ...
    x, y, w, h = rect(cad, "color-wheel")
    cx, cy, ring = x + w / 2, y + h / 2, 90
    a = math.radians(120 - 90)
    cad.drag(cx, cy - ring, cx + ring * math.cos(a), cy + ring * math.sin(a), steps=6)
    assert colour() == "#00ff00"
    # ... and the square inside it the shade: its bottom edge is black
    cad.drag(cx, cy, cx, cy + 54, steps=4)
    r, g, bl = (int(colour()[i:i + 2], 16) for i in (1, 3, 5))
    assert max(r, g, bl) < 40
    cad.click_control("color-mode-RGB")
    cad.key("esc")
    assert cad.ui_state()["popup"] == ""


def test_group_colour_toggles_and_inspector(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("cylinder", x=10)
    g = cad.group("union", ids=[a, b])
    cad.select([g])
    assert {"insp-gt-bundle", "insp-gt-union", "insp-gt-intersect", "insp-groupinfo"} <= ids(cad)
    # the group type buttons
    whole = cad.node(g)["volume"]
    cad.click_control("insp-gt-intersect")
    assert cad.node(g)["group_mode"] == "intersect" and 0 < cad.node(g)["volume"] < whole
    cad.click_control("insp-gt-bundle")
    assert cad.node(g)["group_mode"] == "bundle"
    cad.click_control("insp-gt-union")
    assert cad.node(g)["group_mode"] == "union" and math.isclose(cad.node(g)["volume"], whole)
    # Multicolor and Transparent, in the colour popup
    cad.click_control("insp-solid")
    cad.click_control("color-multi")
    assert cad.node(g)["multicolor"]
    cad.click_control("color-transp")
    assert cad.node(g)["transparent"]
    cad.click_control("color-multi")
    cad.click_control("color-transp")
    n = cad.node(g)
    assert not n["multicolor"] and not n["transparent"]
    cad.key("esc")
    # the panel folds away to its title and back
    cad.select([a_box := cad.add_shape("box", x=60)])
    assert "param-length" in ids(cad)
    cad.click_control("insp-props")
    assert "param-length" not in ids(cad) and "insp-props" in ids(cad)
    cad.click_control("insp-props")
    assert "param-length" in ids(cad)
    cad.click_control("insp-collapse")
    assert "insp-props" not in ids(cad)
    cad.click_control("insp-collapse")
    assert "insp-props" in ids(cad)
    # its lock and hide buttons
    cad.click_control("insp-lock")
    assert cad.node(a_box)["locked"]
    cad.click_control("insp-lock")
    cad.click_control("insp-hide")
    assert cad.node(a_box)["hidden"] and not cad.state()["selection"]


def test_font_list(cad):
    t = cad.add_shape("text")
    cad.select([t])
    before = cad.node(t)
    cad.click_control("param-font")
    assert cad.ui_state()["popup"] == "font"
    fonts = [c["id"][5:] for c in cad.controls() if c["id"].startswith("font-")]
    assert fonts[:3] == cad.fonts()[:3] and before["font"] in cad.fonts()
    # the list scrolls
    x, y, w, h = rect(cad, "font-" + fonts[-1])
    cad.wheel(x + w / 2, y - 40, -6)
    later = [c["id"][5:] for c in cad.controls() if c["id"].startswith("font-")]
    assert later != fonts
    cad.wheel(x + w / 2, y - 40, 60)
    # choosing one sets the text in it, as one undo step
    pick = next(f for f in ("Serif Bold", "Serif", "Sans Bold") if f != before["font"])
    depth = cad.state()["undo_depth"]
    cad.click_control("font-" + pick)
    after = cad.node(t)
    assert cad.ui_state()["popup"] == "" and after["font"] == pick and cad.state()["undo_depth"] == depth + 1
    assert after["bounds"]["size"] != before["bounds"]["size"]
    # Esc closes the list and changes nothing
    cad.click_control("param-font")
    cad.key("esc")
    assert cad.ui_state()["popup"] == "" and cad.node(t)["font"] == pick


def test_split_cancel_ruler_and_note_buttons(cad):
    b = cad.add_shape("box")
    cad.select([b])
    # Split's Cancel leaves the tool, nothing cut
    cad.key("k")
    cad.click_control("split-axis-y")
    cad.click_control("split-keep-above")
    cad.click_control("split-cancel")
    assert cad.ui_state()["mode"] == "none" and len(cad.nodes()) == 1 and cad.state()["undo_depth"] == 1
    # the ruler's own buttons: measure to midpoints, and remove it
    cad.place_ruler(-30, -30)
    cad.select([b])
    assert not cad.state()["ruler"]["mid"]
    cad.click_control("ruler-mid-btn")
    assert cad.state()["ruler"]["mid"]
    cad.click_control("ruler-x-btn")
    assert "ruler" not in cad.state()
    # a note: opened from its marker, its text typed in its box, then deleted
    cad.add_note(b, 0, 0, 20, "first")
    (note,) = cad.notes()
    marker = "note-%d-%d" % (note["id"], note["index"])
    cad.click_control(marker)
    assert cad.notes()[0]["open"]
    assert {"note-box", "note-delete"} <= ids(cad), sorted(i for i in ids(cad) if i.startswith("note"))
    cad.click_control("note-box")
    assert cad.ui_state()["field"]["id"] == "note-text"
    cad.key("ctrl+a")
    cad.type_text("thrown away")
    cad.key("esc")  # Esc in the box drops what was typed, as in any field
    assert cad.notes()[0]["text"] == "first" and "field" not in cad.ui_state()
    cad.click_control("note-box")
    cad.key("ctrl+a")
    cad.type_text("second thoughts")
    cad.click(*cad.project(-70, -70, 0))  # a click elsewhere keeps it
    assert cad.notes()[0]["text"] == "second thoughts"
    cad.click_control(marker)
    cad.click_control("note-delete")
    assert cad.notes() == []
    cad.undo()
    assert cad.notes()[0]["text"] == "second thoughts"


def test_console_save_as_and_save_log(cad, tmp_path):
    new_script(cad)
    cad.type_text("print('kept')")
    first, second, log = tmp_path / "a.star", tmp_path / "b.star", tmp_path / "log.txt"
    cad.answer_dialogs(("save", str(first)))
    cad.click_control("con-save")
    assert first.read_text(encoding="utf-8").strip() == "print('kept')" and cad.console()["script_path"] == str(first)
    # Save again doesn't ask; Save as asks, and the script is then the new file
    cad.key("enter")
    cad.type_text("print('more')")
    cad.click_control("con-save")
    assert "more" in first.read_text(encoding="utf-8")
    cad.answer_dialogs(("save", str(second)))
    cad.click_control("con-saveas")
    assert second.read_text(encoding="utf-8") == first.read_text(encoding="utf-8")
    assert cad.console()["script_path"] == str(second)
    # a save dialog cancelled saves nothing
    cad.answer_dialogs(("save", ""))
    cad.click_control("con-saveas")
    assert cad.console()["script_path"] == str(second)
    # the transcript, as a text file
    cad.click_control("con-run")
    settled(cad)
    cad.answer_dialogs(("save", str(log)))
    cad.click_control("con-savelog")
    text = log.read_text(encoding="utf-8")
    assert "kept" in text and "more" in text
    new_script(cad)
    cad.click_control("con-close")


# --- the console ---------------------------------------------------------------

def settled(cad, timeout=15.0):
    """The console once its script has finished."""
    end = time.time() + timeout
    while time.time() < end:
        con = cad.console()
        if not con["running"]:
            return con
        time.sleep(0.05)
    raise AssertionError("the script did not finish")


def new_script(cad):
    """The console open on an empty script."""
    if not cad.console()["open"]:
        cad.click_control("scripting")
    cad.click_control("con-tab-Editor")
    cad.answer_dialogs(("message", "no"))  # should the script there ask to be saved
    cad.click_control("con-new")
    assert cad.console()["script"] == ""


def test_console_keys(cad, tmp_path):
    new_script(cad)
    cad.type_text("cad.add_shape('box', x=len(cad.nodes()) * 30)")
    # F5 and Ctrl+Enter run the script
    cad.key("f5")
    settled(cad)
    assert len(cad.nodes()) == 1
    cad.key("ctrl+enter")
    settled(cad)
    assert len(cad.nodes()) == 2
    # Ctrl+S saves it (asking where, the first time), Ctrl+O opens one
    path = tmp_path / "two.star"
    cad.answer_dialogs(("save", str(path)))
    cad.key("ctrl+s")
    assert path.read_text(encoding="utf-8").startswith("cad.add_shape('box'")
    assert cad.console()["script_path"] == str(path)
    other = tmp_path / "other.star"
    other.write_text("print('from a file')\n", encoding="utf-8")
    cad.answer_dialogs(("open", str(other)))
    cad.key("ctrl+o")
    con = cad.console()
    assert con["script"].startswith("print('from a file')") and con["script_path"] == str(other)
    cad.key("f5")
    assert any(l["text"] == "from a file" for l in settled(cad)["lines"])
    # Ctrl+C stops a script that won't end
    new_script(cad)
    cad.type_text("while True:")
    cad.key("enter")
    cad.type_text("pass")
    cad.key("f5")
    assert cad.console()["running"]
    cad.key("ctrl+c")
    con = settled(cad)
    assert any(l["text"].startswith("Stopped after") for l in con["lines"])
    # at the prompt, Ctrl+Enter runs what is typed, like Enter
    cad.click_control("con-tab-Console")
    cad.key("ctrl+a")
    cad.type_text("print(6 * 7)")
    cad.key("ctrl+enter")
    assert any(l["text"] == "42" for l in settled(cad)["lines"])
    new_script(cad)  # leave no unsaved script behind
    cad.click_control("con-close")


def test_console_mouse(cad):
    new_script(cad)
    for i in range(40):
        cad.type_text("line%d = %d" % (i, i))
        cad.key("enter")
    x, y, w, h = rect(cad, "con-editor")
    # a click puts the caret there and typing goes in at it
    cad.click(x + 60, y + 8)
    assert cad.console()["focus"] == "editor"
    before = cad.console()["script"]
    cad.type_text("#")
    assert len(cad.console()["script"]) == len(before) + 1
    cad.key("ctrl+z")
    assert cad.console()["script"] == before
    # the wheel scrolls it, the text unchanged
    cad.wheel(x + w / 2, y + h / 2, -3)
    cad.wheel(x + w / 2, y + h / 2, 3)
    assert cad.console()["script"] == before
    # a drag selects; typing replaces what is selected
    cad.drag(x + 60, y + 8, x + 60, y + h - 10, steps=4)
    cad.type_text("x")
    assert len(cad.console()["script"]) < len(before) - 20
    # the divider drags the console taller
    sx, sy, sw, sh = rect(cad, "con-split")
    top = rect(cad, "con-editor")[1]
    cad.drag(sx + sw / 2, sy + sh / 2, sx + sw / 2, sy - 120, steps=4)
    assert rect(cad, "con-editor")[1] < top - 60
    # the transcript scrolls too, and Close closes the console
    cad.click_control("con-tab-Console")
    tx, ty, tw, th = rect(cad, "con-transcript")
    cad.wheel(tx + tw / 2, ty + th / 2, 3)
    cad.click_control("con-close")
    assert not cad.console()["open"]
    new_script(cad)  # leave no unsaved script behind
    cad.click_control("con-close")


def test_copy_buttons(cad):
    """The console's Copy and the colour picker's Copy put their text on the
    clipboard (ScottiCAD's own, headless), from where it pastes."""
    before = system_clipboard()
    # the console's transcript, pasted into the editor
    new_script(cad)
    cad.click_control("con-tab-Console")
    cad.key("ctrl+a")
    cad.type_text("print('copied ' + 'line')")
    cad.key("enter")
    assert any(l["text"] == "copied line" for l in settled(cad)["lines"])
    cad.click_control("con-copy")
    new_script(cad)
    cad.click_control("con-editor")
    cad.key("ctrl+v")
    assert "copied line" in cad.console()["script"]
    new_script(cad)  # leave no unsaved script behind
    cad.click_control("con-close")
    # the colour code, pasted into the code's own field
    b = cad.add_shape("box")
    cad.select([b])
    cad.click_control("insp-solid")
    cad.click_control("color-tab-Custom")
    typein(cad, "color-hex", "ff8800")
    cad.click_control("color-copy")
    assert cad.ui_state()["toast"] == "Copied ff8800"
    typein(cad, "color-hex", "0000ff")
    cad.click_control("color-hex")
    cad.key("ctrl+a")
    cad.key("ctrl+v")
    cad.key("enter")
    assert cad.node(b)["color"] == "#ff8800"
    cad.key("esc")
    assert system_clipboard() == before


def test_stop_script_and_stats(cad):
    assert cad.call("stop_script") == {"stopped": False}
    new_script(cad)
    cad.type_text("while True:")
    cad.key("enter")
    cad.type_text("pass")
    cad.click_control("con-run")
    assert cad.console()["running"]
    assert cad.call("stop_script") == {"stopped": True}
    assert not settled(cad)["running"]
    new_script(cad)
    cad.click_control("con-close")
    # the figures the soak test watches
    cad.add_shape("sphere")
    st = cad.call("stats")
    assert st["heap_alloc"] > 0 and st["goroutines"] > 0 and st["meshes"] >= 1 and st["mesh_triangles"] > 0
    assert st["undo_depth"] == cad.state()["undo_depth"]


# --- the print check's panel ----------------------------------------------------

def checks(cad, timeout=30.0):
    """The panel's print check lines, once it has finished checking."""
    end = time.time() + timeout
    while time.time() < end:
        lines = cad.ui_state()["print_check"]
        if lines != ["Checking…"]:
            return lines
        time.sleep(0.05)
    raise AssertionError("the print check did not finish")


def ball_stl(path, n=240):
    """A sphere of about 2 n² triangles, radius 20, as a binary STL."""
    def p(i, j):
        th, ph = math.pi * i / n, 2 * math.pi * j / n
        return (20 * math.sin(th) * math.cos(ph), 20 * math.sin(th) * math.sin(ph), 20 * math.cos(th))
    tris = []
    for i in range(n):
        for j in range(n):
            q = (p(i, j), p(i + 1, j), p(i + 1, j + 1), p(i, j + 1))
            if i > 0:
                tris.append((q[0], q[1], q[3]))
            if i < n - 1:
                tris.append((q[1], q[2], q[3]))
    with open(path, "wb") as f:
        f.write(b"\0" * 80 + struct.pack("<I", len(tris)))
        for t in tris:
            f.write(struct.pack("<12fH", 0, 0, 0, *t[0], *t[1], *t[2], 0))


def test_print_check_panel_lines(cad, tmp_path):
    assert "print_check" not in cad.ui_state()  # only while the panel is open
    cad.click_control("filament")  # an empty design: there's nothing to open it for
    assert not cad.ui_state()["filament_panel"]
    a = cad.add_shape("box")
    cad.clear_selection()
    cad.click_control("filament")
    try:
        assert checks(cad) == ["Ready to print"]
        cad.hide(ids=[a])
        assert checks(cad) == ["Nothing to print"]
        cad.show_all()
        cad.clear_selection()
        cad.add_shape("box", x=40)
        cad.clear_selection()
        assert checks(cad) == ["2 parts ready to print"]
        # holes and hidden shapes aren't printed, so they aren't counted
        cad.add_shape("cylinder", hole=True, x=-40)
        h = cad.add_shape("box", x=80, z=30)
        cad.hide(ids=[h])
        cad.clear_selection()
        assert checks(cad) == ["2 parts ready to print"]
        # the selection only, when there is one
        cad.add_shape("box", y=40, z=5)
        assert checks(cad) == ["Box: floating 5.0 mm above the plate"]
        cad.select([a])
        assert checks(cad) == ["Ready to print"]
        # more findings than fit: the first five, and a count of the rest
        cad.new_design()
        for i in range(8):
            cad.add_shape("box", x=i * 22 - 80, z=3 + i)
        cad.clear_selection()
        lines = checks(cad)
        assert len(lines) == 6 and lines[-1] == "and 3 more", lines
        assert all(l.startswith("Box: floating") for l in lines[:5])
        assert len([c for c in cad.controls() if c["id"].startswith("check-")]) == 5
        # a big part is checked in the background: "Checking…" first
        cad.new_design()
        stl = tmp_path / "ball.stl"
        ball_stl(stl)
        cad.import_file(str(stl))
        cad.clear_selection()
        # (unless the check has already finished: it runs while this asks)
        first = cad.ui_state()["print_check"]
        assert first in (["Checking…"], ["Ready to print"]) or first[0] == "Ready to print", first
        # a ball's underside leans out past 45°: overhangs, as information
        lines = checks(cad)
        assert lines[0] == "Ready to print" and lines[1].endswith("of overhangs (may need support)"), lines
        # the API's answer never waits: it checks at once
        assert cad.print_check()["parts"][0]["status"] == "ok"
    finally:
        cad.click_control("filament")
    assert not cad.ui_state()["filament_panel"]


# --- starting up -----------------------------------------------------------------

def run_exe(*args):
    return subprocess.run([default_exe(), *args], capture_output=True, text=True, timeout=60)


def test_version_help_and_bad_options():
    p = run_exe("--version")
    assert p.returncode == 0 and p.stdout.startswith("ScottiCAD ") and p.stdout.split()[1][0].isdigit()
    p = run_exe("--help")
    assert p.returncode == 0 and "Usage:" in p.stderr and "--mcp-connect" in p.stderr
    for option in ("api", "headless", "mcp-tools", "root", "scale", "width", "height", "version"):
        assert "-" + option in p.stderr, option
    for args, word in ((["--width", "100"], "--width"), (["--height", "99999"], "--height"), (["--scale", "9"], "--scale"),
                       (["--scale", "0.1"], "--scale"), (["--no-such-option"], "not defined"),
                       (["--headless", "--api", "256.1.1.1:0"], "api"),
                       (["--headless", "--root", os.path.join(os.path.dirname(__file__), "no", "such", "folder")], "not a folder")):
        p = run_exe(*args)
        assert p.returncode != 0 and word in p.stderr, (args, p.returncode, p.stderr)


def test_design_named_on_the_command_line(cad, tmp_path):
    cad.add_shape("box")
    cad.add_shape("benttube", x=40)
    cad.set_camera(yaw=1.0, pitch=0.5)
    path = cad.save(str(tmp_path / "named.scotticad"))
    cam = cad.camera()
    with launch(design=path) as c:
        assert [n["kind"] for n in c.nodes()] == ["box", "benttube"]
        st = c.state()
        assert st["path"] == path and not st["dirty"] and st["name"] == "named"
        # the view it was saved with comes back
        got = c.camera()
        assert math.isclose(got["yaw"], cam["yaw"], abs_tol=1e-9) and math.isclose(got["pitch"], cam["pitch"], abs_tol=1e-9)
    # a file that isn't there, or isn't a design: an empty design, and the reason shown
    junk = tmp_path / "junk.scotticad"
    junk.write_text("not a design")
    for bad in (str(tmp_path / "missing.scotticad"), str(junk)):
        with launch(design=bad) as c:
            assert c.nodes() == [] and c.state()["path"] == ""
            assert os.path.basename(bad) in c.ui_state()["toast"]


# --- the API -----------------------------------------------------------------------

def test_nan_and_infinity_are_refused_before_they_are_sent(cad):
    b = cad.add_shape("box")
    for bad in (float("nan"), float("inf"), -float("inf")):
        with pytest.raises(ScottiCADError, match="NaN or infinite"):
            cad.move(dx=bad, ids=[b])
        with pytest.raises(ScottiCADError, match="NaN or infinite"):
            cad.set_bounds([0, 0, 0], [bad, 1, 1], ids=[b])
    # the connection is still good, and nothing moved
    assert near(cad.node(b)["bounds"]["min"], [-10, -10, 0])
