//go:build windows

package pty

import "os"

// extraSignals is empty on Windows — SIGSTOP and SIGCONT are not supported.
var extraSignals = map[string]os.Signal{}
