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
