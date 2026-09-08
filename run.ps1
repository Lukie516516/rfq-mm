param(
    [switch]$Live,
    [switch]$TestOnly,
    [string]$Scenario = "all"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$env:GOROOT = Join-Path $root "tools\go"
$env:Path = "$(Join-Path $env:GOROOT 'bin');$env:Path"

if (-not (Test-Path (Join-Path $env:GOROOT "bin\go.exe"))) {
    Write-Error "Go not found at $env:GOROOT. Extract go.zip into tools\go first."
}

Push-Location $root
try {
    go test ./cmd/... ./internal/...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    if ($TestOnly) { exit 0 }
    $simArgs = @("./cmd/sim", "-scenario", $Scenario)
    if ($Live) { $simArgs += "-live" }
    go run @simArgs
} finally {
    Pop-Location
}
