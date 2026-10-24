"""Soak test: one long random session (the torture test's operations) with
the program's memory watched. The caches have limits and the undo history
a depth, so after a warm-up the working set, the Go heap, the goroutines
and the cache sizes must stop growing, and a step must not get slower.

Optional: it takes minutes, so it runs only with SCOTTICAD_SOAK=1:

    SCOTTICAD_SOAK=1 python -m pytest python/tests/test_soak.py -s

SCOTTICAD_SOAK_STEPS (default 2000) and SCOTTICAD_SOAK_SEED (default 1)
set its length and its session."""
import ctypes
import ctypes.wintypes
import json
import os
import sys
import time

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from scotticad import launch  # noqa: E402
from test_torture import Session  # noqa: E402

pytestmark = pytest.mark.skipif(not os.environ.get("SCOTTICAD_SOAK"), reason="set SCOTTICAD_SOAK=1 to run the soak test")

STEPS = int(os.environ.get("SCOTTICAD_SOAK_STEPS", "2000"))
SEED = int(os.environ.get("SCOTTICAD_SOAK_SEED", "1"))


class _MemoryCounters(ctypes.Structure):
    _fields_ = [("cb", ctypes.wintypes.DWORD), ("PageFaultCount", ctypes.wintypes.DWORD),
                ("PeakWorkingSetSize", ctypes.c_size_t), ("WorkingSetSize", ctypes.c_size_t),
                ("QuotaPeakPagedPoolUsage", ctypes.c_size_t), ("QuotaPagedPoolUsage", ctypes.c_size_t),
                ("QuotaPeakNonPagedPoolUsage", ctypes.c_size_t), ("QuotaNonPagedPoolUsage", ctypes.c_size_t),
                ("PagefileUsage", ctypes.c_size_t), ("PeakPagefileUsage", ctypes.c_size_t)]


def working_set(pid):
    """The process's working set in bytes (Windows)."""
    PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
    h = ctypes.windll.kernel32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
    assert h, f"OpenProcess({pid}) failed"
    try:
        c = _MemoryCounters()
        c.cb = ctypes.sizeof(c)
        assert ctypes.windll.psapi.GetProcessMemoryInfo(h, ctypes.byref(c), c.cb)
        return c.WorkingSetSize
    finally:
        ctypes.windll.kernel32.CloseHandle(h)


class SoakSession(Session):
    """The torture session without the operation that exports the whole
    design and imports it back: that doubles the design each time, and a
    leak can only be told from a design growing if the design is bounded."""
    OPS = [(op, w) for op, w in Session.OPS if op.__name__ != "op_export"]


def test_long_session_stays_bounded(tmp_path):
    with launch(headless=True, width=1400, height=860) as cad:
        s = SoakSession(cad, SEED, tmp_path)
        every = max(25, STEPS // 20)
        samples = []  # (step, working set MB, heap MB, goroutines, meshes, triangles, bounds, seconds per step)
        t0 = time.time()
        for i in range(1, STEPS + 1):
            s.step()
            if i % every == 0:
                s.check()
                st = cad.stats()
                ws = working_set(cad.process.pid) / 2**20
                design = len(json.dumps(cad.get_document())) / 2**20
                samples.append((i, ws, st["heap_alloc"] / 2**20, st["goroutines"], st["meshes"], st["mesh_triangles"],
                                st["bounds"], (time.time() - t0) / every))
                print(f"step {i:5d}: working set {ws:7.1f} MB, heap {samples[-1][2]:7.1f} MB, goroutines {st['goroutines']:3d}, "
                      f"meshes {st['meshes']:5d} ({st['mesh_triangles']:8d} tris), bounds {st['bounds']:6d}, gpu {st['gpu_meshes']:4d}, "
                      f"undo {st['undo_depth']:3d}, design {cad.state()['node_count']:3d} nodes {design:6.2f} MB, "
                      f"{samples[-1][7] * 1000:6.1f} ms/step")
                t0 = time.time()
        assert len(json.dumps(cad.get_document())) < 50 * 2**20, "the design itself grew huge: the memory figures mean nothing"
        assert len(samples) >= 8, "too few samples to judge growth"
        # after the first quarter (the warm-up: caches filling, the history
        # reaching its depth) nothing may keep climbing
        warm = samples[len(samples) // 4]
        last = samples[-1]
        peak = max(x[1] for x in samples[len(samples) // 4:])
        assert last[1] < 1.5 * warm[1] + 100, f"working set grew from {warm[1]:.0f} MB to {last[1]:.0f} MB"
        assert peak < 1.5 * warm[1] + 150, f"working set peaked at {peak:.0f} MB after {warm[1]:.0f} MB warm"
        assert last[2] < 1.5 * warm[2] + 100, f"heap grew from {warm[2]:.0f} MB to {last[2]:.0f} MB"
        assert last[3] < warm[3] + 20, f"goroutines grew from {warm[3]} to {last[3]}"
        assert last[5] <= 3_000_000 and last[6] <= 200_000, "a cache passed its limit"
        # and a step takes no longer than it did (the program isn't bogging down)
        early = sorted(x[7] for x in samples[1:len(samples) // 2])
        late = sorted(x[7] for x in samples[len(samples) // 2:])
        assert late[len(late) // 2] < 2 * early[len(early) // 2] + 0.05, f"steps slowed from {early[len(early) // 2]:.3f} s to {late[len(late) // 2]:.3f} s"
