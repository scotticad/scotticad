#!/usr/bin/env bash
# Builds the Windows release package on Linux (or macOS), by
# cross-compiling — ScottiCAD is pure Go, so no Windows machine is needed:
#   dist/scotticad-v<version>-windows-amd64.zip
# containing scotticad.exe, README.md, LICENSE, NOTICE and
# THIRD_PARTY_NOTICES.md, and prints its SHA-256 for the release notes.
#
#   ./release.sh           build and package
#   ./release.sh --test    run the tests that work off Windows first
#
# Needs Go (the version in go.mod or newer) and zip (or python3).
# File times inside the zip are set to the last commit's time, so the same
# commit always gives the same zip and the same SHA-256.
set -euo pipefail
cd "$(dirname "$0")"
export GOWORK=off  # one module: a go.work in a folder above mustn't take over the build

version=$(tr -d '[:space:]' < VERSION)
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    echo "VERSION does not hold a version number: '$version'" >&2
    exit 1
fi

if [[ ${1:-} == --test ]]; then
    # The geometry, document, editor and shape packages run anywhere. The
    # window, OpenGL and end-to-end tests need Windows; their code is at
    # least compiled for Windows here.
    go test ./pkg/doc/... ./pkg/editor/... ./pkg/filament/... ./pkg/geom/... ./pkg/script/... ./pkg/shapes/...
    GOOS=windows GOARCH=amd64 go build ./...
fi

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -trimpath -ldflags '-H windowsgui -s -w' -o scotticad.exe ./cmd/scotticad
echo "built scotticad.exe (Windows, amd64)"

name="scotticad-v$version-windows-amd64"
stage="dist/$name"
zipfile="dist/$name.zip"
rm -rf "$stage" "$zipfile"
mkdir -p "$stage"
files=(scotticad.exe README.md LICENSE NOTICE THIRD_PARTY_NOTICES.md)
cp "${files[@]}" "$stage/"

# fixed file times: the last commit's (or a fixed date outside a checkout)
epoch=$(git log -1 --format=%ct 2>/dev/null || echo 1767225600)
for f in "${files[@]}"; do
    touch -d "@$epoch" "$stage/$f" 2>/dev/null || touch -t "$(date -u -r "$epoch" +%Y%m%d%H%M.%S)" "$stage/$f"
done

if command -v zip >/dev/null; then
    (cd "$stage" && TZ=UTC zip -q -X -D "../$name.zip" "${files[@]}")
else
    "${PYTHON:-python3}" - "$stage" "$zipfile" "$epoch" "${files[@]}" <<'EOF'
import os, sys, time, zipfile
stage, out, epoch, files = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4:]
stamp = time.gmtime(max(epoch, 315532800))[:6]  # zip dates start in 1980
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for f in files:
        info = zipfile.ZipInfo(f, stamp)
        info.compress_type = zipfile.ZIP_DEFLATED
        info.external_attr = 0o644 << 16
        with open(os.path.join(stage, f), "rb") as fh:
            z.writestr(info, fh.read())
EOF
fi
rm -rf "$stage"

if command -v sha256sum >/dev/null; then
    hash=$(sha256sum "$zipfile" | cut -d' ' -f1)
else
    hash=$(shasum -a 256 "$zipfile" | cut -d' ' -f1)  # macOS
fi
echo
echo "Release package: $zipfile"
echo "Tag:             v$version"
echo "SHA-256:         $hash"
