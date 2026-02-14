package command

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SyncCommand provides local notebook save/list operations as the
// foundation for future git-based synchronisation. See
// docs/archive/notes/git-sync-design.md for the full design.
type SyncCommand struct {
	*BaseCommand

	// syncDir is the local directory used for storing notebook entries.
	// When empty, defaults to ~/.one-shot-man/sync/notebooks.
	syncDir string

	// TimeNow returns the current time. Override in tests for determinism.
	TimeNow func() time.Time
}

// NewSyncCommand creates a new sync command. An optional syncDir argument
// overrides the default notebook directory (useful for tests).
func NewSyncCommand(syncDir ...string) *SyncCommand {
	cmd := &SyncCommand{
		BaseCommand: NewBaseCommand(
			"sync",
			"Save and list prompt notebook entries (local)",
			"sync <save|list> [options]",
		),
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
		_, _ = fmt.Fprintln(stderr, "Usage: osm sync <save|list>")
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "Subcommands:")
		_, _ = fmt.Fprintln(stderr, "  save   Save a prompt notebook entry")
		_, _ = fmt.Fprintln(stderr, "  list   List saved notebook entries")
		return fmt.Errorf("no subcommand specified")
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "save":
		return c.executeSave(args[1:], stdout, stderr)
	case "list":
		return c.executeList(args[1:], stdout, stderr)
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".one-shot-man", "sync", "notebooks"), nil
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
