"""Torture test: long random sessions against the real program, mixing API
calls, GUI drags and clicks, keys, tools, scripts and save/open round
trips, with the program's invariants checked after every step. Seeded, so
a failure reproduces: rerun with SCOTTICAD_TORTURE_SEED=<seed> (and
SCOTTICAD_TORTURE_STEPS to lengthen it)."""
import json
import math
import os
import random
import sys
import time

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import ScottiCADError, launch  # noqa: E402

SEEDS = [int(s) for s in os.environ.get("SCOTTICAD_TORTURE_SEED", "1,2,3").split(",")]
STEPS = int(os.environ.get("SCOTTICAD_TORTURE_STEPS", "100"))
# the window: SCOTTICAD_TORTURE_SCALE=1.5 (or 2) for another interface
# scale; SCOTTICAD_TORTURE_WINDOW=smallest for the smallest window allowed
SCALE = float(os.environ.get("SCOTTICAD_TORTURE_SCALE", "0")) or None
SMALLEST = os.environ.get("SCOTTICAD_TORTURE_WINDOW") == "smallest"


def _window():
    if not SMALLEST:
        return 1400, 860
    with launch(headless=True) as c:
        return tuple(c.ui_state()["min_window"])

# controls a random click must leave alone: they end the session or wait
# on something no test can answer
SKIP_CONTROLS = {"dm-exit", "quit", "logo"}
KEYS = ["h", "s", "d", "f", "l", "m", "c", "v", "r", "n", "w", "k", "shift+k", "shift+d", "esc", "esc", "delete",
        "left", "right", "up", "down", "shift+left", "ctrl+up", "ctrl+down", "ctrl+z", "ctrl+z", "ctrl+y", "ctrl+d",
        "ctrl+a", "ctrl+g", "ctrl+b", "ctrl+shift+g", "ctrl+h", "ctrl+shift+h", "ctrl+l", "ctrl+t", "ctrl+c",
        "ctrl+v", "ctrl+shift+v", "ctrl+x", "f1", "+", "-", "enter", "tab"]
SCRIPTS = [
    "b = cad.add_shape('box', x=%d)\ncad.set_size(x=15, ids=[b])",
    "for i in range(3):\n    cad.add_shape('cylinder', x=i * 25, y=%d)",
    "cad.select_all()\nif cad.selection()['ids']:\n    cad.align('x', 'center')",
    "print(len(cad.nodes()), %d)",
    "cad.undo()\ncad.redo()",
    "x = cad.state()['node_count']\nif x > 1:\n    cad.group('union', ids=[n['id'] for n in cad.nodes()][:2])",
    "fail('deliberate %d')",
]


class Session:
    def __init__(self, cad, seed, tmp_path):
        self.cad, self.rnd, self.tmp = cad, random.Random(seed), tmp_path
        self.seed = seed
        self.kinds = [k for k in cad.shape_kinds()]
        self.errors = 0
        self.ops = 0

    # --- helpers --------------------------------------------------------------
    def ids(self):
        return [n["id"] for n in self.cad.nodes()]

    def some_ids(self):
        ids = self.ids()
        if not ids:
            return []
        k = self.rnd.randint(1, min(3, len(ids)))
        return self.rnd.sample(ids, k)

    def point(self):
        vp = self.cad.ui_state()["viewport"]
        return vp[0] + self.rnd.random() * vp[2], vp[1] + self.rnd.random() * vp[3]

    def screen_of(self, id):
        b = self.cad.node(id).get("bounds")
        if not b:
            return self.point()
        return self.cad.project(*b["center"])

    def call(self, f, *a, **kw):
        """Runs an operation; most errors are fine (nothing selected, a
        locked shape); an internal error is a bug."""
        self.ops += 1
        try:
            return f(*a, **kw)
        except ScottiCADError as e:
            if "internal error" in str(e):
                raise
            self.errors += 1
            return None

    # --- operations --------------------------------------------------------------
    def op_add(self):
        k = self.rnd.choice(self.kinds)
        self.call(self.cad.add_shape, k["id"], x=self.rnd.uniform(-80, 80), y=self.rnd.uniform(-80, 80),
                  hole=self.rnd.random() < 0.2)

    def op_select(self):
        r = self.rnd.random()
        if r < 0.2:
            self.call(self.cad.clear_selection)
        elif r < 0.3:
            self.call(self.cad.select_all)
        else:
            self.call(self.cad.select, self.some_ids())

    def op_transform(self):
        ids = self.some_ids()
        f = self.rnd.choice(["move", "rotate", "size", "pos", "mirror", "align", "drop", "nudge", "bounds"])
        if f == "move":
            self.call(self.cad.move, dx=self.rnd.uniform(-30, 30), dy=self.rnd.uniform(-30, 30), dz=self.rnd.uniform(-5, 20), ids=ids)
        elif f == "rotate":
            self.call(self.cad.rotate, self.rnd.choice("xyz"), self.rnd.choice([15, 30, 45, 90, -37.5, 180]), ids=ids)
        elif f == "size":
            self.call(self.cad.set_size, x=self.rnd.uniform(0.5, 120), y=self.rnd.uniform(0.5, 120),
                      z=self.rnd.uniform(0.5, 60), uniform=self.rnd.random() < 0.2, ids=ids)
        elif f == "pos":
            self.call(self.cad.set_position, x=self.rnd.uniform(-90, 90), z=self.rnd.uniform(0, 30),
                      anchor=self.rnd.choice(["min", "center"]), ids=ids)
        elif f == "mirror":
            self.call(self.cad.mirror, self.rnd.choice("xyz"), ids=ids)
        elif f == "align":
            self.call(self.cad.align, self.rnd.choice("xyz"), self.rnd.choice(["min", "center", "max"]), ids=ids)
        elif f == "drop":
            self.call(self.cad.drop, ids=ids)
        elif f == "nudge":
            self.call(self.cad.nudge, self.rnd.choice("xyz"), self.rnd.choice([1, -1, 10]))
        else:
            lo = [self.rnd.uniform(-50, 50) for _ in range(3)]
            self.call(self.cad.set_bounds, lo, [v + self.rnd.uniform(0.5, 60) for v in lo], ids=ids)

    def op_group(self):
        f = self.rnd.random()
        if f < 0.5:
            self.call(self.cad.group, self.rnd.choice(["union", "bundle", "intersect"]), ids=self.some_ids())
        elif f < 0.8:
            self.call(self.cad.ungroup, ids=self.some_ids())
        else:
            self.call(self.cad.split, self.rnd.choice("xyz"), keep=self.rnd.choice(["both", "above", "below"]),
                      gap=self.rnd.choice([0, 0, 5]), ids=self.some_ids())

    def op_appearance(self):
        ids = self.some_ids()
        f = self.rnd.choice(["hole", "solid", "color", "transp", "lock", "unlock", "name", "hide", "show", "param", "text"])
        if f == "hole":
            self.call(self.cad.set_hole, True, ids=ids)
        elif f == "solid":
            self.call(self.cad.set_hole, False, ids=ids)
        elif f == "color":
            self.call(self.cad.set_color, "#%06x" % self.rnd.randrange(1 << 24), ids=ids)
        elif f == "transp":
            self.call(self.cad.set_transparent, self.rnd.random() < 0.5, ids=ids)
        elif f == "lock":
            self.call(self.cad.set_locked, True, ids=ids)
        elif f == "unlock":
            self.call(self.cad.set_locked, False, ids=ids)
        elif f == "name":
            self.call(self.cad.set_name, self.rnd.choice(["Base", "", "Lid ✓", "x" * 40]), ids=ids)
        elif f == "hide":
            if ids:
                self.call(self.cad.toggle_hidden, ids[0])
        elif f == "show":
            self.call(self.cad.show_all)
        elif f == "param" and ids:
            n = self.cad.node(ids[0])
            kind = next((k for k in self.kinds if k["id"] == n["kind"]), None)
            if kind and kind["params"]:
                p = self.rnd.choice(kind["params"])
                if p["kind"] == "slider":
                    v = self.rnd.uniform(p["min"] - 1, p["max"] + 1)  # a little outside too
                    self.call(self.cad.set_param, ids[0], p["key"], v)
        elif f == "text" and ids:
            self.call(self.cad.set_text, ids[0], self.rnd.choice(["Hi", "ScottiCAD", "日本", "", "ff"]))

    def op_notes(self):
        ids = self.ids()
        f = self.rnd.random()
        notes = self.cad.notes()
        if f < 0.5 or not notes:
            target = self.rnd.choice(ids) if ids and self.rnd.random() < 0.8 else 0
            self.call(self.cad.add_note, target, self.rnd.uniform(-20, 20), self.rnd.uniform(-20, 20),
                      self.rnd.uniform(0, 20), self.rnd.choice(["", "a note", "line one\nline two"]))
        elif f < 0.8:
            n = self.rnd.choice(notes)
            self.call(self.cad.set_note_text, n["id"], n["index"], "changed")
        else:
            n = self.rnd.choice(notes)
            self.call(self.cad.delete_note, n["id"], n["index"])

    def op_history(self):
        f = self.rnd.random()
        if f < 0.4:
            self.call(self.cad.undo)
        elif f < 0.6:
            self.call(self.cad.redo)
        elif f < 0.75:
            self.call(self.cad.duplicate)
        elif f < 0.9:
            self.call(self.cad.copy)
            self.call(self.cad.paste, in_place=self.rnd.random() < 0.5)
        else:
            self.call(self.cad.cut)
            self.call(self.cad.paste)

    def op_gui_drag(self):
        ids = self.ids()
        mods = self.rnd.choice([(), (), ("shift",), ("alt",)])
        f = self.rnd.random()
        if f < 0.4 and ids:
            # drag a shape (by its middle) to a random point in the view
            id = self.rnd.choice(ids)
            x0, y0 = self.screen_of(id)
            x1, y1 = self.point()
            self.call(self.cad.drag, x0, y0, x1, y1, steps=6, mods=mods)
        elif f < 0.7:
            hs = self.call(self.cad.handles) or []
            if hs:
                h = self.rnd.choice(hs)
                x1, y1 = h["x"] + self.rnd.uniform(-80, 80), h["y"] + self.rnd.uniform(-80, 80)
                self.call(self.cad.drag, h["x"], h["y"], x1, y1, steps=6, mods=mods)
        elif f < 0.85:
            x0, y0 = self.point()
            x1, y1 = self.point()
            self.call(self.cad.drag, x0, y0, x1, y1, steps=4, mods=mods)  # a marquee, or a shape
        else:
            x0, y0 = self.point()
            x1, y1 = self.point()
            self.call(self.cad.drag, x0, y0, x1, y1, steps=4, button=self.rnd.choice(["right", "middle"]))  # orbit, pan

    def op_gui_click(self):
        f = self.rnd.random()
        if f < 0.5:
            cs = [c for c in self.call(self.cad.controls) or [] if c["id"] not in SKIP_CONTROLS]
            if cs:
                self.call(self.cad.click_control, self.rnd.choice(cs)["id"])
        elif f < 0.8:
            x, y = self.point()
            self.call(self.cad.click, x, y, mods=self.rnd.choice([(), ("shift",)]))
        else:
            x, y = self.point()
            self.call(self.cad.wheel, x, y, self.rnd.choice([-3, -1, 1, 3]))

    def op_key(self):
        self.call(self.cad.key, self.rnd.choice(KEYS))
        if self.rnd.random() < 0.3 and "field" in self.cad.ui_state():
            self.call(self.cad.type_text, self.rnd.choice(["12", "-5", "1/2", "abc", "0", "1e3"]))
            self.call(self.cad.key, self.rnd.choice(["enter", "tab", "esc"]))

    def op_tool(self):
        f = self.rnd.random()
        if f < 0.5:
            self.call(self.cad.set_tool, self.rnd.choice(["none", "align", "mirror", "measure", "visibility", "ruler",
                                                          "notes", "construction_plane", "split", "layflat", "cruise"]))
            x, y = self.point()
            self.call(self.cad.click, x, y)
        elif f < 0.7:
            self.call(self.cad.set_section, self.rnd.random() < 0.7, axis=self.rnd.choice("xyz"),
                      keep=self.rnd.choice(["above", "below"]), pin=self.rnd.random() < 0.3)
        elif f < 0.85:
            self.call(self.cad.lay_flat, [self.rnd.uniform(-1, 1), self.rnd.uniform(-1, 1), self.rnd.uniform(-1, 1)], ids=self.some_ids())
        else:
            ids = self.ids()
            self.call(self.cad.attach_construction_plane, self.rnd.choice(ids) if ids and self.rnd.random() < 0.7 else 0)

    def op_settings(self):
        f = self.rnd.random()
        if f < 0.3:
            self.call(self.cad.set_units, self.rnd.choice(["mm", "in"]))
        elif f < 0.5:
            self.call(self.cad.set_snap, self.rnd.choice([0, 0.5, 1, 2.5]))
        elif f < 0.7:
            self.call(self.cad.set_camera, yaw=self.rnd.uniform(-180, 180), pitch=self.rnd.uniform(-89, 89),
                      dist=self.rnd.choice([50, 200, 1000]), ortho=self.rnd.random() < 0.3)
        elif f < 0.85:
            self.call(self.rnd.choice([self.cad.home_view, self.cad.fit_view]))
        else:
            self.call(self.cad.filament_estimate)

    def op_script(self):
        src = self.rnd.choice(SCRIPTS)
        if "%d" in src:
            src = src % self.rnd.randint(-50, 50)
        self.call(self.cad.run_script, src)

    def op_file(self):
        path = str(self.tmp / f"torture-{self.seed}.scotticad")
        before = self.cad.get_document()
        self.call(self.cad.save, path)
        self.call(self.cad.new_design)
        self.call(self.cad.open, path)
        after = self.cad.get_document()
        assert (after["nodes"] or []) == (before["nodes"] or []), "the design changed through save and open"
        assert after.get("notes") == before.get("notes"), "the design's notes changed through save and open"

    def op_export(self):
        """Export in a random format and import the file back."""
        ext = self.rnd.choice(["stl", "obj", "glb", "3mf", "svg"])
        path = str(self.tmp / f"torture-{self.seed}.{ext}")
        if os.path.exists(path):
            os.remove(path)  # an export that fails mustn't leave an old file to import
        self.call(self.cad.export, path, selection_only=self.rnd.random() < 0.3)
        if ext in ("stl", "obj", "svg") and os.path.exists(path):
            self.call(self.cad.import_file, path, x=self.rnd.uniform(-40, 40), y=self.rnd.uniform(-40, 40))

    def op_interleave(self):
        """An API call arriving in the middle of a mouse drag (a script or
        another client acting while the user drags), then the drag ends in
        one of the ways it can."""
        ids = self.ids()
        f = self.rnd.random()
        mods = self.rnd.choice([(), ("alt",), ("shift",)])
        if f < 0.5 and ids:
            x0, y0 = self.screen_of(self.rnd.choice(ids))
        elif f < 0.8:
            hs = self.call(self.cad.handles) or []
            if not hs:
                return
            h = self.rnd.choice(hs)
            x0, y0 = h["x"], h["y"]
        else:
            cs = [c for c in self.call(self.cad.controls) or [] if c["id"].startswith(("slider-", "param-"))]
            if not cs:
                return
            r = self.rnd.choice(cs)["rect"]
            x0, y0 = r[0] + r[2] * self.rnd.random(), r[1] + r[3] / 2
        x1, y1 = x0 + self.rnd.uniform(-60, 60), y0 + self.rnd.uniform(-60, 60)
        self.call(self.cad.mouse_down, x0, y0, mods=mods)
        self.call(self.cad.mouse_move, (x0 + x1) / 2, (y0 + y1) / 2, mods=mods)
        g = self.rnd.random()
        if g < 0.3:
            self.call(self.cad.undo)
        elif g < 0.4:
            self.call(self.cad.redo)
        elif g < 0.55:
            some = self.some_ids()
            if some:
                self.call(self.cad.delete, ids=some)
        elif g < 0.65:
            self.call(self.cad.new_design)
        elif g < 0.8:
            self.call(self.cad.run_script, self.rnd.choice(SCRIPTS).replace("%d", "7"))
        elif g < 0.9:
            self.op_file()
        else:
            self.call(self.cad.add_shape, self.rnd.choice(["box", "sphere"]))
        self.call(self.cad.mouse_move, x1, y1, mods=mods)
        e = self.rnd.random()
        if e < 0.6:
            self.call(self.cad.mouse_up, x1, y1, mods=mods)
        elif e < 0.8:
            self.call(self.cad.key, "esc")
            self.call(self.cad.mouse_up, x1, y1, mods=mods)
        else:
            self.call(self.cad.lose_capture)

    OPS = [(op_add, 14), (op_select, 10), (op_transform, 14), (op_group, 8), (op_appearance, 10), (op_notes, 4),
           (op_history, 10), (op_gui_drag, 10), (op_gui_click, 8), (op_key, 10), (op_tool, 8), (op_settings, 5),
           (op_script, 4), (op_file, 2), (op_export, 2), (op_interleave, 5)]

    def step(self):
        total = sum(w for _, w in self.OPS)
        r = self.rnd.uniform(0, total)
        for op, w in self.OPS:
            r -= w
            if r <= 0:
                op(self)
                return op.__name__
        return "none"

    # --- invariants --------------------------------------------------------------
    def check(self):
        cad = self.cad
        st = cad.state()
        nodes = cad.nodes() or []
        top = [n["id"] for n in nodes]
        assert st["node_count"] == len(top)
        assert set(st["selection"] or []) <= set(top), (st["selection"], top)
        assert st["undo_depth"] <= 500
        doc = cad.get_document()  # no NaN: Go can't marshal it
        seen = set()

        def walk(n):
            assert n["id"] not in seen, f"id {n['id']} twice"
            seen.add(n["id"])
            for v in n["m"]:
                assert math.isfinite(v)
            for c in n.get("children") or []:
                walk(c)
        for n in doc["nodes"] or []:
            walk(n)
        for n in nodes:
            b = n.get("bounds")
            if b:
                assert all(math.isfinite(v) for v in b["min"] + b["max"]), n
            assert math.isfinite(n["volume"]), n
            # a part under 0.1 mm thick (drags can shrink one to 0.1 mm, and
            # splits and cuts thin it further) has detail below the
            # booleans' tolerance: its result can come out open, with no
            # meaningful volume sign. Anything printable (walls of 0.4 mm
            # and up) is still checked.
            tiny = b and min(b["size"]) < 0.1
            assert n["volume"] >= -1e-6 or tiny, n
        c = cad.camera()
        assert all(math.isfinite(v) for v in c["target"]) and math.isfinite(c["dist"]) and 0 < c["dist"] < 1e6
        for ctl in cad.controls():
            assert all(math.isfinite(v) for v in ctl["rect"]), ctl
        json.dumps(cad.ui_state())


@pytest.mark.parametrize("seed", SEEDS)
def test_random_session(seed, tmp_path):
    w, h = _window()
    with launch(headless=True, width=w, height=h, scale=SCALE) as cad:
        s = Session(cad, seed, tmp_path)
        start = time.time()
        log = []
        for i in range(STEPS):
            name = s.step()
            log.append(name)
            try:
                s.check()
            except (AssertionError, ScottiCADError) as e:
                raise AssertionError(f"seed {seed} step {i} ({name}): {e}\nlast ops: {log[-8:]}") from e
        # everything undoes, and redoes
        s.cad.set_tool("none")
        if s.cad.ui_state()["popup"]:
            s.cad.key("esc")
        count = s.cad.state()["node_count"]
        undone = 0
        while s.cad.undo():
            undone += 1
            assert undone < 600
        assert s.cad.state()["node_count"] == 0 or not s.cad.state()["can_undo"]
        for _ in range(undone):
            assert s.cad.redo()
        assert s.cad.state()["node_count"] == count, "redo didn't bring the design back"
        s.check()
        cad.screenshot(str(tmp_path / f"torture-{seed}.png"))
        print(f"seed {seed}: {STEPS} steps, {s.ops} operations, {s.errors} refused, {time.time() - start:.1f} s")
