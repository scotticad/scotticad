"""Regression tests for faults fixed in 0.4.0, each with the steps that
show it."""
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
    if cad.sketch_state()["open"]:
        cad.sketch_cancel()
    cad.new_design()
    cad.set_tool("none")
    if cad.ui_state()["popup"]:
        cad.key("esc")
    yield


@pytest.mark.parametrize("opener", ["f1", "about"])
def test_keys_do_nothing_behind_help_and_about(cad, opener):
    # keys pressed behind Keyboard shortcuts or About change nothing unseen
    a = cad.add_shape("box")
    b = cad.add_shape("cylinder", x=40)
    cad.select([a])
    if opener == "about":
        cad.click_control("filemenu")
        cad.click_control("dm-about")
    else:
        cad.key(opener)
    assert cad.ui_state()["popup"] in ("help", "about")
    before = cad.node(a)["bounds"]
    for key in ("delete", "ctrl+x", "h", "right", "ctrl+a", "ctrl+d"):
        cad.key(key)
    assert sorted(n["id"] for n in cad.nodes()) == [a, b]
    assert cad.node(a)["bounds"] == before and not cad.node(a).get("hidden", False)
    cad.key("esc")
    assert not cad.ui_state()["popup"]
    cad.key("delete")
    assert [n["id"] for n in cad.nodes()] == [b]


def test_selection_replies_are_lists_when_empty(cad):
    assert cad.select([]) == []
    assert cad.select_all() == []
    assert cad.call("list_handles") == []
    i = cad.add_shape("box")
    assert cad.select_all() == [i]
    # an empty drawing: no paths, nothing selected (empty lists, not null)
    cad.edit_sketch(kind="sketch")
    st = cad.sketch_state()
    assert st["paths"] == [] and st["selected"] == []
    cad.sketch_cancel()


def test_edit_sketch_does_not_replace_an_open_sketch(cad):
    sq = [{"points": [[0, 0], [10, 0], [10, 10], [0, 10]], "closed": True}]
    a = cad.add_sketch(sq)
    b = cad.add_sketch(sq, x=40)
    cad.edit_sketch(a)
    for args in ({"id": b}, {"kind": "revolve"}):
        with pytest.raises(ScottiCADError, match="already open"):
            cad.edit_sketch(**args)
    st = cad.sketch_state()
    assert st["open"] and st["id"] == a
    cad.sketch_cancel()
    # a locked sketch says so, with the workspace left shut
    cad.select([b])
    cad.call("set_locked", on=True)
    with pytest.raises(ScottiCADError, match="locked"):
        cad.edit_sketch(b)
    assert not cad.sketch_state()["open"]


def test_patterns_of_patterns_have_a_limit(cad):
    box = cad.add_shape("box")
    cad.select([box])
    inner = cad.pattern(columns=50, rows=10, dx=30, dy=30)
    cad.select([inner])
    # 500 shapes three times over is fine; fifty rows of that is 75 000
    outer = cad.pattern(columns=3, rows=1, dx=2000, dy=2000)
    with pytest.raises(ScottiCADError, match="more than"):
        cad.set_param(outer, "rows", 50)
    assert cad.node(outer)["params"]["rows"] == 1
    cad.set_param(outer, "rows", 2)
    assert cad.node(outer)["params"]["rows"] == 2


def test_3mf_comes_back_in_through_import(cad, tmp_path):
    # pieces laid apart (one of them two colours, one a revolve) export as
    # separate 3MF objects; importing the file brings them all in as one
    # shape of the same size and volume, closed
    a = cad.add_shape("box", x=-40)
    cad.set_color("#d03030", ids=[a])
    b = cad.add_shape("box", x=20)
    c = cad.add_shape("cylinder", x=30, y=10)
    cad.set_color("#3060d0", ids=[c])
    g = cad.group("union", ids=[b, c])
    cad.set_multicolor(True, ids=[g])
    cad.add_sketch([{"points": [[0, 0], [-8, 0], [-8, 4], [-3, 12], [0, 12]], "closed": True}], kind="revolve", y=50)
    want = sum(n["volume"] for n in cad.nodes())
    lo = [min(n["bounds"]["min"][i] for n in cad.nodes()) for i in range(3)]
    hi = [max(n["bounds"]["max"][i] for n in cad.nodes()) for i in range(3)]
    path = str(tmp_path / "parts.3mf")
    cad.export(path)
    cad.new_design()
    i = cad.import_file(path)
    assert [n["id"] for n in cad.nodes()] == [i]
    n = cad.node(i)
    assert n["volume"] == pytest.approx(want, rel=1e-3)
    assert n["bounds"]["size"] == pytest.approx([h - l for l, h in zip(lo, hi)], abs=0.01)
    r = cad.print_check(ids=[i])
    assert not [f for p in r["parts"] for f in p["findings"] if "hole" in f["message"].lower() or "inside out" in f["message"].lower()], r


def test_fit_view_before_anything_is_drawn():
    # (the view has no size yet: the camera's distance must still be a number)
    with launch(headless=True, width=900, height=700) as c:
        c.call("fit_view")
        cam = c.call("get_camera")
        assert cam["dist"] == cam["dist"] and cam["dist"] > 0
