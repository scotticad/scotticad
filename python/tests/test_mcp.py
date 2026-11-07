"""The MCP server: scotticad --mcp (ScottiCAD started by the assistant's
client) and scotticad --mcp-connect (driving a ScottiCAD started with
--api), spoken to over their stdin and stdout as an MCP client would."""
import base64
import json
import os
import queue
import socket
import struct
import subprocess
import sys
import threading
import time

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import Client, default_exe  # noqa: E402

# a short script that makes a real part: a stand for drill bits
DRILL_STAND = """
SIZES = [1, 1.5, 2, 2.5, 3, 3.5, 4, 5, 6]
SPACING = 10
HEIGHT = 20

width = len(SIZES) * SPACING
stand = cad.add_shape("box")
cad.set_bounds(min=[0, 0, 0], max=[width, 20, HEIGHT], ids=[stand])
parts = [stand]
for i, size in enumerate(SIZES):
    x = SPACING / 2 + i * SPACING
    d = size + 0.4
    hole = cad.add_shape("cylinder", hole=True)
    cad.set_bounds(min=[x - d/2, 10 - d/2, 4], max=[x + d/2, 10 + d/2, HEIGHT + 1], ids=[hole])
    parts.append(hole)
cad.group("union", ids=parts)
print("made a stand for", len(SIZES), "bits,", width, "mm wide")
"""


class MCP:
    """A scotticad MCP server, spoken to over its pipes."""

    def __init__(self, *args, cwd=None):
        self.proc = subprocess.Popen([default_exe(), *args], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE, cwd=cwd)
        self.replies: queue.Queue = queue.Queue()
        self.not_json: list = []
        self.stderr: list = []
        self.next_id = 0
        threading.Thread(target=self._read, daemon=True).start()
        threading.Thread(target=self._read_err, daemon=True).start()

    def _read(self):
        for line in self.proc.stdout:
            try:
                self.replies.put(json.loads(line))
            except ValueError:
                self.not_json.append(line)  # stdout must carry MCP and nothing else
        self.replies.put(None)

    def _read_err(self):
        for line in self.proc.stderr:
            self.stderr.append(line.decode("utf-8", "replace"))

    def send(self, msg):
        self.proc.stdin.write(json.dumps(msg).encode() + b"\n")
        self.proc.stdin.flush()

    def request(self, method, params=None, timeout=120):
        self.next_id += 1
        msg = {"jsonrpc": "2.0", "id": self.next_id, "method": method}
        if params is not None:
            msg["params"] = params
        self.send(msg)
        while True:
            r = self.replies.get(timeout=timeout)
            assert r is not None, "the server stopped: " + "".join(self.stderr)
            if r.get("id") == self.next_id:
                assert "error" not in r, r["error"]
                return r["result"]

    def initialize(self):
        r = self.request("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                                        "clientInfo": {"name": "pytest", "version": "1"}})
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return r

    def tool(self, tool_name, **arguments):
        return self.request("tools/call", {"name": tool_name, "arguments": arguments})

    def close(self, timeout=30):
        """Hang up, as a client does, and wait for the server to go."""
        self.proc.stdin.close()
        return self.proc.wait(timeout)

    def kill(self):
        if self.proc.poll() is None:
            self.proc.kill()
            self.proc.wait()


def text(r):
    return "".join(c["text"] for c in r["content"] if c["type"] == "text")


def png_size(data):
    # the IHDR chunk: width and height right after the signature and header
    assert data[:8] == b"\x89PNG\r\n\x1a\n"
    return struct.unpack(">II", data[16:24])


def start_api(port=0):
    """A headless ScottiCAD with its API, and its port."""
    proc = subprocess.Popen([default_exe(), "--api", f"127.0.0.1:{port}", "--headless"],
                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    line = proc.stdout.readline()
    assert line.startswith("SCOTTICAD_API "), line
    threading.Thread(target=lambda: proc.stdout.read(), daemon=True).start()
    return proc, int(line.split()[1].rsplit(":", 1)[1])


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture
def mcp():
    m = MCP("--mcp", "--headless", "--api", "127.0.0.1:0", "--width=1200", "--height=800")
    yield m
    m.kill()


def test_all_in_one_builds_looks_and_undoes(mcp):
    r = mcp.initialize()
    assert r["serverInfo"]["name"] == "scotticad" and "run_script" in r["instructions"]
    tools = {t["name"] for t in mcp.request("tools/list")["tools"]}
    assert {"run_script", "screenshot", "set_bounds", "undo"} <= tools
    assert not tools & {"click", "key", "quit", "describe"}

    r = mcp.tool("run_script", code=DRILL_STAND, name="drill-stand.star")
    assert not r.get("isError"), text(r)
    assert text(r) == "made a stand for 9 bits, 90 mm wide"
    nodes = json.loads(text(mcp.tool("list_nodes")))
    assert len(nodes) == 1 and nodes[0]["kind"] == "group"
    assert nodes[0]["bounds"]["size"] == pytest.approx([90, 20, 20])

    r = mcp.tool("set_size", x=120, ids=[nodes[0]["id"]])
    assert not r.get("isError"), text(r)
    r = mcp.tool("get_node", id=987654)
    assert r.get("isError")

    mcp.tool("fit_view")
    r = mcp.tool("screenshot", max_size=512)
    img = next(c for c in r["content"] if c["type"] == "image")
    w, h = png_size(base64.b64decode(img["data"]))
    assert max(w, h) == 512 and img["mimeType"] == "image/png"

    mcp.tool("undo")  # the resize
    mcp.tool("undo")  # the whole script
    assert json.loads(text(mcp.tool("list_nodes"))) == []

    ref = mcp.request("resources/read", {"uri": "scotticad://script-api"})["contents"][0]["text"]
    assert "cad.add_shape(" in ref and "cad.set_bounds(" in ref

    assert mcp.close() == 0, "headless: hanging up quits"
    assert mcp.not_json == [], "stdout carried something other than MCP"
    assert any(l.startswith("SCOTTICAD_API ") for l in mcp.stderr), "--api's line goes to stderr"


def test_all_in_one_shares_the_window_with_the_api(mcp):
    mcp.initialize()
    port = next(int(l.split()[1].rsplit(":", 1)[1]) for l in mcp.stderr if l.startswith("SCOTTICAD_API "))
    with Client(port=port) as cad:
        box = cad.add_shape("box")
        assert [n["id"] for n in json.loads(text(mcp.tool("list_nodes")))] == [box]


def test_script_errors_are_tool_errors(mcp):
    mcp.initialize()
    r = mcp.tool("run_script", code='print("before")\ncad.add_shape("nonsense")\n', name="bad.star")
    assert r["isError"] and "bad.star:2" in text(r) and "before" in text(r)


def test_cancelling_run_script_stops_it(mcp):
    mcp.initialize()
    port = next(int(l.split()[1].rsplit(":", 1)[1]) for l in mcp.stderr if l.startswith("SCOTTICAD_API "))
    mcp.next_id += 1
    script_id = mcp.next_id
    mcp.send({"jsonrpc": "2.0", "id": script_id, "method": "tools/call",
              "params": {"name": "run_script", "arguments": {"code": "while True:\n    pass\n"}}})
    with Client(port=port) as cad:
        for _ in range(100):
            if cad.call("get_console")["running"]:
                break
            time.sleep(0.02)
        else:
            pytest.fail("the script didn't start")
        assert len(mcp.request("ping")) == 0, "the server answers meanwhile"
        mcp.send({"jsonrpc": "2.0", "method": "notifications/cancelled", "params": {"requestId": script_id}})
        r = mcp.tool("run_script", code="print('next')")  # busy if the first were still running
        assert text(r) == "next", text(r)
        assert not cad.call("get_console")["running"]
    assert all(r.get("id") != script_id for r in list(mcp.replies.queue)), "a cancelled request gets no reply"


def test_root_limits_files(tmp_path):
    root = tmp_path / "work"
    root.mkdir()
    m = MCP("--mcp", "--headless", "--root", str(root))
    try:
        m.initialize()
        tools = {t["name"]: t for t in m.request("tools/list")["tools"]}
        assert str(root) in tools["export"]["description"], "the assistant is told where files go"
        assert str(root) not in tools["add_shape"]["description"]
        m.tool("add_shape", kind="box")
        r = m.tool("export", path="part.stl")
        assert not r.get("isError"), text(r)
        assert (root / "part.stl").exists()
        assert m.tool("export", path="../part.stl")["isError"]
        r = m.tool("run_script", code=f'cad.export({str(tmp_path / "escape.stl")!r})')
        assert r["isError"] and "outside" in text(r)
        assert not (tmp_path / "escape.stl").exists()
        r = m.tool("run_script", code='cad.save("made.scotticad")')
        assert not r.get("isError"), text(r)
        assert (root / "made.scotticad").exists()
    finally:
        m.kill()


def test_connect_drives_a_running_scotticad():
    proc, port = start_api()
    try:
        with Client(port=port) as cad:
            m = MCP("--mcp-connect", f"127.0.0.1:{port}")
            try:
                m.initialize()
                r = m.tool("run_script", code=DRILL_STAND)
                assert text(r) == "made a stand for 9 bits, 90 mm wide"
                assert len(cad.nodes()) == 1, "the change is in the running ScottiCAD"
                r = m.tool("screenshot", max_size=300)
                assert any(c["type"] == "image" for c in r["content"])
                assert m.close() == 0
                assert m.not_json == []
            finally:
                m.kill()
            assert len(cad.nodes()) == 1, "hanging up leaves ScottiCAD running"
    finally:
        proc.kill()
        proc.wait()


def test_connect_before_scotticad_starts_and_after_it_restarts():
    port = free_port()
    m = MCP("--mcp-connect", f"127.0.0.1:{port}")
    proc = None
    try:
        m.initialize()
        tools = {t["name"] for t in m.request("tools/list")["tools"]}
        assert "run_script" in tools, "this build's tools are listed meanwhile"
        r = m.tool("undo")
        assert r["isError"] and f"scotticad --api 127.0.0.1:{port}" in text(r)

        for _ in range(2):  # started, then restarted
            proc, _ = start_api(port)
            r = m.tool("add_shape", kind="box")
            assert not r.get("isError"), text(r)
            assert len(json.loads(text(m.tool("list_nodes")))) == 1
            proc.kill()
            proc.wait()
    finally:
        m.kill()
        if proc and proc.poll() is None:
            proc.kill()


@pytest.mark.parametrize("args", [
    ["--mcp", "--mcp-connect", "127.0.0.1:1"],
    ["--mcp-connect", "0.0.0.0:47800"],
    ["--mcp-connect", "127.0.0.1:1", "--headless"],
    ["--mcp-connect", "127.0.0.1:1", "--root", "."],
    ["--mcp-tools", "core"],
    ["--mcp", "--mcp-tools", "some"],
])
def test_bad_flags(args):
    p = subprocess.run([default_exe(), *args], capture_output=True, text=True, timeout=30, stdin=subprocess.DEVNULL)
    assert p.returncode == 2 and p.stderr.startswith("scotticad:"), (p.returncode, p.stderr)
