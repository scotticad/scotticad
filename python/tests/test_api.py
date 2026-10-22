"""End-to-end tests: drive scotticad.exe (headless) through the API.

Run from the repository root after building:
    go build -o scotticad.exe ./cmd/scotticad
    python -m pytest python/tests
"""

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


def approx(a, b, tol=1e-6):
    return all(abs(x - y) <= tol for x, y in zip(a, b))


def test_describe_lists_methods(cad):
    names = {m["name"] for m in cad.describe()}
    for required in ["add_shape", "group", "ungroup", "export", "import", "undo", "redo", "set_units",
                     "screenshot", "click_control", "drag", "key"]:
        assert required in names


def test_add_move_resize_rotate(cad):
    box = cad.add_shape("box", x=10, y=20)
    b = cad.node(box)["bounds"]
    assert approx(b["min"], [0, 10, 0]) and approx(b["size"], [20, 20, 20])
    cad.move(dx=5, dz=3)
    assert approx(cad.node(box)["bounds"]["min"], [5, 10, 3])
    cad.set_size(x=40)
    assert approx(cad.node(box)["bounds"]["size"], [40, 20, 20])
    cad.rotate("z", 90)
    assert approx(cad.node(box)["bounds"]["size"], [20, 40, 20], 1e-6)
    cad.drop()
    assert abs(cad.node(box)["bounds"]["min"][2]) < 1e-9


def test_union_group_subtracts_holes_and_ungroups(cad):
    box = cad.add_shape("box")
    hole = cad.add_shape("box", hole=True)
    cad.set_size(x=10, y=10, z=40, ids=[hole])
    g = cad.group("union", ids=[box, hole])
    assert math.isclose(cad.node(g)["volume"], 8000 - 2000, rel_tol=1e-6)
    released = cad.ungroup(ids=[g])
    assert sorted(released) == sorted([box, hole])
    assert cad.node(hole)["hole"] is True


def test_bundle_keeps_parts_separate(cad):
    a = cad.add_shape("box", x=-20)
    b = cad.add_shape("box", x=20)
    g = cad.group("bundle", ids=[a, b])
    n = cad.node(g)
    assert n["group_mode"] == "bundle" and n["child_count"] == 2
    assert math.isclose(n["volume"], 16000, rel_tol=1e-6)
    cad.set_group_mode("union", ids=[g])
    assert cad.node(g)["group_mode"] == "union"


def test_enclosure_is_a_bundle_of_parts(cad):
    box = cad.add_shape("extrudedbox")
    n = cad.node(box)
    assert n["assembly"] == "extrudedbox" and n["group_mode"] == "bundle" and n["child_count"] == 3
    cad.set_param(box, "length", 120)
    assert math.isclose(cad.node(box)["bounds"]["size"][0], 120 + 2 * 2.5, rel_tol=1e-6)
    cad.set_param(box, "panels", 0)
    assert cad.node(box)["child_count"] == 1
    cad.set_param(box, "panels", 1)
    parts = cad.ungroup(ids=[box])
    assert sorted(cad.node(p)["kind"] for p in parts) == ["extrudedbox-body", "extrudedbox-panel", "extrudedbox-panel"]


def test_undo_redo(cad):
    cad.add_shape("sphere")
    cad.add_shape("cone")
    assert len(cad.nodes()) == 2
    assert cad.undo() and len(cad.nodes()) == 1
    assert cad.redo() and len(cad.nodes()) == 2


def test_clipboard_and_duplicate(cad):
    s = cad.add_shape("sphere")
    cad.select([s])
    cad.copy()
    pasted = cad.paste()
    assert len(pasted) == 1 and pasted[0] != s
    cad.select([s])
    d = cad.duplicate()
    assert len(cad.nodes()) == 3 and d[0] not in (s, pasted[0])
    cad.cut()
    assert len(cad.nodes()) == 2


def test_hide_show_lock(cad):
    s = cad.add_shape("box")
    cad.hide(ids=[s])
    assert cad.node(s)["hidden"]
    cad.show_all()
    assert not cad.node(s)["hidden"]
    cad.set_locked(True, ids=[s])
    cad.select([s])
    cad.move(dx=10)
    assert approx(cad.node(s)["bounds"]["min"], [-10, -10, 0])


def test_align_and_mirror(cad):
    a = cad.add_shape("box", x=-30)
    b = cad.add_shape("cylinder", x=30, y=15)
    cad.select([a, b])
    cad.align("y", "min")
    assert math.isclose(cad.node(a)["bounds"]["min"][1], cad.node(b)["bounds"]["min"][1], abs_tol=1e-9)
    t = cad.add_shape("text")
    before = cad.node(t)["bounds"]
    cad.mirror("x", ids=[t])
    after = cad.node(t)["bounds"]
    assert approx(before["size"], after["size"], 1e-6)


def test_text_params(cad):
    t = cad.add_shape("text")
    cad.set_text(t, "HELLO", font="Sans")
    n = cad.node(t)
    assert n["text"] == "HELLO" and n["font"] == "Sans"
    cad.set_param(t, "height", 5)
    assert math.isclose(cad.node(t)["bounds"]["size"][2], 5, rel_tol=1e-6)
    with pytest.raises(ScottiCADError):
        cad.set_param(t, "nonsense", 1)


def test_units_and_snap(cad):
    cad.set_units("in")
    st = cad.state()["settings"]
    assert st["units"] == "in" and math.isclose(st["snap"], 25.4 / 16)
    cad.set_units("mm")
    assert cad.state()["settings"]["units"] == "mm"


def test_save_open_preserves_tree(cad, tmp_path):
    box = cad.add_shape("box")
    hole = cad.add_shape("cylinder", hole=True)
    inner = cad.group("union", ids=[box, hole])
    other = cad.add_shape("sphere", x=40)
    outer = cad.group("bundle", ids=[inner, other])
    p = str(tmp_path / "tree.scotticad")
    cad.save(p)
    cad.new_design()
    cad.open(p)
    nodes = cad.nodes(deep=True)
    assert len(nodes) == 1 and nodes[0]["group_mode"] == "bundle"
    released = cad.ungroup(ids=[nodes[0]["id"]])
    assert len(released) == 2
    union = [n for n in cad.nodes() if n["kind"] == "group"][0]
    parts = cad.ungroup(ids=[union["id"]])
    kinds = sorted(cad.node(i)["kind"] for i in parts)
    assert kinds == ["box", "cylinder"]


def test_export_and_import_roundtrip(cad, tmp_path):
    cad.add_shape("box")
    for ext in ("stl", "obj"):
        p = str(tmp_path / f"out.{ext}")
        cad.export(p)
        assert os.path.getsize(p) > 0
        i = cad.import_file(p, x=50)
        assert math.isclose(cad.node(i)["volume"], 8000, rel_tol=1e-4)
        cad.delete(ids=[i])


def test_import_svg(cad, tmp_path):
    p = tmp_path / "s.svg"
    p.write_text('<svg width="30mm" height="10mm" viewBox="0 0 30 10"><rect width="30" height="10"/></svg>')
    i = cad.import_file(str(p), height=4)
    assert approx(cad.node(i)["bounds"]["size"], [30, 10, 4], 1e-6)


def test_gui_drag_tile_and_shortcuts(cad):
    ctl = {c["id"]: c for c in cad.controls()}
    x, y, w, h = ctl["tile-sphere"]["rect"]
    px, py = cad.project(0, 0, 0)
    cad.drag(x + w / 2, y + h / 2, px, py, steps=12)
    nodes = cad.nodes()
    assert len(nodes) == 1 and nodes[0]["kind"] == "sphere"
    cad.select([nodes[0]["id"]])
    cad.key("ctrl+d")
    assert len(cad.nodes()) == 2
    cad.key("h")
    assert cad.nodes()[1]["hole"] is True
    cad.key("ctrl+a")
    cad.key("ctrl+g")
    assert len(cad.nodes()) == 1 and cad.nodes()[0]["kind"] == "group"
    cad.key("ctrl+z")
    assert len(cad.nodes()) == 2
    cad.key("delete")
    assert len(cad.nodes()) == 0


def test_gui_handle_resize_and_label_typing(cad):
    box = cad.add_shape("box")
    cad.select([box])
    hx, hy = cad.handle("edge", 1)  # +X side
    tx, ty = cad.project(30, 0, 0)
    cad.drag(hx, hy, tx, ty, steps=8)
    size = cad.node(box)["bounds"]["size"]
    assert math.isclose(size[0], 40, abs_tol=1e-6) and math.isclose(size[1], 20, abs_tol=1e-6)
    hx, hy = cad.handle("edge", 1)
    cad.mouse_move(hx, hy)
    ids = {c["id"] for c in cad.controls()}
    assert "dim-x" in ids and "dim-z" not in ids  # a side handle shows its side
    hx, hy = cad.handle("top", 0)
    cad.mouse_move(hx, hy)
    cad.click_control("dim-z")
    cad.type_text("12.5")
    cad.key("enter")
    assert math.isclose(cad.node(box)["bounds"]["size"][2], 12.5, abs_tol=1e-6)


def test_screenshot(cad, tmp_path):
    cad.add_shape("torus")
    p = str(tmp_path / "s.png")
    r = cad.screenshot(p)
    assert r["width"] == 1400 and os.path.getsize(p) > 10000


def test_font_menu_selects_font(cad):
    t = cad.add_shape("text")
    cad.select([t])
    cad.click_control("param-font")
    assert cad.ui_state()["popup"] == "font"
    cad.click_control("font-Serif")
    assert cad.node(t)["font"] == "Serif"
    assert cad.ui_state()["popup"] == ""


def test_shape_clicks_leave_tooltips_enabled(cad):
    a = cad.add_shape("box", x=-30)
    b = cad.add_shape("box", x=30)
    for i, (x, mods) in enumerate([(-30, []), (30, ["shift"])]):
        sx, sy = cad.project(x, -10, 5)
        cad.click(sx, sy, mods=mods)
    assert sorted(cad.selection()["ids"]) == sorted([a, b])
    assert cad.ui_state()["pressed"] == ""


def test_hole_keeps_solid_colour(cad):
    h = cad.add_shape("cylinder", hole=True)
    assert cad.node(h)["color"] == "#f6851f"
    cad.select([h])
    cad.key("s")
    assert cad.node(h)["hole"] is False and cad.node(h)["color"] == "#f6851f"


def test_ruler_drag_and_midpoint_split(cad):
    box = cad.add_shape("box", x=20, y=20)
    cad.place_ruler(-40, -40)
    cad.select([box])
    ox, oy = cad.project(-40, -40, 0)
    tx, ty = cad.project(-50, -30, 0)
    cad.drag(ox, oy, tx, ty, steps=6)
    o = cad.state()["ruler"]["origin"]
    o = [o[k] for k in sorted(o)] if isinstance(o, dict) else o
    assert [round(v, 6) for v in o[:2]] == [-50, -30]
    ids = {c["id"] for c in cad.controls()}
    assert {"dim-x", "dim-y"} <= ids
    cad.click_control("ruler-mid-btn")
    # measuring to midpoints: size lines stay whole; the height above the
    # build plate is measured to the middle (a 20 mm box: 10)
    ids = {c["id"] for c in cad.controls()}
    assert {"dim-x", "dim-y", "dim-elev"} <= ids and "dim-x0" not in ids
    cad.click_control("dim-elev")
    assert float(cad.ui_state()["field"]["text"]) == 10
    cad.type_text("15")
    cad.key("enter")
    assert math.isclose(cad.node(box)["bounds"]["min"][2], 5, abs_tol=1e-6)


def test_rotation_angle_stays_editable_after_release(cad):
    import math as m
    box = cad.add_shape("box", x=0, y=0)
    cad.set_size(x=40, ids=[box])
    cad.select([box])
    cad.set_camera(target=[0, 0, 0], yaw=-90, pitch=55, dist=160)
    hz = [h for h in cad.handles() if h["kind"] == "rotate" and h["index"] == 2][0]
    cx, cy = cad.project(0, 0, 0)
    r = m.hypot(hz["x"] - cx, hz["y"] - cy)
    a0 = m.atan2(hz["y"] - cy, hz["x"] - cx)
    cad.mouse_move(hz["x"], hz["y"])
    cad.mouse_down(hz["x"], hz["y"])
    for i in range(1, 7):
        a = a0 - m.radians(30) * i / 6
        cad.mouse_move(cx + r * 0.8 * m.cos(a), cy + r * 0.8 * m.sin(a))
    cad.mouse_up(cx + r * 0.8 * m.cos(a), cy + r * 0.8 * m.sin(a))
    # the protractor stays with an editable angle; typing re-does the turn
    assert "angle" in {c["id"] for c in cad.controls()}
    cad.click_control("angle")
    cad.type_text("90")
    cad.key("enter")
    size = cad.node(box)["bounds"]["size"]
    assert m.isclose(size[0], 20, abs_tol=1e-6) and m.isclose(size[1], 40, abs_tol=1e-6)
    assert "angle" in {c["id"] for c in cad.controls()}
    # selecting nothing drops it
    cad.clear_selection()
    assert "angle" not in {c["id"] for c in cad.controls()}


def test_clicked_corner_keeps_sizes_editable(cad):
    box = cad.add_shape("box", x=0, y=0)
    cad.select([box])
    corner = [h for h in cad.handles() if h["kind"] == "corner"][0]
    cad.mouse_move(corner["x"], corner["y"])
    cad.mouse_down(corner["x"], corner["y"])
    cad.mouse_up(corner["x"], corner["y"])
    cad.mouse_move(5, 400)  # away from every handle
    ids = {c["id"] for c in cad.controls()}
    assert {"dim-x", "dim-y"} <= ids and "dim-z" not in ids
    cad.click_control("dim-x")
    cad.type_text("30")
    cad.key("enter")
    assert math.isclose(cad.node(box)["bounds"]["size"][0], 30, abs_tol=1e-6)
    assert "dim-y" in {c["id"] for c in cad.controls()}
    cad.clear_selection()
    assert "dim-x" not in {c["id"] for c in cad.controls()}


def test_move_without_ruler_shows_editable_distances(cad):
    box = cad.add_shape("box", x=0, y=0)
    cad.select([box])
    x0, y0 = cad.project(0, 0, 10)
    x1, y1 = cad.project(-23, 31, 10)
    cad.drag(x0, y0, x1, y1, steps=8)
    moved = cad.node(box)["bounds"]["center"]
    assert moved[0] != 0 and moved[1] != 0
    ids = {c["id"] for c in cad.controls()}
    assert {"move-x", "move-y"} <= ids
    cad.click_control("move-x")
    cad.type_text("-10")
    cad.key("enter")
    assert math.isclose(cad.node(box)["bounds"]["center"][0], -10, abs_tol=1e-6)
    assert "move-y" in {c["id"] for c in cad.controls()}
    cad.clear_selection()
    assert "move-x" not in {c["id"] for c in cad.controls()}


def test_notes_tool(cad):
    box = cad.add_shape("box", x=0, y=0)
    cad.clear_selection()
    cad.set_tool("notes")
    assert cad.ui_state()["mode"] == "notes"
    # clicking a shape pins a note there and opens it for typing
    x, y = cad.project(0, 0, 20)
    cad.click(x, y)
    ns = cad.notes()
    assert len(ns) == 1 and ns[0]["open"] and ns[0]["id"] == box
    assert abs(ns[0]["world"][2] - 20) < 0.5
    assert cad.ui_state()["field"]["id"] == "note-text"
    cad.type_text("hello")
    cad.key("shift+enter")  # a second line
    cad.type_text("world")
    cad.key("enter")
    assert cad.notes()[0]["text"] == "hello\nworld"
    # the "-" pin closes it; it then shows as a marker
    cad.click_control("note-close")
    assert not cad.notes()[0]["open"]
    marker = "note-%d-0" % box
    assert marker in {c["id"] for c in cad.controls()}
    # an empty note is dropped when closed
    x, y = cad.project(-7, -9, 20)  # clear of the first note's marker
    cad.click(x, y)
    assert len(cad.notes()) == 2
    cad.key("escape")  # stop typing
    cad.key("escape")  # close the note
    assert [n["text"] for n in cad.notes()] == ["hello\nworld"]
    # notes move with their shape
    cad.set_tool("none")
    cad.move(10, 0, 0, ids=[box])
    assert abs(cad.notes()[0]["world"][0] - ns[0]["world"][0] - 10) < 1e-6
    # clicking the marker opens it again; its trash button deletes it
    cad.click_control(marker)
    assert cad.notes()[0]["open"]
    cad.click_control("note-delete")
    assert cad.notes() == []
    # notes can be pinned to the build plate too
    cad.set_tool("notes")
    x, y = cad.project(-60, -40, 0)
    cad.click(x, y)
    ns = cad.notes()
    assert len(ns) == 1 and ns[0]["id"] == 0 and abs(ns[0]["world"][0] + 60) < 1


def test_construction_plane_tool_attaches_to_tilted_shape(cad):
    box = cad.add_shape("box", x=0, y=0)
    cad.select([box])
    cad.rotate("x", 30, ids=[box])
    cad.set_tool("construction_plane")
    assert cad.ui_state()["mode"] == "construction_plane"
    c = cad.node(box)["bounds"]["center"]
    x, y = cad.project(*c)
    cad.click(x, y)
    assert cad.ui_state()["mode"] == "none" and "construction_plane" in cad.state()
    # sizes are now measured along the box's own axes: typing a height
    # stretches it along its own z, with no shear
    cad.select([box])
    hx, hy = cad.handle("top", 0)
    cad.mouse_move(hx, hy)
    cad.click_control("dim-z")
    cad.type_text("30")
    cad.key("enter")
    n = cad.node(box)
    assert math.isclose(n["size"][2], 30, abs_tol=1e-6) and math.isclose(n["size"][1], 20, abs_tol=1e-6)
    # clicking off any shape goes back to the build plate
    cad.set_tool("construction_plane")
    cad.click(30, 700)
    assert "construction_plane" not in cad.state()


def test_construction_plane_button_follows_shape_rotation(cad):
    box = cad.add_shape("box", x=0, y=0)
    cad.select([box])
    cad.rotate("x", 30, ids=[box])
    cad.click_control("shapewp")
    m = cad.state()["construction_plane"]  # column-major frame
    z_axis = m[8:11]
    assert math.isclose(z_axis[1], -0.5, abs_tol=1e-6) and math.isclose(z_axis[2], math.sqrt(3) / 2, abs_tol=1e-6)
    cad.click_control("shapewp")  # toggles off
    assert "construction_plane" not in cad.state()


def test_shape_search(cad):
    def tiles():
        return {c["id"] for c in cad.controls() if c["id"].startswith("tile-")}
    cad.click_control("search")
    assert cad.ui_state()["field"]["id"] == "shape-search"
    cad.type_text("cyl")
    found = tiles()
    assert {"tile-cylinder", "tile-cylinder-hole"} <= found and "tile-box" not in found
    # keywords and the hole/solid words narrow it
    cad.key("enter")
    cad.click_control("search-field")
    for _ in range(3):
        cad.key("backspace")
    cad.type_text("screw")
    found = tiles()
    assert {"tile-thread", "tile-hexbolt", "tile-countersink-hole"} <= found and "tile-box" not in found
    # hole tiles: the three basic hole shapes and the screw-hole cutters
    for _ in range(5):
        cad.key("backspace")
    cad.type_text("hole")
    holes = {t for t in tiles() if t.endswith("-hole")}
    assert holes == {"tile-box-hole", "tile-cylinder-hole", "tile-sphere-hole",
                     "tile-nuttrap-hole", "tile-countersink-hole", "tile-counterbore-hole"}
    assert "tile-countersink" not in tiles()  # cutters only come as holes
    # a found tile still places its shape
    cad.click_control("search-close")
    assert "tile-box-hole" in tiles() and "search" in {c["id"] for c in cad.controls()}


def test_export_glb_and_svg(cad, tmp_path):
    import struct
    cad.add_shape("box", x=0, y=0)
    t = cad.add_shape("cylinder", x=0, y=0, hole=True)
    cad.set_size(x=8, y=8, ids=[t])
    cad.select_all()
    cad.group()
    glb = str(tmp_path / "d.glb")
    cad.export(glb)
    data = open(glb, "rb").read()
    magic, version, length = struct.unpack("<III", data[:12])
    assert magic == 0x46546C67 and version == 2 and length == len(data)
    svg = str(tmp_path / "d.svg")
    cad.export(svg)
    text = open(svg).read()
    # a box with a hole through it: an outer and an inner outline
    assert text.count("M") == 2 and 'width="22mm"' in text
    # 3MF: a zip package holding the model
    import zipfile
    tmf = str(tmp_path / "d.3mf")
    cad.export(tmf)
    with zipfile.ZipFile(tmf) as z:
        names = set(z.namelist())
        assert {"[Content_Types].xml", "_rels/.rels", "3D/3dmodel.model"} <= names
        model = z.read("3D/3dmodel.model").decode()
        assert 'unit="millimeter"' in model and "<triangle " in model
    # the export dialog's buttons are live
    cad.click_control("export")
    ids = {c["id"] for c in cad.controls()}
    assert {"exp-glb", "exp-3mf", "exp-svg"} <= ids
    cad.key("escape")


def test_type_into_slider(cad):
    g = cad.add_shape("gear")
    cad.select([g])
    cad.click_control("param-teeth")
    assert cad.ui_state()["field"]["id"] == "param-teeth"
    cad.type_text("30")
    cad.key("enter")
    assert cad.node(g)["params"]["teeth"] == 30
    # the gear grows with its tooth count (same pitch)
    assert cad.node(g)["bounds"]["size"][0] > 40


def test_click_outside_color_picker_selects(cad):
    a = cad.add_shape("box", x=-40, y=0)
    b = cad.add_shape("box", x=40, y=0)
    cad.select([a])
    cad.click_control("insp-solid")
    swatch = next(c["id"] for c in cad.controls() if c["id"].startswith("swatch-#8"))
    cad.click_control(swatch)
    assert cad.node(a)["color"] == swatch[len("swatch-"):]
    assert cad.ui_state()["popup"] == "color"
    x, y = cad.project(40, 0, 10)
    cad.click(x, y)  # one click: the picker closes and the other box is selected
    assert cad.ui_state()["popup"] == "" and cad.selection()["ids"] == [b]


def test_marquee_selects_touching_shapes(cad):
    # two 20 mm boxes seen from above: a spans x -40..-20, b 20..40, y -10..10
    a = cad.add_shape("box", x=-30, y=0)
    b = cad.add_shape("box", x=30, y=0)
    cad.set_camera(target=[0, 0, 0], yaw=-90, pitch=89.9, dist=200)

    def marquee(p, q):
        cad.clear_selection()
        x0, y0 = cad.project(*p, 0)
        x1, y1 = cad.project(*q, 0)
        cad.drag(x0, y0, x1, y1, steps=6)
        return sorted(cad.selection()["ids"] or [])

    assert marquee((0, 25), (-45, 15)) == []          # touches nothing
    assert marquee((0, 15), (-25, -15)) == [a]        # only crosses a's right part
    assert marquee((0, -25), (25, 5)) == [b]          # only crosses b's left part
    assert marquee((-25, 25), (25, -5)) == sorted([a, b])


def test_custom_color_picker(cad):
    box = cad.add_shape("box")
    cad.select([box])
    cad.click_control("insp-solid")
    cad.click_control("color-tab-Custom")
    ids = {c["id"] for c in cad.controls()}
    assert {"color-wheel", "color-num-red", "color-hex", "color-copy", "color-mode-HSB"} <= ids
    # type a hex code
    cad.click_control("color-hex")
    cad.type_text("e91d2d")
    cad.key("enter")
    assert cad.node(box)["color"] == "#e91d2d"
    # type a channel value
    cad.click_control("color-num-green")
    cad.type_text("200")
    cad.key("enter")
    assert cad.node(box)["color"] == "#e9c82d"
    # HSB: brightness 0 is black
    cad.click_control("color-mode-HSB")
    cad.click_control("color-num-brightness")
    cad.type_text("0")
    cad.key("enter")
    assert cad.node(box)["color"] == "#000000"
    cad.key("escape")


def test_about_dialog_shows_version(cad):
    import pathlib
    want = (pathlib.Path(__file__).resolve().parents[2] / "VERSION").read_text().strip()
    assert cad.version() == want
    cad.click_control("filemenu")
    cad.click_control("dm-about")
    assert cad.ui_state()["popup"] == "about"
    cad.click_control("about-ok")
    assert cad.ui_state()["popup"] == ""


def test_export_defaults_to_selection(cad):
    box = cad.add_shape("box")
    cad.clear_selection()
    cad.click_control("export")
    assert cad.ui_state().get("export_selected") is False
    cad.key("escape")
    cad.select([box])
    cad.click_control("export")
    assert cad.ui_state().get("export_selected") is True
    cad.key("escape")


def test_ruler_origin_click_turns_ruler(cad):
    box = cad.add_shape("box", x=20, y=20)  # 10..30 on both axes
    cad.place_ruler(-40, -40)
    cad.select([box])
    for want in (1, 2, 3, 0):
        cad.click_control("ruler-origin")
        r = cad.state()["ruler"]
        assert r.get("turns", 0) == want
        o = r["origin"]
        o = [o[k] for k in sorted(o)] if isinstance(o, dict) else o
        assert [round(v, 6) for v in o[:2]] == [-40, -40]  # a click doesn't move it
    # a quarter turn: the first arm points along +Y, the second along -X,
    # so the box measures 50 along the first and -50 along the second
    cad.place_ruler(-40, -40, turns=1)
    cad.select([box])
    cad.click_control("ruler-x")
    cad.type_text("60")
    cad.key("enter")
    assert math.isclose(cad.node(box)["bounds"]["min"][1], 20, abs_tol=1e-6)
    cad.click_control("ruler-y")
    cad.type_text("-60")
    cad.key("enter")
    # the box's near edge (x = 10, at -50) moves out to -60: x = 20
    assert math.isclose(cad.node(box)["bounds"]["min"][0], 20, abs_tol=1e-6)


def test_tooltip_redraws_by_itself_after_crossing_buttons(cad):
    import time
    ctl = {c["id"]: c["rect"] for c in cad.controls()}
    for cid in ("paste", "delete"):
        x, y, w, h = ctl[cid]
        cad.mouse_move(x + w / 2, y + h / 2)
        time.sleep(0.1)
    f0 = cad.ui_state()["frames"]
    time.sleep(0.8)
    st = cad.ui_state()
    assert st["hot"] == "delete" and st["frames"] > f0


def test_align_key_object_by_clicking(cad):
    a = cad.add_shape("box", x=-40, y=0)
    b = cad.add_shape("box", x=40, y=30)
    cad.set_size(x=40, ids=[b])
    cad.select([a, b])
    cad.key("l")
    # click the second box: it becomes the key object
    bx, by = cad.project(40, 20, 10)
    cad.click(bx, by)
    st = cad.ui_state()
    assert st["mode"] == "align" and st["align_key"] == b
    assert sorted(cad.selection()["ids"]) == sorted([a, b])
    before_b = cad.node(b)["bounds"]
    # the x-max align dot now sits at the key box's max X
    b_box = cad.node(b)["bounds"]
    cad.align("x", "max", key=b)
    assert cad.node(b)["bounds"] == before_b
    assert math.isclose(cad.node(a)["bounds"]["max"][0], b_box["max"][0], abs_tol=1e-6)
    cad.key("esc")


def test_intersect_group(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=10)
    g = cad.group("union", ids=[a, b])
    assert math.isclose(cad.node(g)["volume"], 12000, rel_tol=1e-6)
    cad.select([g])
    cad.click_control("insp-gt-intersect")
    n = cad.node(g)
    assert n["group_mode"] == "intersect" and n["label"] == "Intersect"
    assert math.isclose(n["volume"], 4000, rel_tol=1e-6)
    assert sorted(cad.ungroup(ids=[g])) == sorted([a, b])


def test_file_menu_button_and_alt_f(cad):
    cad.click_control("filemenu")
    assert cad.ui_state()["popup"] == "design_menu"
    ids = {c["id"] for c in cad.controls()}
    assert {"dm-new", "dm-open", "dm-save", "dm-saveas", "dm-export"} <= ids
    cad.key("esc")
    assert cad.ui_state()["popup"] == ""
    cad.key("alt+f")
    assert cad.ui_state()["popup"] == "design_menu"
    cad.key("esc")
    cad.click_control("logo")
    assert cad.ui_state()["popup"] == "design_menu"
    cad.key("esc")
