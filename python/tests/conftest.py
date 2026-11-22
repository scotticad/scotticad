"""Shared test fixtures."""
import struct
import zlib

import pytest


def write_png(path, width, height):
    """A plain RGB PNG: light paper with a dark frame and cross, so a
    picture to trace has something on it. Standard library only."""
    rows = []
    for y in range(height):
        row = bytearray(b"\x00")  # filter: none
        for x in range(width):
            edge = x < 4 or y < 4 or x >= width - 4 or y >= height - 4
            cross = abs(x - width // 2) < 2 or abs(y - height // 2) < 2
            row += b"\x30\x30\x30" if edge or cross else b"\xf0\xec\xe4"
        rows.append(bytes(row))

    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)

    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n")
        f.write(chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)))
        f.write(chunk(b"IDAT", zlib.compress(b"".join(rows))))
        f.write(chunk(b"IEND", b""))


@pytest.fixture(scope="session")
def picture(tmp_path_factory):
    """A portrait picture to trace (300 x 600 pixels), as a PNG file."""
    path = tmp_path_factory.mktemp("pictures") / "trace.png"
    write_png(path, 300, 600)
    return str(path)
