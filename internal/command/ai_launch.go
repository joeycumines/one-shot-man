package command

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/joeycumines/one-shot-man/internal/gateway"
	"github.com/joeycumines/one-shot-man/internal/userk8s"
)

// AILaunchCommand composes a launch plan with the JavaScript launcher and then
// supervises the tool in Go, so the tool's exit status is the command's status.
// The split is forced by the engine's security model: the scripting surface
// deliberately withholds process control, while composition lives in the
// adapters that are JavaScript.
//
// No credential value crosses the boundary: the plan names the variables the
// tool needs and this command resolves them itself through the engine.
type AILaunchCommand struct {
	*BaseCommand
	config *config.Config

	// The engine registers a command's flags on the FlagSet it hands to
	// SetupFlags and parses them itself, then passes Execute only the
	// positional arguments left over. Values are therefore bound here and
	// read back from the struct; re-parsing the arguments inside Execute
	// silently yields empty values.
	tool          string
	provider      string
	model         string
	directEnv     bool
	preferGateway bool
	launcher      string
	gracePeriod   time.Duration
}

// launchPlan mirrors the value-free plan the launcher emits.
type launchPlan struct {
	Tool    string            `json:"tool"`
	Mode    string            `json:"mode"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	EnvRefs map[string]string `json:"envRefs"`
	Files   []launchPlanFile  `json:"files"`
	Notes   []string          `json:"notes"`
}

type launchPlanFile struct {
	Path    string `json:"path"`
	Mode    int    `json:"mode"`
	Content string `json:"content"`
}

// placeholderNames are the directories a plan may ask the supervisor to
// provide. An adapter that cannot know a directory at composition time writes
// one of these placeholders and this command resolves it to the private
// directory it created.
var placeholderNames = []string{
	"CRUSH_GLOBAL_CONFIG_DIR",
	"PI_CODING_AGENT_DIR",
}

// NewAILaunchCommand creates the ai-launch command.
func NewAILaunchCommand(cfg *config.Config) *AILaunchCommand {
	return &AILaunchCommand{
		BaseCommand: NewBaseCommand(
			"ai-launch",
			"Compose a launch plan and supervise the tool, reporting its exit status",
			"ai-launch [flags] -- [tool args]",
		),
		config: cfg,
	}
}

// SetupFlags configures the command's flags.
func (c *AILaunchCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.tool, "tool", "", "launch target")
	fs.StringVar(&c.provider, "provider", "", "provider to use")
	fs.StringVar(&c.model, "model", "", "model to use")
	fs.BoolVar(&c.directEnv, "direct-env", false, "compose for direct credentials instead of a discovered gateway")
	fs.BoolVar(&c.preferGateway, "prefer-gateway", false, "refuse to fall back to direct credentials")
	fs.StringVar(&c.launcher, "launcher", "", "path to the installed ai-tool.js (default: $HOME/.osm/scripts/ai-tool.js)")
	fs.DurationVar(&c.gracePeriod, "grace-period", 5*time.Second, "how long the tool may take to exit after an interrupt")
}

func (c *AILaunchCommand) launcherPath() (string, error) {
	if path := c.launcher; path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving the home directory for the launcher: %w", err)
	}
	path := filepath.Join(home, ".osm", "scripts", "ai-tool.js")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("the launcher is not installed at %s: %w", path, err)
	}
	return path, nil
}

// Execute composes the plan, resolves its credentials, materializes its files
// and supervises the tool.
func (c *AILaunchCommand) Execute(args []string, stdout, stderr io.Writer) error {
	launcher, err := c.launcherPath()
	if err != nil {
		return err
	}

	ctx := context.Background()
	plan, err := c.composePlan(ctx, launcher)
	if err != nil {
		return err
	}

	environment, err := c.materialize(plan)
	if err != nil {
		return err
	}

	// The plan carries the tool and its extra arguments; the executable is the
	// tool's own command, which is what the launcher resolved for this mode.
	argv := append([]string{plan.Tool}, plan.Args...)
	code, err := gateway.Supervise(ctx, gateway.SuperviseOptions{
		Argv:        argv,
		Environment: environment,
		Stdin:       os.Stdin,
		Stdout:      stdout,
		Stderr:      stderr,
		GracePeriod: c.gracePeriod,
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return &ExitError{Code: code, Err: fmt.Errorf("%s exited with status %d", plan.Tool, code)}
	}
	return nil
}

// composePlan asks the JavaScript launcher for the value-free plan.
func (c *AILaunchCommand) composePlan(ctx context.Context, launcher string) (launchPlan, error) {
	executable, err := os.Executable()
	if err != nil {
		return launchPlan{}, fmt.Errorf("resolving this executable: %w", err)
	}
	argv := []string{"script", launcher, "--", "--print-plan",
		"--tool", c.tool,
		"--provider", c.provider,
		"--model", c.model,
	}
	if c.directEnv {
		argv = append(argv, "--direct-env")
	}
	if c.preferGateway {
		argv = append(argv, "--prefer-gateway")
	}

	command := exec.CommandContext(ctx, executable, argv...)
	command.WaitDelay = 5 * time.Second

	// Capture the plan through a real file rather than a pipe: os/exec has to
	// copy a pipe into a plain io.Writer, and that copy also waits on every
	// descendant the launcher leaves behind, so Wait blocks on a command that
	// has already written its output. A file is handed to the child directly.
	output, err := os.CreateTemp("", "osm-ai-launch-plan-*")
	if err != nil {
		return launchPlan{}, fmt.Errorf("creating the plan output file: %w", err)
	}
	defer func() {
		_ = output.Close()
		_ = os.Remove(output.Name())
	}()
	var stderr strings.Builder
	command.Stdout = output
	command.Stderr = &stderr
	runErr := command.Run()
	if closeErr := output.Close(); closeErr != nil && runErr == nil {
		return launchPlan{}, fmt.Errorf("closing the plan output file: %w", closeErr)
	}
	if runErr != nil {
		return launchPlan{}, fmt.Errorf("composing the launch plan: %w (%s)", runErr, strings.TrimSpace(stderr.String()))
	}
	captured, err := os.ReadFile(output.Name())
	if err != nil {
		return launchPlan{}, fmt.Errorf("reading the launch plan: %w", err)
	}

	body := string(captured)
	start := strings.Index(body, "{")
	if start < 0 {
		return launchPlan{}, fmt.Errorf("the launcher produced no plan: %s", strings.TrimSpace(body))
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(body[start:]), &plan); err != nil {
		return launchPlan{}, fmt.Errorf("decoding the launch plan: %w", err)
	}
	if len(plan.Args) == 0 {
		return launchPlan{}, errors.New("the plan names no command to launch")
	}
	return plan, nil
}

// materialize creates the private directory a plan's files belong in, writes
// them at their declared modes, resolves the environment and substitutes the
// placeholders with that directory.
func (c *AILaunchCommand) materialize(plan launchPlan) ([]string, error) {
	directory := ""
	if len(plan.Files) > 0 {
		created, err := os.MkdirTemp("", "osm-launch-")
		if err != nil {
			return nil, fmt.Errorf("creating the launch directory: %w", err)
		}
		directory = created
	}
	for _, file := range plan.Files {
		target := filepath.Join(directory, filepath.Base(file.Path))
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o600
		}
		if err := os.WriteFile(target, []byte(file.Content), mode); err != nil {
			return nil, fmt.Errorf("materializing %s: %w", file.Path, err)
		}
	}

	values, err := c.resolveReferences(plan.EnvRefs)
	if err != nil {
		return nil, err
	}
	environment := make([]string, 0, len(plan.Env)+len(plan.EnvRefs))
	for name, value := range plan.Env {
		substituted, err := substitutePlaceholders(name, value, directory)
		if err != nil {
			return nil, err
		}
		environment = append(environment, name+"="+substituted)
	}
	for name, reference := range plan.EnvRefs {
		value, ok := values[reference]
		if !ok {
			return nil, fmt.Errorf("no credential resolved for %s (the plan needs %s)", name, reference)
		}
		environment = append(environment, name+"="+value)
	}
	sort.Strings(environment)
	return environment, nil
}

// substitutePlaceholders replaces every directory placeholder a plan uses and
// refuses one it does not understand, so a placeholder never reaches a child as
// a literal.
func substitutePlaceholders(name, value, directory string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	resolved := value
	for _, key := range placeholderNames {
		resolved = strings.ReplaceAll(resolved, "${"+key+"}", directory)
	}
	if strings.Contains(resolved, "${") {
		return "", fmt.Errorf("the plan sets %s to a value with an unresolved placeholder: %s", name, value)
	}
	return resolved, nil
}

// resolveReferences resolves the catalog credential names a plan asks for,
// through the same engine path the other commands use.
func (c *AILaunchCommand) resolveReferences(references map[string]string) (map[string]string, error) {
	needed := map[string]bool{}
	for _, reference := range references {
		needed[reference] = true
	}
	values := map[string]string{}
	if len(needed) == 0 {
		return values, nil
	}

	options := userK8sConfig(c.config)
	backend, err := userk8s.NewFilesBackend(userk8s.FilesBackendOptions{Paths: options.Artifacts, Runner: options.Runner})
	if err != nil {
		return nil, fmt.Errorf("loading the catalog: %w", err)
	}
	accesses, err := backend.ListAccesses(context.Background())
	if err != nil {
		return nil, fmt.Errorf("reading the catalog accesses: %w", err)
	}
	for _, name := range sortedKeys(needed) {
		// Only the access that DECLARES this variable is asked, so the command
		// never runs another provider's resolver chain (which may block on an
		// interactive prompt) looking for a name that access cannot supply.
		owner := ""
		for _, access := range accesses {
			if access.Spec.Auth == nil {
				continue
			}
			for _, declared := range access.Spec.Auth.RequiredEnv {
				if declared == name {
					owner = access.Name
					break
				}
			}
			if owner != "" {
				break
			}
		}
		if owner == "" {
			return nil, fmt.Errorf("no access declares the credential %s the plan needs", name)
		}

		resolveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resolution, err := backend.ResolveCredential(resolveCtx, owner)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", owner, err)
		}
		if resolution.Status != userk8s.StatusResolved {
			return nil, fmt.Errorf("credentials unavailable for %s: %s", owner, resolution.Reason)
		}
		resolved := ""
		for _, credential := range resolution.Credentials {
			if credential.EnvVar == name {
				resolved = credential.Value
				break
			}
		}
		if resolved == "" {
			return nil, fmt.Errorf("the resolution of %s did not cover %s", owner, name)
		}
		values[name] = resolved
	}
	return values, nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// NOTE: dropping the command name and separator is correct hygiene, but it is
// NOT the reason the flags were empty: the engine parses a command's flags
// itself (cmd/osm/main.go:136-146) and hands Execute only fs.Args(), so a
// command must bind its values with fs.StringVar/BoolVar/DurationVar in
// SetupFlags and read those fields in Execute.
// commandArgs drops the leading elements the engine puts in front of a
// command's own flags: the command name itself and any separator it uses to
// introduce the arguments. flag.Parse stops at the first element that is not a
// flag, so leaving either in place silently parses NO flags, which for these
// commands means an empty selection rather than an error.
func commandArgs(args []string) []string {
	trimmed := args
	for len(trimmed) > 0 && (trimmed[0] == "--" || !strings.HasPrefix(trimmed[0], "-")) {
		trimmed = trimmed[1:]
	}
	return trimmed
}
