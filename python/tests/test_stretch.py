"""The Stretch tool: lengthening and shortening shapes at a plane."""
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


def approx(a, b, tol=1e-6):
    return all(math.isclose(x, y, abs_tol=tol) for x, y in zip(a, b))


def test_stretch_tool(cad):
    """T opens the tool across the longest side; the amount is typed with a
    sign to add or take out, or as the length wanted; Stretch makes a
    stretch group and opens its settings."""
    box = cad.add_shape("box")
    cad.set_size(x=40, ids=[box])
    cad.select([box])
    cad.key("t")
    assert cad.ui_state()["mode"] == "stretch"
    ids = {c["id"] for c in cad.controls()}
    assert {"stretch-grip", "stretch-at", "stretch-axis-x", "stretch-amount", "stretch-apply"} <= ids
    # nothing typed yet: nothing to do
    cad.click_control("stretch-apply")
    assert cad.ui_state()["mode"] == "stretch" and cad.node(box)["kind"] == "box"
    cad.click_control("stretch-amount")
    cad.type_text("+15")
    cad.key("enter")
    cad.click_control("stretch-apply")
    assert cad.ui_state()["mode"] == "none"
    (g,) = cad.selection()["ids"]
    n = cad.node(g)
    assert n["group_mode"] == "stretch" and n["label"] == "Stretch"
    assert approx(n["bounds"]["min"], [-20, -10, 0]) and approx(n["bounds"]["size"], [55, 20, 20])
    assert math.isclose(n["volume"], 55 * 400, rel_tol=1e-9)
    assert n["params"]["axis"] == 0 and math.isclose(n["params"]["length"], 55) and math.isclose(n["params"]["at"], 20)
    # its settings in Properties, each its own undo step
    ids = {c["id"] for c in cad.controls()}
    assert {"param-axis", "param-at", "param-amount", "param-length"} <= ids
    cad.click_control("param-length")
    cad.type_text("70")
    cad.key("enter")
    assert approx(cad.node(g)["bounds"]["size"], [70, 20, 20])
    cad.click_control("param-amount")
    cad.type_text("-10")
    cad.key("enter")
    assert approx(cad.node(g)["bounds"]["size"], [30, 20, 20])
    assert cad.undo() and approx(cad.node(g)["bounds"]["size"], [70, 20, 20])
    # the box inside is still a box, and still editable
    cad.set_param(box, "height", 5)
    assert math.isclose(cad.node(g)["volume"], 70 * 20 * 5, rel_tol=1e-9)


def test_stretch_to_a_length_and_shrink(cad):
    """A plain number is the length to reach; a negative amount takes out a
    slab centred on the plane, shown by its two faces."""
    box = cad.add_shape("box")
    cad.select([box])
    cad.key("t")
    cad.key("z")
    cad.click_control("stretch-amount")
    cad.type_text("50")
    cad.key("enter")
    cad.key("enter")  # Enter applies
    (g,) = cad.selection()["ids"]
    assert approx(cad.node(g)["bounds"]["size"], [20, 20, 50])
    cad.new_design()
    box = cad.add_shape("box")
    g = cad.stretch("y", amount=-6, ids=[box])
    n = cad.node(g)
    assert approx(n["bounds"]["min"], [-10, -10, 0]) and approx(n["bounds"]["size"], [20, 14, 20])
    with pytest.raises(ScottiCADError):
        cad.stretch("y", amount=-30, ids=[box])
    with pytest.raises(ScottiCADError):
        cad.stretch("y", at=50, amount=3, ids=[box])
    with pytest.raises(ScottiCADError):
        cad.stretch("y", amount=3, length=40, ids=[box])


def test_stretch_survives_save_and_bakes_on_ungroup(cad, tmp_path):
    """The stretch is kept in the file and can still be changed after it is
    opened again; set to 0 and ungrouped it gives the shapes back as they
    were; ungrouped stretched, the shape it crossed is a mesh, the one beyond
    it has moved and is still a shape."""
    a = cad.add_shape("cylinder")
    b = cad.add_shape("box", x=40)
    g = cad.stretch("x", at=0, amount=12, ids=[a, b])
    f = tmp_path / "stretch.scotticad"
    cad.save(str(f))
    cad.new_design()
    cad.open(str(f))
    (top,) = cad.nodes()
    g = top["id"]
    assert top["group_mode"] == "stretch" and approx(top["bounds"]["size"], [72, 20, 20])
    cad.set_param(g, "amount", 0)
    out = cad.ungroup(ids=[g])
    assert sorted(cad.node(i)["kind"] for i in out) == ["box", "cylinder"]
    assert cad.undo()
    cad.set_param(g, "amount", 12)
    out = cad.ungroup(ids=[g])
    kinds = {cad.node(i)["kind"]: cad.node(i) for i in out}
    assert set(kinds) == {"mesh", "box"}
    assert approx(kinds["mesh"]["bounds"]["size"], [32, 20, 20], 1e-4)
    assert approx(kinds["box"]["bounds"]["min"], [42, -10, 0])
    # and exported closed
    stl = tmp_path / "stretch.stl"
    cad.export(str(stl))
    assert stl.stat().st_size > 84


def test_stretch_from_a_script(cad):
    r = cad.run_script('b = cad.add_shape("box")\nprint(cad.node(cad.stretch("z", length=35, ids=[b]))["bounds"]["size"][2])')
    assert r["error"] is None and float(r["output"]) == 35
