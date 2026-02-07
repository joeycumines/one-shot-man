#!/bin/sh

# ==============================================================================
# SCRIPT: run-on-windows.sh
# PURPOSE:
#   Executes a command on a remote Windows machine (running OpenSSH Server &
#   PowerShell 7) within a temporary, ephemeral clone of the current local
#   git repository state.
#
# BEHAVIOR:
#   1. SNAPSHOT: Captures the current local state using `git ls-files`.
#      - Includes: Committed files, Staged changes, Unstaged files.
#      - Excludes: Files ignored by .gitignore, the .git directory itself.
#   2. TRANSPORT: Pipes the state as a tarball over SSH to the remote host.
#   3. PROVISION: On Windows (via PowerShell):
#      - Creates a temporary directory in $env:TEMP.
#      - Translates the Windows path to a WSL path (using `wslpath`).
#   4. EXTRACTION: Uses remote `bash.exe` to untar the stream into the temp dir.
#   5. EXECUTION: Runs the user-provided arguments ("$@") inside that directory
#      using `bash.exe`.
#   6. CLEANUP: GUARANTEES removal of the temporary directory using a PowerShell
#      `try...finally` block, ensuring cleanup occurs even if execution fails.
#
# REQUIREMENTS:
#   Local:  sh, git, tar, ssh, sed, base64
#   Remote: OpenSSH Server, PowerShell 7 (default shell), WSL enabled (bash.exe)
#
# USAGE:
#   ./run-on-windows.sh <destination> [command] [arguments...]
#   DEBUG=1 ./run-on-windows.sh <destination> [command] [arguments...]  # Enable tracing
#
#   <destination> - The remote user@host (e.g., 'me@192.168.1.50' or just 'hostname')
#   [command]     - Optional command to run (defaults to 'ls -la')
#
#   Examples:
#     ./run-on-windows.sh user@winbox make test
#     ./run-on-windows.sh winbox make clean
#     ./run-on-windows.sh user@winbox bash -c "ls -la && ./build.sh"
#     ./run-on-windows.sh user@winbox
# ==============================================================================

set -e

# Enable debug tracing if DEBUG is set
if [ -n "$DEBUG" ]; then
  set -x
fi

# --- 1. Validation ---

if [ -z "$1" ]; then
  echo "Usage: $0 <destination> [command] [arguments...]" >&2
  echo "Example: $0 user@winbox make test" >&2
  exit 1
fi

SSH_HOST="$1"
shift

if [ ! -d ".git" ]; then
  echo "Error: Must be run from the root of a git repository." >&2
  exit 1
fi

# --- 2. Robust Argument Serialization (Base64) ---
# We serialize the arguments into a NUL-delimited stream, then Base64 encode it.
# This bypasses all quoting hell (single quotes, double quotes, $, etc.)
# and preserves full fidelity (including newlines).
if [ $# -gt 0 ]; then
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
else
  # If no command provided, default to listing the directory
  # Base64 for "ls" "-la"
  ARGS_B64=$(printf '%s\0' "ls" "-la" | base64 | tr -d '\n')
fi

# --- 3. Construct Remote PowerShell Payload ---
# SAFETY CRITICAL: This script must validate the WSL path before any extraction.
# If wslpath fails or returns empty, we MUST NOT extract the tarball.

REMOTE_PS_SCRIPT="
\$ErrorActionPreference = 'Stop';

# Create temporary directory
\$tempDir = New-Item -ItemType Directory -Path \"\$env:TEMP\osm-run-\$(New-Guid)\";
Write-Host \"[DEBUG] Created temp dir: \$tempDir.FullName\";

try {
    # Debug: enable tracing if DEBUG environment variable is set
    if ('\${DEBUG:-0}' -eq '1' -or '\${OSM_DEBUG:-0}' -eq '1') {
        Set-PSDebug -Trace 1;
        \$bashDebug = '-x';
        Write-Host '[DEBUG] Tracing enabled';
    } else {
        \$bashDebug = '';
    }

    # Convert Windows path to WSL path
    # CRITICAL: We must validate this path before using it for extraction
    \$env:WSL_TEMP_DIR = \$tempDir.FullName;

    # Get the environment variable value as a string for use in the bash command
    # We need to use $env:WSL_TEMP_DIR to read the environment variable we just set
    \$wsTempDir = \$env:WSL_TEMP_DIR;

    # Use double quotes in PowerShell so the variable is expanded, then escape for bash
    # The inner bash command will see: wslpath -u \"C:\\Users\\...\"
    # where the actual path is substituted by PowerShell
    \$wslPath = bash.exe \$bashDebug -c \"wslpath -u \\\"\`\$wsTempDir\`\\\"\";
    if (\$LASTEXITCODE -ne 0) {
      throw 'WSL path conversion failed (wslpath returned non-zero)';
    }
    if ([string]::IsNullOrEmpty(\$wslPath)) {
      throw 'WSL path conversion failed (got empty path)';
    }
    if (-not \$wslPath.StartsWith('/')) {
      throw \"WSL path conversion failed (not absolute: \$wslPath)\";
    }
    Write-Host \"[DEBUG] WSL path: \$wslPath\";

    # Decode and extract tarball - stdin is passed through to bash.exe
    # CRITICAL: -C requires a valid path. Empty string would extract to current dir!
    bash.exe \$bashDebug -c \"base64 -d | tar --warning=no-unknown-keyword -x -f - -C \\\"\$wslPath\\\"\";
    if (\$LASTEXITCODE -ne 0) { throw 'Tar extraction failed'; }

    # Execute user command in the extracted directory
    \$env:B64_ARGS = '$ARGS_B64';
    \$env:WSL_WORK_DIR = \$wslPath;

    # The bash script:
    # 1. Changes to the work directory
    # 2. Decodes the base64 arguments
    # 3. Executes them with sh -c
    bash.exe \$bashDebug -c \"cd \\\"\`\$WSL_WORK_DIR\`\\\" && echo \\\"\`\$B64_ARGS\`\\\" | base64 -d | xargs -0 sh -c 'exec \\\"\\\$@\\\"' --\";

    exit \$LASTEXITCODE;
}
catch {
    Write-Error \"\[ERROR\] \`$_.Exception.Message\";
    exit 1;
}
finally {
    # Cleanup: always remove the temp directory (if it was created)
    if (\$tempDir -and \$tempDir.PSPath) {
        Write-Host \"[DEBUG] Cleaning up: \$tempDir.FullName\";
        Remove-Item -Path \$tempDir -Recurse -Force -ErrorAction SilentlyContinue;
    }
}
"

# --- 4. Execution Pipeline ---
echo ">>> Syncing dirty state to $SSH_HOST..." >&2

# Check for pipefail support (POSIX sh doesn't always have it, but bash/zsh do)
if (set -o pipefail 2>/dev/null); then set -o pipefail; fi

# Encode the full script to Base64 to ensure safe transport over SSH.
# This prevents the 'unexpected EOF' and syntax errors caused by nested quotes
# within the SSH command arguments.
PS_PAYLOAD_B64=$(printf '%s' "$REMOTE_PS_SCRIPT" | base64 | tr -d '\n')

# Construct the remote wrapper to decode and execute.
# \$encoded holds the safe Base64 string.
# Invoke-Expression executes the decoded logic in the current scope,
# ensuring Standard Input is inherited by the inner bash commands.
# Pass DEBUG through SSH if set
if [ -n "$DEBUG" ]; then
  REMOTE_WRAPPER="\$env:DEBUG='1';\$encoded='$PS_PAYLOAD_B64';\$script=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$encoded));Invoke-Expression \$script"
else
  REMOTE_WRAPPER="\$encoded='$PS_PAYLOAD_B64';\$script=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$encoded));Invoke-Expression \$script"
fi

# Logic:
# 1. git ls-files: list files
# 2. tar: create archive
# 3. base64: Encode binary tar stream to text (avoids SSH/PS newline corruption)
# 4. ssh: pass text stream (and execute the Base64 wrapper)
git ls-files -c -o --exclude-standard -z |
  tar --null -T - -c -f - |
  base64 |
  ssh -T "$SSH_HOST" "$REMOTE_WRAPPER"
