param(
    [string]$Mode = "public",
    [switch]$Send
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$env:GOROOT = Join-Path $root "tools\go"
$env:Path = "$(Join-Path $env:GOROOT 'bin');$env:Path"

Push-Location $root
try {
    go test ./cmd/... ./internal/...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $args = @("./cmd/hedge", "-mode", $Mode)
    if ($Send) { $args += "-send" }
    go run @args
} finally {
    Pop-Location
}
