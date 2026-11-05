package exec

import (
	"os"
	"runtime"
	"testing"
)

// TestMain re-executes the test binary as an "escaper" helper when
// OSM_EXEC_ESCAPER_HELPER=1 is set: the helper calls setsid() (leaving the
// leader's process group and session), writes a readiness file, and sleeps —
// holding the inherited stdout write end, which is exactly the writer the
// process-group kill cannot reach. Tests use this instead of the `setsid`
// binary so the escape works on every POSIX platform (macOS has no setsid(1)
// in its default PATH) and is deterministic: the leader waits for the
// readiness file, which the helper writes only AFTER setsid() returned, so
// the reaper's pre-reap group kill can never land before the escape.
const escaperHelperEnv = "OSM_EXEC_ESCAPER_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(escaperHelperEnv) == "1" {
		runEscaperHelper()
	}
	os.Exit(m.Run())
}

// escaperScript returns a POSIX shell script that backgrounds the test binary
// re-executed as the setsid helper (writing readyFile only after setsid()),
// waits for that readiness file, and then prints parent-done and exits. The
// leader therefore exits only after the descendant has verifiably left the
// process group, so the escaped writer always survives the pre-reap group
// kill and keeps the stdout write end open. Only called from tests that skip
// on Windows.
func escaperScript(readyFile string) string {
	if runtime.GOOS == "windows" {
		panic("escaperScript is POSIX-only; callers must skip on Windows")
	}
	return `OSM_EXEC_ESCAPER_HELPER=1 "` + os.Args[0] + `" ` + shellQuote(readyFile) + ` & ` +
		`i=0; while [ ! -s ` + shellQuote(readyFile) + ` ] && [ $i -lt 500 ]; do sleep 0.01; i=$((i+1)); done; ` +
		`echo parent-done`
}

// shellQuote single-quotes a string for POSIX sh.
func shellQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
