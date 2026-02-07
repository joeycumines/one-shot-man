#!/bin/sh

# run-on-windows.sh - Execute command on Windows via WSL
# This script snapshots the git repo, transports it to Windows, and executes a command
# Usage: ./hack/run-on-windows.sh <host> [command] [args...]

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

# Serialize arguments (NUL-delimited)
if [ $# -gt 0 ]; then
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
else
  ARGS_B64=$(printf '%s\0' "ls" "-la" | base64 | tr -d '\n')
fi

# Create bash script that will run under bash.exe on Windows
# Gets args from file, reads tar from stdin
BASH_SCRIPT=$(mktemp -t osm-bash-XXXXXX)
WRAPPER_B64_FILE=$(mktemp -t osm-wrapper-XXXXXX)
trap "rm -f '$BASH_SCRIPT' '$WRAPPER_B64_FILE'" EXIT

cat > "$BASH_SCRIPT" << 'EOF'
#!/bin/bash
set -e
echo "[DEBUG] Bash script started" >&2

# Get args from file
ARGS_B64=$(cat args.b64)
if [ -z "$ARGS_B64" ]; then
  echo "[ERROR] ARGS_B64 not set" >&2
  exit 1
fi

# Create temp directory
TEMP_DIR="$(mktemp -d)"
echo "[DEBUG] Temp directory: $TEMP_DIR" >&2

# Cleanup trap
cleanup() {
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# Extract tar from stdin to temp directory
echo "[DEBUG] Extracting tar to $TEMP_DIR..." >&2
base64 -d | tar -x -C "$TEMP_DIR"

# Change to temp directory
cd "$TEMP_DIR"

# Decode and execute arguments
echo "$ARGS_B64" | base64 -d | xargs -0 sh -c 'exec "$@"' --
EOF

# Encode bash script
BASH_B64=$(base64 < "$BASH_SCRIPT" | tr -d '\n')

# Create simple PowerShell wrapper that:
# 1. Sets environment variables
# 2. Reads stdin (base64 tar)
# 3. Pipes stdin to bash.exe with the script
cat << 'PS_EOF' | base64 | tr -d '\n' > "$WRAPPER_B64_FILE"
$ErrorActionPreference = "Stop";
trap { Remove-Item -Recurse -Force $tempDir -ErrorAction SilentlyContinue };

# Create temp directory
$tempDir = Join-Path $env:TEMP "osm-run-$([Guid]::NewGuid())";
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null;
Write-Host "[DEBUG] Created temp dir: $tempDir";

# Decode bash script
$bashScriptPath = "$tempDir\script.sh";
$bashScriptContent = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($env:BASH_B64));
# Ensure LF line endings for bash
$bashScriptContent = $bashScriptContent.Replace("`r`n", "`n");
[System.IO.File]::WriteAllText($bashScriptPath, $bashScriptContent);
Write-Host "[DEBUG] Wrote bash script to: $bashScriptPath";
Write-Host "[DEBUG] Bash script content:`n$bashScriptContent";

# Write args to file
$argsPath = "$tempDir\args.b64";
[System.IO.File]::WriteAllText($argsPath, $env:ARGS_B64);

# Note: bash.exe on Windows doesn't require execute bit for scripts

# Get wsl.exe path (preferred over bash.exe)
$wslExe = "C:\Windows\system32\wsl.exe";
if (!(Test-Path $wslExe)) {
  [Console]::Error.WriteLine("[ERROR] wsl.exe not found");
  exit 1
}

# Resolve WSL path for temp dir using bash to call wslpath
$wslPathCmd = "wslpath -u '$tempDir'";
$wslTempDir = (& $wslExe --exec bash -c $wslPathCmd);

if ($null -eq $wslTempDir -or $wslTempDir -eq "") {
    [Console]::Error.WriteLine("[ERROR] wslpath returned empty/null");
    # Fallback to manual conversion assuming /mnt/c or /c
    # Try to detect mount point
    $mountPoint = (& $wslExe --exec bash -c "if [ -d /mnt/c ]; then echo /mnt/c; else echo /c; fi");
    $wslTempDir = $tempDir.Replace('\', '/').Replace('C:', $mountPoint).Replace('c:', $mountPoint);
    Write-Host "[DEBUG] Fallback WSL path: $wslTempDir";
} else {
    $wslTempDir = $wslTempDir.Trim();
}
Write-Host "[DEBUG] WSL temp dir: $wslTempDir";

# Read all stdin (base64 tar data)
$stdinData = [Console]::In.ReadToEnd();
Write-Host "[DEBUG] Read stdin length: $($stdinData.Length)";

# Execute: pipe stdin to wsl.exe
# The wsl.exe process will inherit the environment variable
$processInfo = New-Object System.Diagnostics.ProcessStartInfo;
$processInfo.FileName = $wslExe;
# Explicitly cd to the directory and run script
$processInfo.Arguments = "--exec bash -c 'cd `"$wslTempDir`" && bash script.sh'";
$processInfo.WorkingDirectory = $tempDir;
$processInfo.RedirectStandardInput = $true;
$processInfo.RedirectStandardOutput = $true;
$processInfo.RedirectStandardError = $true;
$processInfo.UseShellExecute = $false;
# ARGS_B64 is now in a file

$process = New-Object System.Diagnostics.Process;
$process.StartInfo = $processInfo;
$process.Start() | Out-Null;

# Write stdin to the process
try {
    $process.StandardInput.Write($stdinData);
    $process.StandardInput.Close();
} catch {
    [Console]::Error.WriteLine("[ERROR] Failed to write to stdin: $_");
    if ($process.HasExited) {
        $stderr = $process.StandardError.ReadToEnd();
        $stdout = $process.StandardOutput.ReadToEnd();
        [Console]::Error.WriteLine("[ERROR] Process exited early with code $($process.ExitCode). Stderr: $stderr");
        [Console]::Write("STDOUT: $stdout");
    }
}

# Read output BEFORE waiting to avoid deadlocks
$stdout = $process.StandardOutput.ReadToEnd();
$stderr = $process.StandardError.ReadToEnd();
$process.WaitForExit();
$exitCode = $process.ExitCode;
Write-Host "[DEBUG] Exit code: $exitCode";
[Console]::Write($stdout);
if ($stderr) { [Console]::Error.WriteLine($stderr) };

# Cleanup
Remove-Item -Recurse -Force $tempDir -ErrorAction SilentlyContinue;
PS_EOF

WRAPPER_B64=$(cat "$WRAPPER_B64_FILE")

echo ">>> Syncing to $SSH_HOST..." >&2

# Generate tar locally, pipe through SSH with wrapper script
# Capture: tracked files (working dir versions) + untracked files
# Note: Deleted files (working dir deleted but not staged) are filtered out
FILELIST=$(mktemp -t osm-filelist-XXXXXX)
trap "rm -f '$FILELIST'" EXIT

# Get all tracked files that exist in working directory
git ls-files -c -z | while IFS= read -r -d '' f; do
  [ -e "$f" ] && printf '%s\0' "$f"
done > "$FILELIST"

# Add untracked files
git ls-files -o --exclude-standard -z >> "$FILELIST"

# Validate file list is not empty
if [ ! -s "$FILELIST" ]; then
  echo "Error: No files to transfer" >&2
  exit 1
fi

# Construct the PowerShell command directly
PS_CMD="\$env:ARGS_B64='$ARGS_B64'; \$env:BASH_B64='$BASH_B64'; \$s = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('$WRAPPER_B64')); Invoke-Expression \$s"

# Encode the PowerShell command as base64 (UTF-16LE for EncodedCommand)
PS_CMD_B64=$(printf '%s' "$PS_CMD" | iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\n')

tar --null -T "$FILELIST" -c -f - | base64 | tr -d '\n' |
  ssh "$SSH_HOST" "pwsh -NoProfile -NonInteractive -EncodedCommand $PS_CMD_B64" || exit $?
