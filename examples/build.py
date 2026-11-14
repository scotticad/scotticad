"""Runs every example script in examples/star (and its subfolders) in
ScottiCAD and saves the design each one makes in examples/scotticad, under
the script's name and in the same subfolder (coasters.star ->
coasters.scotticad), replacing the old one. A script that published.txt
lists as making designs of other names, with settings changed for each
(raspberrypi/vault-case.star makes raspberrypi/vault-case-pi4.scotticad
with BOARD="pi4", and so on), is run once for each of them instead. A
design keeps the view it was saved with (the camera and the grid), which
may have been set by hand. The original designs there, made by hand with
no script, are left as they are.

    make examples                          (Linux: builds ScottiCAD first)
    python examples/build.py               (any platform, after building)
    python examples/build.py OUTDIR        (the designs somewhere else)
    python examples/build.py --only NAME   (just these designs; repeatable)

It drives the ScottiCAD built at the top of the repository, or the one
SCOTTICAD_EXE names, through the automation API, in a hidden window (on
Linux that still needs an X display; make examples uses xvfb-run when it is
installed). A script that fails is reported and its design not saved, and
the run ends with an error; the other scripts still run.
"""
import argparse
import glob
import json
import math
import os
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "python"))
sys.path.insert(0, HERE)
from scotticad import launch  # noqa: E402
import published  # noqa: E402

STAR = os.path.join(HERE, "star")
ROOT = os.path.dirname(HERE)


def shown(path: str) -> str:
    """path relative to the repository when it is in it, else in full."""
    try:
        rel = os.path.relpath(path, ROOT)
    except ValueError:  # another drive
        return path
    return path if rel.startswith("..") else rel


def jobs():
    """Every design to make, as a published.Design: one per script, or one
    per design published.txt lists as made from it."""
    listed = published.read() if os.path.exists(published.LIST) else []
    variants = {}
    for d in listed:
        if d.script and d.script != d.name:
            variants.setdefault(d.script, []).append(d)
    out = []
    for path in sorted(glob.glob(os.path.join(STAR, "**", "*.star"), recursive=True)):
        # the script's path under star/, without .star: "coasters",
        # "raspberrypi/vault-case"
        name = os.path.splitext(os.path.relpath(path, STAR))[0].replace(os.sep, "/")
        out += variants.get(name, [published.Design(name, name, {})])
    return out


def degrees(rad: float) -> float:
    """rad in degrees, chosen so that ScottiCAD's conversion back
    (deg * pi / 180) gives exactly rad again, if a nearby value does."""
    first = math.degrees(rad)
    up = down = first
    for _ in range(256):
        for deg in (up, down):
            if deg * math.pi / 180 == rad:
                return deg
        up, down = math.nextafter(up, math.inf), math.nextafter(down, -math.inf)
    return first


def view(path: str):
    with open(path, encoding="utf-8") as f:
        doc = json.load(f)
    return doc.get("camera"), doc.get("settings", {}).get("showGrid")


def save_keeping_view(cad, target: str, old: str) -> str:
    """Save the design just made to target, with the camera and grid old
    was saved with. The view is only set when it differs from the one the
    script left, since a camera angle doesn't always survive the trip
    through degrees exactly."""
    if not os.path.exists(old):
        return cad.save(target)
    cam, grid = view(old)  # before saving: target is usually old itself
    saved = cad.save(target)
    now_cam, now_grid = view(saved)
    if cam and cam != now_cam:
        t = cam["target"]
        cad.set_camera(target=[t["X"], t["Y"], t["Z"]], yaw=degrees(cam["yaw"]), pitch=degrees(cam["pitch"]),
                       dist=cam["dist"])
    if grid is not None and grid != now_grid:
        cad.set_settings(show_grid=grid)
    if (cam and cam != now_cam) or (grid is not None and grid != now_grid):
        saved = cad.save(target)
    return saved


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("outdir", nargs="?", default=os.path.join(HERE, "scotticad"))
    ap.add_argument("--only", action="append", metavar="NAME",
                    help='make only this design ("coasters", "raspberrypi/vault-case-pi4"); repeatable')
    args = ap.parse_args()
    out_dir = os.path.abspath(args.outdir)
    todo = jobs()
    if args.only:
        unknown = set(args.only) - {d.name for d in todo}
        if unknown:
            print(f"no example makes {', '.join(sorted(unknown))}", file=sys.stderr)
            return 1
        todo = [d for d in todo if d.name in args.only]
    if not todo:
        print(f"no example scripts in {STAR}", file=sys.stderr)
        return 1
    os.makedirs(out_dir, exist_ok=True)
    failed = []
    with launch(headless=True) as cad:
        for d in todo:
            path = published.script_path(d.script)
            # relative paths in a script resolve against its own folder
            os.chdir(os.path.dirname(path))
            target = os.path.join(out_dir, d.name + ".scotticad")
            os.makedirs(os.path.dirname(target), exist_ok=True)
            cad.new_design()
            t0 = time.time()
            result = cad.run_script(published.source(d), name=os.path.basename(path))
            took = time.time() - t0
            if result.get("error"):
                failed.append(d.name)
                print(f"{d.name}: FAILED after {took:.1f}s\n{result['error'].rstrip()}", file=sys.stderr)
                continue
            saved = save_keeping_view(cad, target, os.path.join(HERE, "scotticad", d.name + ".scotticad"))
            print(f"{d.name}: {len(cad.nodes())} shapes, {took:.1f}s -> {shown(saved)}")
    if failed:
        print(f"{len(failed)} of {len(todo)} designs failed: {', '.join(failed)}", file=sys.stderr)
        return 1
    print(f"all {len(todo)} designs made; in {shown(out_dir)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
