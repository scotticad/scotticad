"""The newer shapes, through the running program: their library
tiles, the bearing seat's dropdown, a QR code's text, and the joints'
sockets arriving as holes."""
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


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


def search(cad, words):
    cad.click_control("search")
    cad.type_text(words)
    found = {i for i in ids(cad) if i.startswith("tile-")}
    cad.click_control("search-close")
    return found


@pytest.mark.parametrize("words,tile", [
    ("stadium", "tile-slot"), ("pie", "tile-arc"),
    ("fillet", "tile-fillet"), ("teardrop", "tile-teardrop"),
    ("quartz", "tile-crystal"), ("keyhole", "tile-keyhole-hole"), ("608", "tile-bearingseat-hole"),
    ("bevel", "tile-bevelgear"), ("bevel", "tile-bevelpair"), ("gt2", "tile-gt2pulley"),
    ("dovetail", "tile-dovetail"), ("dowel", "tile-alignpin"), ("qr", "tile-qrcode"),
    ("rounded box", "tile-roundedbox"), ("moon", "tile-crescent"), ("hdmi", "tile-port"),
    ("usb", "tile-port"), ("rosette", "tile-finishingwasher"),
    ("gridfinity", "tile-gridfinitybin"), ("gridfinity", "tile-gridfinityplate"),
    ("drawer organizer", "tile-gridfinitybin"),
])
def test_found_in_the_library(cad, words, tile):
    assert tile in search(cad, words)


def test_bearing_dropdown(cad):
    s = cad.add_shape("bearingseat", hole=True)
    cad.select([s])
    kinds = {k["id"]: k for k in cad.shape_kinds()}
    p = next(p for p in kinds["bearingseat"]["params"] if p["key"] == "bearing")
    assert p["kind"] == "choice" and p["options"][6].startswith("608")
    wide = cad.node(s)["size"][0]
    cad.click_control("param-bearing")
    opt = "param-bearing-" + p["options"][0]
    assert opt in ids(cad)  # the list is open
    cad.click_control(opt)
    assert opt not in ids(cad)  # and closed again
    n = cad.node(s)
    assert n["params"]["bearing"] == 0 and n["size"][0] < wide
    assert cad.undo() and cad.node(s)["params"]["bearing"] == 6
    # Esc closes an open list without choosing
    cad.click_control("param-bearing")
    cad.key("esc")
    assert opt not in ids(cad) and cad.node(s)["params"]["bearing"] == 6


def test_qr_code_text(cad):
    q = cad.add_shape("qrcode")
    cad.select([q])
    n = cad.node(q)
    assert n["text"] == "https://scotticad.com" and n["child_count"] == 2
    cad.click_control("param-text")
    cad.key("ctrl+a")
    cad.type_text("HELLO")
    cad.key("enter")
    assert cad.node(q)["text"] == "HELLO"
    kids = cad.nodes(deep=True)
    assert all(k.get("text") == "HELLO" for k in kids[0]["children"])
    cad.undo()
    assert cad.node(q)["text"] == "https://scotticad.com"
    # too long: refused, with a message, by the field and by the API
    cad.click_control("param-text")
    cad.key("ctrl+a")
    cad.type_text("x" * 280)
    assert "too long" in cad.ui_state()["toast"]
    cad.key("esc")
    assert cad.node(q)["text"] == "https://scotticad.com"
    with pytest.raises(ScottiCADError):
        cad.set_text(q, "x" * 280)
    cad.set_text(q, "WIFI:T:WPA;S:Net;P:pass;;")
    assert cad.node(q)["text"].startswith("WIFI")


def test_joint_sockets_are_holes(cad):
    for kind in ("dovetail", "alignpin"):
        g = cad.add_shape(kind)
        kids = [k for k in cad.nodes(deep=True) if k["id"] == g][0]["children"]
        assert [k["hole"] for k in kids] == [False, True], kind


def test_port_opening(cad):
    """The port opening is a solid tile in the Hardware library; its
    connector list starts with Custom, picking a connector sizes it, and
    Custom takes its own settings."""
    kinds = {k["id"]: k for k in cad.shape_kinds()}
    p = next(p for p in kinds["port"]["params"] if p["key"] == "connector")
    assert p["kind"] == "choice" and p["options"][:3] == ["Custom", "USB-C", "USB-C Plug"]
    hdmi = p["options"].index("HDMI")
    s = cad.add_shape("port")
    assert [round(v, 3) for v in cad.node(s)["size"]] == [9.4, 10, 3.7]  # USB-C
    cad.set_param(s, "connector", hdmi)
    assert [round(v, 3) for v in cad.node(s)["size"]] == [15.7, 10, 6.2]
    assert cad.undo() and cad.node(s)["params"]["connector"] == 1
    cad.set_param(s, "connector", 0)
    cad.set_param(s, "width", 20)
    cad.set_param(s, "clearance", 0.5)
    assert [round(v, 3) for v in cad.node(s)["size"]] == [21, 10, 4.7]


def test_rounded_box_and_crescent(cad):
    for kind, size in (("roundedbox", [40, 30, 20]), ("crescent", [40, 40, 10])):
        s = cad.add_shape(kind)
        assert [round(v, 6) for v in cad.node(s)["size"]] == size, kind
    b = cad.add_shape("roundedbox")
    cad.set_param(b, "top", 3)
    assert cad.node(b)["params"]["top"] == 3 and cad.node(b)["size"] == [40, 30, 20]


def test_hardware_settings(cad):
    """The settings added to the screw holes, the standoff, the spring and
    the teardrop, and the finishing washer. Each new setting's default
    leaves the shape as it was."""
    w = cad.add_shape("finishingwasher")
    assert [round(v, 6) for v in cad.node(w)["size"]] == [13.4, 13.4, 3.95]
    # a custom countersink is its head's diameter across
    c = cad.add_shape("countersink", hole=True)
    metric = cad.node(c)["size"]
    cad.set_param(c, "screw", 1)
    cad.set_param(c, "head", 9)
    assert round(cad.node(c)["size"][0], 6) == 9
    assert cad.undo() and cad.undo() and cad.node(c)["size"] == metric
    # four gussets reach past the flange
    s = cad.add_shape("standoff")
    plain = cad.node(s)["size"]
    cad.set_param(s, "gussets", 4)
    assert round(cad.node(s)["size"][0], 6) == 14 and cad.node(s)["size"][2] == plain[2]
    # a left-hand spring is the same size
    p = cad.add_shape("spring")
    right = cad.node(p)["size"]
    cad.set_param(p, "hand", 1)
    assert [round(v, 6) for v in cad.node(p)["size"]] == [round(v, 6) for v in right]
    # a sharper tip stands taller
    t = cad.add_shape("teardrop")
    tall = cad.node(t)["size"][2]
    cad.set_param(t, "tip", 60)
    assert cad.node(t)["size"][2] > tall


def test_gridfinity(cad):
    """A Gridfinity bin and baseplate: sizes in grid units, the settings
    changing them, and a bin exporting as one closed part."""
    b = cad.add_shape("gridfinitybin", y=-50)
    # 2 x 1 units, 3 high (21 mm) and the stacking lip (4 mm, its edge cut flat)
    assert [round(v, 6) for v in cad.node(b)["size"]] == [83.5, 41.5, 25]
    cad.set_param(b, "unitsx", 3)
    cad.set_param(b, "unitsz", 5)
    assert [round(v, 6) for v in cad.node(b)["size"]] == [125.5, 41.5, 39]
    cad.set_param(b, "lip", 0)
    assert round(cad.node(b)["size"][2], 6) == 35
    # inside settings leave the size alone
    for key, value in [("compx", 3), ("compy", 2), ("scoop", 8), ("label", 12), ("holes", 3)]:
        cad.set_param(b, key, value)
    assert [round(v, 6) for v in cad.node(b)["size"]] == [125.5, 41.5, 35]
    assert cad.undo() and cad.node(b)["params"]["holes"] == 0
    p = cad.add_shape("gridfinityplate", y=50)
    assert [round(v, 6) for v in cad.node(p)["size"]] == [126, 84, 4.75]
    cad.set_param(p, "bottom", 2)
    assert round(cad.node(p)["size"][2], 6) == 7.75
    for part in (b, p):
        assert cad.print_check(ids=[part])["parts"][0]["status"] == "ok"

