package gateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

type fakeChild struct {
	terminated bool
	code       int
}

func (c *fakeChild) Terminate(context.Context) int {
	c.terminated = true
	return c.code
}

func umansMounts() []Mount {
	return Mounts([]v1alpha1.ModelAccess{{
		ObjectMeta: metav1.ObjectMeta{Name: "umans-shaper"},
		Spec: v1alpha1.ModelAccessSpec{
			Provider:  "umans",
			Mode:      "shaper",
			Auth:      &v1alpha1.ModelAccessAuth{Scheme: v1alpha1.AuthSchemeBearer, RequiredEnv: []string{"UMANS_API_KEY"}},
			Endpoints: surfaceEndpoints(map[string]string{"anthropic": "http://127.0.0.1:11239/umans"}),
		},
	}})
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

func TestRunKeepsCredentialsInTheChildEnvironmentOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.discovery")
	child := &fakeChild{code: 7}
	var environment, args []string
	readinessChecked := false
	discoveryExistedAtReadiness := false

	ctx, cancel := context.WithCancel(context.Background())
	deps := Dependencies{
		Spawn: func(env []string, childArgs []string) (Child, error) {
			environment, args = env, childArgs
			return child, nil
		},
		WaitReady: func(context.Context) error {
			readinessChecked = true
			_, err := os.Stat(path)
			discoveryExistedAtReadiness = err == nil
			return nil
		},
		NewToken: func() (string, error) { return "ephemeral-token", nil },
		Now:      func() time.Time { return time.Unix(0, 0) },
	}
	cfg := Config{ShaperBinary: "shaper", ShaperArgs: []string{"-bind=127.0.0.1:11239"}, Host: "127.0.0.1", Port: 11239, DiscoveryPath: path, ReadyTimeout: time.Second}

	done := make(chan struct{})
	var code int
	var runErr error
	go func() {
		code, runErr = Run(ctx, cfg, umansMounts(), map[string]string{"UMANS_API_KEY": "umans-secret-value"}, deps)
		close(done)
	}()

	waitForFile(t, path)
	if !readinessChecked {
		t.Fatal("readiness was never checked before advertising")
	}
	if discoveryExistedAtReadiness {
		t.Fatal("the advertisement existed before readiness was checked")
	}

	read, err := ReadDiscovery(path)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if read.Token != "ephemeral-token" || read.Port != 11239 || read.Prefixes["umans-shaper"] != "/umans" {
		t.Fatalf("advertisement: got %+v", read)
	}

	cancel()
	<-done

	// The child saw the credential under the shaper's own spelling, and the
	// arguments named the variable without revealing it.
	if len(environment) != 1 || environment[0] != "SHAPER_PROVIDER_UMANS_API_KEY=umans-secret-value" {
		t.Fatalf("child environment: got %v, want exactly the shaper credential", environment)
	}
	if strings.Contains(strings.Join(args, " "), "umans-secret-value") {
		t.Fatalf("the shaper arguments carry the credential value: %v", args)
	}
	if !strings.Contains(strings.Join(args, " "), "-auth-source=env:SHAPER_PROVIDER_UMANS_API_KEY") {
		t.Fatalf("the shaper arguments do not name the credential variable: %v", args)
	}
	// Shutdown removed the advertisement and reaped the child, and the child's
	// exit code reached the caller.
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if code != 7 {
		t.Fatalf("exit code: got %d, want the child's 7", code)
	}
	if !child.terminated {
		t.Fatal("the child was not reaped")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the advertisement survived shutdown: %v", err)
	}
}

func TestRunNeverAdvertisesAnUnreachableShaper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.discovery")
	child := &fakeChild{}
	deps := Dependencies{
		Spawn:     func([]string, []string) (Child, error) { return child, nil },
		WaitReady: func(context.Context) error { return errors.New("nothing accepts connections") },
		NewToken:  func() (string, error) { return "token", nil },
		Now:       time.Now,
	}
	cfg := Config{ShaperBinary: "shaper", Host: "127.0.0.1", Port: 11239, DiscoveryPath: path, ReadyTimeout: time.Second}

	_, err := Run(context.Background(), cfg, umansMounts(), map[string]string{"UMANS_API_KEY": "value"}, deps)
	if err == nil || !strings.Contains(err.Error(), "never became reachable") {
		t.Fatalf("Run: got %v, want a readiness failure", err)
	}
	if !child.terminated {
		t.Fatal("a shaper that never became reachable was not reaped")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("an unreachable shaper was advertised: %v", statErr)
	}
}

func TestRunRefusesToStartWithoutTheQuartet(t *testing.T) {
	base := Config{ShaperBinary: "shaper", Host: "127.0.0.1", Port: 11239, DiscoveryPath: filepath.Join(t.TempDir(), "d"), ReadyTimeout: time.Second}
	credentials := map[string]string{"UMANS_API_KEY": "value"}

	noBinary := base
	noBinary.ShaperBinary = ""
	if _, err := Run(context.Background(), noBinary, umansMounts(), credentials, DefaultDependencies(base)); err == nil {
		t.Error("Run: want an error without a shaper binary")
	}

	noPath := base
	noPath.DiscoveryPath = ""
	if _, err := Run(context.Background(), noPath, umansMounts(), credentials, DefaultDependencies(base)); err == nil {
		t.Error("Run: want an error without a discovery path")
	}

	if _, err := Run(context.Background(), base, umansMounts(), credentials, Dependencies{Spawn: func([]string, []string) (Child, error) { return &fakeChild{}, nil }}); err == nil {
		t.Error("Run: want an error when no readiness check is configured, rather than advertising a gateway that may not be listening")
	}

	if _, err := Run(context.Background(), base, umansMounts(), map[string]string{}, DefaultDependencies(base)); err == nil {
		t.Error("Run: want an error when a credential is missing")
	}
}
