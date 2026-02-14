package command

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// SyncCommand provides local notebook save/list operations and git-based
// sync (init/push/pull). See docs/archive/notes/git-sync-design.md.
type SyncCommand struct {
	*BaseCommand

	// config provides access to sync.* configuration keys.
	config *config.Config

	// syncDir is the local directory used for storing notebook entries.
	// When set, overrides the default (syncRoot/notebooks). Used by tests.
	syncDir string

	// TimeNow returns the current time. Override in tests for determinism.
	TimeNow func() time.Time

	// GitBin overrides the git binary path. Empty uses "git".
	GitBin string
}

// NewSyncCommand creates a new sync command. cfg provides access to sync.*
// config keys. An optional syncDir argument overrides the default notebook
// directory (useful for tests).
func NewSyncCommand(cfg *config.Config, syncDir ...string) *SyncCommand {
	cmd := &SyncCommand{
		BaseCommand: NewBaseCommand(
			"sync",
			"Save and list prompt notebook entries; sync via git",
			"sync <save|list|init|push|pull> [options]",
		),
		config: cfg,
	}
	if len(syncDir) > 0 && syncDir[0] != "" {
		cmd.syncDir = syncDir[0]
	}
	return cmd
}

// SetupFlags registers command-level flags.
func (c *SyncCommand) SetupFlags(fs *flag.FlagSet) {
	// No top-level flags; subcommands handle their own.
}

// Execute dispatches to the appropriate subcommand.
func (c *SyncCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: osm sync <save|list|init|push|pull>")
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "Subcommands:")
		_, _ = fmt.Fprintln(stderr, "  save   Save a prompt notebook entry")
		_, _ = fmt.Fprintln(stderr, "  list   List saved notebook entries")
		_, _ = fmt.Fprintln(stderr, "  init   Clone a sync repository")
		_, _ = fmt.Fprintln(stderr, "  push   Commit and push local changes")
		_, _ = fmt.Fprintln(stderr, "  pull   Fetch and merge remote changes")
		return fmt.Errorf("no subcommand specified")
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "save":
		return c.executeSave(args[1:], stdout, stderr)
	case "list":
		return c.executeList(args[1:], stdout, stderr)
	case "init":
		return c.executeInit(args[1:], stdout, stderr)
	case "push":
		return c.executePush(args[1:], stdout, stderr)
	case "pull":
		return c.executePull(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown sync subcommand: %s\n", sub)
		return fmt.Errorf("unknown sync subcommand: %s", sub)
	}
}

// executeSave writes a notebook entry to the local sync directory.
func (c *SyncCommand) executeSave(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sync-save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var title string
	var tags string
	var body string
	fs.StringVar(&title, "title", "", "Entry title (used in filename slug)")
	fs.StringVar(&tags, "tags", "", "Comma-separated tags")
	fs.StringVar(&body, "body", "", "Prompt body text")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if title == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --title is required")
		return fmt.Errorf("--title is required")
	}

	if body == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --body is required")
		return fmt.Errorf("--body is required")
	}

	dir, err := c.notebooksDir()
	if err != nil {
		return fmt.Errorf("resolving notebooks directory: %w", err)
	}

	now := c.timeNow()
	slug := slugify(title)
	datePart := now.Format("2006-01-02")
	filename := datePart + "-" + slug + ".md"

	// Build year/month subdirectory.
	yearMonth := filepath.Join(dir, now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(yearMonth, 0755); err != nil {
		return fmt.Errorf("creating notebook directory: %w", err)
	}

	// Deduplicate: if file already exists, append a numeric suffix.
	entryPath := filepath.Join(yearMonth, filename)
	entryPath = deduplicatePath(entryPath)

	// Build frontmatter.
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("date: %s\n", now.Format(time.RFC3339)))
	if tags != "" {
		tagList := parseTags(tags)
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(tagList, ", ")))
	}
	sb.WriteString(fmt.Sprintf("title: %q\n", title))
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(body)
	sb.WriteString("\n")

	if err := os.WriteFile(entryPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing notebook entry: %w", err)
	}

	relPath, _ := filepath.Rel(dir, entryPath)
	_, _ = fmt.Fprintf(stdout, "Saved notebook entry: %s\n", relPath)
	return nil
}

// executeList lists saved notebook entries in reverse chronological order.
func (c *SyncCommand) executeList(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sync-list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var limit int
	fs.IntVar(&limit, "limit", 0, "Maximum number of entries to show (0 = all)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	dir, err := c.notebooksDir()
	if err != nil {
		return fmt.Errorf("resolving notebooks directory: %w", err)
	}

	entries, err := discoverEntries(dir)
	if err != nil {
		// If the directory doesn't exist yet, that's fine — no entries.
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintln(stdout, "No notebook entries found.")
			return nil
		}
		return fmt.Errorf("listing notebook entries: %w", err)
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "No notebook entries found.")
		return nil
	}

	// Reverse chronological order (newest first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path > entries[j].path
	})

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	for _, e := range entries {
		_, _ = fmt.Fprintf(stdout, "%s  %s\n", e.date, e.slug)
	}

	return nil
}

// notebookEntry represents a discovered notebook entry.
type notebookEntry struct {
	path string // relative path from notebooks dir
	date string // YYYY-MM-DD
	slug string // title slug
}

// executeInit clones a git repository as the sync root.
func (c *SyncCommand) executeInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sync-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	repoURL := ""
	if fs.NArg() > 0 {
		repoURL = fs.Arg(0)
	}
	if repoURL == "" && c.config != nil {
		repoURL = c.config.GetString("sync.repository")
	}
	if repoURL == "" {
		_, _ = fmt.Fprintln(stderr, "Error: repository URL required")
		_, _ = fmt.Fprintln(stderr, "  Pass as argument: osm sync init <repo-url>")
		_, _ = fmt.Fprintln(stderr, "  Or set sync.repository in config")
		return fmt.Errorf("repository URL required: pass as argument or set sync.repository in config")
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	if isGitRepo(root) {
		return fmt.Errorf("sync directory already initialized: %s", root)
	}

	if err := c.runGit("", stdout, stderr, "clone", repoURL, root); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Sync repository initialized: %s\n", root)
	return nil
}

// executePush stages all changes, commits, and pushes to the remote.
func (c *SyncCommand) executePush(args []string, stdout, stderr io.Writer) error {
	root, err := c.syncRoot()
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return fmt.Errorf("sync directory not initialized: run 'osm sync init' first")
	}

	// Stage all changes.
	if err := c.runGit(root, io.Discard, stderr, "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check for staged changes. git diff --cached --quiet exits 0 = clean.
	if err := c.runGit(root, io.Discard, io.Discard, "diff", "--cached", "--quiet"); err == nil {
		_, _ = fmt.Fprintln(stdout, "Nothing to push — no changes.")
		return nil
	}

	// Commit with timestamp.
	timestamp := c.timeNow().Format(time.RFC3339)
	if err := c.runGit(root, io.Discard, stderr, "commit", "-m", fmt.Sprintf("osm sync: %s", timestamp)); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Push.
	if err := c.runGit(root, io.Discard, stderr, "push", "origin", "HEAD"); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Sync push complete.")
	return nil
}

// executePull fetches and merges remote changes, or clones if not initialized.
func (c *SyncCommand) executePull(args []string, stdout, stderr io.Writer) error {
	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	if !isGitRepo(root) {
		// Not initialized — attempt to clone from config.
		repoURL := ""
		if c.config != nil {
			repoURL = c.config.GetString("sync.repository")
		}
		if repoURL == "" {
			return fmt.Errorf("sync directory not initialized and no sync.repository configured")
		}
		if err := c.runGit("", stdout, stderr, "clone", repoURL, root); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "Sync repository cloned: %s\n", root)
		return nil
	}

	// Pull with rebase.
	var gitStderr bytes.Buffer
	multiStderr := io.MultiWriter(stderr, &gitStderr)
	if err := c.runGit(root, io.Discard, multiStderr, "pull", "--rebase", "origin", "HEAD"); err != nil {
		// Check for conflict indicators.
		errOutput := gitStderr.String()
		if strings.Contains(errOutput, "CONFLICT") || strings.Contains(errOutput, "could not apply") {
			_, _ = fmt.Fprintln(stderr, "")
			_, _ = fmt.Fprintf(stderr, "Resolve conflicts manually in: %s\n", root)
			_, _ = fmt.Fprintln(stderr, "Then run: osm sync push")
			return fmt.Errorf("sync pull encountered merge conflicts")
		}
		return fmt.Errorf("git pull failed: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Sync pull complete.")
	return nil
}

// syncRoot returns the root directory of the sync repository.
func (c *SyncCommand) syncRoot() (string, error) {
	if c.config != nil {
		p := c.config.GetString("sync.local-path")
		if p != "" {
			return p, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".one-shot-man", "sync"), nil
}

// runGit executes a git command. If dir is empty, CWD is used.
func (c *SyncCommand) runGit(dir string, stdout, stderr io.Writer, args ...string) error {
	gitBin := "git"
	if c.GitBin != "" {
		gitBin = c.GitBin
	}
	cmd := exec.Command(gitBin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// isGitRepo checks whether dir contains a .git directory.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// discoverEntries walks the notebooks directory and returns all entries.
func discoverEntries(dir string) ([]notebookEntry, error) {
	var entries []notebookEntry

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Only look at .md files.
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)
		name := strings.TrimSuffix(info.Name(), ".md")

		// Parse date prefix: YYYY-MM-DD-<slug>
		if len(name) < 11 {
			return nil // too short to have date prefix
		}
		datePart := name[:10]
		// Validate date format.
		if _, err := time.Parse("2006-01-02", datePart); err != nil {
			return nil // not a valid date prefix, skip
		}

		slug := ""
		if len(name) > 11 {
			slug = name[11:]
		}

		entries = append(entries, notebookEntry{
			path: relPath,
			date: datePart,
			slug: slug,
		})
		return nil
	})

	return entries, err
}

// notebooksDir returns the path to the notebooks directory.
func (c *SyncCommand) notebooksDir() (string, error) {
	if c.syncDir != "" {
		return c.syncDir, nil
	}
	root, err := c.syncRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "notebooks"), nil
}

// timeNow returns the current time, using the TimeNow field if set.
func (c *SyncCommand) timeNow() time.Time {
	if c.TimeNow != nil {
		return c.TimeNow()
	}
	return time.Now()
}

// slugify converts a title to a filename-safe slug.
func slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse consecutive hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	// Truncate to 50 characters.
	if len(s) > 50 {
		s = s[:50]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

// parseTags splits a comma-separated tag string into cleaned tags.
func parseTags(tags string) []string {
	parts := strings.Split(tags, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// deduplicatePath appends a numeric suffix if the path already exists.
func deduplicatePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	// Extremely unlikely fallback.
	return path
}
