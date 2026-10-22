"""Long boolean sequences through the API: parameters edited deep inside
nested groups, modes switched, groups dissolved and remade, with the
volume worked out by hand at every step and undo and redo walking it all
back and forward; the result exported as an STL that is checked
independently."""
import math
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402
from test_stl import check_stl  # noqa: E402


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    yield


def vol(cad, id):
    return cad.node(id)["volume"]


def near(a, b):
    return math.isclose(a, b, rel_tol=1e-9, abs_tol=1e-6)


def box(cad, lo, hi, hole=False):
    i = cad.add_shape("box", hole=hole)
    cad.set_bounds(lo, hi, ids=[i])
    return i


def test_edits_deep_inside_nested_groups(cad, tmp_path):
    # a plate with a square hole, inside a group with a boss, inside a group
    # clipped by a block: three levels of booleans
    plate = box(cad, [0, 0, 0], [60, 40, 10])
    hole = box(cad, [10, 10, -5], [14, 14, 15], hole=True)
    g1 = cad.group("union", ids=[plate, hole])
    assert near(vol(cad, g1), 24000 - 160)
    boss = box(cad, [30, 10, 10], [50, 30, 20])
    g2 = cad.group("union", ids=[g1, boss])
    assert near(vol(cad, g2), 23840 + 4000)
    clip = box(cad, [0, 0, 0], [40, 40, 40])
    g3 = cad.group("intersect", ids=[g2, clip])
    # left 40 of the plate (16000 - 160) and 10 of the boss's 20 (2000)
    assert near(vol(cad, g3), 15840 + 2000)
    volumes = [vol(cad, g3)]

    # the hole, three levels down, made twice as long in x: it removes
    # 8 x 4 x 10 instead of 4 x 4 x 10
    cad.set_param(hole, "length", 8)
    assert near(vol(cad, g3), 15840 + 2000 - 160)
    volumes.append(vol(cad, g3))
    # the boss, two levels down, made taller: 10 more in z over its clipped 10 x 20
    cad.set_param(boss, "height", 20)
    assert near(vol(cad, g3), volumes[-1] + 10 * 20 * 10)
    volumes.append(vol(cad, g3))
    # the plate itself thinner: 5 less in z over 40 x 40, and the hole removes 5 less
    cad.set_param(plate, "height", 5)
    assert near(vol(cad, g3), volumes[-1] - 40 * 40 * 5 + 8 * 4 * 5)
    volumes.append(vol(cad, g3))

    # every edit undone and redone restores the volume exactly
    for v in reversed(volumes[:-1]):
        assert cad.undo()
        assert near(vol(cad, g3), v)
    for v in volumes[1:]:
        assert cad.redo()
        assert near(vol(cad, g3), v)

    # exported, the file holds that solid
    p = str(tmp_path / "nested.stl")
    cad.export(p)
    check_stl(p, volumes[-1], "nested edits")


def test_modes_switched_and_groups_remade(cad, tmp_path):
    a = box(cad, [0, 0, 0], [20, 20, 20])
    b = box(cad, [10, 0, 0], [30, 20, 20])
    h = box(cad, [5, 5, -5], [25, 15, 25], hole=True)
    g = cad.group("union", ids=[a, b, h])
    union = 8000 + 8000 - 4000 - 20 * 10 * 20
    assert near(vol(cad, g), union)
    cad.set_group_mode("intersect", ids=[g])
    # a and b share x 10..20; the hole cuts 10 x 10 x 20 of that
    assert near(vol(cad, g), 4000 - 2000)
    cad.set_group_mode("bundle", ids=[g])
    # a bundle's parts stand alone: a and b each whole, the hole left out
    assert near(vol(cad, g), 16000)
    cad.set_group_mode("union", ids=[g])
    assert near(vol(cad, g), union)
    # dissolved and remade in another order: the same solid
    parts = cad.ungroup(ids=[g])
    assert sorted(parts) == sorted([a, b, h])
    assert cad.node(h)["hole"]
    g2 = cad.group("union", ids=[h, b, a])
    assert near(vol(cad, g2), union)
    # the result as a hole in a bigger block
    k = box(cad, [-10, -10, -10], [40, 30, 30])
    cad.set_hole(True, ids=[g2])
    g3 = cad.group("union", ids=[k, g2])
    assert near(vol(cad, g3), 50 * 40 * 40 - union)
    p =str(tmp_path / "remade.stl")
    cad.export(p)
    check_stl(p, 50 * 40 * 40 - union, "block minus the remade union")
    # undo all the way: the three loose shapes come back, then nothing
    while cad.undo():
        pass
    assert cad.state()["node_count"] == 0
    while cad.redo():
        pass
    assert near(vol(cad, g3), 50 * 40 * 40 - union)


def test_split_result_volumes_add_up(cad, tmp_path):
    """A boolean result split in two gives halves whose volumes add up to
    the whole, each a closed solid in the file."""
    a = box(cad, [0, 0, 0], [40, 20, 20])
    h = box(cad, [15, -5, 5], [25, 25, 15], hole=True)  # a slot through the middle
    g = cad.group("union", ids=[a, h])
    whole = 16000 - 10 * 20 * 10
    assert near(vol(cad, g), whole)
    halves = cad.split("x", at=20, ids=[g])
    assert len(halves) == 2
    total = sum(vol(cad, i) for i in halves)
    assert near(total, whole)
    for i in halves:
        cad.select([i])
        p = str(tmp_path / f"half-{i}.stl")
        cad.export(p, selection_only=True)
        check_stl(p, vol(cad, i), f"half {i}")
    # split again along another axis: four pieces, still the whole
    quarters = cad.split("y", at=10, ids=halves)
    assert len(quarters) == 4
    assert near(sum(vol(cad, i) for i in quarters), whole)
    cad.undo()
    assert near(sum(vol(cad, i) for i in halves), whole)
