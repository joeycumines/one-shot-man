//go:build unix

package exec

import (
	"os"
	"syscall"
	"time"
)

// runEscaperHelper runs inside the re-executed test binary (see TestMain in
// spawn_escaper_test.go): call setsid() to leave the leader's process group
// and session, write the readiness file so the leader knows the escape is
// complete, then hold the inherited stdout write end for 30s — exactly the
// writer the process-group kill cannot reach.
func runEscaperHelper() {
	// os.Args[1] is the readiness file path.
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	if _, err := syscall.Setsid(); err != nil {
		os.Exit(3)
	}
	_ = os.WriteFile(os.Args[1], []byte("ok"), 0o644)
	time.Sleep(30 * time.Second)
	os.Exit(0)
}
