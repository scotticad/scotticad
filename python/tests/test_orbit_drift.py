"""The center of rotation under ordinary zooming, panning and turning.

The center (the camera's target) is always the middle of the view, and
orbiting turns around it, so wherever it ends up is where the next turn
pivots. These check that zoom, pan and orbit move it only as they should,
and never fling it far from what's on screen.

Wheel zoom keeps the point under the cursor still: target' = p + (target -
p) * f, with f the change in distance. So the anchor p can be recovered from
the camera before and after, and checked.
"""
import math
import os
import random
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402

CRANK = os.path.join(os.path.dirname(__file__), "..", "..", "examples", "scotticad", "crank-reducer.scotticad")


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    cad.set_section(on=False)
    cad.set_camera(ortho=False)
    cad.home_view()
    yield


# --- helpers ---------------------------------------------------------------

def sub(a, b):
    return [x - y for x, y in zip(a, b)]


def length(v):
    return math.sqrt(sum(x * x for x in v))


def unit(yaw, pitch):
    """From the target toward the eye."""
    y, p = math.radians(yaw), math.radians(pitch)
    return [math.cos(p) * math.cos(y), math.cos(p) * math.sin(y), math.sin(p)]


def eye(c):
    return [t + c["dist"] * k for t, k in zip(c["target"], unit(c["yaw"], c["pitch"]))]


def viewport(cad):
    return cad.ui_state()["viewport"]


def grid(cad, nx=7, ny=7, margin=0.06):
    """Points spread over the viewport, kept clear of its edges."""
    x, y, w, h = viewport(cad)
    return [(x + w * (margin + (1 - 2 * margin) * i / (nx - 1)), y + h * (margin + (1 - 2 * margin) * j / (ny - 1)))
            for j in range(ny) for i in range(nx)]


def zoom(cad, x, y, delta):
    """One wheel step at (x, y): the camera before, after, and the anchor
    (None when the distance didn't change, at a zoom limit)."""
    before = cad.camera()
    cad.wheel(x, y, delta)
    after = cad.camera()
    f = after["dist"] / before["dist"]
    anchor = None
    if abs(1 - f) > 1e-9:
        anchor = [(a - f * b) / (1 - f) for a, b in zip(after["target"], before["target"])]
    return before, after, anchor


def moved(before, after):
    return length(sub(after["target"], before["target"]))


# --- zoom ------------------------------------------------------------------

def test_zooming_in_keeps_the_point_under_the_cursor_still(cad):
    box = cad.add_shape("box")
    cad.clear_selection()
    b = cad.node(box)["bounds"]
    top = [b["center"][0], b["center"][1], b["max"][2]]
    for p in (top, [40, 30, 0], [-50, 60, 0]):
        for delta in (1, 3):
            sx, sy = cad.project(*p)
            cad.wheel(sx, sy, delta)
            nx, ny = cad.project(*p)
            assert math.hypot(nx - sx, ny - sy) < 1.0, (p, delta, (sx, sy), (nx, ny))


def test_zooming_out_keeps_the_point_at_the_centers_depth_still(cad):
    """Zooming out keeps still the point under the cursor as deep as the
    center, so the center slides across but never nearer or farther."""
    cad.add_shape("box")
    cad.clear_selection()
    for side in (40, -70):
        c = cad.camera()
        y = math.radians(c["yaw"])
        p = [t + side * k for t, k in zip(c["target"], (-math.sin(y), math.cos(y), 0))]   # beside the center
        sx, sy = cad.project(*p)
        cad.wheel(sx, sy, -2)
        nx, ny = cad.project(*p)
        assert math.hypot(nx - sx, ny - sy) < 1.0, (side, (sx, sy), (nx, ny))


def test_zooming_out_never_moves_the_center_nearer_or_farther(cad):
    """Over shapes, the plate or the sky: a zoom out moves the center only
    across the view."""
    cad.add_shape("box")
    cad.clear_selection()
    for x, y in grid(cad, 4, 4):
        before = cad.camera()
        cad.wheel(x, y, -2)
        after = cad.camera()
        along = sum(a * b for a, b in zip(sub(after["target"], before["target"]), unit(before["yaw"], before["pitch"])))
        assert abs(along) <= 1e-6 * before["dist"] + 1e-6, (x, y, along)


# One wheel step out scales the distance by 1/0.87, and moves the center by
# 0.15 of its offset from the anchor; an anchor at most 3 distances from
# the eye is at most 4 from the center, so the center moves at most 0.6.
STEP_LIMIT = 0.6


@pytest.mark.parametrize("pitch", [45, 20, 5, 2, -10])
def test_zoom_never_flings_the_center(cad, pitch):
    """However low the view, one wheel step moves the center by a fraction
    of the camera's distance, wherever the cursor is: near the horizon the
    plate under the cursor can be dozens of distances away."""
    cad.set_camera(pitch=pitch)
    start = cad.camera()
    worst = (0, None)
    # a coarse grid, and a fine scan down the middle: the band just under
    # the horizon, where the plate is farthest, is only a few pixels tall
    vx, vy, vw, vh = viewport(cad)
    # (zooming out moves the center farther than zooming in)
    scan = [(vx + vw * 0.4, vy + vh * k / 160, (-1,)) for k in range(4, 157)]
    for x, y, deltas in [(x, y, (1, -1)) for x, y in grid(cad, 5, 5)] + scan:
        for delta in deltas:
            cad.set_camera(target=start["target"], yaw=start["yaw"], pitch=start["pitch"], dist=start["dist"])
            before, after, anchor = zoom(cad, x, y, delta)
            m = moved(before, after) / before["dist"]
            if m > worst[0]:
                worst = (m, (x, y, delta, anchor))
    assert worst[0] <= STEP_LIMIT, f"a wheel step moved the center {worst[0]:.2f} camera distances: {worst[1]}"


def test_zoom_anchor_is_in_front_and_near(cad):
    """The point zoomed toward is in front of the camera and not far past
    the plate's visible part."""
    cad.add_shape("box")
    cad.clear_selection()
    for pitch in (30, 8, 3):
        cad.set_camera(target=[0, 0, 0], pitch=pitch, dist=260)
        for x, y in grid(cad, 6, 6):
            before, after, anchor = zoom(cad, x, y, 1)
            cad.set_camera(target=before["target"], dist=before["dist"])
            if anchor is None:
                continue
            e = eye(before)
            to_anchor = sub(anchor, e)
            fwd = sub(before["target"], e)
            assert sum(a * b for a, b in zip(to_anchor, fwd)) > 0, (pitch, x, y, anchor)   # in front
            assert length(to_anchor) <= 4 * before["dist"], (pitch, x, y, anchor)


def test_orthographic_zoom_moves_the_center_only_across(cad):
    """In an orthographic view depth doesn't show, so a zoom moving the
    center along the line of sight is invisible until the next turn."""
    cad.add_shape("box")
    cad.clear_selection()
    for pitch in (30, 5):
        cad.set_camera(target=[0, 0, 0], pitch=pitch, dist=260, ortho=True)
        for x, y in grid(cad, 5, 5):
            before, after, _ = zoom(cad, x, y, -1)
            d = sub(after["target"], before["target"])
            u = unit(before["yaw"], before["pitch"])
            along = sum(a * b for a, b in zip(d, u))
            assert abs(along) <= 1e-6 * before["dist"] + 1e-6, (pitch, x, y, along)
            cad.set_camera(target=before["target"], dist=before["dist"])


# --- orbit and pan ---------------------------------------------------------

def test_orbit_and_the_view_cube_leave_the_center_alone(cad):
    cad.add_shape("box")
    cad.clear_selection()
    cad.wheel(*grid(cad, 3, 3)[2], 3)                    # somewhere other than home
    start = cad.camera()
    x, y, w, h = viewport(cad)
    cx, cy = x + w / 2, y + h / 2
    cad.drag(cx, cy, cx + 240, cy - 130, steps=12, button="right")
    c = cad.camera()
    assert c["target"] == pytest.approx(start["target"], abs=1e-9)
    assert c["dist"] == pytest.approx(start["dist"])
    assert (c["yaw"], c["pitch"]) != (start["yaw"], start["pitch"])


def test_pan_there_and_back_comes_back(cad):
    start = cad.camera()
    x, y, w, h = viewport(cad)
    cx, cy = x + w / 2, y + h / 2
    for button, mods in (("middle", ()), ("right", ("shift",))):
        cad.drag(cx, cy, cx + 180, cy + 90, steps=9, button=button, mods=mods)
        c = cad.camera()
        assert (c["yaw"], c["pitch"], c["dist"]) == pytest.approx((start["yaw"], start["pitch"], start["dist"]))
        assert moved(start, c) > 1
        cad.drag(cx + 180, cy + 90, cx, cy, steps=9, button=button, mods=mods)
        assert cad.camera()["target"] == pytest.approx(start["target"], abs=1e-6)


# --- with the section tool -------------------------------------------------

def test_section_zoom_ignores_what_it_hides(cad):
    """Zooming at a section looks through the cut-away half: the anchor
    is on what shows, not on the hidden half nearer the camera."""
    box = cad.add_shape("box")
    cad.clear_selection()
    e = eye(cad.camera())
    near_side = "below" if e[0] < 0 else "above"         # the half facing the camera
    far_side = "above" if near_side == "below" else "below"
    cad.set_section(on=True, axis="x", at=0, keep=far_side, ids=[box])
    sx, sy = cad.project(0, 0, 10)
    _, _, anchor = zoom(cad, sx, sy, 1)
    assert anchor is not None
    shown = anchor[0] >= -1e-6 if far_side == "above" else anchor[0] <= 1e-6
    assert shown, (far_side, anchor)


# --- zoomed inside a design ------------------------------------------------

def bounds_gap(p, lo, hi):
    """How far p is outside the box lo..hi (0 inside)."""
    return length([max(l - v, 0, v - h) for v, l, h in zip(p, lo, hi)])


def design_bounds(cad):
    lo, hi = [math.inf] * 3, [-math.inf] * 3
    for n in cad.nodes():
        b = cad.node(n["id"])["bounds"]
        lo = [min(a, c) for a, c in zip(lo, b["min"])]
        hi = [max(a, c) for a, c in zip(hi, b["max"])]
    return lo, hi


@pytest.mark.parametrize("where", [(0.5, 0.5), (0.3, 0.6), (0.7, 0.35)])
def test_zooming_out_from_inside_keeps_the_center_by_the_design(cad, where):
    """Zoom deep into the crank reducer, turn around, zoom back out: the
    center must stay with the design, not end up out in front of it."""
    cad.open(CRANK)
    cad.fit_view()
    lo, hi = design_bounds(cad)
    size = length(sub(hi, lo))
    x, y, w, h = viewport(cad)
    cx, cy = x + w / 2, y + h / 2
    for _ in range(40):                                  # in, to the nearest the camera goes
        cad.wheel(cx, cy, 1)
    cad.drag(cx - 210, cy, cx + 210, cy, steps=20, button="right")   # about 180 degrees
    px, py = x + w * where[0], y + h * where[1]
    for i in range(30):
        cad.wheel(px, py, -1)
        c = cad.camera()
        gap = bounds_gap(c["target"], lo, hi)
        assert gap <= 0.25 * size, f"after {i + 1} steps out the center is {gap:.0f} mm outside the design"


# --- a long session --------------------------------------------------------

def test_a_long_session_on_the_crank_reducer(cad):
    """Many zooms, pans and turns around a real design's base, with the
    section tool coming and going. No single step may throw the center
    far, and every zoom anchors on something near and in front."""
    cad.open(CRANK)
    cad.fit_view()
    rnd = random.Random(20261107)
    x, y, w, h = viewport(cad)
    log = []
    for step in range(300):
        before = cad.camera()
        r = rnd.random()
        if r < 0.55:                                     # wheel, mostly over the lower half
            px, py = x + w * rnd.uniform(0.05, 0.95), y + h * rnd.uniform(0.2, 0.98)
            d = rnd.choice((1, 1, 2, -1, -1, -2))
            cad.wheel(px, py, d)
            what = ("wheel", round(px), round(py), d)
        elif r < 0.75:                                   # a turn, kept low and around
            px, py = x + w / 2, y + h / 2
            cad.drag(px, py, px + rnd.uniform(-200, 200), py + rnd.uniform(-60, 80), steps=6, button="right")
            what = ("orbit",)
        elif r < 0.9:                                    # a short pan
            px, py = x + w / 2, y + h / 2
            cad.drag(px, py, px + rnd.uniform(-120, 120), py + rnd.uniform(-80, 80), steps=6, button="middle")
            what = ("pan",)
        else:
            if rnd.random() < 0.5:
                cad.set_section(on=True, axis=rnd.choice("xyz"))
                what = ("section on",)
            else:
                cad.set_section(on=False)
                what = ("section off",)
        after = cad.camera()
        log.append(what)
        m = moved(before, after) / before["dist"]
        # a zoom moves it by its change in distance times the anchor's
        # offset (at most 4 distances, as STEP_LIMIT); a pan by less than
        # a view's width
        if what[0] == "wheel":
            limit = abs(after["dist"] / before["dist"] - 1) * 4 + 1e-9
        else:
            limit = 1.5
        assert m <= limit, f"step {step} {what} moved the center {m:.2f} distances; last steps {log[-6:]}"
        if what[0] == "wheel":
            f = after["dist"] / before["dist"]
            if abs(1 - f) > 1e-9:
                anchor = [(a - f * b) / (1 - f) for a, b in zip(after["target"], before["target"])]
                e = eye(before)
                ahead = sum(p * q for p, q in zip(sub(anchor, e), sub(before["target"], e)))
                assert ahead > 0, f"step {step} {what} zoomed toward a point behind the camera"
                far = length(sub(anchor, e)) / before["dist"]
                assert far <= 4, f"step {step} {what} zoomed toward a point {far:.1f} distances away"
