# Shell Completion

`osm completion` generates shell completion scripts. Once installed, you can press **Tab** to complete:

- `osm` subcommands (e.g. `help`, `goal`, `script`, `prompt-flow`, `code-review`, `session`, `completion`, plus discovered script commands)
- flags (where supported by the shell completion implementation)
- some positional arguments (notably the `completion` subcommand’s shell name list)

You generate a completion script using:

```sh
osm completion [shell]
```

Supported shells: `bash` (default), `zsh`, `fish`, and `powershell` (alias: `pwsh`).

---

## Bash

### Installation

```bash
# Install system-wide (requires root)
osm completion bash | sudo tee /etc/bash_completion.d/osm >/dev/null

# Install for current user only
mkdir -p ~/.local/share/bash-completion/completions
osm completion bash >~/.local/share/bash-completion/completions/osm

# Or source directly in your ~/.bashrc
echo 'source <(osm completion bash)' >>~/.bashrc

# For brew + brew-installed bash users of interactive shells on MacOS
# (in your ~/.profile, OR, if it exists ~/.bash_profile, AFTER necessary PATH and HOMEBREW_PREFIX setup)
if [ -n "$BASH_VERSION" ]; then
  [[ -r "${HOMEBREW_PREFIX:-/opt/homebrew}/etc/profile.d/bash_completion.sh" ]] &&
    source "${HOMEBREW_PREFIX:-/opt/homebrew}/etc/profile.d/bash_completion.sh"

  # osm shell completion inclusive of hook to reload completion after changing directories
  if command -v osm &>/dev/null && source <(osm completion bash) && __osm_update_completion_on_cd() {
    if [[ "$PWD" != "${__osm_last_dir-}" ]]; then
      if command -v osm &>/dev/null; then
        source <(osm completion bash)
      fi
      __osm_last_dir="$PWD"
    fi
  } && ! [[ "$PROMPT_COMMAND" =~ __osm_update_completion_on_cd ]]; then
    if [ -z "$PROMPT_COMMAND" ]; then
      PROMPT_COMMAND="__osm_update_completion_on_cd"
    else
      PROMPT_COMMAND="${PROMPT_COMMAND}; __osm_update_completion_on_cd"
    fi
  fi
fi
```

### Usage

After installation (or after starting a new shell / sourcing your rc file), try:

```sh
osm <TAB> # shows available commands
osm he<TAB> # completes to "help"
osm completion <TAB> # shows: bash, zsh, fish, powershell
```

**Notes (bash-specific):**

- If you used the “source directly” approach, completion will be available in new interactive shells after you reload `~/.bashrc` or open a new terminal.
- The Homebrew macOS block includes a hook to refresh completion when your working directory changes (useful when `osm` dynamically discovers commands/goals based on the filesystem).

---

## Zsh

Zsh completion works via `compinit` and functions placed on your `fpath`.

### Installation

#### Install system-wide

If your system has a shared completion directory on `fpath`, you can install there (requires root). A common location is:

- `/usr/local/share/zsh/site-functions`
- or `/usr/share/zsh/site-functions`

Example (choose the one that exists on your system):

```zsh
# Install system-wide (requires root)
sudo mkdir -p /usr/local/share/zsh/site-functions
osm completion zsh | sudo tee /usr/local/share/zsh/site-functions/_osm >/dev/null
```

#### Install for current user only

```zsh
# Create completions directory if it doesn't exist
mkdir -p ~/.zsh/completions

# Install completion script
osm completion zsh >~/.zsh/completions/_osm

# Ensure these lines exist in your ~/.zshrc (add them if missing)
# Add completions directory to your function path
grep -q "fpath=(~/.zsh/completions $fpath)" ~/.zshrc || echo 'fpath=(~/.zsh/completions $fpath)' >>~/.zshrc
# Initialize completion system (only once)
grep -q "autoload -U compinit" ~/.zshrc || echo 'autoload -U compinit && compinit' >>~/.zshrc

# Optionally source directly for the current session
# source <(osm completion zsh)
```

### Usage

After installation and `compinit` is active (typically by opening a new shell, or running `autoload -U compinit && compinit`), try:

```zsh
osm <TAB> # shows available commands
osm he<TAB> # completes to "help"
osm completion <TAB> # shows: bash, zsh, fish, powershell
```

**Notes (zsh-specific):**

- Zsh completion requires `compinit` to be initialized (commonly done in `~/.zshrc`).
- Zsh loads completion functions from directories listed in `fpath`. Installing `_osm` into a directory on `fpath` is the key step.

---

## Fish

Fish loads completions from its completions directories automatically (user-local is the most common).

### Installation

#### Install system-wide

A typical system-wide completions directory for fish is:

- `/usr/share/fish/completions`

Example (requires root):

```fish
# Install system-wide (requires root)
sudo mkdir -p /usr/share/fish/completions
osm completion fish | sudo tee /usr/share/fish/completions/osm.fish >/dev/null
```

#### Install for current user only

```fish
# Install completion script
osm completion fish >~/.config/fish/completions/osm.fish
```

### Usage

Fish usually picks up new completion files automatically for new shells. For the current shell, you can restart fish (open a new terminal) or reload config.

Then try:

```fish
osm <TAB>            # shows available commands
osm he<TAB>          # completes to "help"
osm completion <TAB> # shows: bash, zsh, fish, powershell
```

**Notes (fish-specific):**

- Fish completion scripts live under `.../completions/*.fish` and are loaded by fish for interactive sessions.
- If completion doesn’t appear immediately, start a new fish session (new terminal tab/window).

---

## PowerShell

PowerShell completion is typically installed by adding it to your PowerShell profile, or loaded for a single session.

### Installation

#### Install for current user (persist via PowerShell profile)

```powershell
# Add to your PowerShell profile
osm completion powershell >> $PROFILE
```

#### Install for current session only

```powershell
# Or run directly in current session
osm completion powershell | Invoke-Expression
```

### Usage

After installation (or after loading it into the current session), try:

```powershell
osm <TAB>             # cycles through available commands
osm he<TAB>           # completes to "help"
osm completion <TAB>  # shows: bash, zsh, fish, powershell
```

**Notes (PowerShell-specific):**

- `$PROFILE` is the per-user profile script path PowerShell runs on startup (for the corresponding host/scope). Appending the completion script there enables completion in future sessions.
- `Invoke-Expression` loads completion into the current session only.
