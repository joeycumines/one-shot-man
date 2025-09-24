package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// CommitGenCommand generates commit messages based on git changes.
type CommitGenCommand struct {
	*BaseCommand
	staged     bool
	commit     string
	short      bool
	config     *config.Config
}

// NewCommitGenCommand creates a new commit-gen command.
func NewCommitGenCommand(cfg *config.Config) *CommitGenCommand {
	return &CommitGenCommand{
		BaseCommand: NewBaseCommand(
			"commit-gen",
			"Generate commit messages based on git changes",
			"commit-gen [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the commit-gen command.
func (c *CommitGenCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.staged, "staged", false, "Generate commit message for staged changes only")
	fs.StringVar(&c.commit, "commit", "", "Generate commit message for changes in specific commit (e.g., HEAD, HEAD~1)")
	fs.BoolVar(&c.short, "short", false, "Generate short commit message (single line)")
}

// Execute runs the commit-gen command.
func (c *CommitGenCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Get git diff based on options
	diffOutput, err := c.getGitDiff(ctx)
	if err != nil {
		return fmt.Errorf("failed to get git diff: %w", err)
	}

	if diffOutput == "" {
		_, _ = fmt.Fprintln(stdout, "No changes found to generate commit message.")
		return nil
	}

	// Generate commit message based on diff
	commitMsg, err := c.generateCommitMessage(diffOutput)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	_, _ = fmt.Fprint(stdout, commitMsg)
	return nil
}

// getGitDiff retrieves git diff based on the command options.
func (c *CommitGenCommand) getGitDiff(ctx context.Context) (string, error) {
	var args []string

	if c.staged {
		args = []string{"--cached"}
	} else if c.commit != "" {
		// For specific commit, show changes in that commit
		args = []string{c.commit + "^", c.commit}
	} else {
		// Use default logic similar to ctxutil
		args = c.getDefaultGitDiffArgs(ctx)
	}

	return c.runGitDiff(ctx, args)
}

// runGitDiff executes git diff command.
func (c *CommitGenCommand) runGitDiff(ctx context.Context, args []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	argv := append([]string{"diff"}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return string(output), nil
}

// getDefaultGitDiffArgs returns default args for git diff.
func (c *CommitGenCommand) getDefaultGitDiffArgs(ctx context.Context) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	// Check if HEAD~1 exists
	if err := exec.CommandContext(ctx, "git", "rev-parse", "-q", "--verify", "HEAD~1").Run(); err == nil {
		return []string{"HEAD~1"}
	}
	// Fallback: empty tree vs HEAD
	return []string{"4b825dc642cb6eb9a060e54bf8d69288fbee4904", "HEAD"}
}

// generateCommitMessage analyzes the diff and generates an appropriate commit message.
func (c *CommitGenCommand) generateCommitMessage(diff string) (string, error) {
	analysis := c.analyzeDiff(diff)

	if c.short {
		return c.generateShortMessage(analysis), nil
	}

	return c.generateDetailedMessage(analysis), nil
}

// DiffAnalysis contains the analysis of git diff.
type DiffAnalysis struct {
	FilesAdded    []string
	FilesModified []string
	FilesDeleted  []string
	LinesAdded    int
	LinesRemoved  int
	MainAction    string
	FileTypes     map[string]int
}

// analyzeDiff parses the git diff output and extracts relevant information.
func (c *CommitGenCommand) analyzeDiff(diff string) *DiffAnalysis {
	analysis := &DiffAnalysis{
		FilesAdded:    []string{},
		FilesModified: []string{},
		FilesDeleted:  []string{},
		FileTypes:     make(map[string]int),
	}

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Extract filename from diff --git a/file b/file
			re := regexp.MustCompile(`diff --git a/([^\s]+) b/([^\s]+)`)
			if matches := re.FindStringSubmatch(line); len(matches) > 2 {
				filename := matches[2]
				analysis.addFileType(filename)
			}
		} else if strings.HasPrefix(line, "new file mode") {
			// Find the previous diff --git line to get filename
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.HasPrefix(lines[i], "diff --git") {
					re := regexp.MustCompile(`diff --git a/([^\s]+) b/([^\s]+)`)
					if matches := re.FindStringSubmatch(lines[i]); len(matches) > 2 {
						analysis.FilesAdded = append(analysis.FilesAdded, matches[2])
					}
					break
				}
			}
		} else if strings.HasPrefix(line, "deleted file mode") {
			// Find the previous diff --git line to get filename
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.HasPrefix(lines[i], "diff --git") {
					re := regexp.MustCompile(`diff --git a/([^\s]+) b/([^\s]+)`)
					if matches := re.FindStringSubmatch(lines[i]); len(matches) > 2 {
						analysis.FilesDeleted = append(analysis.FilesDeleted, matches[1])
					}
					break
				}
			}
		} else if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			// Modified files
			if !strings.Contains(line, "/dev/null") {
				re := regexp.MustCompile(`[+-]{3} [ab]/(.+)`)
				if matches := re.FindStringSubmatch(line); len(matches) > 1 {
					filename := matches[1]
					if !c.containsString(analysis.FilesAdded, filename) && 
					   !c.containsString(analysis.FilesDeleted, filename) &&
					   !c.containsString(analysis.FilesModified, filename) {
						analysis.FilesModified = append(analysis.FilesModified, filename)
					}
				}
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			analysis.LinesAdded++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			analysis.LinesRemoved++
		}
	}

	// Determine main action
	if len(analysis.FilesAdded) > len(analysis.FilesModified) && len(analysis.FilesAdded) > len(analysis.FilesDeleted) {
		analysis.MainAction = "add"
	} else if len(analysis.FilesDeleted) > len(analysis.FilesModified) {
		analysis.MainAction = "remove"
	} else if len(analysis.FilesModified) > 0 {
		analysis.MainAction = "update"
	} else {
		analysis.MainAction = "change"
	}

	return analysis
}

// addFileType adds file extension to the file types map.
func (analysis *DiffAnalysis) addFileType(filename string) {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		ext := parts[len(parts)-1]
		analysis.FileTypes[ext]++
	}
}

// containsString checks if a slice contains a string.
func (c *CommitGenCommand) containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// generateShortMessage creates a concise single-line commit message.
func (c *CommitGenCommand) generateShortMessage(analysis *DiffAnalysis) string {
	totalFiles := len(analysis.FilesAdded) + len(analysis.FilesModified) + len(analysis.FilesDeleted)
	
	if totalFiles == 0 {
		return "Update files\n"
	}

	var action string
	switch analysis.MainAction {
	case "add":
		if len(analysis.FilesAdded) == 1 {
			return fmt.Sprintf("Add %s\n", analysis.FilesAdded[0])
		}
		action = "Add"
	case "remove":
		if len(analysis.FilesDeleted) == 1 {
			return fmt.Sprintf("Remove %s\n", analysis.FilesDeleted[0])
		}
		action = "Remove"
	case "update":
		if len(analysis.FilesModified) == 1 {
			return fmt.Sprintf("Update %s\n", analysis.FilesModified[0])
		}
		action = "Update"
	default:
		action = "Change"
	}

	return fmt.Sprintf("%s %d files\n", action, totalFiles)
}

// generateDetailedMessage creates a detailed multi-line commit message.
func (c *CommitGenCommand) generateDetailedMessage(analysis *DiffAnalysis) string {
	var msg strings.Builder

	// Summary line
	msg.WriteString(c.generateShortMessage(analysis))

	totalFiles := len(analysis.FilesAdded) + len(analysis.FilesModified) + len(analysis.FilesDeleted)
	if totalFiles <= 1 {
		return msg.String() // Don't add details for single file changes
	}

	msg.WriteString("\n")

	// Details
	if len(analysis.FilesAdded) > 0 {
		msg.WriteString(fmt.Sprintf("- Added %d file(s):\n", len(analysis.FilesAdded)))
		for _, file := range analysis.FilesAdded {
			msg.WriteString(fmt.Sprintf("  + %s\n", file))
		}
	}

	if len(analysis.FilesModified) > 0 {
		msg.WriteString(fmt.Sprintf("- Modified %d file(s):\n", len(analysis.FilesModified)))
		for _, file := range analysis.FilesModified {
			msg.WriteString(fmt.Sprintf("  ~ %s\n", file))
		}
	}

	if len(analysis.FilesDeleted) > 0 {
		msg.WriteString(fmt.Sprintf("- Removed %d file(s):\n", len(analysis.FilesDeleted)))
		for _, file := range analysis.FilesDeleted {
			msg.WriteString(fmt.Sprintf("  - %s\n", file))
		}
	}

	if analysis.LinesAdded > 0 || analysis.LinesRemoved > 0 {
		msg.WriteString(fmt.Sprintf("\nChanges: +%d -%d lines\n", analysis.LinesAdded, analysis.LinesRemoved))
	}

	return msg.String()
}