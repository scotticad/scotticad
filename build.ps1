# Builds scotticad.exe (GUI subsystem, no console window) and optionally
# runs the tests.
#   .\build.ps1          build only
#   .\build.ps1 -Test    build, then run Go and Python tests
param([switch]$Test)
# Native tools report failure through their exit code; some (python -m venv)
# also print harmless warnings on stderr, which 'Stop' would treat as fatal.
$ErrorActionPreference = 'Continue'
$env:GOTOOLCHAIN = 'local'
$env:GOWORK = 'off'  # one module: a go.work in a folder above mustn't take over the build
Set-Location $PSScriptRoot

function Check([string]$what) {
    if ($LASTEXITCODE -ne 0) {
        Write-Host "$what failed" -ForegroundColor Red
        exit 1
    }
}

# -trimpath keeps the build machine's folders out of the executables
go build -trimpath -ldflags '-H windowsgui -s -w' -o scotticad.exe ./cmd/scotticad
Check 'go build'
Write-Host 'built scotticad.exe'

if ($Test) {
    go test ./pkg/...
    Check 'go test'
    if (-not (Test-Path .venv)) {
        # the py launcher finds a real Python 3 even where "python" is only
        # the Microsoft Store alias
        if (Get-Command py -ErrorAction SilentlyContinue) { py -3 -m venv .venv } else { python -m venv .venv }
        Check 'creating the Python virtual environment'
        .\.venv\Scripts\python.exe -m pip install --quiet pytest
        Check 'installing pytest'
    }
    .\.venv\Scripts\python.exe -m pytest python/tests -q
    Check 'the Python tests'
}
