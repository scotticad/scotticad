"""Reads published.txt, the list of published example designs.

Each line names a design: NAME stands for scotticad/NAME.scotticad, made by
the script star/NAME.star when there is one. A design made by a script of
another name, with some of its settings changed, says so after the name:

    raspberrypi/vault-case-pi4   from raspberrypi/vault-case   BOARD="pi4"

That design is what star/raspberrypi/vault-case.star makes with its line
BOARD = ... set to "pi4". A setting is a NAME=VALUE with no spaces; it
replaces the value of the script's own top-level NAME = line. # starts a
comment.
"""
import os
import re
from typing import Dict, List, NamedTuple, Optional

HERE = os.path.dirname(os.path.abspath(__file__))
LIST = os.path.join(HERE, "published.txt")


class Design(NamedTuple):
    name: str                  # "coasters", "raspberrypi/vault-case-pi4"
    script: Optional[str]      # the script that makes it, or None for an original design
    settings: Dict[str, str]   # the script's settings changed for it


def read(path: str = LIST) -> List[Design]:
    designs = []
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            words = line.split("#", 1)[0].split()
            if not words:
                continue
            name, rest = words[0], words[1:]
            if rest:
                if rest[0] != "from" or len(rest) < 2:
                    raise ValueError(f"{path}:{n}: expected NAME [from SCRIPT [SETTING=VALUE ...]]")
                script, settings = rest[1], {}
                for s in rest[2:]:
                    key, eq, value = s.partition("=")
                    if not eq or not re.fullmatch(r"[A-Za-z_]\w*", key) or not value:
                        raise ValueError(f"{path}:{n}: {s!r} is not SETTING=VALUE")
                    settings[key] = value
            else:
                own = os.path.join(HERE, "star", name + ".star")
                script, settings = (name if os.path.exists(own) else None), {}
            designs.append(Design(name, script, settings))
    return designs


def script_path(script: str) -> str:
    return os.path.join(HERE, "star", script + ".star")


def source(design: Design) -> str:
    """The text of the design's script with its settings changed."""
    with open(script_path(design.script), encoding="utf-8") as f:
        text = f.read()
    for key, value in design.settings.items():
        text, found = re.subn(rf'^({re.escape(key)}\s*=\s*)(?:"[^"\n]*"|[^#\n]*?)(\s*(?:#.*)?)$',
                              lambda m: m.group(1) + value + m.group(2), text, count=1, flags=re.M)
        if not found:
            raise ValueError(f"{design.script}.star has no top-level {key} = line to set")
    return text
