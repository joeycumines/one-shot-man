package command

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
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
		_, _ = fmt.Fprintf(stderr, "Too many arguments: %v\n", args[1:])
		_, _ = fmt.Fprintln(stderr, "Usage: osm completion [shell]")
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
		_, _ = fmt.Fprintf(stderr, "Unsupported shell: %s\n", shell)
		_, _ = fmt.Fprintln(stderr, "Supported shells: bash, zsh, fish, powershell")
		return fmt.Errorf("unsupported shell: %s", shell)
	}
}

// configKeys returns a sorted list of all global configuration option keys
// from the default schema, for use in shell completion scripts.
func configKeys() []string {
	schema := config.DefaultSchema()
	opts := schema.GlobalOptions()
	keys := make([]string, len(opts))
	for i, o := range opts {
		keys[i] = o.Key
	}
	sort.Strings(keys)
	return keys
}

// generateBashCompletion generates a bash completion script.
func (c *CompletionCommand) generateBashCompletion(w io.Writer) error {
	commands := c.registry.List()
	commandList := strings.Join(commands, " ")

	goals := c.goalRegistry.List()
	goalList := strings.Join(goals, " ")

	keys := configKeys()
	configKeyList := strings.Join(keys, " ")

	script := fmt.Sprintf(`#!/bin/bash
# Bash completion script for osm

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
            COMPREPLY=($(compgen -W "paths %s" -- ${cur}))
            return 0
            ;;
        script)
            COMPREPLY=($(compgen -W "paths" -- ${cur}))
            return 0
            ;;
        session)
            COMPREPLY=($(compgen -W "list clean purge delete info path id" -- ${cur}))
            return 0
            ;;
        sync)
            COMPREPLY=($(compgen -W "save list load init push pull config-push config-pull" -- ${cur}))
            return 0
            ;;
        config)
            COMPREPLY=($(compgen -W "validate schema list diff reset %s" -- ${cur}))
            return 0
            ;;
        schema)
            COMPREPLY=($(compgen -W "--json" -- ${cur}))
            return 0
            ;;
        log)
            COMPREPLY=($(compgen -W "tail follow" -- ${cur}))
            return 0
            ;;
        claude-mux)
            COMPREPLY=($(compgen -W "status start stop submit" -- ${cur}))
            return 0
            ;;
        pr-split)
            COMPREPLY=($(compgen -W "--base --strategy --max --prefix --verify --dry-run --ai --provider --model --json --interactive --test --session --store --log-level --log-file" -- ${cur}))
            return 0
            ;;
        --strategy)
            COMPREPLY=($(compgen -W "directory directory-deep extension chunks auto" -- ${cur}))
            return 0
            ;;
        --provider)
            COMPREPLY=($(compgen -W "ollama claude-code" -- ${cur}))
            return 0
            ;;
        help)
            COMPREPLY=($(compgen -W "${commands}" -- ${cur}))
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
`, commandList, goalList, configKeyList)

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

	keys := configKeys()
	zshConfigParts := make([]string, len(keys))
	for i, k := range keys {
		zshConfigParts[i] = fmt.Sprintf("'%s'", k)
	}
	zshConfigKeyList := strings.Join(zshConfigParts, " ")

	script := fmt.Sprintf(`#compdef osm

# Zsh completion script for osm

_osm() {
    local state line
    typeset -A opt_args

    _arguments -C \
        '1: :->commands' \
        '*: :->args' && return 0

    local commands
    commands=(
%s    )

    case "$state" in
        commands)
            _describe 'commands' commands
            ;;
        args)
            # Argument completion based on selected subcommand
            case ${words[2]} in
                completion)
                    _values 'shell' 'bash' 'zsh' 'fish' 'powershell'
                    ;;
                goal)
                    _values 'goal-name' 'paths' '%s'
                    ;;
                script)
                    _values 'script-subcommand' 'paths'
                    ;;
                session)
                    if (( CURRENT == 3 )); then
                        _values 'session-subcommand' 'list' 'clean' 'purge' 'delete' 'info' 'path' 'id'
                    else
                        _files
                    fi
                    ;;
                sync)
                    _values 'sync-subcommand' 'save' 'list' 'load' 'init' 'push' 'pull' 'config-push' 'config-pull'
                    ;;
                config)
                    _values 'config-subcommand' 'validate' 'schema' 'list' 'diff' 'reset' %s
                    ;;
                schema)
                    _values 'schema-flag' '--json'
                    ;;
                log)
                    _values 'log-subcommand' 'tail' 'follow'
                    ;;
                claude-mux)
                    _values 'claude-mux-subcommand' 'status' 'start' 'stop' 'submit'
                    ;;
                pr-split)
                    _arguments \
                        '--base[Base branch]:branch:' \
                        '--strategy[Grouping strategy]:strategy:(directory directory-deep extension chunks auto)' \
                        '--max[Max files per split]:number:' \
                        '--prefix[Branch prefix]:prefix:' \
                        '--verify[Verify command]:command:' \
                        '--dry-run[Show plan without executing]' \
                        '--ai[Use AI classification]' \
                        '--provider[AI provider]:provider:(ollama claude-code)' \
                        '--model[Model identifier]:model:' \
                        '--json[Output results as JSON]'
                    ;;
                help)
                    _describe 'commands' commands
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
`, commandDescriptions.String(), goalList, zshConfigKeyList)

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
	sessionCompletions.WriteString("complete -c osm -n '__fish_seen_subcommand_from session' -a 'list clean purge delete info path id' -d 'Session subcommands'\n")

	keys := configKeys()
	configKeyList := strings.Join(keys, " ")

	commandList := strings.Join(commands, " ")

	script := fmt.Sprintf(`# Fish completion script for osm

# Complete commands
%s
# Completion for 'completion' subcommand args (shells)
complete -c osm -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell' -d 'Shell'

# Completion for 'goal' subcommand args (goal names)
complete -c osm -n '__fish_seen_subcommand_from goal' -a 'paths' -d 'Show discovery paths'
%s
# Completion for 'script' subcommand args
complete -c osm -n '__fish_seen_subcommand_from script' -a 'paths' -d 'Show discovery paths'
# Completion for 'session' subcommand
%s
# Completion for 'sync' subcommand
complete -c osm -n '__fish_seen_subcommand_from sync' -a 'save list load init push pull config-push config-pull' -d 'Sync subcommands'

# Completion for 'config' subcommand
complete -c osm -n '__fish_seen_subcommand_from config' -a 'validate schema list diff reset %s' -d 'Config subcommands'

# Completion for 'config schema' --json flag
complete -c osm -n '__fish_seen_subcommand_from schema' -a '--json' -d 'Output schema as JSON'

# Completion for 'log' subcommand
complete -c osm -n '__fish_seen_subcommand_from log' -a 'tail follow' -d 'Log subcommands'

# Completion for 'claude-mux' subcommand
complete -c osm -n '__fish_seen_subcommand_from claude-mux' -a 'status start stop submit' -d 'Claude-mux subcommands'

# Completion for 'pr-split' flags
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l base -d 'Base branch'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l strategy -a 'directory directory-deep extension chunks auto' -d 'Grouping strategy'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l max -d 'Max files per split'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l prefix -d 'Branch prefix'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l verify -d 'Verify command'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l dry-run -d 'Show plan without executing'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l ai -d 'Use AI classification'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l provider -a 'ollama claude-code' -d 'AI provider'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l model -d 'Model identifier'
complete -c osm -n '__fish_seen_subcommand_from pr-split' -l json -d 'Output results as JSON'

# Completion for 'help' subcommand (command names)
complete -c osm -n '__fish_seen_subcommand_from help' -a '%s' -d 'Command'

# Installation instructions (as comments):
# To install this completion script:
# 1. Copy this script to ~/.config/fish/completions/osm.fish
# 2. Or pipe it directly: osm completion fish > ~/.config/fish/completions/osm.fish
`, completions.String(), goalCompletions.String(), sessionCompletions.String(), configKeyList, commandList)

	_, err := w.Write([]byte(script))
	return err
}

// generatePowerShellCompletion generates a PowerShell completion script.
func (c *CompletionCommand) generatePowerShellCompletion(w io.Writer) error {
	commands := c.registry.List()
	commandList := strings.Join(commands, "', '")

	goals := c.goalRegistry.List()
	goalList := strings.Join(goals, "', '")

	keys := configKeys()
	psConfigParts := make([]string, len(keys))
	for i, k := range keys {
		psConfigParts[i] = fmt.Sprintf("'%s'", k)
	}
	psConfigKeyList := strings.Join(psConfigParts, ",")

	// No per-ID completions for session — only subcommand names are provided.

	script := fmt.Sprintf(`# PowerShell completion script for osm

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
        @('paths') + $goals | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'script') {
        $subs = @('paths')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'session') {
    $subs = @('list','clean','purge','delete','info','path','id')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'sync') {
        $subs = @('save','list','load','init','push','pull','config-push','config-pull')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'config') {
        $subs = @('validate','schema','list','diff','reset',%s)
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 4 -and $command -eq 'config') {
        $sub2 = if ($tokens.Count -ge 3) { $tokens[2] } else { '' }
        if ($sub2 -eq 'schema') {
            $flags = @('--json')
            $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
            return
        }
    }

    if ($tokenCount -eq 3 -and $command -eq 'log') {
        $subs = @('tail','follow')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'claude-mux') {
        $subs = @('status','start','stop','submit')
        $subs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    if ($tokenCount -eq 3 -and $command -eq 'help') {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
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
`, commandList, goalList, psConfigKeyList)

	_, err := w.Write([]byte(script))
	return err
}
