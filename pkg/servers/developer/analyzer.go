package developer

import "context"

// DeepAnalyzer performs a wide, read-only analysis pass over a workspace and
// returns a Markdown report. It is the extension point for an in-engine
// Dagger-LLM analyzer (dag.LLM().WithEnv(...)), deliberately not implemented in
// Phase 1: Dagger's LLM API is experimental, an in-engine loop collapses
// per-tool-call approval granularity, and CI cannot exercise it.
type DeepAnalyzer interface {
	// Analyze runs a read-only pass. mode is one of the LifecycleStage values.
	// It MUST NOT mutate the workspace.
	Analyze(ctx context.Context, workspacePath string, mode LifecycleStage) (string, error)
}
