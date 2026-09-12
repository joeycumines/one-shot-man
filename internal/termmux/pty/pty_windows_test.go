//go:build windows

package pty

import (
	"strings"
	"testing"
)

func TestBuildCommandLineQuotesExecutableWithoutArguments(t *testing.T) {
	got := buildCommandLine(`C:\Users\under user\AppData\Local\Temp\helper.exe`, nil)
	if !strings.HasPrefix(got, `"C:\Users\under user\AppData\Local\Temp\helper.exe"`) {
		t.Fatalf("buildCommandLine returned unquoted executable path: %q", got)
	}
}

func TestBuildCommandLineEscapesArguments(t *testing.T) {
	got := buildCommandLine(`C:\Users\under user\helper.exe`, []string{"hello world"})
	if !strings.HasPrefix(got, `"C:\Users\under user\helper.exe"`) || !strings.Contains(got, `"hello world"`) {
		t.Fatalf("buildCommandLine did not escape executable and argument: %q", got)
	}
}
