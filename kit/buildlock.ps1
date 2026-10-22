# Build lock for job "@@JOB@@": run one heavy build or test at a time.
# Usage: ./buildlock.ps1 COMMAND [ARGS...]   (exit code = the command's; 75 = timed out waiting)
# Env: BUILDLOCK_DIR (lock folder), BUILDLOCK_TIMEOUT (seconds to wait, default 1800).
# The lock is a folder published by one rename (Directory.Move fails when the target exists, so exactly one waiter wins, and
# the pid file is already inside when the lock appears); a lock whose owner process is gone is taken over, one waiter at a time.
# No param() block: it would make this an advanced script and bind the wrapped command's -v, -o, -e to common parameters.
if ($args.Count -lt 1) { [Console]::Error.WriteLine('usage: buildlock.ps1 COMMAND [ARGS...]'); exit 2 }
$Command = $args[0]
$Rest = @($args | Select-Object -Skip 1)
$lock = if ($env:BUILDLOCK_DIR) { $env:BUILDLOCK_DIR } else { Join-Path ([System.IO.Path]::GetTempPath()) 'buildlock-@@JOB@@' }
$timeout = if ($env:BUILDLOCK_TIMEOUT) { [int]$env:BUILDLOCK_TIMEOUT } else { 1800 }
$waited = 0
# New-Item is check-then-create and can succeed for two waiters at once; a rename onto an existing, non-empty folder cannot.
function New-Claim([string]$Path) {
  $tmp = "$Path.$PID.tmp"
  try {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    [void][System.IO.Directory]::CreateDirectory($tmp)
    [System.IO.File]::WriteAllText((Join-Path $tmp 'pid'), [string]$PID)
    [System.IO.Directory]::Move($tmp, $Path)
    return $true
  } catch {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    return $false
  }
}
while ($true) {
  if (New-Claim $lock) { break }
  $owner = $null
  try { $owner = [int](Get-Content -Path (Join-Path $lock 'pid') -ErrorAction Stop) } catch {}
  if ($owner -and -not (Get-Process -Id $owner -ErrorAction SilentlyContinue)) {
    # only the holder of $lock.takeover removes a stale lock, so the lock cannot be replaced under us;
    # re-read the owner first, another waiter may have finished the takeover already
    $mutex = "$lock.takeover"
    if (New-Claim $mutex) {
      $now = $null
      try { $now = [int](Get-Content -Path (Join-Path $lock 'pid') -ErrorAction Stop) } catch {}
      if ($now -eq $owner) { Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue }
      Remove-Item -Recurse -Force $mutex -ErrorAction SilentlyContinue
      continue
    }
  }
  # a takeover folder left by a crashed waiter is cleared after a minute
  if ((Test-Path "$lock.takeover") -and ((Get-Date) - (Get-Item "$lock.takeover").LastWriteTime).TotalSeconds -gt 60) { Remove-Item -Recurse -Force "$lock.takeover" -ErrorAction SilentlyContinue }
  if ($waited -ge $timeout) {
    [Console]::Error.WriteLine("buildlock: timed out after ${timeout}s waiting for $lock")
    exit 75
  }
  Start-Sleep -Seconds 1
  $waited += 1
}
$code = 0
try {
  & $Command @Rest
  if ($null -ne $LASTEXITCODE) { $code = $LASTEXITCODE }
} finally {
  Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue
}
exit $code
