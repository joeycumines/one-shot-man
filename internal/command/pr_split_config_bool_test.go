package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestPrSplitApplyConfigDefaultsPreservesExplicitFalseFlags(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = map[string]string{
		"dry-run":            "true",
		"resume":             "true",
		"cleanup-on-failure": "true",
	}
	cmd := NewPrSplitCommand(cfg)
	cmd.applyConfigDefaults([]string{
		"--dry-run=false",
		"--resume=false",
		"--cleanup-on-failure=false",
	})
	if cmd.dryRun || cmd.resume || cmd.cleanupOnFailure {
		t.Fatalf("explicit false flags were overwritten: dryRun=%v resume=%v cleanupOnFailure=%v", cmd.dryRun, cmd.resume, cmd.cleanupOnFailure)
	}
}

func TestPrSplitApplyConfigDefaultsUsesCaseInsensitiveBooleans(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"on", "TRUE", "Yes"} {
		cfg := config.NewConfig()
		cfg.Commands["pr-split"] = map[string]string{
			"dry-run":            value,
			"resume":             value,
			"cleanup-on-failure": value,
		}

		cmd := NewPrSplitCommand(cfg)
		cmd.applyConfigDefaults(nil)
		if !cmd.dryRun || !cmd.resume || !cmd.cleanupOnFailure {
			t.Errorf("value %q: dryRun=%v resume=%v cleanupOnFailure=%v; want all true", value, cmd.dryRun, cmd.resume, cmd.cleanupOnFailure)
		}
	}
}
