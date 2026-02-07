#!/bin/sh

# run-on-windows.sh - Execute command on Windows via WSL
set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <host> [command] [args...]" >&2
  exit 1
fi

SSH_HOST="$1"
shift

if [ ! -d ".git" ]; then
  echo "Error: Must be run from git root" >&2
  exit 1
fi

# Serialize arguments
if [ $# -gt 0 ]; then
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
else
  ARGS_B64=$(printf '%s\0' "ls" "-la" | base64 | tr -d '\n')
fi

# Create PowerShell script using a temp file to avoid quoting issues
PS_FILE=$(mktemp)
trap "rm -f '$PS_FILE'" EXIT

cat > "$PS_FILE" << 'POWERSELLEND'
$ErrorActionPreference = "Stop"
if ($env:DEBUG -eq "1") { Set-PSDebug -Trace 1 }
$guid = [Guid]::NewGuid().ToString()
$tempDir = New-Item -ItemType Directory -Path "$env:TEMP\osm-run-$guid" -Force
try {
    $winPath = $tempDir.FullName
    $cmd = "wslpath -u ""$winPath"""
    $wslPath = bash.exe -c $cmd
    if ($LASTEXITCODE -ne 0) { throw "wslpath failed" }
    $wslPath = $wslPath.Trim()
    Write-Host "[DEBUG] WSL: $wslPath"
    $cmd = "base64 -d | tar -x -C ""$wslPath"""
    bash.exe -c $cmd
    if ($LASTEXITCODE -ne 0) { throw "tar failed" }
    $env:B64_ARGS = "@ARGS_B64@"
    $cmd = "cd ""$wslPath"" && echo ""$env:B64_ARGS"" | base64 -d | xargs -0 sh -c 'exec ""\$@""' --"
    bash.exe -c $cmd
}
catch {
    Write-Host "[ERROR] $_"
    exit 1
}
finally {
    Remove-Item -Path $tempDir.FullName -Recurse -Force -ErrorAction SilentlyContinue
}
POWERSELLEND

# Replace @ARGS_B64@ placeholder with actual value
sed "s/@ARGS_B64@/$ARGS_B64/g" "$PS_FILE" > "$PS_FILE.tmp"
mv "$PS_FILE.tmp" "$PS_FILE"

# Encode and execute
PS_B64=$(base64 < "$PS_FILE" | tr -d '\n')

if [ -n "$DEBUG" ]; then
  DEBUG_PREFIX='$env:DEBUG="1";'
else
  DEBUG_PREFIX=""
fi

echo ">>> Syncing to $SSH_HOST..." >&2
git ls-files -c -o --exclude-standard -z |
  tar --null -T - -c -f - |
  base64 |
  ssh -T "$SSH_HOST" "${DEBUG_PREFIX} \$script = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('$PS_B64')); Invoke-Expression \$script"
