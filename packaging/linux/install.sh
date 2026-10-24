#!/usr/bin/env bash
# Installs ScottiCAD for the current user (no root needed), from the folder
# this script is in: the program in ~/.local/bin, and a menu entry, an icon
# and the .scotticad file type so designs open with a double click.
#   ./install.sh              install (or update)
#   ./install.sh --uninstall  remove what install.sh put in place
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
data="${XDG_DATA_HOME:-$HOME/.local/share}"
bin="$HOME/.local/bin"

refresh() {
    command -v update-mime-database >/dev/null && update-mime-database "$data/mime" >/dev/null 2>&1 || true
    command -v update-desktop-database >/dev/null && update-desktop-database "$data/applications" >/dev/null 2>&1 || true
    command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q -t "$data/icons/hicolor" >/dev/null 2>&1 || true
}

if [[ ${1:-} == --uninstall ]]; then
    rm -f "$bin/scotticad" "$data/applications/scotticad.desktop" \
        "$data/icons/hicolor/scalable/apps/scotticad.svg" "$data/mime/packages/scotticad.xml"
    refresh
    echo "ScottiCAD removed (your designs are untouched)."
    exit 0
fi

mkdir -p "$bin" "$data/applications" "$data/icons/hicolor/scalable/apps" "$data/mime/packages"
install -m 755 "$here/scotticad" "$bin/scotticad"
install -m 644 "$here/scotticad.svg" "$data/icons/hicolor/scalable/apps/scotticad.svg"
install -m 644 "$here/scotticad-mime.xml" "$data/mime/packages/scotticad.xml"
# the menu entry runs the installed program by its full path, so it works
# even where ~/.local/bin isn't on PATH
sed "s|^Exec=scotticad|Exec=$bin/scotticad|" "$here/scotticad.desktop" > "$data/applications/scotticad.desktop"
chmod 644 "$data/applications/scotticad.desktop"
refresh

echo "ScottiCAD installed: find it in your applications menu, or run $bin/scotticad"
case ":$PATH:" in
    *":$bin:"*) ;;
    *) echo "(add $bin to your PATH to run it as just 'scotticad')" ;;
esac
