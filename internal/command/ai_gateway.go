package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/gateway"
	"github.com/joeycumines/one-shot-man/internal/userk8s"
)

// AIGatewayCommand runs the credential-custody gateway: it reads the model
// catalog, resolves the credentials the shaper-fronted providers need, starts
// the transcode shaper with those credentials in its environment only, waits
// for the bind port, advertises itself in a discovery file and, on shutdown,
// removes that file and reaps the child.
type AIGatewayCommand struct {
	*BaseCommand
	config *config.Config

	// Bound in SetupFlags, which the engine parses for the command; Execute
	// receives only the positional arguments left over, so a FlagSet created
	// here would parse nothing and every value would read as its zero value.
	shaper       string
	host         string
	port         int
	discovery    string
	readyTimeout time.Duration
}

// NewAIGatewayCommand creates the ai-gateway command.
func NewAIGatewayCommand(cfg *config.Config) *AIGatewayCommand {
	return &AIGatewayCommand{
		BaseCommand: NewBaseCommand(
			"ai-gateway",
			"Run the model-credential gateway in front of the local shaper",
			"ai-gateway [flags]",
		),
		config: cfg,
	}
}

// SetupFlags configures the command's flags.
func (c *AIGatewayCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.shaper, "shaper", "ai-concurrency-shaper", "transcode shaper binary to run")
	fs.StringVar(&c.host, "host", "127.0.0.1", "address the shaper binds and clients dial")
	fs.IntVar(&c.port, "port", 11239, "port the shaper binds and clients dial")
	fs.StringVar(&c.discovery, "discovery", "", "discovery file path (default: $HOME/.osm/gateway.discovery)")
	fs.DurationVar(&c.readyTimeout, "ready-timeout", 30*time.Second, "how long the shaper may take to accept connections")
}

// Execute runs the gateway until it is signalled.
func (c *AIGatewayCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}

	discoveryPath := c.discovery
	if discoveryPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving the home directory for the discovery file: %w", err)
		}
		discoveryPath = filepath.Join(home, ".osm", "gateway.discovery")
	}

	host := c.host
	port := c.port
	cfg := gateway.Config{
		ShaperBinary: c.shaper,
		Host:         host,
		Port:         port,
		// The shaper must be told where to bind, or readiness could never
		// succeed: the bind address is the same one clients dial.
		ShaperArgs:    []string{"-bind=" + host + ":" + strconv.Itoa(port)},
		DiscoveryPath: discoveryPath,
		ReadyTimeout:  c.readyTimeout,
	}

	options := userK8sConfig(c.config)
	if len(options.Artifacts) == 0 {
		return fmt.Errorf("no catalog artifacts configured: set userk8s.artifacts (or OSM_USERK8S_ARTIFACTS)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backend, err := userk8s.NewFilesBackend(userk8s.FilesBackendOptions{Paths: options.Artifacts, Runner: options.Runner})
	if err != nil {
		return fmt.Errorf("loading the catalog: %w", err)
	}
	accesses, err := backend.ListAccesses(ctx)
	if err != nil {
		return fmt.Errorf("reading the catalog accesses: %w", err)
	}
	mounts := gateway.Mounts(accesses)
	if len(mounts) == 0 {
		return fmt.Errorf("the catalog declares no shaper-fronted access, so there is nothing to serve")
	}

	credentials, err := resolveMountCredentials(ctx, backend, mounts)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "serving %d mount(s) on %s:%d\n", len(mounts), cfg.Host, cfg.Port)
	for _, mount := range mounts {
		_, _ = fmt.Fprintf(stdout, "  %s -> %s (%s)\n", mount.Prefix, mount.Provider, mount.Credential)
	}

	code, runErr := gateway.Run(ctx, cfg, mounts, credentials, gateway.DefaultDependencies(cfg))
	if runErr != nil {
		return runErr
	}
	if code != 0 {
		return fmt.Errorf("the shaper exited with status %d", code)
	}
	return nil
}

// resolveMountCredentials resolves every mount's credential through the engine,
// so the gateway carries values only for the variables the shaper reads.
func resolveMountCredentials(ctx context.Context, backend *userk8s.FilesBackend, mounts []gateway.Mount) (map[string]string, error) {
	credentials := make(map[string]string, len(mounts))
	for _, mount := range mounts {
		resolution, err := backend.ResolveCredential(ctx, mount.Access)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", mount.Access, err)
		}
		if resolution.Status != userk8s.StatusResolved {
			return nil, fmt.Errorf("credentials unavailable for %s: %s", mount.Access, resolution.Reason)
		}
		found := false
		for _, credential := range resolution.Credentials {
			if credential.EnvVar == mount.Required {
				credentials[mount.Required] = credential.Value
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("the resolution of %s did not cover %s", mount.Access, mount.Required)
		}
	}
	return credentials, nil
}
