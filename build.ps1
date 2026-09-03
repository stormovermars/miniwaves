param(
    [ValidateSet("build", "clean")]
    [string]$Action = "build"
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$out = if ($env:MINIWAVES_BUILD_DIR) { $env:MINIWAVES_BUILD_DIR } else { Join-Path $root "build" }
$binary = Join-Path $out "mw.exe"

if ($Action -eq "clean") {
    if (Test-Path $binary) { Remove-Item -Force $binary }
    if ((Test-Path $out) -and -not (Get-ChildItem -Force $out)) { Remove-Item -Force $out }
    Write-Output "cleaned $out"
    exit 0
}

New-Item -ItemType Directory -Force -Path $out | Out-Null
Push-Location $root
try {
    go build -trimpath -o $binary .
} finally {
    Pop-Location
}
Write-Output "built $binary"
