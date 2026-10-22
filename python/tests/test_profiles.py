"""Outline and loft shapes: shapes made from point data, from Python and
from scripts."""
import json
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1200, height=800) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    yield


def square(h, dx=0.0):
    return [(dx - h, -h), (dx + h, -h), (dx + h, h), (dx - h, h)]


def circle(r, n):
    return [(r * math.cos(2 * math.pi * i / n), r * math.sin(2 * math.pi * i / n)) for i in range(n)]


def close(a, b, tol=1e-6):
    return all(abs(x - y) <= tol for x, y in zip(a, b))


def test_outline_lands_where_its_points_say(cad):
    i = cad.add_outline([square(10, dx=40), square(4, dx=40)], z0=2, z1=7)
    n = cad.node(i)
    assert n["kind"] == "outline"
    assert close(n["bounds"]["min"], [30, -10, 2]) and close(n["bounds"]["max"], [50, 10, 7])
    assert n["volume"] == pytest.approx((400 - 64) * 5, rel=1e-9)
    assert len(n["sections"]) == 2 and len(n["sections"][0]["contours"]) == 2
    assert cad.selection()["ids"] == [i]


def test_loft_volumes(cad):
    # square frustum: same point counts join corner to corner, exactly
    i = cad.add_loft([(0, square(10)), (20, square(5))])
    assert cad.node(i)["volume"] == pytest.approx(20 / 3 * (400 + 100 + 200), rel=1e-9)
    # a cylinder from circles of different point counts
    j = cad.add_loft([(0, circle(10, 200)), (10, circle(10, 151))])
    assert cad.node(j)["volume"] == pytest.approx(math.pi * 100 * 10, rel=0.002)


def test_set_outline_and_loft_keep_the_shapes_frame(cad):
    i = cad.add_loft([(0, square(10)), (10, square(5))])
    cad.set_loft(i, [(0, square(10, dx=50)), (10, square(5, dx=50))])
    b = cad.node(i)["bounds"]
    assert close(b["min"], [40, -10, 0]) and close(b["max"], [60, 10, 10])
    cad.move(dy=30, ids=[i])
    cad.set_loft(i, [(0, square(10)), (10, square(5))])
    assert close(cad.node(i)["bounds"]["min"], [-10, 20, 0])
    o = cad.add_outline(square(5), z0=0, z1=3)
    cad.set_outline(o, square(6))           # heights kept
    assert close(cad.node(o)["bounds"]["max"], [6, 6, 3])
    cad.set_outline(o, square(6), z1=9)     # or changed
    assert close(cad.node(o)["bounds"]["max"], [6, 6, 9])
    with pytest.raises(ScottiCADError, match="not a loft"):
        cad.set_loft(o, [(0, square(5)), (1, square(5))])


def test_bad_data_is_refused_with_a_reason(cad):
    depth = cad.state()["undo_depth"]
    bow = [(0, 0), (10, 10), (10, 0), (0, 10)]
    for call, why in [
        (lambda: cad.add_outline(bow), "crosses itself"),
        (lambda: cad.add_outline([(0, 0), (1, 1)]), "at least 3"),
        (lambda: cad.add_outline(square(5), z0=5, z1=5), "above its bottom"),
        (lambda: cad.add_loft([(0, square(5))]), "at least 2"),
        (lambda: cad.add_loft([(5, square(5)), (2, square(5))]), "heights must go up"),
        (lambda: cad.add_loft([(0, [square(5), square(2)]), (5, square(5))]), "exactly one outline"),
        (lambda: cad.add_loft([(0, "nonsense"), (5, square(5))]), "list of"),
        (lambda: cad.add_outline([(0, 0), (1, "x"), (2, 0)]), "two numbers"),
    ]:
        with pytest.raises(ScottiCADError, match=why):
            call()
    assert cad.nodes() == [] and cad.state()["undo_depth"] == depth


def test_smoothing_is_a_parameter(cad):
    i = cad.add_outline(square(10), z0=0, z1=5)
    v = cad.node(i)["volume"]
    cad.set_param(i, "smooth", 1)
    n = cad.node(i)
    assert n["volume"] > v and n["bounds"]["size"][0] > 20      # a curve through the corners bulges
    cad.undo()
    assert cad.node(i)["volume"] == pytest.approx(v)


def test_profiles_save_reopen_and_combine(cad, tmp_path):
    hull = cad.add_loft([(0, square(10)), (10, circle(8, 40)), (14, circle(9, 40))])
    hole = cad.add_outline(circle(3, 30), z0=-1, z1=20, hole=True)
    g = cad.group("union", ids=[hull, hole])
    v = cad.node(g)["volume"]
    p = str(tmp_path / "loft.scotticad")
    cad.save(p)
    with open(p, encoding="utf-8") as f:
        assert json.load(f)["version"] == 2
    cad.new_design()
    cad.open(p)
    (n,) = cad.nodes()
    assert n["volume"] == pytest.approx(v, rel=1e-9)
    # a loft and outline export as one closed solid
    cad.export(str(tmp_path / "loft.stl"))
    assert os.path.getsize(tmp_path / "loft.stl") > 1000


def test_scripts_build_lofts(cad):
    r = cad.run_script("""
def ring(r, n):
    return [(r * math.cos(2 * math.pi * i / n), r * math.sin(2 * math.pi * i / n)) for i in range(n)]

vase = cad.add_loft([(0, ring(10, 6)), (15, ring(14, 48)), (30, ring(6, 48)), (40, ring(8, 48))])
cut = cad.add_loft([(2, ring(8, 6)), (15, ring(12, 48)), (30, ring(4, 48)), (41, ring(6, 48))], hole = True)
v = cad.group("union", ids = [vase, cut])
print(len(cad.node(vase)["sections"]), cad.node(v)["bounds"]["size"][2])
""", name="vase.star")
    assert r["error"] is None, r["error"]
    assert r["output"] == "4 40"
    r = cad.run_script("cad.add_loft([(0, [(0, 0), (1, 0), (0, 1)])])", name="bad.star")
    assert "at least 2" in r["error"]
