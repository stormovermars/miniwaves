$ErrorActionPreference = "Stop"

$dest = if ($env:MINIWAVES_INSTALL_DIR) { $env:MINIWAVES_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$binary = Join-Path $dest "mw.exe"
if (Test-Path $binary -PathType Leaf) {
    Remove-Item -Force $binary
    Write-Output "removed $binary"
} else {
    Write-Output "miniwaves is not installed at $binary"
}
