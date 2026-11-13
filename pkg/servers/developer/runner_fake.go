package developer

import (
	"context"
	"strings"
	"sync"
)

// FakeRunner is an in-memory Runner for tests and for consumers that want to
// exercise the tool surface without a Dagger engine.
type FakeRunner struct {
	// Respond returns the result for a request. When nil, Exec returns a
	// zero-exit result echoing the command.
	Respond func(ExecRequest) (ExecResult, error)
	// Requests records every received request, in order.
	Requests []ExecRequest
	mu       sync.Mutex
}

// Exec records the request and delegates to Respond.
func (f *FakeRunner) Exec(_ context.Context, req ExecRequest) (ExecResult, error) {
	f.mu.Lock()
	f.Requests = append(f.Requests, req)
	f.mu.Unlock()
	if f.Respond != nil {
		return f.Respond(req)
	}
	return ExecResult{ExitCode: 0, Stdout: strings.Join(req.Command, " ")}, nil
}

// Close is a no-op.
func (f *FakeRunner) Close() error { return nil }

// RecordedRequests returns a copy of the recorded requests.
func (f *FakeRunner) RecordedRequests() []ExecRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ExecRequest(nil), f.Requests...)
}
