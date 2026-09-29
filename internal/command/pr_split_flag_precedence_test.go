package command

import (
	"flag"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func parsedPrSplitCommand(t *testing.T, values map[string]string, args ...string) *PrSplitCommand {
	t.Helper()
	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = values
	cmd := NewPrSplitCommand(cfg)
	flags := flag.NewFlagSet("pr-split", flag.ContinueOnError)
	cmd.SetupFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	cmd.CaptureProvidedFlags(flags)
	cmd.applyConfigDefaults(flags.Args())
	return cmd
}

func TestPrSplitConfigDefaultsRespectParsedFlagPresence(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		args   []string
		check  func(*testing.T, *PrSplitCommand)
	}{
		{
			name:   "explicit false overrides true config",
			values: map[string]string{"dry-run": "true"},
			args:   []string{"--dry-run=false"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.dryRun {
					t.Fatal("dry-run = true, want explicit false")
				}
			},
		},
		{
			name:   "explicit true overrides false config",
			values: map[string]string{"dry-run": "false"},
			args:   []string{"--dry-run=true"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if !cmd.dryRun {
					t.Fatal("dry-run = false, want explicit true")
				}
			},
		},
		{
			name:   "omitted boolean accepts config",
			values: map[string]string{"dry-run": "true"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if !cmd.dryRun {
					t.Fatal("dry-run = false, want configured true")
				}
			},
		},
		{
			name:   "explicit default string overrides config",
			values: map[string]string{"strategy": "extension"},
			args:   []string{"--strategy=directory"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.strategy != "directory" {
					t.Fatalf("strategy = %q, want explicit default directory", cmd.strategy)
				}
			},
		},
		{
			name:   "omitted string accepts config",
			values: map[string]string{"strategy": "extension"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.strategy != "extension" {
					t.Fatalf("strategy = %q, want configured extension", cmd.strategy)
				}
			},
		},
		{
			name:   "explicit empty string overrides config",
			values: map[string]string{"verify": "make"},
			args:   []string{"--verify="},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.verifyCommand != "" {
					t.Fatalf("verify command = %q, want explicit empty string", cmd.verifyCommand)
				}
			},
		},
		{
			name:   "explicit default integer overrides config",
			values: map[string]string{"max": "20"},
			args:   []string{"--max=10"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.maxFiles != 10 {
					t.Fatalf("max files = %d, want explicit default 10", cmd.maxFiles)
				}
			},
		},
		{
			name:   "explicit zero duration overrides config",
			values: map[string]string{"timeout": "5s"},
			args:   []string{"--timeout=0"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.timeout != 0 {
					t.Fatalf("timeout = %v, want explicit zero", cmd.timeout)
				}
			},
		},
		{
			name:   "omitted duration accepts config",
			values: map[string]string{"timeout": "5s"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.timeout != 5*time.Second {
					t.Fatalf("timeout = %v, want 5s", cmd.timeout)
				}
			},
		},
		{
			name:   "explicit false overrides true resume config",
			values: map[string]string{"resume": "true"},
			args:   []string{"--resume=false"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.resume {
					t.Fatal("resume = true, want explicit false")
				}
			},
		},
		{
			name:   "explicit false overrides cleanup config",
			values: map[string]string{"cleanup-on-failure": "true"},
			args:   []string{"--cleanup-on-failure=false"},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if cmd.cleanupOnFailure {
					t.Fatal("cleanup-on-failure = true, want explicit false")
				}
			},
		},
		{
			name:   "explicit empty repeated argument overrides config",
			values: map[string]string{"agent-arg": "config-arg"},
			args:   []string{"--agent-arg="},
			check: func(t *testing.T, cmd *PrSplitCommand) {
				if len(cmd.agentArgs) != 1 || cmd.agentArgs[0] != "" {
					t.Fatalf("agent args = %v, want one explicit empty argument", cmd.agentArgs)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cmd := parsedPrSplitCommand(t, test.values, test.args...)
			test.check(t, cmd)
		})
	}
}

func TestPrSplitDispatchHookPopulatesProvidedFlags(t *testing.T) {
	// Regression: cmd/osm run() must call CaptureProvidedFlags after
	// Parse so config defaults cannot override explicit CLI flags.
	// Without the hook, providedFlags stays nil and flagWasProvided
	// scans already-stripped positional args, always returning false.
	// This test replays the exact dispatch sequence run() performs.
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = map[string]string{"dry-run": "false"}
	cmd := NewPrSplitCommand(cfg)
	flags := flag.NewFlagSet("pr-split", flag.ContinueOnError)
	cmd.SetupFlags(flags)
	if err := flags.Parse([]string{"--dry-run"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	// This is the run() hook: interface-assert then capture.
	capturer, ok := any(cmd).(interface{ CaptureProvidedFlags(*flag.FlagSet) })
	if !ok {
		t.Fatal("PrSplitCommand must implement CaptureProvidedFlags(*flag.FlagSet) for the dispatch hook")
	}
	capturer.CaptureProvidedFlags(flags)
	cmd.applyConfigDefaults(flags.Args())
	if !cmd.dryRun {
		t.Fatal("dry-run = false, want explicit --dry-run to beat config false")
	}
}
