package session

import (
	"context"
	"os/exec"
)

// newExecCommand creates a new exec.Cmd. Factored out for testability
// and to isolate platform-specific process attributes.
func newExecCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// exitCoder is satisfied by *exec.ExitError.
type exitCoder interface {
	ExitCode() int
}

// newContextWithCancel creates a cancellable context.
func newContextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
