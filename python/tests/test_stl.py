"""STL exports checked by an independent reader: the binary layout, the
normals against each triangle's winding, closedness of the file as
written (float32), and the volume against the design's, for single
shapes, bundles, nested boolean groups, rotated results, hidden shapes
and selection-only exports; plus imports of STLs other programs write."""
import math
import os
import struct
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


def read_stl(path):
    """A binary STL taken apart by hand: (header, [(normal, (p0, p1, p2), attr)])."""
    data = open(path, "rb").read()
    assert len(data) >= 84, f"{len(data)} bytes"
    (n,) = struct.unpack_from("<I", data, 80)
    assert len(data) == 84 + 50 * n, f"{n} triangles but {len(data)} bytes"
    tris = []
    for i in range(n):
        v = struct.unpack_from("<12fH", data, 84 + 50 * i)
        tris.append((v[0:3], (v[3:6], v[6:9], v[9:12]), v[12]))
    return data[:80].rstrip(b"\0").decode("ascii", "replace"), tris


def sub(a, b):
    return (a[0] - b[0], a[1] - b[1], a[2] - b[2])


def cross(a, b):
    return (a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0])


def dot(a, b):
    return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]


def norm(a):
    return math.sqrt(dot(a, a))


def check_stl(path, volume, label=""):
    """The file is well formed, every normal matches its triangle, the mesh
    is closed as written, and its volume is `volume`. Returns the triangles."""
    header, tris = read_stl(path)
    assert header.startswith("ScottiCAD"), header
    assert tris, f"{label}: no triangles"
    edges = {}
    vol = 0.0
    for i, (n, (p0, p1, p2), attr) in enumerate(tris):
        for p in (p0, p1, p2):
            assert all(math.isfinite(c) for c in p), f"{label}: triangle {i} has {p}"
        assert attr == 0, f"{label}: triangle {i} attribute bytes {attr}"
        c = cross(sub(p1, p0), sub(p2, p0))
        area2 = norm(c)
        if area2 > 1e-6:
            assert abs(norm(n) - 1) < 1e-5, f"{label}: triangle {i} normal {n}"
            assert dot(n, c) / area2 > 0.999, f"{label}: triangle {i} normal {n} against winding {c}"
        vol += dot(p0, cross(p1, p2)) / 6
        for a, b in ((p0, p1), (p1, p2), (p2, p0)):
            if a != b:
                edges[(a, b)] = edges.get((a, b), 0) + 1
    open_edges = sum(1 for (a, b), k in edges.items() if (b, a) not in edges)
    assert open_edges == 0, f"{label}: {open_edges} open edges of {len(edges)}"
    assert math.isclose(vol, volume, rel_tol=1e-5, abs_tol=1e-3), f"{label}: file volume {vol}, want {volume}"
    return tris


@pytest.mark.parametrize("kind", ["box", "cylinder", "sphere", "cone", "torus", "tube", "text", "gear", "projectbox", "heart", "star3d"])
def test_each_shape_exports_closed(cad, kind, tmp_path):
    i = cad.add_shape(kind)
    cad.rotate("y", 25, ids=[i])  # off the axes, so coordinates round in float32
    v = cad.node(i)["volume"]
    assert v > 0
    p = str(tmp_path / f"{kind}.stl")
    cad.export(p)
    check_stl(p, v, kind)


def test_boolean_design_stl(cad, tmp_path):
    """A plate with corner holes, a boss with a bore, clipped, then cut from a
    block: the exported volume is the hand-worked one at each stage."""
    def box(lo, hi, hole=False):
        i = cad.add_shape("box", hole=hole)
        cad.set_bounds(lo, hi, ids=[i])
        return i

    plate = box([0, 0, 0], [60, 40, 10])
    holes = [box([x, y, -5], [x + 4, y + 4, 15], hole=True) for x, y in ((5, 5), (51, 5), (5, 31), (51, 31))]
    g1 = cad.group("union", ids=[plate] + holes)
    assert math.isclose(cad.node(g1)["volume"], 24000 - 640, rel_tol=1e-9)
    boss = box([20, 10, 10], [40, 30, 25])
    bore = box([25, 15, -5], [35, 25, 30], hole=True)
    g2 = cad.group("union", ids=[g1, boss, bore])
    assert math.isclose(cad.node(g2)["volume"], 23360 + 6000 - 2500, rel_tol=1e-9)
    clip = box([0, 0, 0], [30, 40, 30])
    g3 = cad.group("intersect", ids=[g2, clip])
    left = 12000 - 320 + 3000 - 1250
    assert math.isclose(cad.node(g3)["volume"], left, rel_tol=1e-9)
    p = str(tmp_path / "clipped.stl")
    cad.export(p)
    check_stl(p, left, "clipped")
    # as a hole in a block
    cad.set_hole(True, ids=[g3])
    block = box([-10, -10, -10], [70, 50, 40])
    g4 = cad.group("union", ids=[block, g3])
    assert math.isclose(cad.node(g4)["volume"], 240000 - left, rel_tol=1e-9)
    p = str(tmp_path / "block.stl")
    cad.export(p)
    check_stl(p, 240000 - left, "block minus result")
    # turned so nothing lies on the grid, still closed in float32
    cad.rotate("z", 37, ids=[g4])
    cad.rotate("x", 20, ids=[g4])
    p = str(tmp_path / "turned.stl")
    cad.export(p)
    check_stl(p, 240000 - left, "turned")
    # the file imports back as the same solid, and exports the same again
    i = cad.import_file(p, x=200)
    assert math.isclose(cad.node(i)["volume"], 240000 - left, rel_tol=1e-5)
    cad.select([i])
    p2 = str(tmp_path / "again.stl")
    cad.export(p2, selection_only=True)
    assert read_stl(p2)[1] == read_stl(p)[1] or math.isclose(
        sum(dot(a, cross(b, c)) / 6 for _, (a, b, c), _ in read_stl(p2)[1]), 240000 - left, rel_tol=1e-5)


def test_export_selection_hidden_and_bundles(cad, tmp_path):
    a = cad.add_shape("box")
    b = cad.add_shape("box", x=50)
    c = cad.add_shape("box", x=100)
    cad.hide(ids=[c])
    p = str(tmp_path / "visible.stl")
    cad.export(p)
    check_stl(p, 16000, "hidden left out")
    cad.select([a])
    cad.export(p, selection_only=True)
    check_stl(p, 8000, "selection only")
    # a bundle exports its parts as they are, holes left out
    cad.show_all()
    h = cad.add_shape("box", x=50, hole=True)
    cad.set_bounds([45, -5, 5], [55, 5, 25], ids=[h])
    bundle = cad.group("bundle", ids=[b, h])
    cad.select([bundle])
    cad.export(p, selection_only=True)
    check_stl(p, 8000, "bundle")
    # as a union the hole cuts
    cad.set_group_mode("union", ids=[bundle])
    cad.export(p, selection_only=True)
    check_stl(p, 8000 - 10 * 10 * 15, "bundle made a union")
    # nothing to export is an error, not an empty file
    cad.new_design()
    with pytest.raises(ScottiCADError):
        cad.export(p)


def test_import_stls_from_other_programs(cad, tmp_path):
    """ASCII STL, binary STL whose header starts with "solid", and a padded
    binary STL all import as the solid they describe."""
    # a 10 mm cube as ASCII, with the odd spacing and casing files have
    faces = []
    cube = [  # wound outward: the bottom face's normal points down, the top's up
        ((0, 0, 0), (10, 10, 0), (10, 0, 0)), ((0, 0, 0), (0, 10, 0), (10, 10, 0)),
        ((0, 0, 10), (10, 0, 10), (10, 10, 10)), ((0, 0, 10), (10, 10, 10), (0, 10, 10)),
        ((0, 0, 0), (10, 0, 10), (0, 0, 10)), ((0, 0, 0), (10, 0, 0), (10, 0, 10)),
        ((0, 10, 0), (0, 10, 10), (10, 10, 10)), ((0, 10, 0), (10, 10, 10), (10, 10, 0)),
        ((0, 0, 0), (0, 10, 10), (0, 10, 0)), ((0, 0, 0), (0, 0, 10), (0, 10, 10)),
        ((10, 0, 0), (10, 10, 0), (10, 10, 10)), ((10, 0, 0), (10, 10, 10), (10, 0, 10)),
    ]
    assert math.isclose(sum(dot(a, cross(b, c)) / 6 for a, b, c in cube), 1000)
    for p0, p1, p2 in cube:
        n = cross(sub(p1, p0), sub(p2, p0))
        faces.append(f"  facet normal {n[0]} {n[1]} {n[2]}\n    outer loop\n" +
                     "".join(f"      vertex {p[0]:.3e} {p[1]} {p[2]}\n" for p in (p0, p1, p2)) +
                     "    endloop\n  endfacet\n")
    ascii_path = tmp_path / "cube.stl"
    ascii_path.write_text("solid cube\n" + "".join(faces) + "endsolid cube\n")
    i = cad.import_file(str(ascii_path))
    assert math.isclose(cad.node(i)["volume"], 1000, rel_tol=1e-9)
    # the same cube as binary, header starting with "solid", and padded
    body = b"".join(struct.pack("<12fH", 0, 0, 0, *p0, *p1, *p2, 0) for p0, p1, p2 in cube)
    for name, data in (("solid-header", b"solid made elsewhere".ljust(80, b"\0") + struct.pack("<I", 12) + body),
                       ("padded", b"\0" * 80 + struct.pack("<I", 12) + body + b"\0" * 10)):
        bp = tmp_path / f"{name}.stl"
        bp.write_bytes(data)
        i = cad.import_file(str(bp), x=30)
        assert math.isclose(cad.node(i)["volume"], 1000, rel_tol=1e-9), name
    # inside-out (every face wound the wrong way) is turned right side out
    body = b"".join(struct.pack("<12fH", 0, 0, 0, *p0, *p2, *p1, 0) for p0, p1, p2 in cube)
    bp = tmp_path / "insideout.stl"
    bp.write_bytes(b"\0" * 80 + struct.pack("<I", 12) + body)
    i = cad.import_file(str(bp), x=60)
    assert math.isclose(cad.node(i)["volume"], 1000, rel_tol=1e-9)
    # right side out, it adds in a union instead of cutting, and exports closed
    b = cad.add_shape("box")
    cad.set_bounds([55, -5, 5], [65, 5, 15], ids=[b])  # the import's bottom centre is at (60, 0): over its top half
    g = cad.group("union", ids=[i, b])
    assert math.isclose(cad.node(g)["volume"], 1000 + 1000 - 500, rel_tol=1e-6)
    out = str(tmp_path / "union.stl")
    cad.select([g])
    cad.export(out, selection_only=True)
    check_stl(out, 1500, "union with an inside-out import")
    # a truncated binary file is refused
    bp = tmp_path / "short.stl"
    bp.write_bytes((b"\0" * 80 + struct.pack("<I", 12) + body)[:-30])
    with pytest.raises(ScottiCADError):
        cad.import_file(str(bp))


def test_exports_are_reproducible(tmp_path):
    """The same design exported by two separate runs of the program gives
    byte-identical STLs (nothing depends on map order or timing)."""
    def build_and_export(path):
        with launch(headless=True, width=1000, height=700) as c:
            a = c.add_shape("box")
            c.set_size(x=40, y=30, z=12, ids=[a])
            h = c.add_shape("cylinder", hole=True)
            c.set_bounds([-6, -6, -5], [6, 6, 20], ids=[h])
            t = c.add_shape("text", z=12)
            c.set_size(x=30, y=8, z=3, ids=[t])
            s = c.add_shape("sphere", x=15, y=10)
            c.rotate("z", 33, ids=[s])
            g = c.group("union", ids=[a, h, t, s])
            k = c.add_shape("box", x=5)
            c.set_bounds([-30, -30, 6], [30, 30, 40], ids=[k])
            c.group("intersect", ids=[g, k])
            c.export(str(path))
        return path.read_bytes()

    first = build_and_export(tmp_path / "a.stl")
    second = build_and_export(tmp_path / "b.stl")
    assert len(first) > 84 and first == second, f"{len(first)} bytes against {len(second)}"
