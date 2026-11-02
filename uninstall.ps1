$ErrorActionPreference = "Stop"

# mw and wr install together, so they uninstall together.
$dest = if ($env:MINIWAVES_INSTALL_DIR) { $env:MINIWAVES_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
foreach ($b in "mw", "wr") {
    $binary = Join-Path $dest "$b.exe"
    if (Test-Path $binary -PathType Leaf) {
        Remove-Item -Force $binary
        Write-Output "removed $binary"
    } else {
        Write-Output "$b is not installed at $binary"
    }
}
