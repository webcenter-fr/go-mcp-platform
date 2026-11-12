package developer

import (
	"context"
	"time"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/egress"
)

// Runner executes a command in an isolated container. It is the seam that keeps
// the family unit-testable: the real implementation connects to a Dagger
// engine, while tests inject a fake. Implementations MUST NOT make any
// credential available to the executed command.
type Runner interface {
	// Exec runs req and returns the result. It must honor req.Timeout, cap
	// captured output at req.MaxOutputBytes, and return ErrSandboxBusy when
	// concurrency limits are exceeded.
	Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
	// Close releases engine resources. Safe to call more than once.
	Close() error
}

// ExecRequest is a fully-resolved sandbox execution. Every field is decided
// server-side; nothing here is taken verbatim from tool arguments except
// Command, Stdin, Env and RelWorkdir (all validated beforehand).
type ExecRequest struct {
	BaseImage      string
	Profile        string
	WorkspacePath  string // absolute host path, mounted at /workspace
	RelWorkdir     string // validated path relative to /workspace
	Command        []string
	Stdin          string
	Env            map[string]string
	User           string
	CacheMounts    []string // container paths backed by cache volumes
	CacheKeyPrefix string
	Egress         *egress.Policy
	Timeout        time.Duration
	MaxOutputBytes int
	PersistChanges bool
}

// ExecResult is the outcome of an execution.
type ExecResult struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Truncated bool
	Duration  time.Duration
	Persisted bool
}
