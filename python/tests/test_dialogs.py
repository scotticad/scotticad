"""The paths behind prompts and file dialogs: Save changes? on New, Open and
closing the window; Save, Save as, Open, Export and Import through their
dialogs; files dropped on the window; the script editor's own prompts.
Headless runs answer them through answer_dialogs."""
import math
import os
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
    cad.set_tool("none")
    if cad.ui_state()["popup"]:
        cad.key("esc")
    if cad.console()["open"]:
        cad.click_control("scripting")
    yield
    # no answer may be left over for the next test
    assert cad.call("answer_dialogs", answers=[])["queued"] == 0


def dirty_box(cad):
    b = cad.add_shape("box")
    assert cad.state()["dirty"]
    return b


def test_new_design_prompt(cad, tmp_path):
    b = dirty_box(cad)
    cad.answer_dialogs(("message", "cancel"))
    cad.key("ctrl+n")
    assert cad.node(b)  # kept
    cad.answer_dialogs(("message", "no"))
    cad.key("ctrl+n")
    assert cad.state()["node_count"] == 0
    # yes: saved first, to the path the save dialog is given
    b = dirty_box(cad)
    path = tmp_path / "kept.scotticad"
    cad.answer_dialogs(("message", "yes"), ("save", str(path)))
    cad.key("ctrl+n")
    assert path.exists() and cad.state()["node_count"] == 0
    # yes, but the save dialog cancelled: nothing is lost
    b = dirty_box(cad)
    cad.answer_dialogs(("message", "yes"), ("save", ""))
    cad.key("ctrl+n")
    assert cad.node(b) and cad.state()["dirty"]


def test_save_and_save_as(cad, tmp_path):
    dirty_box(cad)
    path = tmp_path / "a.scotticad"
    cad.answer_dialogs(("save", str(path)))
    cad.key("ctrl+s")
    st = cad.state()
    assert not st["dirty"] and st["path"] == str(path)
    cad.add_shape("cylinder", x=40)
    cad.key("ctrl+s")  # a named design saves without a dialog
    assert not cad.state()["dirty"]
    other = tmp_path / "b.scotticad"
    cad.answer_dialogs(("save", str(other)))
    cad.click_control("filemenu")
    cad.click_control("dm-saveas")
    assert cad.state()["path"] == str(other) and other.exists() and path.exists()
    # a save as to a bad place fails, and the design stays as it was
    cad.add_shape("sphere", x=80)
    cad.answer_dialogs(("save", str(tmp_path / "missing" / "c.scotticad")))
    cad.key("ctrl+shift+s")
    st = cad.state()
    assert st["dirty"] and st["path"] == str(other) and cad.ui_state()["toast"]


def test_open_prompt_and_dialog(cad, tmp_path):
    cad.add_shape("box")
    path = tmp_path / "saved.scotticad"
    cad.save(str(path))
    cad.new_design()
    dirty_box(cad)
    # cancelled at the prompt: nothing opens
    cad.answer_dialogs(("message", "cancel"))
    cad.key("ctrl+o")
    assert cad.state()["dirty"] and cad.state()["path"] == ""
    # no, then the open dialog
    cad.answer_dialogs(("message", "no"), ("open", str(path)))
    cad.key("ctrl+o")
    assert cad.state()["path"] == str(path) and cad.state()["node_count"] == 1
    # a damaged file: an error, the open design untouched
    bad = tmp_path / "bad.scotticad"
    bad.write_text('{"version":1,"nodes":[{"id":1,"kind":"bo')
    cad.answer_dialogs(("open", str(bad)))
    cad.key("ctrl+o")
    assert cad.state()["path"] == str(path) and cad.ui_state()["toast"]


def test_close_window_prompt(cad, tmp_path):
    assert cad.close_window()  # nothing to lose
    b = dirty_box(cad)
    cad.answer_dialogs(("message", "cancel"))
    assert not cad.close_window()
    cad.answer_dialogs(("message", "yes"), ("save", ""))  # save cancelled: stays open
    assert not cad.close_window()
    assert cad.node(b)
    path = tmp_path / "onclose.scotticad"
    cad.answer_dialogs(("message", "yes"), ("save", str(path)))
    assert cad.close_window() and path.exists()
    cad.add_shape("box", x=40)
    cad.answer_dialogs(("message", "no"))
    assert cad.close_window()


def test_export_dialog(cad, tmp_path):
    b = cad.add_shape("box")
    cad.select([b])
    cad.key("ctrl+e")
    assert cad.ui_state()["popup"] == "export"
    stl = tmp_path / "box.stl"
    cad.answer_dialogs(("save", str(stl)))
    cad.click_control("exp-stl")
    assert stl.stat().st_size == 84 + 50 * 12 and cad.ui_state()["popup"] == ""
    # cancelled: no file, the dialog stays
    cad.key("ctrl+e")
    cad.answer_dialogs(("save", ""))
    cad.click_control("exp-obj")
    assert not (tmp_path / "box.obj").exists() and cad.ui_state()["popup"] == "export"
    cad.key("esc")
    # an empty design has nothing to export
    cad.new_design()
    cad.key("ctrl+e")
    cad.answer_dialogs(("save", str(tmp_path / "empty.stl")))
    cad.click_control("exp-stl")
    assert not (tmp_path / "empty.stl").exists() and cad.ui_state()["toast"]
    cad.key("esc")


def test_import_dialog_and_dropped_files(cad, tmp_path):
    b = cad.add_shape("box")
    stl = tmp_path / "cube.stl"
    cad.export(str(stl), selection_only=False)
    cad.new_design()
    cad.key("ctrl+i")
    assert cad.ui_state()["popup"] == "import"
    cad.answer_dialogs(("open", str(stl)))
    cad.click_control("imp-choose")
    cad.click_control("imp-go")
    nodes = cad.nodes()
    assert len(nodes) == 1 and nodes[0]["kind"] == "mesh"
    assert math.isclose(nodes[0]["bounds"]["size"][0], 20, abs_tol=1e-3)
    # dropped files import too; one that isn't a model is refused
    cad.new_design()
    txt = tmp_path / "notes.txt"
    txt.write_text("hello")
    cad.drop_files(str(stl), str(txt))
    assert len(cad.nodes()) == 1 and cad.ui_state()["toast"]


def test_script_editor_prompts(cad, tmp_path):
    cad.click_control("scripting")
    cad.click_control("con-tab-Editor")
    cad.type_text("x = 1")
    assert cad.console()["script"] == "x = 1"
    # New with changes: cancel keeps them
    cad.answer_dialogs(("message", "cancel"))
    cad.click_control("con-new")
    assert cad.console()["script"] == "x = 1"
    # New: yes saves first, to the path given
    path = tmp_path / "s.star"
    cad.answer_dialogs(("message", "yes"), ("save", str(path)))
    cad.click_control("con-new")
    assert path.read_text() == "x = 1" and cad.console()["script"] == ""
    # Open takes a file into the editor
    cad.answer_dialogs(("open", str(path)))
    cad.click_control("con-open")
    assert cad.console()["script"] == "x = 1" and cad.console()["script_path"] == str(path)


def test_bad_answers_are_refused(cad):
    with pytest.raises(ScottiCADError):
        cad.call("answer_dialogs", answers=[{"kind": "beep", "value": "yes"}])


def test_save_log_and_run_script_file(cad, tmp_path):
    # Run script from the file menu runs a file, as one undo step
    script = tmp_path / "make.star"
    script.write_text("print('from a file')\nfor i in range(3):\n    cad.add_shape('box', x=i * 30)\n")
    depth = cad.state()["undo_depth"]
    cad.answer_dialogs(("open", str(script)))
    cad.click_control("filemenu")
    cad.click_control("dm-runscript")
    for _ in range(200):  # the script runs in the background
        if not cad.console()["running"] and len(cad.nodes() or []) == 3:
            break
    assert len(cad.nodes()) == 3 and cad.state()["undo_depth"] == depth + 1
    # Save log writes the transcript, the script's output included
    if not cad.console()["open"]:
        cad.click_control("scripting")
    log = tmp_path / "log.txt"
    cad.answer_dialogs(("save", str(log)))
    cad.click_control("con-savelog")
    assert "from a file" in log.read_text(encoding="utf-8")
    # cancelled: nothing written
    log.unlink()
    cad.answer_dialogs(("save", ""))
    cad.click_control("con-savelog")
    assert not log.exists()
