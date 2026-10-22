"""Python client for the ScottiCAD automation API.

ScottiCAD serves JSON-RPC 2.0 over a localhost TCP socket (one JSON object
per line) when started with ``--api HOST:PORT``. Everything the GUI can do
is available, including simulated mouse/keyboard input and clicking named
on-screen controls.

    from scotticad import launch
    with launch(headless=True) as cad:
        box = cad.add_shape("box", x=0, y=0)
        hole = cad.add_shape("cylinder", hole=True)
        cad.select([box, hole])
        cad.align("x", "center"); cad.align("y", "center")
        g = cad.group("union")
        cad.export("part.stl")

All lengths are millimetres; angles are degrees. Methods that act on "the
selection" also accept ``ids=[...]``. "The current plane" is the
construction plane, if one is placed, otherwise the build plate.
"""

from __future__ import annotations

import json
import os
import socket
import subprocess
import threading
from typing import Any, Iterable, Optional, Sequence

__all__ = ["Client", "ScottiCADError", "launch", "default_exe"]


class ScottiCADError(RuntimeError):
    """The server rejected a call."""


def default_exe() -> str:
    """Path of scotticad.exe: $SCOTTICAD_EXE or the repository root."""
    env = os.environ.get("SCOTTICAD_EXE")
    if env:
        return env
    here = os.path.dirname(os.path.abspath(__file__))
    return os.path.normpath(os.path.join(here, "..", "..", "scotticad.exe"))


class Client:
    """A connection to a running ScottiCAD."""

    def __init__(self, host: str = "127.0.0.1", port: int = 47800, timeout: float = 600.0):
        self._sock = socket.create_connection((host, port), timeout=timeout)
        self._file = self._sock.makefile("rwb")
        self._next = 1
        self._lock = threading.Lock()
        self.process: Optional[subprocess.Popen] = None

    # -- transport ---------------------------------------------------------

    def call(self, method: str, **params: Any) -> Any:
        """Invoke any API method by name (see :meth:`describe`)."""
        params = {k: v for k, v in params.items() if v is not None}
        with self._lock:
            rid = self._next
            self._next += 1
            msg = json.dumps({"jsonrpc": "2.0", "id": rid, "method": method, "params": params})
            try:
                self._file.write(msg.encode("utf-8") + b"\n")
                self._file.flush()
                line = self._file.readline()
            except socket.timeout:
                # the reply may still come, out of step with the next call:
                # this connection can't be used again
                self.close()
                raise TimeoutError(f"{method}: no reply from ScottiCAD in time; the connection is closed") from None
        if not line:
            raise ConnectionError("ScottiCAD closed the connection")
        resp = json.loads(line)
        if resp.get("id") is None and resp.get("error"):
            # the server couldn't read the request (and hangs up)
            self.close()
            raise ScottiCADError(f"{method}: {resp['error']['message']}")
        if resp.get("id") != rid:
            self.close()
            raise ConnectionError(f"{method}: a reply to another request ({resp.get('id')}) came back")
        if resp.get("error"):
            raise ScottiCADError(f"{method}: {resp['error']['message']}")
        return resp.get("result")

    def close(self) -> None:
        try:
            self._file.close()
            self._sock.close()
        except OSError:
            pass

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *exc: Any) -> None:
        if self.process is not None:
            try:
                self.quit()
            except Exception:
                pass
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()
        self.close()

    # -- design files --------------------------------------------------------

    def describe(self) -> list:
        """Every method with its parameters."""
        return self.call("describe")

    def new_design(self) -> None:
        self.call("new_design")

    def open(self, path: str) -> None:
        self.call("open", path=os.path.abspath(path))

    def save(self, path: Optional[str] = None) -> str:
        return self.call("save", path=os.path.abspath(path) if path else None)["path"]

    def get_document(self) -> dict:
        """The design in native .scotticad form (full group tree)."""
        return self.call("get_document")

    def state(self) -> dict:
        return self.call("get_state")

    def nodes(self, deep: bool = False) -> list:
        """Top-level nodes with bounds and volume."""
        return self.call("list_nodes", deep=deep)

    def node(self, id: int) -> dict:
        return self.call("get_node", id=id)

    def shape_kinds(self) -> list:
        return self.call("shape_kinds")

    def fonts(self) -> list:
        return self.call("fonts")

    # -- creation --------------------------------------------------------------

    def add_shape(self, kind: str, x: float = 0, y: float = 0, z: Optional[float] = None, hole: bool = False) -> int:
        """Place a shape with its bottom centre at (x, y, z); returns its id."""
        return self.call("add_shape", kind=kind, x=x, y=y, z=z, hole=hole)["id"]

    def add_outline(self, points: Sequence, z0: float = 0, z1: float = 10, hole: bool = False) -> int:
        """Extrude a closed outline straight up from z0 to z1; returns its id.

        ``points`` is ``[(x, y), ...]``, or a list of such outlines (ones
        inside others are holes). Points are world millimetres: the shape
        lands exactly where they say. Set the shape's ``smooth`` parameter
        (:meth:`set_param`) to make each outline a curve through its points
        (a point given twice in a row is a sharp corner)."""
        return self.call("add_outline", points=points, z0=z0, z1=z1, hole=hole)["id"]

    def add_loft(self, sections: Sequence, hole: bool = False) -> int:
        """Join closed outlines at rising heights with straight walls and
        close the ends; returns its id.

        ``sections`` is ``[(z, [(x, y), ...]), ...]``: at least two, z
        rising, one outline each. Outlines with the same number of points
        are joined point to point; others by position along their length.
        Points are world millimetres."""
        return self.call("add_loft", sections=sections, hole=hole)["id"]

    def import_file(self, path: str, x: float = 0, y: float = 0, scale: float = 1.0,
                    units: str = "mm", height: float = 10.0) -> int:
        return self.call("import", path=os.path.abspath(path), x=x, y=y, scale=scale, units=units, height=height)["id"]

    def export(self, path: str, selection_only: bool = False) -> None:
        self.call("export", path=os.path.abspath(path), selection_only=selection_only)

    # -- selection ---------------------------------------------------------------

    def select(self, ids: Iterable[int]) -> list:
        return self.call("select", ids=list(ids))

    def add_to_selection(self, id: int) -> list:
        """Add a shape to the selection; returns the selection."""
        return self.call("add_to_selection", id=id)

    def select_all(self) -> list:
        return self.call("select_all")

    def clear_selection(self) -> None:
        self.call("clear_selection")

    def selection(self) -> dict:
        return self.call("get_selection")

    # -- transforms ------------------------------------------------------------------

    def move(self, dx: float = 0, dy: float = 0, dz: float = 0, ids: Optional[Sequence[int]] = None) -> None:
        self.call("move", dx=dx, dy=dy, dz=dz, ids=_ids(ids))

    def set_position(self, x: Optional[float] = None, y: Optional[float] = None, z: Optional[float] = None,
                     anchor: str = "min", ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_position", x=x, y=y, z=z, anchor=anchor, ids=_ids(ids))

    def set_size(self, x: Optional[float] = None, y: Optional[float] = None, z: Optional[float] = None,
                 uniform: bool = False, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_size", x=x, y=y, z=z, uniform=uniform, ids=_ids(ids))

    def set_bounds(self, min: Sequence[float], max: Sequence[float], ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_bounds", min=list(min), max=list(max), ids=_ids(ids))

    def rotate(self, axis: str, degrees: float, ids: Optional[Sequence[int]] = None) -> None:
        self.call("rotate", axis=axis, degrees=degrees, ids=_ids(ids))

    def mirror(self, axis: str, ids: Optional[Sequence[int]] = None) -> None:
        self.call("mirror", axis=axis, ids=_ids(ids))

    def align(self, axis: str, where: str, ids: Optional[Sequence[int]] = None, key: Optional[int] = None) -> None:
        """where: "min", "center" or "max". key: shape the others align to."""
        self.call("align", axis=axis, where=where, ids=_ids(ids), key=key)

    def drop(self, ids: Optional[Sequence[int]] = None) -> None:
        self.call("drop", ids=_ids(ids))

    def lay_flat(self, normal: Sequence[float], ids: Optional[Sequence[int]] = None) -> None:
        """Turn the selection so the face with this outward normal (world
        [x, y, z]) lies on the current plane."""
        self.call("lay_flat", normal=list(normal), ids=_ids(ids))

    def nudge(self, axis: str, steps: float = 1) -> None:
        self.call("nudge", axis=axis, steps=steps)

    # -- appearance ----------------------------------------------------------------------

    def set_hole(self, hole: bool = True, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_hole", hole=hole, ids=_ids(ids))

    def set_color(self, color: str, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_color", color=color, ids=_ids(ids))

    def set_transparent(self, on: bool = True, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_transparent", on=on, ids=_ids(ids))

    def set_multicolor(self, on: bool = True, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_multicolor", on=on, ids=_ids(ids))

    def set_locked(self, on: bool = True, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_locked", on=on, ids=_ids(ids))

    def set_name(self, name: str, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_name", name=name, ids=_ids(ids))

    def set_param(self, id: int, key: str, value: float) -> None:
        self.call("set_param", id=id, key=key, value=value)

    def set_text(self, id: int, text: str, font: Optional[str] = None) -> None:
        self.call("set_text", id=id, text=text, font=font)

    def set_outline(self, id: int, points: Sequence, z0: Optional[float] = None, z1: Optional[float] = None) -> None:
        """Replace an outline's points (and heights, if given). They are in
        the shape's own frame: an unmoved shape puts them where they say."""
        self.call("set_outline", id=id, points=points, z0=z0, z1=z1)

    def set_loft(self, id: int, sections: Sequence) -> None:
        """Replace a loft's sections (as :meth:`add_loft`), in the shape's
        own frame."""
        self.call("set_loft", id=id, sections=sections)

    # -- visibility -------------------------------------------------------------------------

    def hide(self, ids: Optional[Sequence[int]] = None) -> None:
        self.call("hide", ids=_ids(ids))

    def toggle_hidden(self, id: int) -> None:
        self.call("toggle_hidden", id=id)

    def show_all(self) -> None:
        self.call("show_all")

    # -- grouping and clipboard ----------------------------------------------------------------

    def group(self, mode: str = "union", ids: Optional[Sequence[int]] = None) -> int:
        """mode: "union", "bundle" or "intersect". Returns the group id."""
        return self.call("group", mode=mode, ids=_ids(ids))["id"]

    def ungroup(self, ids: Optional[Sequence[int]] = None) -> list:
        return self.call("ungroup", ids=_ids(ids))["ids"]

    def set_group_mode(self, mode: str, ids: Optional[Sequence[int]] = None) -> None:
        self.call("set_group_mode", mode=mode, ids=_ids(ids))

    def split(self, axis: str, at: Optional[float] = None, keep: str = "both", gap: float = 0,
              ids: Optional[Sequence[int]] = None) -> list:
        """Cut shapes with a plane square to an axis ("x", "y" or "z") of the
        current plane, at `at` mm along it (default: the selection's centre). keep:
        "both", "above" or "below" (the + or - side). gap moves the half
        above that far away; the one below stays. Each half is a group of
        the shape and a hole box; returns the halves' ids."""
        return self.call("split", axis=axis, at=at, keep=keep, gap=gap, ids=_ids(ids))["ids"]

    def duplicate(self) -> list:
        return self.call("duplicate")["ids"]

    def copy(self) -> None:
        self.call("copy")

    def cut(self) -> None:
        self.call("cut")

    def paste(self, in_place: bool = False) -> list:
        return self.call("paste", in_place=in_place)["ids"]

    def delete(self, ids: Optional[Sequence[int]] = None) -> None:
        self.call("delete", ids=_ids(ids))

    def undo(self) -> bool:
        return self.call("undo")["done"]

    def redo(self) -> bool:
        return self.call("redo")["done"]

    # -- settings and tools ---------------------------------------------------------------------

    def set_units(self, units: str) -> None:
        """"mm" or "in"."""
        self.call("set_units", units=units)

    def set_snap(self, mm: float) -> None:
        self.call("set_snap", mm=mm)

    def set_build_plate(self, width: float, length: float) -> None:
        """The build plate's size in mm."""
        self.call("set_build_plate", width=width, length=length)

    def set_settings(self, show_grid: Optional[bool] = None, show_shadows: Optional[bool] = None) -> None:
        self.call("set_settings", show_grid=show_grid, show_shadows=show_shadows)

    def place_ruler(self, x: float, y: float, midpoint: bool = False, turns: int = 0) -> None:
        """turns: quarter turns counter-clockwise (as clicking the origin)."""
        self.call("place_ruler", x=x, y=y, midpoint=midpoint, turns=turns)

    def add_note(self, id: int, x: float, y: float, z: float, text: str = "") -> int:
        """Pin a note to shape id at a world point; returns its index on that shape."""
        return self.call("add_note", id=id, x=x, y=y, z=z, text=text)["index"]

    def notes(self) -> list:
        """Every note: id, index, text, world, hidden, open."""
        return self.call("notes") or []

    def set_note_text(self, id: int, index: int, text: str) -> None:
        self.call("set_note_text", id=id, index=index, text=text)

    def delete_note(self, id: int, index: int) -> None:
        self.call("delete_note", id=id, index=index)

    def remove_ruler(self) -> None:
        self.call("remove_ruler")

    def toggle_construction_plane(self) -> None:
        """The Construction plane on shape button: put a construction plane on
        the selected shape, or remove it."""
        self.call("toggle_construction_plane")

    def attach_construction_plane(self, id: int) -> None:
        """Put a construction plane on shape id (its own axes, on its bottom);
        id 0 removes it, back to the build plate."""
        self.call("attach_construction_plane", id=id)

    def filament_estimate(self, ids: Optional[Sequence[int]] = None) -> dict:
        """Filament for the selection (or ids, or the whole design): grams,
        meters, cost, volume (mm³), shell, shapes, by_color, settings.
        Supports, brims and purge aren't included."""
        return self.call("filament_estimate", ids=_ids(ids))

    def set_filament_settings(self, material: Optional[str] = None, density: Optional[float] = None,
                              diameter: Optional[float] = None, layer_height: Optional[float] = None,
                              line_width: Optional[float] = None, walls: Optional[int] = None,
                              top_layers: Optional[int] = None, bottom_layers: Optional[int] = None,
                              infill: Optional[float] = None, price_per_kg: Optional[float] = None,
                              calibration: Optional[float] = None) -> dict:
        """Change the filament estimate's settings (others are kept); returns
        them all. material: PLA, PETG, ABS, ASA, TPU, PC or Nylon."""
        return self.call("set_filament_settings", material=material, density=density, diameter=diameter,
                         layer_height=layer_height, line_width=line_width, walls=walls, top_layers=top_layers,
                         bottom_layers=bottom_layers, infill=infill, price_per_kg=price_per_kg, calibration=calibration)

    def set_section(self, on: bool = True, axis: Optional[str] = None, at: Optional[float] = None,
                    keep: Optional[str] = None, pin: Optional[bool] = None) -> None:
        """Section view: hide everything on one side of a plane square to an
        axis of the current plane ("x", "y" or "z"; default "y") at `at` mm
        (default: the design's middle). keep: "above" or "below", the side that shows
        (default: the side away from the camera). pin keeps it open when the
        selection changes. The design is unchanged."""
        self.call("set_section", on=on, axis=axis, at=at, keep=keep, pin=pin)

    def set_tool(self, tool: str) -> None:
        """none, align, mirror, measure, visibility, ruler, notes,
        construction_plane, split, layflat or cruise."""
        self.call("set_tool", tool=tool)

    # -- camera and output ----------------------------------------------------------------------

    def camera(self) -> dict:
        return self.call("get_camera")

    def set_camera(self, target: Optional[Sequence[float]] = None, yaw: Optional[float] = None,
                   pitch: Optional[float] = None, dist: Optional[float] = None, ortho: Optional[bool] = None) -> None:
        self.call("set_camera", target=list(target) if target else None, yaw=yaw, pitch=pitch, dist=dist, ortho=ortho)

    def home_view(self) -> None:
        self.call("home_view")

    def fit_view(self) -> None:
        self.call("fit_view")

    def screenshot(self, path: str) -> dict:
        return self.call("screenshot", path=os.path.abspath(path))

    def project(self, x: float, y: float, z: float) -> tuple:
        """World mm → window pixels (logical)."""
        r = self.call("project", x=x, y=y, z=z)
        return r["x"], r["y"]

    def version(self) -> str:
        """ScottiCAD's version."""
        return self.call("version")

    def ui_state(self) -> dict:
        return self.call("get_ui_state")

    def controls(self) -> list:
        """Every clickable control on screen: id, rect, tooltip."""
        return self.call("list_controls")

    def handles(self) -> list:
        """Selection handles on screen: kind, index, x, y."""
        return self.call("list_handles") or []

    def handle(self, kind: str, index: int = 0) -> tuple:
        for h in self.handles():
            if h["kind"] == kind and h["index"] == index:
                return h["x"], h["y"]
        raise KeyError(f"no {kind} handle {index}")

    def click_control(self, id: str) -> None:
        self.call("click_control", id=id)

    # -- input simulation (window coordinates, logical pixels) ---------------------------------

    def mouse_move(self, x: float, y: float, mods: Sequence[str] = ()) -> None:
        self.call("mouse_move", x=x, y=y, mods=list(mods))

    def mouse_down(self, x: float, y: float, button: str = "left", mods: Sequence[str] = ()) -> None:
        self.call("mouse_down", x=x, y=y, button=button, mods=list(mods))

    def mouse_up(self, x: float, y: float, button: str = "left", mods: Sequence[str] = ()) -> None:
        self.call("mouse_up", x=x, y=y, button=button, mods=list(mods))

    def click(self, x: float, y: float, button: str = "left", mods: Sequence[str] = ()) -> None:
        self.call("click", x=x, y=y, button=button, mods=list(mods))

    def drag(self, x0: float, y0: float, x1: float, y1: float, steps: int = 10,
             button: str = "left", mods: Sequence[str] = ()) -> None:
        self.call("drag", x0=x0, y0=y0, x1=x1, y1=y1, steps=steps, button=button, mods=list(mods))

    def wheel(self, x: float, y: float, delta: float) -> None:
        self.call("wheel", x=x, y=y, delta=delta)

    def key(self, keys: str) -> None:
        """A chord such as "ctrl+g", "shift+ctrl+z", "delete", "esc", "h"."""
        self.call("key", keys=keys)

    def type_text(self, text: str) -> None:
        self.call("type_text", text=text)

    # -- scripting ----------------------------------------------------------------------------------

    def run_script(self, code: str, name: str = "<api>") -> dict:
        """Run a Starlark script in ScottiCAD, as its console's Editor tab does (one undo
        step). Returns {"output": printed text, "error": traceback or None}.
        Relative paths in the script resolve against this process's working
        directory."""
        return self.call("run_script", code=code, name=name, cwd=os.getcwd())

    def console(self) -> dict:
        """The Scripting console: open, tab, running, lines, input, script."""
        return self.call("get_console")

    def answer_dialogs(self, *answers: tuple) -> int:
        """Queue answers to the prompts and file dialogs that come next, for
        automated runs: ("message", "yes"|"no"|"cancel"), ("open", path),
        ("save", path); an empty path cancels. Returns how many are queued."""
        return self.call("answer_dialogs", answers=[{"kind": k, "value": v} for k, v in answers])["queued"]

    def close_window(self) -> bool:
        """What closing the window would do, Save changes? prompt included:
        True if it would close. The window stays open."""
        return self.call("close_window")["closes"]

    def lose_capture(self) -> None:
        """The mouse is taken away mid-drag (another window, Alt+Tab)."""
        self.call("lose_capture")

    def stats(self) -> dict:
        """Memory and cache figures: heap_alloc, heap_sys, goroutines, meshes,
        mesh_triangles, bounds, gpu_meshes, background_jobs, undo_depth."""
        return self.call("stats")

    def drop_files(self, *paths: str) -> None:
        """Drop files on the window (imports them)."""
        self.call("drop_files", paths=[os.path.abspath(p) for p in paths])

    def quit(self) -> None:
        self.call("quit")


def _ids(ids: Optional[Sequence[int]]) -> Optional[list]:
    return None if ids is None else list(ids)


def launch(exe: Optional[str] = None, headless: bool = True, width: int = 1600, height: int = 1000,
           design: Optional[str] = None, timeout: float = 30.0, scale: Optional[float] = None) -> Client:
    """Start scotticad.exe with the API on a free port and connect to it.

    Use as a context manager; leaving the block quits ScottiCAD.
    """
    cmd = [exe or default_exe(), "--api", "127.0.0.1:0", f"--width={width}", f"--height={height}"]
    if headless:
        cmd.append("--headless")
    if scale:
        cmd.append(f"--scale={scale}")
    if design:
        cmd.append(os.path.abspath(design))
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                            encoding="utf-8", errors="replace")
    port: list = []

    def read() -> None:
        assert proc.stdout is not None
        for line in proc.stdout:
            if line.startswith("SCOTTICAD_API "):
                port.append(int(line.split()[1].rsplit(":", 1)[1]))
                return

    t = threading.Thread(target=read, daemon=True)
    t.start()
    t.join(timeout)
    if not port:
        proc.kill()
        err = proc.stderr.read() if proc.stderr else ""
        raise RuntimeError(f"ScottiCAD did not start: {err.strip()}")
    # keep draining its output: a full pipe would stop it in its tracks
    def drain(s: Any) -> None:
        while s.read(65536):
            pass
    for stream in (proc.stdout, proc.stderr):
        threading.Thread(target=drain, args=(stream,), daemon=True).start()
    c = Client(port=port[0])
    c.process = proc
    return c
