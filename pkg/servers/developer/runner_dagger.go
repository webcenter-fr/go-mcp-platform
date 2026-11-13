package developer

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"dagger.io/dagger"
	emperrors "emperror.dev/errors"

	daggerlib "github.com/webcenter-fr/eino-ext/libs/toolkit/dagger"
)

// daggerRunner is the real Dagger-backed Runner. The engine connection is lazy:
// it is established on the first Exec, so NewServer, Info and tools/list work
// with no engine available (and `make test` needs none).
type daggerRunner struct {
	cfg *Config
	sem chan struct{}

	mu     sync.Mutex
	client *daggerlib.Client
}

// NewDaggerRunner builds the real Dagger-backed Runner. The engine connection
// is LAZY: it is established on the first Exec, so NewServer, Info and
// tools/list work with no engine available (and `make test` needs none).
func NewDaggerRunner(ctx context.Context, cfg *Config) (Runner, error) {
	if cfg == nil {
		return nil, emperrors.New("developer: config is required")
	}
	applyEngineEnv(cfg)
	return &daggerRunner{
		cfg: cfg,
		sem: make(chan struct{}, cfg.MaxConcurrentExecs),
	}, nil
}

// EgressEnforced reports that this runner applies the egress policy through
// the upstream proxy. It satisfies the unexported egressEnforcer interface.
func (r *daggerRunner) EgressEnforced() bool { return true }

// applyEngineEnv exports the Dagger CLI/engine environment once at
// construction. The SDK's runner-host path execs the CLI binary, so these must
// be in the process environment before the first connection.
func applyEngineEnv(cfg *Config) {
	for key, value := range engineEnv(cfg) {
		_ = os.Setenv(key, value)
	}
}

// engineEnv returns the environment variables implied by the engine mode. It is
// a pure helper so the mode mapping is unit-testable.
func engineEnv(cfg *Config) map[string]string {
	env := map[string]string{}
	if cfg.Engine.CLIPath != "" {
		env["_EXPERIMENTAL_DAGGER_CLI_BIN"] = cfg.Engine.CLIPath
	}
	if cfg.Engine.RunnerHost != "" {
		env["_EXPERIMENTAL_DAGGER_RUNNER_HOST"] = cfg.Engine.RunnerHost
	}
	if cfg.Engine.CloudURL != "" {
		env["DAGGER_CLOUD_URL"] = cfg.Engine.CloudURL
	}
	if cfg.Engine.CloudToken != "" {
		env["DAGGER_CLOUD_TOKEN"] = cfg.Engine.CloudToken
	}
	if cfg.Engine.EngineVersion != "" {
		env["_EXPERIMENTAL_DAGGER_TAG"] = cfg.Engine.EngineVersion
	}
	return env
}

// connect establishes the engine connection on first use. A failed connection
// is not cached: the next Exec retries.
func (r *daggerRunner) connect(ctx context.Context) (*daggerlib.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	client, err := daggerlib.NewClient(ctx, &daggerlib.EngineConfig{
		Workdir: r.cfg.WorkspaceRoot,
	})
	if err != nil {
		return nil, emperrors.Wrap(ErrEngineUnavailable, err.Error())
	}
	r.client = client
	return client, nil
}

// Exec runs the command in a fresh container.
func (r *daggerRunner) Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	default:
		return ExecResult{}, ErrSandboxBusy
	}

	client, err := r.connect(ctx)
	if err != nil {
		return ExecResult{}, err
	}

	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	opts := []daggerlib.ContainerOpt{
		daggerlib.WithWorkspace(req.WorkspacePath),
		daggerlib.WithUser(req.User),
		daggerlib.WithEgressPolicy(req.Egress),
	}
	for _, mount := range req.CacheMounts {
		opts = append(opts, daggerlib.WithCacheVolume(mount, req.CacheKeyPrefix+":"+mount))
	}
	if len(r.cfg.Engine.RegistryAuth) > 0 {
		auth := make(map[string]daggerlib.RegistryAuth, len(r.cfg.Engine.RegistryAuth))
		for host, entry := range r.cfg.Engine.RegistryAuth {
			auth[host] = daggerlib.RegistryAuth{Username: entry.Username, Password: entry.Password}
		}
		opts = append(opts, daggerlib.WithRegistryAuth(auth))
	}

	cont, err := client.Container(execCtx, req.BaseImage, opts...)
	if err != nil {
		return ExecResult{}, emperrors.Wrapf(err, "failed to build container from image %q", req.BaseImage)
	}

	// Never forward the MCP process environment: start from the base image's
	// own environment, remove every credential-shaped name, then apply the
	// validated per-call Env.
	for _, name := range scrubEnvNames() {
		cont = cont.WithoutEnvVariable(name)
	}
	for key, value := range buildExecEnv(req) {
		cont = cont.WithEnvVariable(key, value)
	}
	if req.RelWorkdir != "" {
		cont = cont.WithWorkdir("/workspace/" + strings.TrimPrefix(req.RelWorkdir, "/"))
	}

	start := time.Now()
	cont = cont.WithExec(req.Command, dagger.ContainerWithExecOpts{
		Expect: dagger.ReturnTypeAny,
		Stdin:  req.Stdin,
	})
	if _, err := cont.Sync(execCtx); err != nil {
		return ExecResult{}, emperrors.Wrapf(err, "failed to execute command in image %q", req.BaseImage)
	}
	stdout, err := cont.Stdout(execCtx)
	if err != nil {
		return ExecResult{}, emperrors.Wrap(err, "failed to read command stdout")
	}
	stderr, err := cont.Stderr(execCtx)
	if err != nil {
		return ExecResult{}, emperrors.Wrap(err, "failed to read command stderr")
	}
	exitCode, err := cont.ExitCode(execCtx)
	if err != nil {
		return ExecResult{}, emperrors.Wrap(err, "failed to read command exit code")
	}

	persisted := false
	if req.PersistChanges {
		if err := client.ExportWorkspace(execCtx, cont, req.WorkspacePath); err != nil {
			return ExecResult{}, emperrors.Wrap(err, "failed to export workspace changes")
		}
		persisted = true
	}

	return ExecResult{
		ExitCode:  exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
		Duration:  time.Since(start),
		Persisted: persisted,
	}, nil
}

// Close releases the engine connection. Safe to call more than once.
func (r *daggerRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client == nil {
		return nil
	}
	err := r.client.Close()
	r.client = nil
	return err
}

// scrubEnvNames returns the environment variable names removed from every
// container before the per-call Env is applied.
//
// The proxy variables are intentionally absent: daggerlib.WithEgressPolicy in
// Container() sets them to route traffic through the in-process egress proxy,
// so scrubbing them would disable enforcement. The container starts from the
// base image environment, not the MCP process environment, so this scrub
// remains defense-in-depth for the remaining names.
func scrubEnvNames() []string {
	return []string{
		"GITHUB_TOKEN", "GH_TOKEN", "KUBECONFIG",
		"LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT",
		"DAGGER_CLOUD_TOKEN", "DAGGER_CLOUD_URL", "DAGGER_SESSION_TOKEN",
		"_EXPERIMENTAL_DAGGER_CLI_BIN", "_EXPERIMENTAL_DAGGER_RUNNER_HOST",
		"_EXPERIMENTAL_DAGGER_TAG",
	}
}

// buildExecEnv returns the validated per-call environment, dropping any key
// that slipped past validation (defense in depth).
func buildExecEnv(req ExecRequest) map[string]string {
	env := make(map[string]string, len(req.Env))
	for key, value := range req.Env {
		if validateEnv(map[string]string{key: value}) != nil {
			continue
		}
		env[key] = value
	}
	return env
}
