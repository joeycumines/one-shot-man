package command

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/gitops"
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
			"sync <save|list|load|init|push|pull> [options]",
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
		_, _ = fmt.Fprintln(stderr, "Usage: osm sync <save|list|load|init|push|pull|config-push|config-pull>")
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "Subcommands:")
		_, _ = fmt.Fprintln(stderr, "  save          Save a prompt notebook entry")
		_, _ = fmt.Fprintln(stderr, "  list          List saved notebook entries")
		_, _ = fmt.Fprintln(stderr, "  load          Load a saved notebook entry")
		_, _ = fmt.Fprintln(stderr, "  init          Clone a sync repository")
		_, _ = fmt.Fprintln(stderr, "  push          Commit and push local changes")
		_, _ = fmt.Fprintln(stderr, "  pull          Fetch and merge remote changes")
		_, _ = fmt.Fprintln(stderr, "  config-push   Push local config to sync repository")
		_, _ = fmt.Fprintln(stderr, "  config-pull   Pull shared config from sync repository")
		return &SilentError{Err: fmt.Errorf("no subcommand specified")}
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "save":
		return c.executeSave(args[1:], stdout, stderr)
	case "list":
		return c.executeList(args[1:], stdout, stderr)
	case "load":
		return c.executeLoad(args[1:], stdout, stderr)
	case "init":
		return c.executeInit(args[1:], stdout, stderr)
	case "push":
		return c.executePush(args[1:], stdout, stderr)
	case "pull":
		return c.executePull(args[1:], stdout, stderr)
	case "config-push":
		return c.executeConfigPush(args[1:], stdout, stderr)
	case "config-pull":
		return c.executeConfigPull(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown sync subcommand: %s\n", sub)
		return &SilentError{Err: fmt.Errorf("unknown sync subcommand: %s", sub)}
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
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w for save: %v", ErrUnexpectedArguments, fs.Args())
	}

	if title == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --title is required")
		return &SilentError{Err: fmt.Errorf("--title is required")}
	}

	if body == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --body is required")
		return &SilentError{Err: fmt.Errorf("--body is required")}
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
	entryPath, err = deduplicatePath(entryPath)
	if err != nil {
		return fmt.Errorf("deduplicating entry path: %w", err)
	}

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
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w for list: %v", ErrUnexpectedArguments, fs.Args())
	}

	dir, err := c.notebooksDir()
	if err != nil {
		return fmt.Errorf("resolving notebooks directory: %w", err)
	}

	entries, err := discoverEntries(dir)
	if err != nil {
		// If the directory doesn't exist yet, that's fine — no entries.
		if errors.Is(err, os.ErrNotExist) {
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
	slices.SortFunc(entries, func(a, b notebookEntry) int {
		return cmp.Compare(b.path, a.path)
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
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() > 1 {
		return fmt.Errorf("%w for init: %v", ErrUnexpectedArguments, fs.Args()[1:])
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
		return &SilentError{Err: fmt.Errorf("repository URL required: pass as argument or set sync.repository in config")}
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	if gitops.IsRepo(root) {
		return fmt.Errorf("sync directory already initialized: %s", root)
	}

	if _, err := gitops.Clone(context.Background(), repoURL, root); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Sync repository initialized: %s\n", root)
	return nil
}

// executePush stages all changes, commits, and pushes to the remote.
func (c *SyncCommand) executePush(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("%w for push: %v", ErrUnexpectedArguments, args)
	}
	root, err := c.syncRoot()
	if err != nil {
		return err
	}
	if !gitops.IsRepo(root) {
		return fmt.Errorf("sync directory not initialized: run 'osm sync init' first")
	}

	repo, err := gitops.Open(root)
	if err != nil {
		return fmt.Errorf("opening sync repo: %w", err)
	}

	// Stage all changes.
	if err := repo.AddAll(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check for staged changes.
	hasChanges, err := repo.HasStagedChanges()
	if err != nil {
		return fmt.Errorf("checking staged changes: %w", err)
	}
	if !hasChanges {
		_, _ = fmt.Fprintln(stdout, "Nothing to push — no changes.")
		return nil
	}

	// Commit with timestamp.
	now := c.timeNow()
	timestamp := now.Format(time.RFC3339)
	if _, err := repo.Commit(fmt.Sprintf("osm sync: %s", timestamp), now); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Push.
	if err := repo.Push(context.Background()); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Sync push complete.")
	return nil
}

// executePull fetches and merges remote changes, or clones if not initialized.
func (c *SyncCommand) executePull(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("%w for pull: %v", ErrUnexpectedArguments, args)
	}
	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	if !gitops.IsRepo(root) {
		// Not initialized — attempt to clone from config.
		repoURL := ""
		if c.config != nil {
			repoURL = c.config.GetString("sync.repository")
		}
		if repoURL == "" {
			return fmt.Errorf("sync directory not initialized and no sync.repository configured")
		}
		if _, err := gitops.Clone(context.Background(), repoURL, root); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "Sync repository cloned: %s\n", root)
		return nil
	}

	// Pull with rebase — delegated to gitops.PullRebase (the only
	// shell-out in the sync flow; go-git v6 does not support rebase).
	err = gitops.PullRebase(context.Background(), gitops.PullRebaseOptions{
		Dir:    root,
		GitBin: c.GitBin,
		Stderr: stderr,
	})
	if err != nil {
		if errors.Is(err, gitops.ErrConflict) {
			_, _ = fmt.Fprintln(stderr, "")
			_, _ = fmt.Fprintf(stderr, "Resolve conflicts manually in: %s\n", root)
			_, _ = fmt.Fprintln(stderr, "Then run: osm sync push")
			return &SilentError{Err: fmt.Errorf("sync pull encountered merge conflicts: %w", err)}
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
	return filepath.Join(home, ".osm", "sync"), nil
}

// discoverEntries walks the notebooks directory and returns all entries.
func discoverEntries(dir string) ([]notebookEntry, error) {
	var entries []notebookEntry

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Only look at .md files.
		if d.IsDir() {
			return nil
		}
		name, ok := strings.CutSuffix(d.Name(), ".md")
		if !ok {
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)

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

// executeLoad reads a saved notebook entry and writes its body to stdout.
// The query can be a date (YYYY-MM-DD), a slug, or a date-slug combination.
func (c *SyncCommand) executeLoad(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "Usage: osm sync load <slug-or-date>")
		return &SilentError{Err: fmt.Errorf("load requires exactly one argument")}
	}
	query := args[0]

	dir, err := c.notebooksDir()
	if err != nil {
		return fmt.Errorf("resolving notebooks directory: %w", err)
	}

	entries, err := discoverEntries(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no notebook entries found")
		}
		return fmt.Errorf("listing notebook entries: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("no notebook entries found")
	}

	// Find matching entry: try exact date-slug, then slug-only, then date-only.
	match := matchEntry(entries, query)
	if match == nil {
		return fmt.Errorf("no entry matching %q", query)
	}

	entryPath := filepath.Join(dir, match.path)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return fmt.Errorf("reading entry: %w", err)
	}

	body := stripFrontmatter(string(data))
	_, _ = fmt.Fprint(stdout, body)
	return nil
}

// matchEntry finds a notebook entry matching the query. Tries exact
// date-slug match first, then slug-only, then date prefix.
func matchEntry(entries []notebookEntry, query string) *notebookEntry {
	if query == "" {
		return nil
	}

	// Exact date-slug match (e.g., "2025-01-15-my-review").
	for i := range entries {
		dateSlug := entries[i].date + "-" + entries[i].slug
		if dateSlug == query {
			return &entries[i]
		}
	}

	// Slug-only match (e.g., "my-review"). Returns most recent if ambiguous.
	// Sort reverse chronological first. Copy to avoid mutating caller's slice.
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b notebookEntry) int {
		return cmp.Compare(b.path, a.path)
	})
	for i := range sorted {
		if sorted[i].slug == query {
			return &sorted[i]
		}
	}

	// Date prefix match (e.g., "2025-01-15"). Returns first match.
	for i := range sorted {
		if sorted[i].date == query {
			return &sorted[i]
		}
	}

	// Partial slug match (e.g., "review" matches "my-code-review").
	for i := range sorted {
		if strings.Contains(sorted[i].slug, query) {
			return &sorted[i]
		}
	}

	return nil
}

// stripFrontmatter removes YAML frontmatter delimited by "---" lines
// and any leading blank lines from the remaining content.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	// Find closing "---\n".
	rest := content[4:]
	_, after, ok := strings.Cut(rest, "---\n")
	if !ok {
		return content // no closing delimiter, return as-is
	}
	body := after
	// Trim leading blank lines.
	body = strings.TrimLeft(body, "\n")
	return body
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
func deduplicatePath(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("too many entries with base name %q", filepath.Base(path))
}
