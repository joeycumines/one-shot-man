package command

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

// stdin reader used for interactive prompts. It's stored on the command
// instance so tests can inject a custom reader safely without relying on
// package-global mutable state.

// SessionCommand manages persisted sessions on disk.
type SessionCommand struct {
	*BaseCommand
	cfg   *config.Config
	dry   bool
	yes   bool
	stdin io.Reader
}

// NewSessionCommand creates the session management command.
func NewSessionCommand(cfg *config.Config) *SessionCommand {
	return &SessionCommand{
		BaseCommand: NewBaseCommand("session", "Manage persisted sessions", "session [list|clean|purge|delete|info|path|id]"),
		cfg:         cfg,
		stdin:       os.Stdin,
	}
}

func (c *SessionCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.dry, "dry-run", false, "Don't actually delete; show what would be deleted")
	fs.BoolVar(&c.yes, "y", false, "Assume yes to confirmation prompts")
}

func (c *SessionCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return c.list(stdout, "text", "default")
	}
	sub := strings.ToLower(args[0])
	// Allow subcommands to parse their own flags (e.g. -h, -y) by handing
	// off the remainder of args after the subcommand name into a new
	// FlagSet local to that subcommand. Do NOT inspect args manually for
	// help tokens - rely on the flag package to handle help behavior.
	switch sub {
	case "id":
		fs := flag.NewFlagSet("session-id", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		// Accept a local --session override (mirrors other built-ins)
		var localSession string
		fs.StringVar(&localSession, "session", "", "Session ID for state persistence (overrides auto-discovery)")
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s id\n\n", c.Name())
			fmt.Fprintln(stderr, "Resolve and print the session id that would be used for this terminal.")
			// Only show the Options block if this FlagSet actually defines flags
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "Options:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		// Resolve session id using scripting package
		id := scripting.GetSessionID(localSession)
		fmt.Fprintln(stdout, id)
		return nil
	case "list":
		fs := flag.NewFlagSet("session-list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var formatLocal string
		var sortLocal string
		fs.StringVar(&formatLocal, "format", "text", "output format: text|json")
		fs.StringVar(&sortLocal, "sort", "default", "sorting: default|active")
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s list\n\n", c.Name())
			fmt.Fprintln(stderr, "Show all existing sessions with metadata.")
			fmt.Fprintln(stderr, "\nOptions:")
			fmt.Fprintln(stderr, "  -format <text|json>   Output format (default: text).\n    'text' prints tab-separated lines; 'json' prints a pretty JSON array of session objects.")
			fmt.Fprintln(stderr, "  -sort <default|active>  Sorting behavior (default: filesystem discovery order).\n    'active' surfaces active sessions first, then orders by update time (newest first).")
			fs.SetOutput(stderr)
			fs.PrintDefaults()
			fs.SetOutput(io.Discard)
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		return c.list(stdout, formatLocal, sortLocal)
	case "clean":
		// parse subcommand flags
		fs := flag.NewFlagSet("session-clean", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var yesLocal bool
		fs.BoolVar(&yesLocal, "y", false, "Assume yes to confirmation prompts")
		// Register dry-run locally so `clean -dry-run` works correctly.
		// Uses the same struct field pointer, so it updates c.dry directly.
		fs.BoolVar(&c.dry, "dry-run", c.dry, "Don't actually delete; show what would be deleted")
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s clean\n\n", c.Name())
			fmt.Fprintln(stderr, "Run automatic cleanup based on configured policies.")
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "\nOptions:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		// confirmation
		if !c.dry && !c.yes && !yesLocal {
			br := bufio.NewReader(c.stdin)
			fmt.Fprint(stdout, "This will permanently remove sessions according to your configured policies. Proceed? (y/N): ")
			t, err := br.ReadString('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to read confirmation: %w", err)
			}
			t = strings.TrimSpace(t)
			if !strings.EqualFold(t, "y") && !strings.EqualFold(t, "yes") {
				fmt.Fprintln(stdout, "aborted")
				return nil
			}
		}
		return c.clean(stdout)
	case "purge":
		// parse subcommand flags
		fs := flag.NewFlagSet("session-purge", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var yesLocal bool
		fs.BoolVar(&yesLocal, "y", false, "Assume yes to confirmation prompts")
		// Register dry-run locally so `purge -dry-run` works correctly.
		fs.BoolVar(&c.dry, "dry-run", c.dry, "Don't actually delete; show what would be deleted")
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s purge\n\n", c.Name())
			fmt.Fprintln(stderr, "Permanently purge sessions (ignores configured retention policies).")
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "\nOptions:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		// confirmation
		if !c.dry && !c.yes && !yesLocal {
			br := bufio.NewReader(c.stdin)
			fmt.Fprint(stdout, "This will permanently purge sessions (ignoring retention). Proceed? (y/N): ")
			t, err := br.ReadString('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to read confirmation: %w", err)
			}
			t = strings.TrimSpace(t)
			if !strings.EqualFold(t, "y") && !strings.EqualFold(t, "yes") {
				fmt.Fprintln(stdout, "aborted")
				return nil
			}
		}
		return c.purge(stdout)
	case "delete":
		// Pre-scan arguments to support '-y' and '-dry-run' even when placed after non-flag
		// arguments. The default flag package stops parsing flags at the first
		// non-flag token which can lead to surprising UX (e.g. `delete id -y`).
		// We also strictly respect '--' to allow deleting IDs that look like flags.
		rawDelArgs := args[1:]
		var explicitIDs []string
		var flagParsableArgs []string

		scanningFlags := true
		var manualYes bool
		var manualDry bool

		// Known flag aliases -> canonical key. This avoids duplicating string
		// literals across the manual scanning loop and the FlagSet bindings.
		knownFlags := map[string]string{
			"-y":        "yes",
			"--y":       "yes",
			"-yes":      "yes",
			"--yes":     "yes",
			"-dry-run":  "dry",
			"--dry-run": "dry",
		}

		for _, a := range rawDelArgs {
			if scanningFlags {
				if a == "--" {
					scanningFlags = false
					continue
				}
				// Check for known flags using the local descriptor map
				if k, ok := knownFlags[a]; ok {
					switch k {
					case "yes":
						manualYes = true
					case "dry":
						manualDry = true
					}
					continue
				}
			}

			if scanningFlags {
				// Still scanning: this argument is either an ID or a flag we don't know (like -h)
				// We keep it in a list to pass to fs.Parse later.
				flagParsableArgs = append(flagParsableArgs, a)
			} else {
				// Terminator passed: this is definitely an ID.
				explicitIDs = append(explicitIDs, a)
			}
		}

		fs := flag.NewFlagSet("session-delete", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var yesLocal bool
		fs.BoolVar(&yesLocal, "y", false, "Assume yes to confirmation prompts")
		// Register dry-run so Usage is correct and fs.Parse accepts it if placed before IDs.
		// We use a local bool here to avoid double-toggling if c.dry was already true,
		// though OR-ing logic below handles it safely.
		var dryLocal bool
		fs.BoolVar(&dryLocal, "dry-run", false, "Don't actually delete; show what would be deleted")

		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s delete <session-id>...\n\n", c.Name())
			fmt.Fprintln(stderr, "Remove a specific session from storage. This is irreversible.")
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "Options:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		// Parse the args that were before '--' and weren't stripped.
		// This handles help (-h) and any other flags.
		if err := fs.Parse(flagParsableArgs); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		// Merge all sources of configuration
		c.yes = c.yes || yesLocal || manualYes
		c.dry = c.dry || dryLocal || manualDry

		// Combine IDs found by flagset (standard args) and IDs found after '--'
		rem := append(fs.Args(), explicitIDs...)
		if len(rem) < 1 {
			return fmt.Errorf("delete requires a session id")
		}
		id := rem[0]
		if !c.yes && !yesLocal {
			// If multiple IDs are supplied we'll ask for a single confirmation
			// before attempting to delete. Use the command's stdin reader so
			// tests can inject behavior.
			br := bufio.NewReader(c.stdin)
			if len(rem) > 1 {
				_, _ = fmt.Fprintf(stdout, "Are you sure you want to delete %d sessions? This is irreversible. (y/N): ", len(rem))
			} else {
				_, _ = fmt.Fprintf(stdout, "Are you sure you want to delete session '%s'? This is irreversible. (y/N): ", id)
			}
			t, err := br.ReadString('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to read confirmation: %w", err)
			}
			t = strings.TrimSpace(t)
			if !strings.EqualFold(t, "y") && !strings.EqualFold(t, "yes") {
				fmt.Fprintln(stdout, "aborted")
				return nil
			}
		}
		// Support deleting multiple ids in a single command invocation.
		var failed []string
		for _, id := range rem {
			if err := c.delete(stdout, id); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", id, err))
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("failed to delete: %s", strings.Join(failed, "; "))
		}
		return nil
	case "info":
		fs := flag.NewFlagSet("session-info", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s info <session-id>\n\n", c.Name())
			fmt.Fprintln(stderr, "Show the raw data for a specific session.")
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "Options:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		rem := fs.Args()
		if len(rem) < 1 {
			return fmt.Errorf("info requires a session id")
		}
		return c.info(stdout, rem[0])
	case "path":
		fs := flag.NewFlagSet("session-path", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Usage = func() {
			_, _ = fmt.Fprintf(stderr, "Usage: %s path [session-id]\n\n", c.Name())
			fmt.Fprintln(stderr, "Show the sessions directory or a specific session file path.")
			var hasFlags bool
			fs.VisitAll(func(_ *flag.Flag) { hasFlags = true })
			if hasFlags {
				fmt.Fprintln(stderr, "Options:")
				fs.SetOutput(stderr)
				fs.PrintDefaults()
				fs.SetOutput(io.Discard)
			}
		}
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		rem := fs.Args()
		if len(rem) == 0 {
			// Use SessionFilePath to derive a directory using the package-local
			// sessionDirectory helper (tests override this via SetTestPaths).
			p, err := storage.SessionFilePath("__osm_probe__")
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, filepath.Dir(p))
			return nil
		}
		// If an ID is provided print the full session file path
		p, err := storage.SessionFilePath(rem[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, p)
		return nil
	default:
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func (c *SessionCommand) list(w io.Writer, format, sortMode string) error {
	infos, err := storage.ScanSessions()
	if err != nil {
		return err
	}

	// Validate format
	if format != "text" && format != "json" {
		return fmt.Errorf("invalid format: %q", format)
	}

	// Validate sort
	if sortMode != "default" && sortMode != "active" {
		return fmt.Errorf("invalid sort: %q", sortMode)
	}

	// Apply 'active' sorting: active sessions first, then idle; within groups sort by UpdatedAt (desc), then ID asc
	if sortMode == "active" {
		sort.SliceStable(infos, func(i, j int) bool {
			a := infos[i]
			b := infos[j]
			// Active sessions first
			if a.IsActive != b.IsActive {
				return a.IsActive && !b.IsActive
			}
			// Most recently updated first
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			// Tiebreaker: ID ascending
			return a.ID < b.ID
		})
	}

	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if infos == nil {
			infos = []storage.SessionInfo{}
		}
		if err := enc.Encode(infos); err != nil {
			return err
		}
		return nil
	}

	// text format: if no sessions, show friendly message
	if len(infos) == 0 {
		fmt.Fprintln(w, "No sessions found")
		return nil
	}

	for _, si := range infos {
		active := "idle"
		if si.IsActive {
			active = "active"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d bytes\t%s\n", si.ID, si.UpdatedAt.Format(time.RFC3339), si.Size, active)
	}
	return nil
}

func (c *SessionCommand) clean(w io.Writer) error {
	return c.runCleanup(w, false)
}

func (c *SessionCommand) purge(w io.Writer) error {
	return c.runCleanup(w, true)
}

func (c *SessionCommand) runCleanup(w io.Writer, purge bool) error {
	cfg := c.cfg
	sc := cfg.Sessions
	cleaner := &storage.Cleaner{MaxAgeDays: sc.MaxAgeDays, MaxCount: sc.MaxCount, MaxSizeMB: sc.MaxSizeMB, DryRun: c.dry, Purge: purge}

	if c.dry {
		if purge {
			fmt.Fprintln(w, "Dry-run: the following would be purged:")
		} else {
			fmt.Fprintln(w, "Dry-run: the following would be removed:")
		}
		report, err := cleaner.ExecuteCleanup("")
		if err != nil {
			return err
		}
		for _, id := range report.Removed {
			fmt.Fprintln(w, id)
		}
		return nil
	}

	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		return err
	}
	for _, id := range report.Removed {
		if purge {
			fmt.Fprintln(w, "purged:", id)
		} else {
			fmt.Fprintln(w, "removed:", id)
		}
	}
	for _, id := range report.Skipped {
		fmt.Fprintln(w, "skipped:", id)
	}
	return nil
}

func (c *SessionCommand) delete(w io.Writer, id string) error {
	if c.dry {
		_, _ = fmt.Fprintf(w, "Dry-run: would delete session %s\n", id)
		return nil
	}
	p, err := storage.SessionFilePath(id)
	if err != nil {
		return err
	}
	// Try to acquire lock - if active, refuse. If acquired we'll remove the
	// session while holding the lock and then release it (which removes the
	// lock file). This avoids leaking file descriptors and leaving lock
	// artifacts behind.
	lockPath, _ := storage.SessionLockFilePath(id)
	f, ok, err := storage.AcquireLockHandle(lockPath)
	if err != nil {
		if f != nil {
			_ = f.Close()
		}
		return fmt.Errorf("failed to check session lock: %w", err)
	}
	if !ok {
		// Defensive: if AcquireLockHandle implementation ever returns a
		// non-nil file when it reports !ok, close it to avoid leaking FDs.
		if f != nil {
			_ = f.Close()
		}
		return fmt.Errorf("session %s appears active or locked", id)
	}
	// We now own the lock. If we succeed in deleting the session file we
	// should remove the lock artifact as well. If deletion fails we must
	// NOT remove the lockfile — close the FD only and leave the lockfile in
	// place to avoid leaving an unlocked session file without a lock.
	//
	// Use Close without removal on failure, and ReleaseLockHandle to remove
	// the lock on successful deletion.

	// Attempt to remove the session file
	if err := os.Remove(p); err != nil {
		// Close descriptor to avoid fd leaks but keep lockfile artifact
		_ = f.Close()
		return err
	}

	// File removed successfully — also remove the lockfile
	if rerr := storage.ReleaseLockHandle(f); rerr != nil {
		// best-effort: warn, but the session data was removed
		_, _ = fmt.Fprintf(w, "deleted %s (warning: failed to remove lock: %v)\n", id, rerr)
		return nil
	}
	fmt.Fprintln(w, "deleted", id)
	return nil
}

func (c *SessionCommand) info(w io.Writer, id string) error {
	p, err := storage.SessionFilePath(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(data))
	return nil
}
