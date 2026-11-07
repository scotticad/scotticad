"""Section view and Lay flat."""
import base64
import collections
import math
import os
import struct
import sys
import zlib

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
    cad.set_section(on=False)
    cad.home_view()
    yield


def centre_on_screen(cad, id):
    b = cad.node(id)["bounds"]
    return cad.project(*b["center"])


def read_png(data):
    """Width, height and rows of (r, g, b) from an 8-bit RGB or RGBA PNG,
    as Go's encoder writes them."""
    pos, idat, w = 8, b"", 0
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        chunk = data[pos + 8:pos + 8 + n]
        if kind == b"IHDR":
            w, h, depth, ctype = struct.unpack(">IIBB", chunk[:10])
            assert depth == 8 and ctype in (2, 6), (depth, ctype)
            bpp = 3 if ctype == 2 else 4
        elif kind == b"IDAT":
            idat += chunk
        pos += 12 + n
    raw, stride, rows, prev = zlib.decompress(idat), w * bpp, [], bytearray(w * bpp)
    for y in range(h):
        f, line = raw[y * (stride + 1)], bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b, c = prev[i], prev[i - bpp] if i >= bpp else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + (a + b) // 2) & 255
            elif f == 4:
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                line[i] = (line[i] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append([tuple(line[x * bpp:x * bpp + 3]) for x in range(w)])
        prev = line
    return w, h, rows


def test_an_api_section_cuts_what_it_was_aimed_at_after_grouping(cad):
    # an assistant edits shapes (each edit selects them), opens a section,
    # then groups everything: the section must still cut, and calling
    # set_section again must aim it afresh
    a = cad.add_shape("box", x=-15)
    b = cad.add_shape("box", x=15)
    cad.select([b])
    r = cad.set_section(axis="z", at=10, keep="below")
    assert r["ids"] is None and r["pinned"] and r["crosses"], r  # every shape, not the selection
    g = cad.group(ids=[a, b])
    sec = cad.ui_state()["section"]
    assert sec["crosses"] and sec["pinned"], "grouping (a selection change) keeps it open"
    cad.clear_selection()
    cad.set_camera(target=[0, 0, 10], yaw=-90, pitch=20, dist=120, ortho=False)
    cad.click(*cad.project(-15, -10, 18))  # the cut-away top of the group
    assert not cad.selection()["ids"], "the group is cut: its top half can't be clicked"
    cad.click(*cad.project(-15, -10, 4))
    assert cad.selection()["ids"] == [g]

    # ids name a shape inside the group: the group is cut
    cad.clear_selection()
    r = cad.set_section(axis="z", at=10, keep="below", ids=[a])
    assert r["ids"] == [a] and r["crosses"]
    cad.click(*cad.project(-15, -10, 18))
    assert not cad.selection()["ids"]
    # a plane past the shapes says so
    assert not cad.set_section(axis="z", at=50)["crosses"]
    # an unknown id is refused, and the view is left as it was
    with pytest.raises(Exception):
        cad.set_section(ids=[987654])
    assert cad.ui_state()["section"]["at"] == 50
    # unpinned, it closes when the selection changes, as the toolbar's does
    cad.set_section(axis="z", at=10, pin=False)
    cad.select([g])
    cad.controls()  # a frame
    assert "section" not in cad.ui_state()


def test_the_cut_face_hides_the_build_plate(cad):
    # Looking down into a Z section, the cut face is drawn where the part's
    # base shows through the cut, and the base lies on the build plate. The
    # cut face must cover the plate's sheet and grid, not fight with them.
    # A hole along X through the middle, at the cut's height: through the
    # cut, its lower wall shows, and the cut face mustn't cover it.
    box = cad.add_shape("box")
    cad.set_bounds(min=[-20, -20, 0], max=[20, 20, 20], ids=[box])
    hole = cad.add_shape("cylinder", hole=True)
    cad.rotate("y", 90, ids=[hole])
    cad.set_bounds(min=[-25, -4, 6], max=[25, 4, 14], ids=[hole])
    part = cad.group(ids=[box, hole])
    cad.set_color("#2f80ed", ids=[part])
    cad.clear_selection()
    cad.set_section(axis="z", at=10, keep="below")
    for ortho, yaw, pitch in ((True, 0, 89.9), (False, 30, 70)):
        cad.set_camera(target=[0, 0, 10], yaw=yaw, pitch=pitch, dist=150, ortho=ortho)
        shot = cad.call("screenshot")
        w, h, rows = read_png(base64.b64decode(shot["png"]))
        dpi = shot["dpi"]

        def colours(pts):
            return collections.Counter(rows[int(py * dpi)][int(px * dpi)] for px, py in (cad.project(*p) for p in pts))
        # points across the cut, clear of the walls and of the hole
        cut = colours([(x, y, 10) for x in range(-14, 15) for y in range(-14, 15) if abs(y) >= 7])
        # the cut face is one flat shade and its hatching: two colours
        face = [c for c, _ in cut.most_common(2)]
        assert sum(cut[c] for c in face) / sum(cut.values()) > 0.99, (ortho, cut.most_common(6))
        # down the hole's middle: its wall, shaded, not the cut face
        bore = colours([(x, 0, 10) for x in range(-14, 15)])
        assert not set(bore) & set(face), (ortho, bore.most_common(4), face)


def test_section_view_hides_and_cannot_be_clicked(cad):
    front = cad.add_shape("box", x=-40, y=-30)
    back = cad.add_shape("box", x=40, y=30)
    cad.clear_selection()
    cad.fit_view()  # both boxes in view
    fx, fy = centre_on_screen(cad, front)
    # without the section, clicking the front box selects it
    cad.click(fx, fy)
    assert cad.selection()["ids"] == [front]
    cad.clear_selection()
    # section across Y at 0 showing the back (+Y) side: the front box is gone
    cad.set_section(axis="y", at=0, keep="above")
    sec = cad.ui_state()["section"]
    assert (sec["axis"], sec["at"], sec["keep"], sec["ids"]) == ("y", 0, "above", None)  # nothing selected: cuts all
    cad.click(fx, fy)
    assert not cad.selection()["ids"]
    # the back box still can be clicked
    cad.click(*centre_on_screen(cad, back))
    assert cad.selection()["ids"] == [back]
    # the design is unchanged: both boxes are whole
    assert len(cad.nodes()) == 2 and all(math.isclose(n["volume"], 8000, rel_tol=1e-6) for n in cad.nodes())


def test_section_view_controls(cad):
    cad.add_shape("box")
    cad.click_control("section")
    sec = cad.ui_state()["section"]
    assert sec["axis"] == "y"
    # the default shows the side away from the camera (which looks from -Y)
    assert sec["keep"] == "above"
    ids = {c["id"] for c in cad.controls()}
    assert {"section-grip", "section-at", "section-axis-x", "section-flip", "section-close"} <= ids
    cad.click_control("section-flip")
    assert cad.ui_state()["section"]["keep"] == "below"
    cad.click_control("section-axis-z")
    assert cad.ui_state()["section"]["axis"] == "z"
    cad.click_control("section-at")
    cad.type_text("5")
    cad.key("enter")
    assert math.isclose(cad.ui_state()["section"]["at"], 5, abs_tol=1e-9)
    cad.key("shift+k")
    assert "section" not in cad.ui_state()


def test_section_follows_the_selection(cad):
    a = cad.add_shape("box", x=-40)
    b = cad.add_shape("cylinder", x=40, y=20)
    cad.clear_selection()
    cad.fit_view()
    cad.select([a])
    cad.key("shift+k")
    sec = cad.ui_state()["section"]
    assert sec["ids"] == [a] and math.isclose(sec["at"], 0, abs_tol=1e-9)  # through a's middle
    # switching axis stays on a
    cad.click_control("section-axis-x")
    assert math.isclose(cad.ui_state()["section"]["at"], -40, abs_tol=1e-9)
    # b isn't cut, so it can be clicked; selecting it closes the section,
    # as selecting another shape closes the other tools
    cad.click(*centre_on_screen(cad, b))
    cad.controls()  # a frame
    assert "section" not in cad.ui_state()
    assert cad.selection()["ids"] == [b]
    # clicking off closes it
    cad.select([a])
    cad.key("shift+k")
    assert "section" in cad.ui_state()
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + 60, vy + vh - 40)
    cad.controls()
    assert "section" not in cad.ui_state()
    # so does Esc
    cad.select([a])
    cad.key("shift+k")
    assert "section" in cad.ui_state()
    cad.key("esc")
    assert "section" not in cad.ui_state()


def test_pinned_section_stays_open(cad):
    a = cad.add_shape("box", x=-40)
    b = cad.add_shape("cylinder", x=40, y=20)
    cad.clear_selection()
    cad.fit_view()
    cad.select([a])
    cad.key("shift+k")
    cad.click_control("section-pin")
    assert cad.ui_state()["section"]["pinned"]
    # selecting and moving another shape keeps it open, still cutting a
    cad.click(*centre_on_screen(cad, b))
    cad.move(dx=5)
    cad.controls()
    sec = cad.ui_state()["section"]
    assert sec["ids"] == [a] and cad.selection()["ids"] == [b]
    # clicking off keeps it open too; Esc closes it
    vx, vy, vw, vh = cad.ui_state()["viewport"]
    cad.click(vx + 60, vy + vh - 40)
    cad.controls()
    assert "section" in cad.ui_state()
    cad.key("esc")
    assert "section" not in cad.ui_state()
    # it opens unpinned next time
    cad.select([a])
    cad.key("shift+k")
    assert not cad.ui_state()["section"]["pinned"]


def test_lay_flat_api(cad):
    w = cad.add_shape("wedge")
    size0 = cad.node(w)["bounds"]["size"]
    # any face works; the one facing straight up turns it upside down
    cad.lay_flat([0, 0, 1], ids=[w])
    b = cad.node(w)["bounds"]
    assert math.isclose(b["min"][2], 0, abs_tol=1e-6)
    assert math.isclose(b["size"][2], size0[2], abs_tol=1e-6)


def test_lay_flat_tool_clicks_a_face(cad):
    box = cad.add_shape("box")
    cad.rotate("x", 30, ids=[box])
    cad.drop(ids=[box])
    assert cad.node(box)["bounds"]["size"][2] > 25        # tilted
    # the middle of the (tilted) top face
    c = cad.node(box)["bounds"]["center"]
    top = [c[0], c[1] - 10 * math.sin(math.radians(30)), c[2] + 10 * math.cos(math.radians(30))]
    cad.clear_selection()
    cad.key("shift+d")
    assert cad.ui_state()["mode"] == "layflat"
    x, y = cad.project(*top)
    cad.mouse_move(x, y)
    cad.click(x, y)
    assert cad.ui_state()["mode"] == "none"
    b = cad.node(box)["bounds"]
    assert all(math.isclose(s, 20, abs_tol=1e-6) for s in b["size"]), b   # square again
    assert math.isclose(b["min"][2], 0, abs_tol=1e-6)
    assert cad.selection()["ids"] == [box]
    cad.undo()
    assert cad.node(box)["bounds"]["size"][2] > 25        # one undo step
