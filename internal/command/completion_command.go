package command

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// CompletionCommand generates shell completion scripts.
type CompletionCommand struct {
	*BaseCommand
	registry *Registry
	shell    string
}

// NewCompletionCommand creates a new completion command.
func NewCompletionCommand(registry *Registry) *CompletionCommand {
	return &CompletionCommand{
		BaseCommand: NewBaseCommand(
			"completion",
			"Generate shell completion scripts",
			"completion [shell]",
		),
		registry: registry,
	}
}

// SetupFlags configures the flags for the completion command.
func (c *CompletionCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.shell, "shell", "", "Shell to generate completion for (bash, zsh, fish, powershell)")
}

// Execute generates the completion script for the specified shell.
func (c *CompletionCommand) Execute(args []string, stdout, stderr io.Writer) error {
	shell := c.shell
	if shell == "" && len(args) > 0 {
		shell = args[0]
	}

	if shell == "" {
		shell = "bash" // Default to bash
	}

	shell = strings.ToLower(shell)

	switch shell {
	case "bash":
		return c.generateBashCompletion(stdout)
	case "zsh":
		return c.generateZshCompletion(stdout)
	case "fish":
		return c.generateFishCompletion(stdout)
	case "powershell", "pwsh":
		return c.generatePowerShellCompletion(stdout)
	default:
		fmt.Fprintf(stderr, "Unsupported shell: %s\n", shell)
		fmt.Fprintln(stderr, "Supported shells: bash, zsh, fish, powershell")
		return fmt.Errorf("unsupported shell: %s", shell)
	}
}

// generateBashCompletion generates a bash completion script.
func (c *CompletionCommand) generateBashCompletion(w io.Writer) error {
	commands := c.registry.List()
	commandList := strings.Join(commands, " ")

	script := fmt.Sprintf(`#!/bin/bash
# Bash completion script for osm (one-shot-man)

_osm_completion() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Available commands
    commands="%s"

    # Complete first argument (command name)
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- ${cur}))
        return 0
    fi

	# For subsequent arguments, provide per-command completions
	case "${prev}" in
		completion)
			COMPREPLY=($(compgen -W "bash zsh fish powershell" -- ${cur}))
			return 0
			;;
		*)
			COMPREPLY=($(compgen -f -- ${cur}))
			return 0
			;;
	esac
}

# Register the completion function
complete -F _osm_completion osm

# Installation instructions (as comments):
# To install this completion script:
# 1. Copy this script to /etc/bash_completion.d/osm (system-wide)
#    or ~/.local/share/bash-completion/completions/osm (user-specific)
# 2. Or source it directly in your ~/.bashrc:
#    source <(osm completion bash)
`, commandList)

	_, err := w.Write([]byte(script))
	return err
}

// generateZshCompletion generates a zsh completion script.
func (c *CompletionCommand) generateZshCompletion(w io.Writer) error {
	commands := c.registry.List()
	var commandDescriptions strings.Builder

	for _, cmd := range commands {
		if command, err := c.registry.Get(cmd); err == nil {
			commandDescriptions.WriteString(fmt.Sprintf("    '%s:%s'\n", cmd, command.Description()))
		}
	}

	script := fmt.Sprintf(`#compdef osm

# Zsh completion script for osm (one-shot-man)

_osm() {
    local state line
    typeset -A opt_args

    _arguments -C \
        '1: :->commands' \
        '*: :->args' && return 0

    case "$state" in
        commands)
            local commands
            commands=(
%s            )
            _describe 'commands' commands
            ;;
		args)
			# Argument completion based on selected subcommand
			case ${words[2]} in
				completion)
					_values 'shell' 'bash' 'zsh' 'fish' 'powershell'
					;;
				*)
					_files
					;;
			esac
			;;
    esac
}

_osm "$@"

# Installation instructions (as comments):
# To install this completion script:
# 1. Copy this script to a directory in your $fpath (e.g., ~/.zsh/completions/)
# 2. Make sure the directory is in your $fpath in ~/.zshrc:
#    fpath=(~/.zsh/completions $fpath)
# 3. Regenerate completions: rm ~/.zcompdump && compinit
# 4. Or source it directly: source <(osm completion zsh)
`, commandDescriptions.String())

	_, err := w.Write([]byte(script))
	return err
}

// generateFishCompletion generates a fish completion script.
func (c *CompletionCommand) generateFishCompletion(w io.Writer) error {
	commands := c.registry.List()
	var completions strings.Builder

	for _, cmd := range commands {
		if command, err := c.registry.Get(cmd); err == nil {
			completions.WriteString(fmt.Sprintf("complete -c osm -n '__fish_use_subcommand' -a '%s' -d '%s'\n",
				cmd, command.Description()))
		}
	}

	script := fmt.Sprintf(`# Fish completion script for osm (one-shot-man)

# Complete commands
%s
# Completion for 'completion' subcommand args (shells)
complete -c osm -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell' -d 'Shell'
# Installation instructions (as comments):
# To install this completion script:
# 1. Copy this script to ~/.config/fish/completions/osm.fish
# 2. Or pipe it directly: osm completion fish > ~/.config/fish/completions/osm.fish
`, completions.String())

	_, err := w.Write([]byte(script))
	return err
}

// generatePowerShellCompletion generates a PowerShell completion script.
func (c *CompletionCommand) generatePowerShellCompletion(w io.Writer) error {
	commands := c.registry.List()
	commandList := strings.Join(commands, "', '")

	script := fmt.Sprintf(`# PowerShell completion script for osm (one-shot-man)

Register-ArgumentCompleter -Native -CommandName osm -ScriptBlock {
	param($commandName, $wordToComplete, $cursorPosition)

	$line = $MyInvocation.Line.Substring(0, $cursorPosition)
	$tokens = $line.TrimStart().Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries)
	$tokenCount = $tokens.Length

	$commands = @('%s')
	$shells = @('bash', 'zsh', 'fish', 'powershell')

	if ($line.TrimEnd().EndsWith(' ')) {
		$tokenCount++
	}

	# Completing the command name (token 2: first arg after command)
	if ($tokenCount -le 2) {
		$commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
			[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
		}
		return
	}

	$command = if ($tokens.Count -ge 2) { $tokens[1] } else { '' }

	if ($tokenCount -eq 3 -and $command -eq 'completion') {
		$shells | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
			[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
		}
		return
	}

	# Default to file completion for other commands
	if ($command -ne 'completion') {
		Get-ChildItem -Path . -Name "$wordToComplete*" | ForEach-Object {
			[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
		}
	}
}

# Installation instructions (as comments):
# To install this completion script:
# 1. Add the above code to your PowerShell profile
# 2. Find your profile location with: $PROFILE
# 3. Or run directly: osm completion powershell | Invoke-Expression
`, commandList)

	_, err := w.Write([]byte(script))
	return err
}
