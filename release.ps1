# Builds the release package for the version in VERSION:
#   dist\scotticad-v<version>-windows-amd64.zip
# containing scotticad.exe, README.md, LICENSE, NOTICE and
# THIRD_PARTY_NOTICES.md, and prints its SHA-256 for the release notes.
#   .\release.ps1          build and package
#   .\release.ps1 -Test    run all tests first
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

$name = "scotticad-v$version-windows-amd64"
$stage = Join-Path 'dist' $name
Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null
Copy-Item scotticad.exe, README.md, LICENSE, NOTICE, THIRD_PARTY_NOTICES.md $stage

$zip = Join-Path 'dist' "$name.zip"
Remove-Item -Force $zip -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
Remove-Item -Recurse -Force $stage

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
Write-Host ''
Write-Host "Release package: $zip" -ForegroundColor Green
Write-Host "Tag:             v$version"
Write-Host "SHA-256:         $hash"
