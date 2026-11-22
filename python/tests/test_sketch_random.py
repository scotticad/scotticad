"""A seeded random walk over the sketch workspace: long sessions of the
moves a user makes (clicks with each Pen tool, closing and ending paths,
selecting and dragging points, segments, paths and boxes, dropping ends on
other points, nudges, deletes, Refine, Open/Close, strokes, point kinds,
typed sizes and positions, the clipboard, undo and redo, tool switches,
Clear, and the view), with the drawing's invariants checked after every
step:

- the state is well formed: finite numbers, indices in range, the path being
  drawn open and drawn with a Pen tool, points of one path only while it is
  being drawn;
- no open path has its last point on its first (ends that meet close),
  except the one being drawn and a line there and back (three points);
- on a revolve or coil board no point is right of the center line;
- undo then redo gives back exactly the same drawing;
- Finish sketch either builds a shape (volume finite and above 0, a closed
  surface by the print check, saved as drawn, and the same drawing when it
  is opened again) or says why and leaves the workspace open.

Things the code allows on purpose (it drops them when it builds) are
counted, not asserted, unless SCOTTICAD_SKETCH_STRICT=1: a side of no length
(a point on the one before), a closed path of two points, an open path of
two points in one place, and an undo step that changed nothing.

Positions are in drawing millimeters, turned into window pixels with the
view as it is at that moment, and land on existing points (snap targets),
just beside them, on segments, inside areas and on the grid, since that's
where the bugs are.

Seeded, so a failure reproduces: SCOTTICAD_SKETCH_SEED=7 (or 1,2,3, or a
range 1-40) and SCOTTICAD_SKETCH_STEPS=400; SCOTTICAD_SKETCH_IGNORE=a,b
looks past known failures of invariants a and b. A failure prints the seed, the
step, the invariant and the calls leading up to it, as code to paste;
SCOTTICAD_SKETCH_SHRINK=1 also cuts the session down to the fewest steps
that still break it."""
import copy
import math
import os
import random
import sys
import time

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402


def _seeds(text):
    out = []
    for part in text.split(","):
        part = part.strip()
        if "-" in part[1:]:
            a, b = part.split("-", 1)
            out += list(range(int(a), int(b) + 1))
        elif part:
            out.append(int(part))
    return out


SEEDS = _seeds(os.environ.get("SCOTTICAD_SKETCH_SEED", "1,2,3,4,5"))
STEPS = int(os.environ.get("SCOTTICAD_SKETCH_STEPS", "150"))
STRICT = os.environ.get("SCOTTICAD_SKETCH_STRICT") == "1"
SHRINK = os.environ.get("SCOTTICAD_SKETCH_SHRINK") == "1"
# invariants to look past (a known failure, to find the next): a,b
IGNORE = set(filter(None, os.environ.get("SCOTTICAD_SKETCH_IGNORE", "").split(",")))

PEN = ("line", "bezier", "smooth")
TOOLS = ("select", "line", "bezier", "smooth", "refine", "shape")
EPS = 1e-9  # (as the workspace's own test for ends that meet)
# a finished shape's surface: these mean it isn't closed
BAD_SURFACE = {"hole", "non-manifold", "inside-out"}


def program():
    """A fresh ScottiCAD for each session: the workspace's clipboard lasts
    from one sketch to the next, so sharing one would make a seed's run
    depend on the seeds before it."""
    return launch(headless=True, width=1400, height=900)


# --- the view ---------------------------------------------------------------------


def to_px(st, x, y):
    bx, by, bw, bh = st["board"]
    (cx, cy), z = st["center"], st["zoom"]
    return bx + bw / 2 + (x - cx) * z, by + bh / 2 - (y - cy) * z


def to_mm(st, px, py):
    bx, by, bw, bh = st["board"]
    (cx, cy), z = st["center"], st["zoom"]
    return cx + (px - bx - bw / 2) / z, cy - (py - by - bh / 2) / z


def at(cad, x, y):
    """Window pixels of drawing millimeters (x, y), with the view as it is
    now (a revolve's or coil's center line is pinned two thirds across)."""
    return to_px(cad.sketch_state(), x, y)


class MM:
    """A position on the board in millimeters, turned into pixels when the
    call it is in is made."""

    def __init__(self, x, y):
        self.x, self.y = round(x, 3), round(y, 3)

    def off(self, dx, dy):
        return MM(self.x + dx, self.y + dy)

    def __repr__(self):
        return f"*at(cad, {self.x:g}, {self.y:g})"


# --- the drawing's geometry ---------------------------------------------------------


def pt(q):
    return q["p"]


def same(a, b):
    return abs(a[0] - b[0]) <= EPS and abs(a[1] - b[1]) <= EPS


def handles(path, i):
    """Point i's curve handles as used (an automatic point's worked out), as
    the workspace does."""
    pts = path["points"]
    q = pts[i]
    if q.get("kind", 0) != 2:
        return q.get("in", [0, 0]), q.get("out", [0, 0])
    n, closed = len(pts), path.get("closed", False)
    prev = pts[(i + n - 1) % n]["p"] if i > 0 or closed else None
    nxt = pts[(i + 1) % n]["p"] if i < n - 1 or closed else None
    if prev and nxt:
        d = [(nxt[0] - prev[0]) / 4, (nxt[1] - prev[1]) / 4]
        return [-d[0], -d[1]], d
    return [0, 0], [0, 0]


def on_segment(path, i, t):
    pts = path["points"]
    j = (i + 1) % len(pts)
    a, b = pts[i]["p"], pts[j]["p"]
    _, out = handles(path, i)
    inn, _ = handles(path, j)
    c = [a, [a[0] + out[0], a[1] + out[1]], [b[0] + inn[0], b[1] + inn[1]], b]
    u = 1 - t
    w = [u * u * u, 3 * u * u * t, 3 * u * t * t, t * t * t]
    return sum(w[k] * c[k][0] for k in range(4)), sum(w[k] * c[k][1] for k in range(4))


def norm_paths(paths):
    """Paths with every field filled in, to compare."""
    out = []
    for p in paths or []:
        out.append((tuple((tuple(q["p"]), tuple(q.get("in", [0, 0])), tuple(q.get("out", [0, 0])), q.get("kind", 0))
                          for q in p["points"]),
                    bool(p.get("closed", False)), p.get("stroke", 0), p.get("join", 0), p.get("cap", 0), p.get("align", 0)))
    return out


def finished(st):
    """The drawing Finish sketch should save: the path being drawn ended (a
    single point dropped; ends that meet closed) and paths of one point left
    out."""
    paths = copy.deepcopy(st["paths"])
    d = st["drawing"]
    if d >= 0:
        p = paths[d]
        n = len(p["points"])
        if n < 2:
            del paths[d]
        elif not p.get("closed") and n >= 4 and same(p["points"][0]["p"], p["points"][-1]["p"]):
            last = p["points"].pop()
            if last.get("in", [0, 0]) != [0, 0]:
                p["points"][0]["in"] = last["in"]
            p["closed"] = True
    return [p for p in paths if len(p["points"]) >= 2]


def problems(st, kind):
    """What is wrong with a workspace state: (hard, soft) lists of
    (invariant, message)."""
    hard, soft = [], []

    def finite(v):
        return isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v)

    def pair(v):
        return isinstance(v, list) and len(v) == 2 and all(finite(c) for c in v)

    if not st.get("open"):
        return [("closed", "the workspace closed by itself")], []
    if st.get("tool") not in TOOLS:
        hard.append(("well-formed", f"tool {st.get('tool')!r}"))
    if not (finite(st.get("zoom")) and st["zoom"] > 0) or not pair(st.get("center")):
        hard.append(("well-formed", f"view {st.get('center')} at {st.get('zoom')}"))
    board = st.get("board")
    if not (isinstance(board, list) and len(board) == 4 and all(finite(v) for v in board) and board[2] > 0 and board[3] > 0):
        hard.append(("well-formed", f"board {board}"))
    paths = st.get("paths")
    if not isinstance(paths, list):
        return hard + [("well-formed", f"paths {paths!r}")], soft
    d = st.get("drawing")
    if not isinstance(d, int) or not -1 <= d < len(paths):
        hard.append(("well-formed", f"drawing {d!r} with {len(paths)} paths"))
        d = -1
    elif d >= 0:
        if paths[d].get("closed"):
            hard.append(("well-formed", f"path {d} is being drawn but closed"))
        if st["tool"] not in PEN:
            hard.append(("drawing-tool", f"path {d} is being drawn with the {st['tool']} tool"))
    sel = st.get("selected")
    if not isinstance(sel, list) or len({tuple(s) for s in sel}) != len(sel):
        hard.append(("well-formed", f"selected {sel!r}"))
    else:
        for s in sel:
            if not (len(s) == 2 and 0 <= s[0] < len(paths) and 0 <= s[1] < len(paths[s[0]]["points"])):
                hard.append(("well-formed", f"selected point {s} isn't in the drawing"))
    for i, p in enumerate(paths):
        pts = p.get("points")
        if not isinstance(pts, list) or not pts:
            hard.append(("well-formed", f"path {i} has no points"))
            continue
        for j, q in enumerate(pts):
            if not pair(q.get("p")) or any(k in q and not pair(q[k]) for k in ("in", "out")) or q.get("kind", 0) not in (0, 1, 2):
                hard.append(("well-formed", f"path {i} point {j}: {q}"))
        if not isinstance(p.get("closed", False), bool) or not finite(p.get("stroke", 0)) or not 0 <= p.get("stroke", 0) <= 1000 \
                or any(p.get(k, 0) not in (0, 1, 2) for k in ("join", "cap", "align")):
            hard.append(("well-formed", f"path {i}'s settings: { {k: v for k, v in p.items() if k != 'points'} }"))
    if hard:
        return hard, soft
    for i, p in enumerate(paths):
        ps = [q["p"] for q in p["points"]]
        n, closed = len(ps), p.get("closed", False)
        if n < 2 and i != d:
            hard.append(("one-point-path", f"path {i} has {n} point and isn't being drawn"))
        if kind in ("revolve", "coil"):
            for j, q in enumerate(ps):
                if q[0] > EPS:
                    hard.append(("right-of-center", f"path {i} point {j} at {q} is right of the center line"))
        if not closed and i != d and n >= 2 and same(ps[0], ps[-1]):
            if n >= 4:
                hard.append(("ends-meet-open", f"path {i} is open with its last point on its first: {ps}"))
            elif n == 2:
                soft.append(("zero-length-line", f"path {i}: an open line from {ps[0]} to the same place"))
            # (three points, the ends together: a line there and back, kept
            # open on purpose)
        for j in range(n if closed else n - 1):
            if n > 1 and same(ps[j], ps[(j + 1) % n]):
                soft.append(("zero-length-side", f"path {i}: points {j} and {(j + 1) % n} both at {ps[j]}"))
                break
        if closed and n < 3:
            soft.append(("closed-under-3", f"path {i} is closed with {n} points"))
    return hard, soft


# --- a session ---------------------------------------------------------------------


class Violation(Exception):
    pass


class Walk:
    def __init__(self, cad, seed):
        self.cad, self.seed, self.rnd = cad, seed, random.Random(seed)
        self.kind = None
        self.node = None
        self.step, self.snap = 0, 1
        self.setup = []  # the ops that open the session
        self.actions = []  # [(step, name, ops)]
        self.ops = None
        self.hard = []  # (step, invariant, message, action)
        self.soft = {}  # invariant: [count, first step, message, action]
        self.seen = set()
        self.refused = 0
        self.built, self.not_built = 0, 0  # Finish sketch's outcomes

    # --- running calls, kept to show and replay ---------------------------------------

    def run(self, op, check=None):
        """Runs an op: ("call", name, args, kw), ("done",) or
        ("undoredo",); keeps it in the current action's list."""
        if self.ops is not None:
            self.ops.append(op)
        if op[0] == "done":
            return self.done_check(check)
        if op[0] == "undoredo":
            return self.undo_redo_check(check)
        _, name, args, kw = op
        real = []
        for a in args:
            if isinstance(a, MM):
                real += list(at(self.cad, a.x, a.y))
            else:
                real.append(a)
        try:
            return getattr(self.cad, name)(*real, **kw)
        except ScottiCADError as e:
            if "internal error" in str(e):
                raise
            self.refused += 1
            return None

    def do(self, name, *args, **kw):
        return self.run(("call", name, args, kw))

    def key(self, k):
        self.do("key", k)

    def st(self):
        return self.cad.sketch_state()

    # --- where things are ----------------------------------------------------------------

    def safe(self, st):
        """The part of the board clear of its buttons and panels, in pixels."""
        bx, by, bw, bh = st["board"]
        x0, x1 = bx + 90, bx + bw - 50
        if self.kind in ("revolve", "coil"):
            x1 = to_px(st, 0, 0)[0] + 25  # (a little past the line: kept on it)
        return x0, by + 100, x1, by + bh - 110

    def visible(self, st, x, y):
        px, py = to_px(st, x, y)
        x0, y0, x1, y1 = self.safe(st)
        return x0 <= px <= x1 and y0 <= py <= y1

    def points(self, st, which="any"):
        """[(path, index, [x, y])] of points on screen: any, ends (of open
        paths not being drawn) or starts."""
        out = []
        for i, p in enumerate(st["paths"]):
            pts = p["points"]
            for j, q in enumerate(pts):
                if which == "ends" and (p.get("closed") or i == st["drawing"] or j not in (0, len(pts) - 1)):
                    continue
                if self.visible(st, *q["p"]):
                    out.append((i, j, q["p"]))
        return out

    def grid(self, st):
        x0, y0, x1, y1 = self.safe(st)
        a, b = to_mm(st, x0, y1), to_mm(st, x1, y0)
        g = self.snap or 1
        # (a coarse grid now and then: the points line up)
        if self.rnd.random() < 0.5:
            g *= 5
        gx = self.rnd.randint(math.ceil(a[0] / g), max(math.ceil(a[0] / g), math.floor(b[0] / g)))
        gy = self.rnd.randint(math.ceil(a[1] / g), max(math.ceil(a[1] / g), math.floor(b[1] / g)))
        return MM(gx * g, gy * g)

    def free(self, st):
        x0, y0, x1, y1 = self.safe(st)
        return MM(*to_mm(st, self.rnd.uniform(x0, x1), self.rnd.uniform(y0, y1)))

    def target(self, st=None, weights=None):
        """A position: on a point, beside one (just out of its snap), on a
        segment, inside an area, on the grid, or anywhere."""
        st = st or self.st()
        w = {"point": 4, "end": 3, "near": 2, "seg": 2, "inside": 1, "grid": 4, "free": 1}
        w.update(weights or {})
        for _ in range(6):
            k = self.rnd.choices(list(w), list(w.values()))[0]
            if k in ("point", "end", "near"):
                pts = self.points(st, "ends" if k == "end" else "any")
                if not pts:
                    continue
                q = self.rnd.choice(pts)[2]
                if k == "near":
                    a, r = self.rnd.uniform(0, 2 * math.pi), self.rnd.uniform(9, 18) / st["zoom"]
                    return MM(q[0] + r * math.cos(a), q[1] + r * math.sin(a))
                if self.rnd.random() < 0.3:  # a few pixels off: still on it
                    r = self.rnd.uniform(0, 5) / st["zoom"]
                    return MM(q[0] + r, q[1] - r)
                return MM(*q)
            if k == "seg":
                segs = [(i, j) for i, p in enumerate(st["paths"]) for j in range(len(p["points"]) - (0 if p.get("closed") else 1))
                        if len(p["points"]) >= 2]
                if not segs:
                    continue
                i, j = self.rnd.choice(segs)
                m = on_segment(st["paths"][i], j, self.rnd.choice([0.5, 0.5, self.rnd.uniform(0.1, 0.9)]))
                if self.visible(st, *m):
                    return MM(*m)
            if k == "inside":
                cs = [p for p in st["paths"] if p.get("closed") and len(p["points"]) >= 3]
                if not cs:
                    continue
                ps = [q["p"] for q in self.rnd.choice(cs)["points"]]
                m = (sum(q[0] for q in ps) / len(ps), sum(q[1] for q in ps) / len(ps))
                if self.visible(st, *m):
                    return MM(*m)
            if k == "grid":
                return self.grid(st)
            if k == "free":
                return self.free(st)
        return self.grid(st)

    def controls(self, prefix=""):
        return [c["id"] for c in self.cad.controls() if c["id"].startswith(prefix)]

    # --- tools -------------------------------------------------------------------------

    def ensure(self, tool):
        st = self.st()
        if st["tool"] == tool:
            return
        f = self.rnd.random()
        keys = {"line": "1", "bezier": "2", "smooth": "3", "refine": "4", "select": "v"}
        ctl = {"line": "sk-line", "bezier": "sk-bezier", "smooth": "sk-smooth", "refine": "sk-refine", "select": "sk-select",
               "shape": "sk-back"}
        if tool == "shape":
            if f < 0.4 and "sk-back" in self.controls("sk-back"):
                self.do("click_control", "sk-back")
            elif f < 0.7:
                for _ in range(3):
                    if self.st()["tool"] == "shape":
                        break
                    self.key("esc")
            else:
                self.do("sketch_tool", "shape")
        elif f < 0.5:
            self.key(keys[tool])
        elif f < 0.75 and ctl[tool] in self.controls(ctl[tool]):
            self.do("click_control", ctl[tool])
        else:
            self.do("sketch_tool", tool)

    def ensure_pen(self):
        st = self.st()
        if st["tool"] in PEN and self.rnd.random() < 0.8:
            return
        if self.rnd.random() < 0.3:
            self.key("p")  # the Pen, with the tool it had
        else:
            self.ensure(self.rnd.choice(["line", "line", "bezier", "smooth"]))

    def mods(self):
        return self.rnd.choice([[], [], [], [], ["shift"], ["alt"]])

    # --- actions -----------------------------------------------------------------------

    def a_pen_click(self):
        self.ensure_pen()
        st = self.st()
        t = self.target(st)
        if st["tool"] == "bezier" and self.rnd.random() < 0.5:
            r = self.rnd.uniform(15, 60) / st["zoom"]
            a = self.rnd.uniform(0, 2 * math.pi)
            self.do("drag", t, t.off(r * math.cos(a), r * math.sin(a)), steps=4, mods=self.mods())
        else:
            self.do("click", t, mods=self.mods())

    def a_pen_path(self):
        """A few clicks in a row, then ended one of the ways a path ends."""
        self.ensure_pen()
        for _ in range(self.rnd.randint(2, 5)):
            st = self.st()
            t = self.target(st, {"point": 2, "end": 2, "near": 2, "grid": 8, "inside": 0, "free": 1})
            if st["tool"] == "bezier" and self.rnd.random() < 0.3:
                self.do("drag", t, t.off(self.rnd.uniform(-5, 5), self.rnd.uniform(-5, 5)), steps=4)
            else:
                self.do("click", t, mods=self.mods())
        self.a_end_drawing()

    def a_end_drawing(self):
        st = self.st()
        d = st["drawing"]
        if d < 0:
            return
        pts = st["paths"][d]["points"]
        f = self.rnd.random()
        if f < 0.35:
            q = pts[0]["p"]  # its first point: closed
            r = self.rnd.choice([0, 0, 3]) / st["zoom"]
            self.do("click", MM(q[0] + r, q[1]))
        elif f < 0.5:
            self.do("click", MM(*pts[-1]["p"]))  # its last: ended
        elif f < 0.65:
            self.key("enter")
        elif f < 0.75:
            self.key("esc")
        elif f < 0.85 and "sk-finish-open" in self.controls("sk-finish-open"):
            self.do("click_control", "sk-finish-open")
        # else: left being drawn

    def a_tab_length(self):
        st = self.st()
        if st["drawing"] < 0:
            return
        self.do("mouse_move", self.target(st))
        self.key("tab")
        self.do("type_text", self.rnd.choice(["5", "7.5", "10", "100", "0", "-3", "abc", "1e9"]))
        self.key(self.rnd.choice(["enter", "enter", "esc"]))

    def a_select_click(self):
        self.ensure("select" if self.rnd.random() < 0.8 else "shape")
        self.do("click", self.target(), mods=self.rnd.choice([[], [], ["shift"]]))

    def a_drag_point(self):
        """A point dragged somewhere, often an end onto its own path's other
        end or onto another path's end."""
        self.ensure("select" if self.rnd.random() < 0.8 else "shape")
        st = self.st()
        ends = self.points(st, "ends")
        pts = ends if ends and self.rnd.random() < 0.6 else self.points(st)
        if not pts:
            return self.a_pen_path()
        i, j, q = self.rnd.choice(pts)
        p = st["paths"][i]["points"]
        f = self.rnd.random()
        dest = None
        if f < 0.35 and len(p) >= 2:
            # onto its own path's other end (or, from the middle, either end)
            k = 0 if j == len(p) - 1 else len(p) - 1 if j == 0 else self.rnd.choice([0, len(p) - 1])
            dest = MM(*p[k]["p"])
        elif f < 0.55:
            others = [e for e in ends if e[0] != i]
            if others:
                dest = MM(*self.rnd.choice(others)[2])
        elif f < 0.65 and len(p) >= 2:
            dest = MM(*p[(j + self.rnd.choice([1, -1])) % len(p)]["p"])  # onto its neighbor
        if dest is None:
            dest = self.target(st, {"inside": 0})
        self.do("drag", MM(*q), dest, steps=self.rnd.randint(3, 8), mods=self.rnd.choice([[], [], [], ["alt"]]))

    def a_drag_segment(self):
        self.ensure(self.rnd.choice(["select", "shape"]))
        st = self.st()
        t = self.target(st, {"seg": 6, "inside": 3, "point": 0, "end": 0, "near": 0, "grid": 0, "free": 0})
        self.do("drag", t, self.target(st, {"inside": 0}), steps=5)

    def a_box(self):
        self.ensure(self.rnd.choice(["select", "select", "shape"]))
        st = self.st()
        a, b = self.free(st), self.free(st)
        self.do("drag", a, b, steps=4, mods=self.rnd.choice([[], [], ["shift"]]))

    def a_nudge(self):
        if self.rnd.random() < 0.3:
            st = self.st()
            ends = self.points(st, "ends")
            if ends:
                self.ensure("select")
                self.do("click", MM(*self.rnd.choice(ends)[2]))
        for _ in range(self.rnd.choice([1, 1, 2, 5])):
            self.key(self.rnd.choice(["left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down"]))

    def a_end_onto_start_typed(self):
        """An open path's last point typed onto its first: X, then Y."""
        st = self.st()
        cands = [(i, p) for i, p in enumerate(st["paths"])
                 if not p.get("closed") and i != st["drawing"] and len(p["points"]) >= 3 and self.visible(st, *p["points"][-1]["p"])]
        if not cands:
            return self.a_pen_path()
        i, p = self.rnd.choice(cands)
        first, last = p["points"][0]["p"], p["points"][-1]["p"]
        self.ensure("select")
        self.do("click", MM(*last))
        for axis in (0, 1):
            if f"sk-pos-{axis}" in self.controls("sk-pos-"):
                self.do("click_control", f"sk-pos-{axis}")
                self.do("type_text", f"{first[axis]:g}")
                self.key("enter")

    def a_delete(self):
        f = self.rnd.random()
        if f < 0.4:
            self.key("delete")
        elif f < 0.7:
            self.key("backspace")
        elif "sk-delete" in self.controls("sk-delete"):
            self.do("click_control", "sk-delete")

    def a_refine(self):
        self.ensure("refine")
        st = self.st()
        f = self.rnd.random()
        if f < 0.45:
            t = self.target(st, {"seg": 8, "point": 0, "end": 0, "near": 0, "inside": 0, "grid": 0, "free": 1})
            self.do("click", t)
        elif f < 0.7:
            t = self.target(st, {"seg": 8, "point": 0, "end": 0, "near": 0, "inside": 0, "grid": 0, "free": 0})
            self.do("drag", t, self.target(st, {"inside": 0}), steps=4)
        else:
            self.do("click", self.target(st, {"point": 6, "end": 3, "near": 0, "seg": 0, "inside": 0, "grid": 0, "free": 0}))

    def a_toggle_closed(self):
        if self.rnd.random() < 0.3:
            # a single stretch picked first: the side Open leaves out
            self.ensure("select")
            st = self.st()
            self.do("click", self.target(st, {"seg": 8, "point": 0, "end": 0, "near": 0, "inside": 0, "grid": 0, "free": 0}))
        if self.rnd.random() < 0.5:
            self.key("o")
        elif "sk-close" in self.controls("sk-close"):
            self.do("click_control", "sk-close")

    def a_stroke(self):
        if "sk-stroke" not in self.controls("sk-stroke"):
            return self.a_select_click()
        if not any(c.startswith(("sk-as-line", "sk-join-")) for c in self.controls("sk-")):
            self.do("click_control", "sk-stroke")
        cs = [c for c in self.controls("sk-") if c.startswith(("sk-as-line", "sk-align-", "sk-join-", "sk-cap-", "sk-width"))]
        if not cs:
            return
        c = self.rnd.choice(cs)
        self.do("click_control", c)
        if c == "sk-width":
            self.do("type_text", self.rnd.choice(["0.4", "2", "5", "12", "0", "2000", "x"]))
            self.key("enter")
        if self.rnd.random() < 0.3:
            self.do("click_control", "sk-stroke")  # closed again

    def a_hole_mirror(self):
        """The hole switch, or a flip, turn or mirror copy of the selection."""
        if "sk-hole" not in self.controls("sk-hole"):
            return self.a_select_click()
        f = self.rnd.random()
        if f < 0.2:
            self.key("h")
        elif f < 0.35:
            self.do("click_control", "sk-hole")
        else:
            if not any(c.startswith("sk-flip-") for c in self.controls("sk-")):
                if self.rnd.random() < 0.5:
                    self.key("m")
                else:
                    self.do("click_control", "sk-xform")
            cs = [c for c in self.controls("sk-") if c.startswith(("sk-flip-", "sk-turn-", "sk-mirror-"))]
            if not cs:
                return
            c = self.rnd.choice(cs)
            self.do("click_control", c)
            if c == "sk-turn-by":
                self.do("type_text", self.rnd.choice(["15", "90", "-45", "7.5", "360", "1e9", "x"]))
                self.key(self.rnd.choice(["enter", "enter", "esc"]))

    def a_point_kind(self):
        cs = [c for c in self.controls("sk-") if c in ("sk-sharp", "sk-make-smooth", "sk-split")]
        if not cs:
            return self.a_select_click()
        self.do("click_control", self.rnd.choice(cs))

    def a_handle(self):
        """A selected curve point's handle dragged: somewhere, or back onto
        its point."""
        st = self.st()
        hs = []
        for i, j in st["selected"]:
            q = st["paths"][i]["points"][j]
            if q.get("kind", 0) == 2:
                continue
            for h in (q.get("in"), q.get("out")):
                if h and h != [0, 0]:
                    hp = (q["p"][0] + h[0], q["p"][1] + h[1])
                    if self.visible(st, *hp):
                        hs.append((q["p"], hp))
        if not hs or st["tool"] not in ("select",):
            return self.a_point_kind()
        p, hp = self.rnd.choice(hs)
        dest = MM(*p) if self.rnd.random() < 0.3 else self.target(st, {"inside": 0})
        self.do("drag", MM(*hp), dest, steps=4, mods=self.rnd.choice([[], [], ["alt"]]))

    def a_typed(self):
        cs = [c for c in self.controls("sk-") if c in ("sk-pos-0", "sk-pos-1", "sk-dim-w", "sk-dim-h", "sk-dim-len")]
        if not cs:
            return self.a_select_click()
        c = self.rnd.choice(cs)
        st = self.st()
        if c.startswith("sk-pos"):
            axis = int(c[-1])
            ends = self.points(st)
            vals = [f"{self.rnd.choice(ends)[2][axis]:g}"] * 3 if ends else []
            vals += [f"{self.rnd.randint(-30, 30)}", "2.5", "-0", "1e20", "abc"]
        else:
            vals = [f"{self.rnd.choice([1, 5, 10, 20, 37.5, 60])}", "0.5", "0", "-5", "1e12", "wide"]
        self.do("click_control", c)
        self.do("type_text", self.rnd.choice(vals))
        self.key(self.rnd.choice(["enter", "enter", "enter", "tab", "esc"]))

    def a_clipboard(self):
        f = self.rnd.random()
        if f < 0.15 and "sk-copy" in self.controls("sk-copy"):
            self.do("click_control", "sk-copy")
        elif f < 0.25 and "sk-paste" in self.controls("sk-paste"):
            self.do("click_control", "sk-paste")
        else:
            self.key(self.rnd.choice(["ctrl+c", "ctrl+v", "ctrl+v", "ctrl+x", "ctrl+d", "ctrl+d", "ctrl+a"]))
        if self.rnd.random() < 0.3:
            # the copy dragged away from the original
            self.a_drag_segment()

    def a_undo(self):
        f = self.rnd.random()
        if f < 0.2 and "sk-undo" in self.controls("sk-undo"):
            self.do("click_control", "sk-undo")
        elif f < 0.3 and "sk-redo" in self.controls("sk-redo"):
            self.do("click_control", "sk-redo")
        else:
            for _ in range(self.rnd.choice([1, 1, 2, 4])):
                self.key(self.rnd.choice(["ctrl+z", "ctrl+z", "ctrl+y", "ctrl+shift+z"]))

    def a_tool(self):
        f = self.rnd.random()
        if f < 0.6:
            self.key(self.rnd.choice(["p", "a", "v", "1", "2", "3", "4", "esc", "esc"]))
        else:
            cs = [c for c in self.controls("sk-") if c in ("sk-pen", "sk-edit", "sk-back", "sk-line", "sk-bezier", "sk-smooth",
                                                           "sk-select", "sk-refine")]
            if cs:
                self.do("click_control", self.rnd.choice(cs))

    def a_clear(self):
        if "sk-clear" in self.controls("sk-clear"):
            self.do("click_control", "sk-clear")
        else:
            self.a_pen_click()

    def a_view(self):
        st = self.st()
        z = st["zoom"]
        f = self.rnd.random()
        if f < 0.25:
            d = -1 if z > 40 else 1 if z < 6 else self.rnd.choice([1, -1])  # (kept between 6 and 40 px per mm)
            self.do("wheel", self.target(st), d)
        elif f < 0.35:
            self.key("+" if z < 40 else "-")
        elif f < 0.45:
            self.key("-" if z > 6 else "+")
        elif f < 0.55:
            self.key("f")
        elif f < 0.7:
            cs = [c for c in self.controls("sk-") if c in ("sk-home", "sk-fit", "sk-zoom-in", "sk-zoom-out")]
            if cs:
                c = self.rnd.choice(cs)
                if (c == "sk-zoom-in" and z > 40) or (c == "sk-zoom-out" and z < 6):
                    c = "sk-home"
                self.do("click_control", c)
        elif f < 0.85:
            self.do("sketch_view", self.rnd.choice([0, 5, -10]), self.rnd.choice([0, 5, 10, 15]), self.rnd.choice([8, 12, 20, 27.3]))
        else:
            a, b = self.free(st), self.free(st)
            self.do("drag", a, b, steps=4, button=self.rnd.choice(["middle", "right"]))

    def a_misc_key(self):
        self.key(self.rnd.choice(["d", "d", "enter", "esc", "o", "tab"]))

    def a_interrupt(self):
        """A drag with a key pressed part way (an undo, a delete, Esc)."""
        st = self.st()
        a = self.target(st, {"grid": 1, "free": 0})
        b = self.target(st, {"inside": 0})
        self.do("mouse_down", a)
        self.do("mouse_move", MM((a.x + b.x) / 2, (a.y + b.y) / 2))
        self.key(self.rnd.choice(["ctrl+z", "esc", "delete", "o", "ctrl+v", "enter"]))
        self.do("mouse_move", b)
        self.do("mouse_up", b)

    def a_done(self):
        """Finish part way (then opened again, to carry on)."""
        self.run(("done",), lambda v, hard: self.note(self.step, "done", v, hard))

    ACTIONS = [(a_pen_click, 10), (a_pen_path, 12), (a_end_drawing, 4), (a_tab_length, 2), (a_select_click, 8),
               (a_drag_point, 14), (a_drag_segment, 4), (a_box, 4), (a_nudge, 5), (a_end_onto_start_typed, 2),
               (a_delete, 4), (a_refine, 5), (a_toggle_closed, 5), (a_stroke, 4), (a_point_kind, 3), (a_handle, 3),
               (a_typed, 4), (a_clipboard, 5), (a_undo, 7), (a_tool, 5), (a_clear, 2), (a_view, 4), (a_misc_key, 2),
               (a_interrupt, 1), (a_done, 1), (a_hole_mirror, 3)]

    # --- the checks that are steps -----------------------------------------------------

    def undo_redo_check(self, check):
        st = self.st()
        if not st.get("can_undo"):
            return
        before = (norm_paths(st["paths"]), st["drawing"])
        self.cad.key("ctrl+z")
        mid = self.st()
        self.cad.key("ctrl+y")
        after = self.st()
        if check is not None:
            if (norm_paths(after["paths"]), after["drawing"]) != before:
                check(("undo-redo", f"undo then redo changed the drawing:\n  before {st['paths']} (drawing {st['drawing']})\n"
                                    f"  after  {after['paths']} (drawing {after['drawing']})"), True)
            if (norm_paths(mid["paths"]), mid["drawing"]) == before:
                check(("undo-no-change", "an undo step that changed nothing in the drawing"), False)

    def done_check(self, check):
        cad = self.cad
        st = self.st()
        want = finished(st)
        try:
            nid = cad.sketch_done()
        except ScottiCADError as e:
            if "internal error" in str(e):
                raise
            self.not_built += 1
            st2 = self.st()
            if check is not None:
                if not st2.get("open"):
                    check(("done", f"Finish refused ({e}) but closed the workspace"), True)
                if not str(e).strip():
                    check(("done", "Finish refused without a word why"), True)
            return
        self.built += 1
        if check is None:
            self.node = nid
            cad.edit_sketch(nid)
            return
        if self.st().get("open"):
            check(("done", "Finish returned but the workspace is still open"), True)
            return
        n = cad.node(nid)
        if not (math.isfinite(n["volume"]) and n["volume"] > 0):
            check(("done-volume", f"built a shape of volume {n['volume']} from {want}"), True)
        if norm_paths(n["sketch"]) != norm_paths(want):
            check(("done-saved", f"saved a different drawing:\n  drawn {want}\n  saved {n['sketch']}"), True)
        for part in cad.print_check(ids=[nid])["parts"]:
            bad = [f["kind"] for f in part["findings"] if f["kind"] in BAD_SURFACE]
            if bad:
                check(("done-surface", f"the shape's surface isn't closed ({bad}), from {want}"), True)
        self.node = nid
        cad.edit_sketch(nid)
        st3 = self.st()
        if norm_paths(st3["paths"]) != norm_paths(want):
            check(("reopen", f"opened again it shows a different drawing:\n  saved {want}\n  shown {st3['paths']}"), True)

    # --- the session -----------------------------------------------------------------------

    def drawing(self):
        """A drawing to start from: rectangles, triangles, curves, lines
        (one with its end near its start), on the grid."""
        spins = self.kind in ("revolve", "coil")
        out = []
        for _ in range(self.rnd.randint(1, 3)):
            x = self.rnd.randint(-30, -6) if spins else self.rnd.randint(-30, 15)
            y = self.rnd.randint(0, 15)
            w, h = self.rnd.randint(3, 12), self.rnd.randint(3, 12)
            if spins:
                w = min(w, -x)
            f = self.rnd.random()
            if f < 0.3:
                path = {"points": [[x, y], [x + w, y], [x + w, y + h], [x, y + h]], "closed": True}
            elif f < 0.45:
                path = {"points": [[x, y], [x + w, y], [x, y + h]], "closed": True}
            elif f < 0.6:
                path = {"points": [{"p": [x, y], "kind": "auto"}, {"p": [x + w, y + h / 2], "kind": "auto"},
                                   {"p": [x, y + h], "kind": "auto"}], "closed": True}
            elif f < 0.8:
                # nearly round: its end a step from its start
                path = {"points": [[x, y], [x + w, y], [x + w, y + h], [x, y + h], [x, y + 1]]}
            else:
                path = {"points": [[x, y], [x + w, y + h]]}
            if self.rnd.random() < 0.25:
                path.update(stroke=self.rnd.choice([1, 2, 3]), join=self.rnd.choice(["sharp", "round", "bevel"]))
            out.append(path)
        return out

    def open_session(self):
        self.ops = self.setup
        cad = self.cad
        if cad.sketch_state().get("open"):
            cad.sketch_cancel()
        self.do("new_design")
        self.snap = self.rnd.choice([1, 1, 1, 0.5, 2, 0])
        self.do("set_snap", self.snap)
        self.kind = self.rnd.choice(["sketch", "sketch", "revolve", "coil"])
        if self.rnd.random() < 0.5:
            self.node = self.do("add_sketch", self.drawing(), kind=self.kind)
            self.do("edit_sketch", self.node)
        else:
            self.do("edit_sketch", kind=self.kind)
        self.do("sketch_view", 0, 10 if self.kind != "sketch" else 0, self.rnd.choice([10, 14, 20, 27.3]))
        self.ops = None

    def show(self, op):
        if op[0] == "done":
            return "cad.sketch_done()  # then checked, and opened again with cad.edit_sketch(id)"
        if op[0] == "undoredo":
            return "cad.key('ctrl+z'); cad.key('ctrl+y')  # the drawing should be as it was"
        _, name, args, kw = op
        parts = [repr(a) for a in args] + [f"{k}={v!r}" for k, v in kw.items() if v not in ([], ())]
        if name == "add_sketch":
            return f"node = cad.add_sketch({', '.join(parts)})"
        if name == "edit_sketch" and args:
            return "cad.edit_sketch(node)"
        return f"cad.{name}({', '.join(parts)})"

    def history(self, last=15):
        lines = [self.show(op) for op in self.setup]
        acts = self.actions[-last:]
        if len(self.actions) > last:
            lines.append(f"# ... {len(self.actions) - last} steps left out ...")
        for step, name, ops in acts:
            lines.append(f"# step {step}: {name}")
            lines += [self.show(op) for op in ops]
        return "\n".join(lines)

    def check(self, step, name):
        st = self.st()
        hard, soft = problems(st, self.kind)
        for v in hard:
            self.note(step, name, v, True)
        for v in soft:
            self.note(step, name, v, False)

    def note(self, step, name, v, hard):
        inv, msg = v
        if inv in IGNORE:
            return
        if hard or STRICT:
            if inv not in self.seen:
                self.seen.add(inv)
                self.hard.append((step, inv, msg, name, self.history()))
        else:
            s = self.soft.setdefault(inv, [0, step, msg, name])
            s[0] += 1

    def walk(self, steps):
        self.open_session()
        for step in range(steps):
            self.step = step
            fn = self.rnd.choices([a for a, _ in self.ACTIONS], [w for _, w in self.ACTIONS])[0]
            ops = []
            self.ops = ops
            self.actions.append((step, fn.__name__[2:], ops))
            if not self.st().get("open"):  # (a Finish that worked: carry on with the shape)
                self.run(("call", "edit_sketch", (self.node,), {}) if self.node else ("call", "edit_sketch", (), {"kind": self.kind}))
            fn(self)
            # (a box left open for typing takes the keys: Esc closes it)
            if self.cad.ui_state().get("field") is not None:
                self.key("esc")
            if step % 4 == 3 or self.rnd.random() < 0.1:
                self.run(("undoredo",), lambda v, hard: self.note(self.step, "undo-redo check", v, hard))
            self.ops = None
            self.check(step, fn.__name__[2:])
        # at the end: Finish, with its checks
        self.ops = []
        self.actions.append((steps, "finish", self.ops))
        self.run(("done",), lambda v, hard: self.note(steps, "finish", v, hard))
        self.ops = None
        if self.cad.sketch_state().get("open"):
            self.cad.sketch_cancel()


# --- replaying and shrinking a failure -------------------------------------------------


def replay(setup, actions, kind, inv):
    """Runs the setup and the actions' ops in a fresh program; True if
    invariant inv breaks on the way."""
    with program() as cad:
        return _replay(cad, setup, actions, kind, inv)


def _replay(cad, setup, actions, kind, inv):
    w = Walk(cad, 0)
    w.kind = kind
    hit = []

    def check(v, hard):
        if v[0] == inv:
            hit.append(v)
    try:
        for op in setup:
            if op[1] == "edit_sketch" and op[2]:
                op = ("call", "edit_sketch", (w.node,), {})
            r = w.run(op)
            if op[1] == "add_sketch":
                w.node = r
        for _, _, ops in actions:
            for op in ops:
                # (the shape's id is the one made in this run)
                if op[0] == "call" and op[1] == "edit_sketch" and op[2]:
                    op = ("call", "edit_sketch", (w.node,), {})
                w.run(op, check)
            st = cad.sketch_state()
            if not st.get("open"):
                return bool(hit)
            hard, soft = problems(st, kind)
            for v in hard + soft:
                check(v, True)
            if hit:
                return True
    except ScottiCADError:
        return False
    return bool(hit)


def shrink(walk, step, inv):
    """The fewest of the steps up to step that still break inv (ddmin)."""
    acts = [a for a in walk.actions if a[0] <= step]
    n = 2
    while len(acts) >= 2:
        size = max(1, len(acts) // n)
        chunks = [acts[i:i + size] for i in range(0, len(acts), size)]
        cut = False
        for k in range(len(chunks)):
            rest = [a for j, c in enumerate(chunks) if j != k for a in c]
            if rest and replay(walk.setup, rest, walk.kind, inv):
                acts, n, cut = rest, max(n - 1, 2), True
                break
        if not cut:
            if n >= len(acts):
                break
            n = min(len(acts), n * 2)
    w = Walk(None, 0)
    w.setup, w.actions = walk.setup, acts
    return w.history(last=len(acts))


@pytest.mark.parametrize("seed", SEEDS)
def test_random_sketch_session(seed):
    start = time.time()
    with program() as cad:
        w = Walk(cad, seed)
        w.walk(STEPS)
    soft = ", ".join(f"{k} ×{v[0]} (first at step {v[1]}, {v[3]}: {v[2]})" for k, v in sorted(w.soft.items()))
    print(f"\nseed {seed} ({w.kind}): {STEPS} steps, {sum(len(a[2]) for a in w.actions)} calls, {w.refused} refused, "
          f"Finish built {w.built}, refused {w.not_built}, "
          f"{time.time() - start:.0f} s" + (f"; allowed oddities: {soft}" if soft else ""))
    if w.hard:
        out = []
        for step, inv, msg, name, hist in w.hard:
            out.append(f"seed {seed} ({w.kind}) step {step} ({name}): {inv}: {msg}\n"
                       f"rerun: SCOTTICAD_SKETCH_SEED={seed} SCOTTICAD_SKETCH_STEPS={step + 1}\n"
                       f"the calls (at(cad, x, y) is drawing mm to window px, from the view as it is):\n{hist}")
        if SHRINK:
            step, inv = w.hard[0][0], w.hard[0][1]
            out.append(f"shrunk ({inv}):\n" + shrink(w, step, inv))
        raise AssertionError("\n\n".join(out))
