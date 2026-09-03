$ErrorActionPreference = "Stop"

$repo = if ($env:MINIWAVES_REPO) { $env:MINIWAVES_REPO } else { "stormovermars/miniwaves" }
$dest = if ($env:MINIWAVES_INSTALL_DIR) { $env:MINIWAVES_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$version = if ($env:MINIWAVES_VERSION) { $env:MINIWAVES_VERSION } else { "latest" }
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "x86_64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}
$asset = "miniwaves_Windows_$arch.zip"
$base = if ($version -eq "latest") {
    "https://github.com/$repo/releases/latest/download"
} else {
    "https://github.com/$repo/releases/download/$version"
}
$temp = Join-Path ([IO.Path]::GetTempPath()) ("miniwaves-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Force -Path $temp, $dest | Out-Null
    $archive = Join-Path $temp $asset
    $checksums = Join-Path $temp "checksums.txt"
    Invoke-WebRequest -UseBasicParsing "$base/$asset" -OutFile $archive
    Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile $checksums
    $line = Get-Content $checksums | Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $line) { throw "Release checksum not found" }
    $expected = ($line -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "Release checksum mismatch" }
    Expand-Archive -Force $archive $temp
    Copy-Item -Force (Join-Path $temp "mw.exe") (Join-Path $dest "mw.exe")
    Write-Output "installed miniwaves to $(Join-Path $dest 'mw.exe')"
    if (($env:PATH -split ";") -notcontains $dest) {
        Write-Output "add $dest to PATH"
    }
} finally {
    if (Test-Path $temp) { Remove-Item -Recurse -Force $temp }
}
