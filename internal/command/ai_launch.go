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
	fs.String("tool", "", "launch target")
	fs.String("provider", "", "provider to use")
	fs.String("model", "", "model to use")
	fs.Bool("direct-env", false, "compose for direct credentials instead of a discovered gateway")
	fs.Bool("prefer-gateway", false, "refuse to fall back to direct credentials")
	fs.String("launcher", "", "path to the installed ai-tool.js (default: $HOME/.osm/scripts/ai-tool.js)")
	fs.Duration("grace-period", 5*time.Second, "how long the tool may take to exit after an interrupt")
}

func (c *AILaunchCommand) launcherPath(fs *flag.FlagSet) (string, error) {
	if path := c.flagString(fs, "launcher"); path != "" {
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
	fs := flag.NewFlagSet("ai-launch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	c.SetupFlags(fs)
	if err := fs.Parse(trimSeparators(args)); err != nil {
		return err
	}
	launcher, err := c.launcherPath(fs)
	if err != nil {
		return err
	}

	ctx := context.Background()
	plan, err := c.composePlan(ctx, launcher, fs)
	if err != nil {
		return err
	}

	environment, err := c.materialize(plan)
	if err != nil {
		return err
	}

	code, err := gateway.Supervise(ctx, gateway.SuperviseOptions{
		Argv:        plan.Args,
		Environment: environment,
		Stdin:       os.Stdin,
		Stdout:      stdout,
		Stderr:      stderr,
		GracePeriod: c.flagDuration(fs, "grace-period"),
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s exited with status %d", plan.Tool, code)
	}
	return nil
}

// composePlan asks the JavaScript launcher for the value-free plan.
func (c *AILaunchCommand) composePlan(ctx context.Context, launcher string, fs *flag.FlagSet) (launchPlan, error) {
	executable, err := os.Executable()
	if err != nil {
		return launchPlan{}, fmt.Errorf("resolving this executable: %w", err)
	}
	argv := []string{"script", launcher, "--", "--print-plan",
		"--tool", c.flagString(fs, "tool"),
		"--provider", c.flagString(fs, "provider"),
		"--model", c.flagString(fs, "model"),
	}
	if c.flagBool(fs, "direct-env") {
		argv = append(argv, "--direct-env")
	}
	if c.flagBool(fs, "prefer-gateway") {
		argv = append(argv, "--prefer-gateway")
	}

	command := exec.CommandContext(ctx, executable, argv...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return launchPlan{}, fmt.Errorf("composing the launch plan: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	body := stdout.String()
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

func (c *AILaunchCommand) flagString(fs *flag.FlagSet, name string) string {
	if value := fs.Lookup(name); value != nil {
		return value.Value.String()
	}
	return ""
}

func (c *AILaunchCommand) flagBool(fs *flag.FlagSet, name string) bool {
	return c.flagString(fs, name) == "true"
}

func (c *AILaunchCommand) flagDuration(fs *flag.FlagSet, name string) time.Duration {
	parsed, err := time.ParseDuration(c.flagString(fs, name))
	if err != nil {
		return 0
	}
	return parsed
}

// trimSeparators drops the separator the engine uses to introduce a command's
// arguments; leaving it in place makes the flag package treat every flag as a
// positional argument.
func trimSeparators(args []string) []string {
	trimmed := args
	for len(trimmed) > 0 && trimmed[0] == "--" {
		trimmed = trimmed[1:]
	}
	return trimmed
}
