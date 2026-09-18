package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"
)

// Child is the running shaper process as the runner sees it.
type Child interface {
	// Terminate asks the child to stop and reaps it, returning its exit code.
	Terminate(ctx context.Context) int
}

// Dependencies are the effects the runner performs, injectable so the custody
// protocol can be exercised without a real shaper.
type Dependencies struct {
	// Spawn starts the shaper with exactly the environment given and returns
	// the running child.
	Spawn func(environment []string, args []string) (Child, error)

	// WaitReady blocks until the bind port accepts a connection.
	WaitReady func(ctx context.Context) error

	// NewToken produces the per-start ephemeral token.
	NewToken func() (string, error)

	// Now supplies the start timestamp.
	Now func() time.Time
}

// Config describes one gateway run.
type Config struct {
	// ShaperBinary is the transcode shaper executable.
	ShaperBinary string

	// ShaperArgs are the arguments the shaper is started with, before the
	// per-access mounts derived from the catalog.
	ShaperArgs []string

	// Host and Port are the address the shaper binds and the launcher dials.
	Host string
	Port int

	// DiscoveryPath is where the advertisement is written.
	DiscoveryPath string

	// ReadyTimeout bounds how long readiness may take.
	ReadyTimeout time.Duration
}

// Run owns the shaper for as long as ctx lives: it starts the child with the
// credentials in its environment only, waits for readiness, advertises the
// gateway, and on shutdown removes the advertisement and reaps the child,
// returning the child's exit code.
func Run(ctx context.Context, cfg Config, mounts []Mount, credentials map[string]string, deps Dependencies) (int, error) {
	if cfg.ShaperBinary == "" {
		return 0, fmt.Errorf("no shaper binary configured")
	}
	if cfg.DiscoveryPath == "" {
		return 0, fmt.Errorf("no discovery path configured")
	}
	if deps.WaitReady == nil {
		return 0, errors.New("no readiness check configured: refusing to advertise a gateway that may not be listening")
	}
	deps = withDefaults(deps)

	environment, err := ChildEnvironment(mounts, credentials)
	if err != nil {
		return 0, err
	}

	// The spawner receives the full command line, binary first, exactly as it
	// would be invoked from a shell.
	args := append([]string{cfg.ShaperBinary}, cfg.ShaperArgs...)
	for _, mount := range mounts {
		// The mount order is already deterministic; the args carry the prefix
		// and the variable the child reads, never the value.
		args = append(args,
			"-prefix="+mount.Prefix,
			"-auth-source=env:"+mount.Credential,
		)
	}

	child, err := deps.Spawn(environment, args)
	if err != nil {
		return 0, fmt.Errorf("starting the shaper: %w", err)
	}

	readyCtx, cancelReady := context.WithTimeout(ctx, cfg.ReadyTimeout)
	readyErr := deps.WaitReady(readyCtx)
	cancelReady()
	if readyErr != nil {
		code := child.Terminate(ctx)
		return code, fmt.Errorf("the shaper never became reachable: %w", readyErr)
	}

	token, err := deps.NewToken()
	if err != nil {
		code := child.Terminate(ctx)
		return code, fmt.Errorf("generating the gateway token: %w", err)
	}
	discovery := NewDiscovery(os.Getpid(), cfg.Port, cfg.Host, prefixesOf(mounts), token, deps.Now())
	if err := WriteDiscovery(cfg.DiscoveryPath, discovery, ProcessAlive); err != nil {
		code := child.Terminate(ctx)
		return code, fmt.Errorf("advertising the gateway: %w", err)
	}

	// Shutdown: the advertisement goes first so a launcher cannot dial a gateway
	// that is already stopping, then the child is reaped.
	<-ctx.Done()
	removeErr := RemoveDiscovery(cfg.DiscoveryPath)
	code := child.Terminate(context.WithoutCancel(ctx))
	if removeErr != nil {
		return code, fmt.Errorf("removing the discovery file: %w", removeErr)
	}
	return code, nil
}

// prefixesOf is the access-to-prefix map the discovery document advertises.
func prefixesOf(mounts []Mount) map[string]string {
	prefixes := make(map[string]string, len(mounts))
	for _, mount := range mounts {
		prefixes[mount.Access] = mount.Prefix
	}
	return prefixes
}

func withDefaults(deps Dependencies) Dependencies {
	if deps.Spawn == nil {
		deps.Spawn = spawnExec
	}
	if deps.NewToken == nil {
		deps.NewToken = NewToken
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return deps
}

// NewToken produces a per-start ephemeral token: machine-local, short-lived and
// not a provider secret.
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// DefaultDependencies wires the real effects for one config.
func DefaultDependencies(cfg Config) Dependencies {
	return withDefaults(Dependencies{
		WaitReady: func(ctx context.Context) error {
			return WaitFor(ctx, cfg.Host, cfg.Port, time.Now().Add(cfg.ReadyTimeout), DefaultProbeInterval)
		},
	})
}

// spawnExec starts the shaper with the given environment, inheriting the
// runner's streams so the operator sees the shaper's own output.
func spawnExec(environment []string, args []string) (Child, error) {
	command := exec.Command(args[0], args[1:]...)
	command.Env = append(append([]string{}, minimalEnvironment()...), environment...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &execChild{command: command}, nil
}

// minimalEnvironment is the non-credential environment the shaper inherits.
func minimalEnvironment() []string {
	keys := []string{"HOME", "PATH", "TMPDIR", "USER", "LANG", "LC_ALL", "TERM"}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	sort.Strings(environment)
	return environment
}

type execChild struct {
	command *exec.Cmd
}

func (c *execChild) Terminate(ctx context.Context) int {
	if c.command.Process == nil {
		return 0
	}
	_ = c.command.Process.Signal(os.Interrupt)
	err := c.command.Wait()
	if err == nil {
		return 0
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	return 1
}
