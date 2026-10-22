"""The Scripting console: Starlark scripts drive ScottiCAD through the same
methods as this Python client."""
import inspect
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import Client, ScottiCADError, launch  # noqa: E402

# Python client methods scripts don't get (see pkg/script NotInScripts),
# and the generic call(), which scripts have as cad.call()
NOT_IN_SCRIPTS = {"quit", "close", "run_script", "call", "answer_dialogs", "close_window", "drop_files", "lose_capture", "stats"}


@pytest.fixture(scope="module")
def cad():
    with launch(headless=True, width=1400, height=860) as c:
        yield c


@pytest.fixture(autouse=True)
def fresh(cad):
    cad.new_design()
    yield


def same(a, b):
    if isinstance(a, tuple):
        a = list(a)
    return (type(a) is bool) == (type(b) is bool) and a == b


def test_scripts_match_the_python_client(cad):
    spec = {m["name"]: m for m in cad.call("script_api")}
    client = {n: f for n, f in inspect.getmembers(Client, inspect.isfunction) if not n.startswith("_")}
    assert set(client) - NOT_IN_SCRIPTS == set(spec)
    for name, m in spec.items():
        params = list(inspect.signature(client[name]).parameters.values())[1:]
        assert [p.name for p in params] == [p["name"] for p in m["params"] or []], name
        for p, s in zip(params, m["params"] or []):
            if p.default is inspect.Parameter.empty:
                assert s.get("required"), f"{name}({p.name}) is required in Python"
            else:
                assert not s.get("required") and same(p.default, s["default"]), f"{name}({p.name}={p.default!r}) vs {s}"


def test_run_script_builds_and_is_one_undo_step(cad, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    depth = cad.state()["undo_depth"]
    r = cad.run_script("""
ids = []
for i in range(5):
    ids.append(cad.add_shape("cylinder", x=i * 25))
cad.set_size(z=40, ids=ids[:2])
g = cad.group("union", ids=ids[:2])
n = cad.node(g)
print(len(cad.nodes()), n["kind"], n["bounds"]["size"][2])
cad.save("made.scotticad")
""", name="build.star")
    assert r["error"] is None, r["error"]
    assert r["output"] == "4 group 40"
    assert (tmp_path / "made.scotticad").exists()
    assert cad.state()["undo_depth"] == depth + 1
    cad.undo()
    assert cad.nodes() == []


def test_run_script_errors(cad):
    r = cad.run_script('x = 1\ncad.add_shape("nonsense")\n', name="bad.star")
    assert "bad.star:2" in r["error"] and "nonsense" in r["error"]
    r = cad.run_script('cad.call("quit")')
    assert "can't close" in r["error"]
    r = cad.run_script('open("C:/Windows/win.ini")')
    assert "undefined: open" in r["error"]
    lines = cad.console()["lines"]
    assert any(l["kind"] == "error" and l.get("line") == 2 for l in lines)


def open_console(cad):
    if not cad.console()["open"]:
        cad.click_control("scripting")


def test_console_prompt(cad):
    open_console(cad)
    cad.click_control("con-tab-Console")
    con = cad.console()
    assert con["open"] and con["focus"] == "input"
    cad.type_text("w = 6 * 7")
    cad.key("enter")
    cad.type_text("for i in range(3):")
    cad.key("enter")
    cad.type_text("cad.add_shape('box', x=i * 30)")
    cad.key("enter")      # still inside the block: a new line
    cad.key("enter")      # the empty line ends the block and runs it
    cad.type_text("w")
    cad.key("enter")
    con = _settled(cad)
    texts = [l["text"] for l in con["lines"]]
    assert ">>> w = 6 * 7" in texts and "42" in texts, texts
    assert len(cad.nodes()) == 3
    # history: Up brings back the last entry
    cad.key("up")
    assert cad.console()["input"] == "w"
    cad.key("esc")
    assert cad.console()["focus"] == ""


def test_select_and_copy_the_transcript(cad):
    open_console(cad)
    cad.click_control("con-tab-Console")
    cad.click_control("con-clear")
    cad.key("ctrl+a")  # replace whatever is at the prompt
    cad.type_text("print('hello there')")
    cad.key("enter")
    _settled(cad)
    # drag across the whole transcript area
    x, y, w, h = next(c["rect"] for c in cad.controls() if c["id"] == "con-transcript")
    cad.drag(x + 2, y + 2, x + w - 4, y + h - 4)
    sel = cad.console()["selected"]
    # typed input copies without its prompt; output as printed
    assert "print('hello there')\r\nhello there" in sel, repr(sel)
    assert ">>>" not in sel
    cad.key("ctrl+a")
    assert cad.console()["selected"].startswith("print(")
    cad.key("esc")
    assert cad.console()["selected"] == ""


def test_script_tab_run_and_error_link(cad):
    open_console(cad)
    cad.click_control("con-tab-Editor")
    cad.click_control("con-new")
    for line in ["def ring(n, r):", "for i in range(n):"]:
        cad.type_text(line)
        cad.key("enter")
    cad.type_text("print(i)")
    cad.key("enter")
    cad.key("backspace")  # back out of the loop body
    cad.key("backspace")
    cad.type_text("ring(2, 10)")
    cad.key("enter")
    cad.type_text("cad.rotate('q', 1)")
    cad.click_control("con-run")
    con = _settled(cad)
    texts = [l["text"] for l in con["lines"]]
    assert "0" in texts and "1" in texts, texts
    err = [l for l in con["lines"] if l["kind"] == "error" and l.get("line")]
    assert err and err[-1]["line"] == 5 and err[-1]["file"] == "untitled.star", err
    # the error links to its line in the script
    links = [c for c in cad.controls() if c["id"].startswith("con-err-")]
    assert links
    cad.click_control(links[0]["id"])
    con = cad.console()
    assert con["tab"] == "editor" and con["focus"] == "editor"


def test_stop_a_runaway_script(cad):
    open_console(cad)
    cad.click_control("con-tab-Editor")
    cad.click_control("con-new")
    cad.type_text("while True:")
    cad.key("enter")
    cad.type_text("pass")
    cad.click_control("con-run")
    assert cad.console()["running"]
    with pytest.raises(ScottiCADError):
        cad.run_script("print(1)")  # one script at a time
    cad.click_control("con-stop")
    con = _settled(cad)
    assert not con["running"]
    assert any(l["text"].startswith("Stopped after") for l in con["lines"])


def _settled(cad, timeout=10.0):
    """The console once its script has finished."""
    import time
    end = time.time() + timeout
    while time.time() < end:
        con = cad.console()
        if not con["running"]:
            return con
        time.sleep(0.05)
    raise AssertionError("the script did not finish")
