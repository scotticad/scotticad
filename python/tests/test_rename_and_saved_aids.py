"""Renaming a shape in the Properties header, and the ruler and
construction plane saved with the design."""
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_tool("none")
    yield


def typein(cad, id, text):
    cad.click_control(id)
    cad.key("ctrl+a")
    cad.type_text(text)
    cad.key("enter")


def test_rename_in_the_properties_header(cad):
    b = cad.add_shape("box")
    cad.select([b])
    assert "insp-name" in {c["id"] for c in cad.controls()}
    typein(cad, "insp-name", "Base plate")
    assert cad.node(b)["name"] == "Base plate"
    assert cad.state()["dirty"]
    # one undo step
    cad.undo()
    assert cad.node(b)["name"] == ""
    cad.redo()
    # an empty name gives the shape back its kind's name
    cad.click_control("insp-name")
    cad.key("ctrl+a")
    cad.key("backspace")
    cad.key("enter")
    assert cad.node(b)["name"] == "" and cad.node(b)["label"] == "Box"
    # Esc leaves the name alone
    cad.click_control("insp-name")
    cad.type_text("nope")
    cad.key("esc")
    assert cad.node(b)["name"] == ""


def test_several_selected_shapes_have_no_name_field(cad):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=40)
    cad.select([a, b])
    assert "insp-name" not in {c["id"] for c in cad.controls()}


def test_ruler_and_construction_plane_reopen_with_the_design(cad, tmp_path):
    b = cad.add_shape("box", x=30)
    cad.rotate("x", 30, ids=[b])
    cad.attach_construction_plane(b)
    cad.place_ruler(-40, -30, midpoint=True, turns=1)
    st = cad.state()
    plane, ruler = st["construction_plane"], st["ruler"]
    path = str(tmp_path / "aids.scotticad")
    cad.save(path)
    # placing the ruler is no unsaved change
    cad.place_ruler(10, 10)
    assert not cad.state()["dirty"]

    cad.new_design()
    assert "ruler" not in cad.state() and "construction_plane" not in cad.state()
    cad.open(path)
    st = cad.state()
    assert st["ruler"] == ruler
    assert st["construction_plane"] == pytest.approx(plane)
    assert not st["dirty"] and not st["can_undo"]
