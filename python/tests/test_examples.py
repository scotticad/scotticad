"""The published examples (examples/published.txt): each script still runs
and makes exactly the design saved for it, each design opens, and the
examples README describes exactly these designs."""
import json
import os
import re
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402

EXAMPLES = os.path.normpath(os.path.join(os.path.dirname(__file__), "..", "..", "examples"))
LIST = os.path.join(EXAMPLES, "published.txt")

if not os.path.exists(LIST):
    pytest.skip("no examples folder beside the Python client", allow_module_level=True)

sys.path.insert(0, EXAMPLES)
import published  # noqa: E402

DESIGNS = published.read(LIST)
NAMES = [d.name for d in DESIGNS]
MADE = [d for d in DESIGNS if d.script]


def test_every_listed_design_is_there():
    assert NAMES == sorted(set(NAMES)), "published.txt: keep the names sorted, each once"
    for d in DESIGNS:
        assert os.path.exists(os.path.join(EXAMPLES, "scotticad", d.name + ".scotticad")), d.name
        if d.script:
            assert os.path.exists(published.script_path(d.script)), d.script
            published.source(d)  # every setting it changes is in the script


def test_the_readme_describes_exactly_the_published_designs():
    with open(os.path.join(EXAMPLES, "README.md"), encoding="utf-8") as f:
        text = f.read()
    entries = text.split("## The examples", 1)[1]
    described = set(re.findall(r"^- \*\*\[`[^`]+`\]\((?:star/([\w/-]+)\.star|scotticad/([\w/-]+)\.scotticad)\)",
                               entries, re.M))
    described = {a or b for a, b in described}
    listed = {d.script or d.name for d in DESIGNS}
    assert described == listed, {"not described": listed - described,
                                 "described but not published": described - listed}
    # a script making several designs links each of them
    for d in DESIGNS:
        if d.script and d.script != d.name:
            assert f"(scotticad/{d.name}.scotticad)" in entries, d.name


def design(data: bytes) -> dict:
    """A saved design without the view it opens with (the camera and the
    grid), which can be set by hand after the script has made it."""
    doc = json.loads(data)
    doc.pop("camera", None)
    doc.get("settings", {}).pop("showGrid", None)
    return doc


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True) as c:
        yield c


@pytest.mark.parametrize("d", MADE, ids=[d.name for d in MADE])
def test_script_makes_its_saved_design(cad, d, tmp_path, monkeypatch):
    # relative paths in a script resolve against the script's own folder
    script = published.script_path(d.script)
    monkeypatch.chdir(os.path.dirname(script))
    cad.new_design()
    result = cad.run_script(published.source(d), name=os.path.basename(script))
    assert not result.get("error"), result["error"]
    made = cad.save(str(tmp_path / (os.path.basename(d.name) + ".scotticad")))
    with open(made, "rb") as f:
        got = design(f.read())
    with open(os.path.join(EXAMPLES, "scotticad", d.name + ".scotticad"), "rb") as f:
        want = design(f.read())
    # python examples/build.py remakes the saved designs after a deliberate change
    assert got == want, f"{d.script}.star no longer makes {d.name}.scotticad"


@pytest.mark.parametrize("name", [d.name for d in DESIGNS if not d.script])
def test_original_design_opens(cad, name):
    cad.new_design()
    cad.open(os.path.join(EXAMPLES, "scotticad", name + ".scotticad"))
    assert cad.nodes()
