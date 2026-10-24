# Builds the release packages for the version in VERSION:
#   dist\scotticad-v<version>-windows-amd64.zip
#       scotticad.exe, README.md, LICENSE, NOTICE, THIRD_PARTY_NOTICES.md
#   dist\scotticad-v<version>-linux-amd64.tar.gz
#   dist\scotticad-v<version>-linux-arm64.tar.gz
#       scotticad, install.sh, the menu entry, icon and file type, and the
#       same documents (cross-compiled: ScottiCAD is pure Go)
# and prints each one's SHA-256 for the release notes.
#   .\release.ps1          build and package
#   .\release.ps1 -Test    run all tests first (the Windows ones)
param([switch]$Test)
$ErrorActionPreference = 'Continue'
Set-Location $PSScriptRoot

$version = (Get-Content VERSION -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') {
    Write-Host "VERSION does not hold a version number: '$version'" -ForegroundColor Red
    exit 1
}

if ($Test) { & .\build.ps1 -Test } else { & .\build.ps1 }
if ($LASTEXITCODE -ne 0) { exit 1 }
$packages = @()
New-Item -ItemType Directory -Force dist | Out-Null

# entries dated by the last commit, as 'make release' does on Linux, so both
# give the same packages for the same commit
$epoch = git log -1 --format=%ct 2>$null
if (-not $epoch) { $epoch = '1767225600' }
$env:GOWORK = 'off'

# --- Windows: a zip ----------------------------------------------------------
$zip = "dist\scotticad-v$version-windows-amd64.zip"
go run ./tools/tarball -o $zip -mtime $epoch scotticad.exe README.md LICENSE NOTICE THIRD_PARTY_NOTICES.md
if ($LASTEXITCODE -ne 0) { Write-Host 'packaging failed' -ForegroundColor Red; exit 1 }
$packages += $zip

# --- Linux: a tarball per architecture ----------------------------------------
$env:CGO_ENABLED = '0'
$env:GOOS = 'linux'
foreach ($arch in 'amd64', 'arm64') {
    $name = "scotticad-v$version-linux-$arch"
    $stage = Join-Path 'dist' $name
    Remove-Item -Recurse -Force $stage, "dist\$name.tar.gz" -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force $stage | Out-Null
    $env:GOARCH = $arch
    go build -trimpath -ldflags '-s -w' -o "$stage/scotticad" ./cmd/scotticad
    $built = $LASTEXITCODE
    Remove-Item env:GOARCH
    if ($built -ne 0) { Remove-Item env:GOOS; Write-Host "building for Linux ($arch) failed" -ForegroundColor Red; exit 1 }
    Write-Host "built scotticad (Linux, $arch)"
    Remove-Item env:GOOS
    go run ./tools/tarball -o "dist/$name.tar.gz" -folder $name -mtime $epoch -exec scotticad,install.sh `
        "$stage/scotticad" packaging/linux/install.sh packaging/linux/scotticad.desktop packaging/linux/scotticad.svg `
        packaging/linux/scotticad-mime.xml README.md LICENSE NOTICE THIRD_PARTY_NOTICES.md
    if ($LASTEXITCODE -ne 0) { Write-Host 'packaging failed' -ForegroundColor Red; exit 1 }
    $env:GOOS = 'linux'
    Remove-Item -Recurse -Force $stage
    $packages += "dist\$name.tar.gz"
}
Remove-Item env:GOOS

Write-Host ''
Write-Host "Tag: v$version" -ForegroundColor Green
foreach ($p in $packages) {
    $hash = (Get-FileHash $p -Algorithm SHA256).Hash.ToLower()
    Write-Host ("{0,-48} SHA-256 {1}" -f $p, $hash)
}
