package exec

import (
	"os"
	"runtime"
	"testing"
)

// TestMain re-executes the binary as an escaper helper when set.
const escaperHelperEnv = "OSM_EXEC_ESCAPER_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(escaperHelperEnv) == "1" {
		runEscaperHelper()
	}
	os.Exit(m.Run())
}

// escaperScript builds a POSIX script that escapes into its own session.
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
