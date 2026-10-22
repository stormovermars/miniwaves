# Build lock for job "@@JOB@@": run one heavy build or test at a time.
# Usage: ./buildlock.ps1 COMMAND [ARGS...]   (exit code = the command's; 75 = timed out waiting)
# Env: BUILDLOCK_DIR (lock folder), BUILDLOCK_TIMEOUT (seconds to wait, default 1800).
# The lock is a folder made with New-Item, which fails when it exists; a lock whose owner process is gone is taken over.
param([Parameter(Mandatory = $true, Position = 0)][string]$Command, [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)
$lock = if ($env:BUILDLOCK_DIR) { $env:BUILDLOCK_DIR } else { Join-Path ([System.IO.Path]::GetTempPath()) 'buildlock-@@JOB@@' }
$timeout = if ($env:BUILDLOCK_TIMEOUT) { [int]$env:BUILDLOCK_TIMEOUT } else { 1800 }
$waited = 0
while ($true) {
  try { New-Item -ItemType Directory -Path $lock -ErrorAction Stop | Out-Null; break } catch {}
  $owner = $null
  try { $owner = [int](Get-Content -Path (Join-Path $lock 'pid') -ErrorAction Stop) } catch {}
  if ($owner -and -not (Get-Process -Id $owner -ErrorAction SilentlyContinue)) {
    Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue
    continue
  }
  if ($waited -ge $timeout) {
    [Console]::Error.WriteLine("buildlock: timed out after ${timeout}s waiting for $lock")
    exit 75
  }
  Start-Sleep -Seconds 1
  $waited += 1
}
Set-Content -Path (Join-Path $lock 'pid') -Value $PID
$code = 0
try {
  & $Command @Rest
  if ($null -ne $LASTEXITCODE) { $code = $LASTEXITCODE }
} finally {
  Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue
}
exit $code
