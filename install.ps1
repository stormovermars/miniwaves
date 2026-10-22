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
    # Put $dest on the user PATH (persisted for new shells, and for this one).
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @($userPath -split ";" | Where-Object { $_ })
    if (-not $env:MINIWAVES_NO_PATH -and ($parts | ForEach-Object { $_.TrimEnd("\") }) -notcontains $dest.TrimEnd("\")) {
        [Environment]::SetEnvironmentVariable("Path", (($parts + $dest) -join ";"), "User")
        Write-Output "added $dest to the user PATH (open a new terminal to use mw)"
    }
    if (($env:PATH -split ";") -notcontains $dest) { $env:PATH = "$env:PATH;$dest" }
    # Claude Code plugins: best effort, never fails the binary install. MW_NO_PLUGINS=1 skips.
    $ErrorActionPreference = "Continue"
    if ($env:MW_NO_PLUGINS -eq "1") {
        Write-Output "plugins: skipped (MW_NO_PLUGINS=1)"
    } elseif (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
        Write-Output "plugins: claude not found; later run: mw update --plugins"
    } else {
        try {
            & claude plugin marketplace add $repo *> $null
            $mo = (& claude plugin marketplace update miniwaves 2>&1 | Out-String).Trim()
            if ($LASTEXITCODE -ne 0) {
                Write-Output ("plugins: failed: marketplace update: " + ($mo -split "`n" | Select-Object -Last 1))
            } else {
                $listing = (& claude plugin list 2>&1 | Out-String)
                foreach ($p in "mw", "tidewatch") {
                    $id = "$p@miniwaves"
                    $cmd = if ($listing -match "(?m)\s$([regex]::Escape($id))\s*$") { "update" } else { "install" }
                    $po = (& claude plugin $cmd $id 2>&1 | Out-String).Trim()
                    if ($LASTEXITCODE -eq 0) {
                        Write-Output ("plugin ${p}: " + $(if ($cmd -eq "update") { "updated" } else { "installed" }))
                    } else {
                        Write-Output ("plugin ${p}: failed: " + ($po -split "`n" | Select-Object -Last 1))
                    }
                }
            }
        } catch {
            Write-Output "plugins: failed: $($_.Exception.Message)"
        }
    }
} finally {
    if (Test-Path $temp) { Remove-Item -Recurse -Force $temp }
}
