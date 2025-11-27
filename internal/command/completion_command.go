package command

import (
	"fmt"
	"io"
	"strings"
)

// CompletionCommand generates shell completion scripts.
type CompletionCommand struct {
	*BaseCommand
	registry     *Registry
	goalRegistry GoalRegistry
}

// NewCompletionCommand creates a new completion command.
func NewCompletionCommand(registry *Registry, goalRegistry GoalRegistry) *CompletionCommand {
	return &CompletionCommand{
		BaseCommand: NewBaseCommand(
			"completion",
			"Generate shell completion scripts",
			"completion [shell]",
		),
		registry:     registry,
		goalRegistry: goalRegistry,
	}
}

// Execute generates the completion script for the specified shell.
func (c *CompletionCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) > 1 {
		fmt.Fprintf(stderr, "Too many arguments: %v\n", args[1:])
		fmt.Fprintln(stderr, "Usage: osm completion [shell]")
		return fmt.Errorf("too many arguments")
	}

	shell := "bash"
	if len(args) > 0 {
		shell = args[0]
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

	goals := c.goalRegistry.List()
	goalList := strings.Join(goals, " ")

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
        goal)
            COMPREPLY=($(compgen -W "%s" -- ${cur}))
            return 0
            ;;
        session)
            COMPREPLY=($(compgen -W "list clean delete info" -- ${cur}))
            return 0
            ;;
        # For delete/info let shell default to filename completion (no session ids)
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
`, commandList, goalList)

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

	goals := c.goalRegistry.List()
	goalList := strings.Join(goals, "' '")

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
                goal)
                    _values 'goal-name' '%s'
                    ;;
                session)
                    if (( CURRENT == 3 )); then
                        _values 'session-subcommand' 'list' 'clean' 'delete' 'info'
                    else
                        _files
                    fi
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
`, commandDescriptions.String(), goalList)

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

	goals := c.goalRegistry.List()
	var goalCompletions strings.Builder
	for _, goalName := range goals {
		goal, err := c.goalRegistry.Get(goalName)
		if err == nil {
			// Escape single quotes in description to prevent shell injection
			escapedDesc := strings.ReplaceAll(goal.Description, "'", "'\\''")
			goalCompletions.WriteString(fmt.Sprintf("complete -c osm -n '__fish_seen_subcommand_from goal' -a '%s' -d '%s'\n",
				goalName, escapedDesc))
		}
	}

	var sessionCompletions strings.Builder
	sessionCompletions.WriteString("complete -c osm -n '__fish_seen_subcommand_from session' -a 'list clean delete info' -d 'Session subcommands'\n")

	script := fmt.Sprintf(`# Fish completion script for osm (one-shot-man)

# Complete commands
%s
# Completion for 'completion' subcommand args (shells)
complete -c osm -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell' -d 'Shell'

# Completion for 'goal' subcommand args (goal names)
%s
# Completion for 'session' subcommand
%s# Installation instructions (as comments):
# To install this completion script:
# 1. Copy this script to ~/.config/fish/completions/osm.fish
# 2. Or pipe it directly: osm completion fish > ~/.config/fish/completions/osm.fish
`, completions.String(), goalCompletions.String(), sessionCompletions.String())

	_, err := w.Write([]byte(script))
	return err
}

// generatePowerShellCompletion generates a PowerShell completion script.
func (c *CompletionCommand) generatePowerShellCompletion(w io.Writer) error {
	commands := c.registry.List()
	commandList := strings.Join(commands, "', '")

	goals := c.goalRegistry.List()
	goalList := strings.Join(goals, "', '")

	// No per-ID completions for session — only subcommand names are provided.

	script := fmt.Sprintf(`# PowerShell completion script for osm (one-shot-man)

Register-ArgumentCompleter -Native -CommandName osm -ScriptBlock {
    param($commandName, $wordToComplete, $cursorPosition)

    $line = $MyInvocation.Line.Substring(0, $cursorPosition)
    $tokens = $line.TrimStart().Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries)
    $tokenCount = $tokens.Length

    $commands = @('%s')
    $shells = @('bash', 'zsh', 'fish', 'powershell')
    $goals = @('%s')

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

    if ($tokenCount -eq 3 -and $command -eq 'goal') {
        $goals | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'session') {
        $subs = @('list','clean','delete','info')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
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
`, commandList, goalList)

	_, err := w.Write([]byte(script))
	return err
}
