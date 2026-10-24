"""Runs every example script in examples/star in ScottiCAD and saves the
design each one makes in examples/scotticad, under the script's name
(coasters.star -> coasters.scotticad), replacing the old one. The original
designs there, made by hand with no script, are left as they are.

    make examples                      (Linux: builds ScottiCAD first)
    python examples/build.py           (any platform, after building)
    python examples/build.py OUTDIR    (the designs somewhere else)

It drives the ScottiCAD built at the top of the repository, or the one
SCOTTICAD_EXE names, through the automation API, in a hidden window (on
Linux that still needs an X display; make examples uses xvfb-run when it is
installed). A script that fails is reported and its design not saved, and
the run ends with an error; the other scripts still run.
"""
import glob
import os
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "python"))
from scotticad import launch  # noqa: E402

STAR = os.path.join(HERE, "star")
ROOT = os.path.dirname(HERE)
OUT = os.path.abspath(sys.argv[1]) if len(sys.argv) > 1 else os.path.join(HERE, "scotticad")


def shown(path: str) -> str:
    """path relative to the repository when it is in it, else in full."""
    try:
        rel = os.path.relpath(path, ROOT)
    except ValueError:  # another drive
        return path
    return path if rel.startswith("..") else rel


def main() -> int:
    scripts = sorted(glob.glob(os.path.join(STAR, "*.star")))
    if not scripts:
        print(f"no example scripts in {STAR}", file=sys.stderr)
        return 1
    os.makedirs(OUT, exist_ok=True)
    failed = []
    # relative paths in a script resolve against the scripts' own folder
    os.chdir(STAR)
    with launch(headless=True) as cad:
        for path in scripts:
            name = os.path.splitext(os.path.basename(path))[0]
            cad.new_design()
            t0 = time.time()
            with open(path, encoding="utf-8") as f:
                result = cad.run_script(f.read(), name=os.path.basename(path))
            took = time.time() - t0
            if result.get("error"):
                failed.append(name)
                print(f"{name}: FAILED after {took:.1f}s\n{result['error'].rstrip()}", file=sys.stderr)
                continue
            saved = cad.save(os.path.join(OUT, name + ".scotticad"))
            print(f"{name}: {len(cad.nodes())} shapes, {took:.1f}s -> {shown(saved)}")
    if failed:
        print(f"{len(failed)} of {len(scripts)} scripts failed: {', '.join(failed)}", file=sys.stderr)
        return 1
    print(f"all {len(scripts)} scripts ran; designs in {shown(OUT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
