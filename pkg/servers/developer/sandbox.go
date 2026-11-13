package developer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	daggerlib "github.com/webcenter-fr/eino-ext/libs/toolkit/dagger"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/egress"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/profile"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/safety"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/workspace"
)

// SandboxExecParams are the parameters of developer_sandbox_exec.
//
// Deliberately absent, by comparison with eino-ext shell.Params:
//   - allowLocalNetwork — an LLM must never be able to widen the egress policy.
//   - profile           — an LLM must never be able to choose the base image.
type SandboxExecParams struct {
	// Command is the argv to execute. It is NOT passed through a shell, so
	// there is no shell metacharacter interpretation (no pipes, no redirection,
	// no command substitution).
	Command []string `json:"command" validate:"required,min=1,dive,required" jsonschema:"(required) Command to execute as an argv array. It is executed directly (no shell), so pipes, redirection and substitution are not interpreted."`

	// Owner and Repo select the workspace <root>/<identity>/<owner>/<repo>.
	// They must be provided together. When both are empty the command runs in
	// the identity's workspace root.
	Owner string `json:"owner,omitempty" validate:"omitempty,max=100" jsonschema:"(optional) Repository owner; selects the workspace together with repo"`
	Repo  string `json:"repo,omitempty" validate:"omitempty,max=100" jsonschema:"(optional) Repository name; selects the workspace together with owner"`

	// Workdir is an optional path relative to the workspace, used as the
	// command's working directory. It must stay inside the workspace;
	// traversal and symlinks are rejected.
	Workdir string `json:"workdir,omitempty" validate:"omitempty,max=512" jsonschema:"(optional) Working directory relative to the workspace root"`

	// Stdin is text piped to the command's standard input.
	Stdin string `json:"stdin,omitempty" validate:"omitempty,max=1048576" jsonschema:"(optional) Text piped to the command's standard input"`

	// Env sets additional environment variables for this command. Keys are
	// validated against a safe pattern and a denylist (see §8.4): the loader
	// variables, the proxy variables and anything credential-shaped are
	// rejected.
	Env map[string]string `json:"env,omitempty" validate:"omitempty" jsonschema:"(optional) Extra environment variables for this command"`

	// PersistChanges exports the container /workspace back onto the host
	// workspace (excluding .git) after a successful exec. Default false: the
	// run is a throwaway sandbox.
	PersistChanges bool `json:"persistChanges,omitempty" jsonschema:"(optional) Export container workspace changes back to the host workspace, excluding .git (default false)"`

	// Timeout is a Go duration string, capped by Config.MaxTimeout.
	Timeout string `json:"timeout,omitempty" validate:"omitempty" jsonschema:"(optional) Timeout duration string (e.g. '30s', '5m'), capped by the server maximum"`

	// FilterPattern is a Go RE2 regex applied per output line.
	FilterPattern string `json:"filterPattern,omitempty" validate:"omitempty,max=512" jsonschema:"(optional) Go RE2 regex applied per output line. RE2 has no lookahead, lookbehind or backreferences."`

	// DryRun returns the resolved plan (image, workspace, argv, caches, egress)
	// without executing.
	DryRun bool `json:"dryRun,omitempty" jsonschema:"(optional) Return the resolved execution plan without executing"`
}

// SandboxExecOutput is the JSON result of developer_sandbox_exec.
type SandboxExecOutput struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	Truncated      bool   `json:"truncated"`
	DurationMillis int64  `json:"durationMillis"`
	BaseImage      string `json:"baseImage"`
	Profile        string `json:"profile"`
	Workspace      string `json:"workspace"`
	Persisted      bool   `json:"persisted"`
	DryRun         bool   `json:"dryRun,omitempty"`
}

// SandboxInfoParams has no fields: the tool reports server-side configuration.
type SandboxInfoParams struct{}

// SandboxInfoOutput describes the sandbox an identity will get.
type SandboxInfoOutput struct {
	Profiles             map[string]string `json:"profiles"` // profile -> base image
	DefaultBaseImage     string            `json:"defaultBaseImage"`
	EgressDefaultDeny    bool              `json:"egressDefaultDeny"`
	EgressAllowHosts     []string          `json:"egressAllowHosts"`
	EgressEnforced       bool              `json:"egressEnforced"` // true when the runner applies a deny-by-default policy
	AllowLocalNetwork    bool              `json:"allowLocalNetwork"`
	CredentialsInSandbox bool              `json:"credentialsInSandbox"` // always false
	DefaultTimeout       string            `json:"defaultTimeout"`
	MaxTimeout           string            `json:"maxTimeout"`
	MaxOutputBytes       int               `json:"maxOutputBytes"`
	ContainerUser        string            `json:"containerUser"`
	WorkspaceRoot        string            `json:"workspaceRoot"` // the identity's own root
	EngineMode           string            `json:"engineMode"`
}

// egressEnforcer is implemented by runners that apply the egress policy. It is
// deliberately unexported and not part of Runner, so external Runner
// implementations keep compiling; a runner that does not implement it is
// reported as not enforcing egress.
type egressEnforcer interface{ EgressEnforced() bool }

// sandboxService holds the per-identity sandbox tool state.
type sandboxService struct {
	cfg          *Config
	runner       Runner
	blocklist    []*regexp.Regexp
	resolver     *profile.Resolver
	policy       *egress.Policy
	identityRoot string
	redactor     *redactor
}

// newSandboxTools builds the two sandbox tools for one identity.
func newSandboxTools(cfg *Config, runner Runner, blocklist []*regexp.Regexp, resolver *profile.Resolver, policy *egress.Policy, identityRoot string, redactor *redactor) ([]tool.InvokableTool, error) {
	svc := &sandboxService{
		cfg:          cfg,
		runner:       runner,
		blocklist:    blocklist,
		resolver:     resolver,
		policy:       policy,
		identityRoot: identityRoot,
		redactor:     redactor,
	}
	execTool, err := utils.InferTool("developer_sandbox_exec", sandboxExecDescription, svc.exec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build developer_sandbox_exec")
	}
	infoTool, err := utils.InferTool("developer_sandbox_info", sandboxInfoDescription, svc.info)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build developer_sandbox_info")
	}
	return []tool.InvokableTool{execTool, infoTool}, nil
}

const sandboxExecDescription = `
** General Purpose **
Runs a command in an isolated, credential-free Dagger container. The command is
executed as an argv array (no shell), so pipes, redirection and command
substitution are not interpreted. The container has no GitHub token, no
kubeconfig, no SSH keys and no cloud credentials. Network egress is restricted
by the operator's policy.

** Workspace **
Pass owner and repo to run inside a repository cloned with github_repo_clone.
Omit both to run in your identity's workspace root. Set persistChanges=true to
export the container workspace back to the host (excluding .git).

** Output **
Returns JSON with exitCode, stdout, stderr, truncated, durationMillis, baseImage,
profile, workspace and persisted. A non-zero exit code is a normal result, not a
tool error: read the compiler/test output and self-correct.
`

const sandboxInfoDescription = `
** General Purpose **
Reports the effective sandbox configuration for the caller: available profiles
and base images, egress policy, timeouts, output cap, container user, workspace
root and engine mode. It does not contact the Dagger engine.
`

func (s *sandboxService) exec(ctx context.Context, params *SandboxExecParams) (string, error) {
	if err := validate.Struct(params); err != nil {
		return "", err
	}
	if err := validateEnv(params.Env); err != nil {
		return "", err
	}
	if err := safety.CheckBlocklist(s.blocklist, params.Command); err != nil {
		return "", err
	}

	workspacePath, err := resolveWorkspace(s.identityRoot, params.Owner, params.Repo)
	if err != nil {
		return "", err
	}
	if params.PersistChanges && params.Owner == "" && params.Repo == "" {
		return "", errors.New("persistChanges requires owner and repo: the bare identity workspace cannot be exported")
	}

	relWorkdir, err := resolveWorkdir(workspacePath, params.Workdir)
	if err != nil {
		return "", err
	}

	profileName, image, err := resolveImage(ctx, s.cfg, s.resolver, workspacePath)
	if err != nil {
		return "", err
	}

	timeout, err := s.resolveTimeout(params.Timeout)
	if err != nil {
		return "", err
	}
	filter, err := compileFilter(params.FilterPattern)
	if err != nil {
		return "", err
	}

	if params.DryRun {
		return marshalJSON(SandboxExecOutput{
			BaseImage: image,
			Profile:   profileName,
			Workspace: workspacePath,
			DryRun:    true,
		})
	}

	req := ExecRequest{
		BaseImage:      image,
		Profile:        profileName,
		WorkspacePath:  workspacePath,
		RelWorkdir:     relWorkdir,
		Command:        params.Command,
		Stdin:          params.Stdin,
		Env:            params.Env,
		User:           s.cfg.ContainerUser,
		CacheMounts:    daggerlib.CacheMountsForProfile(profileName),
		CacheKeyPrefix: image + "\x00" + profileName,
		Egress:         s.policy,
		Timeout:        timeout,
		MaxOutputBytes: s.cfg.MaxOutputBytes,
		PersistChanges: params.PersistChanges,
	}

	result, err := s.runner.Exec(ctx, req)
	if err != nil {
		return "", errors.Wrapf(err, "sandbox execution failed (image %q, profile %q)", image, profileName)
	}

	stdout := filterOutput(result.Stdout, filter)
	stderr := filterOutput(result.Stderr, filter)
	stdout, stderr, truncated := truncateOutput(stdout, stderr, s.cfg.MaxOutputBytes)
	stdout = s.redactor.redact(strings.ToValidUTF8(stdout, "\uFFFD"))
	stderr = s.redactor.redact(strings.ToValidUTF8(stderr, "\uFFFD"))

	return marshalJSON(SandboxExecOutput{
		ExitCode:       result.ExitCode,
		Stdout:         stdout,
		Stderr:         stderr,
		Truncated:      truncated || result.Truncated,
		DurationMillis: result.Duration.Milliseconds(),
		BaseImage:      image,
		Profile:        profileName,
		Workspace:      workspacePath,
		Persisted:      result.Persisted,
	})
}

func (s *sandboxService) info(_ context.Context, _ *SandboxInfoParams) (string, error) {
	profiles := make(map[string]string, len(s.resolver.ImageMap))
	for name, image := range s.resolver.ImageMap {
		profiles[name] = image
	}
	e, ok := s.runner.(egressEnforcer)
	enforced := ok && e.EgressEnforced() && s.policy != nil && s.policy.DefaultDeny
	return marshalJSON(SandboxInfoOutput{
		Profiles:             profiles,
		DefaultBaseImage:     s.cfg.BaseImage,
		EgressDefaultDeny:    s.cfg.Egress.DefaultDeny != nil && *s.cfg.Egress.DefaultDeny,
		EgressAllowHosts:     append([]string(nil), s.cfg.Egress.AllowHosts...),
		EgressEnforced:       enforced,
		AllowLocalNetwork:    s.cfg.Egress.AllowLocalNetwork,
		CredentialsInSandbox: false,
		DefaultTimeout:       s.cfg.DefaultTimeout.String(),
		MaxTimeout:           s.cfg.MaxTimeout.String(),
		MaxOutputBytes:       s.cfg.MaxOutputBytes,
		ContainerUser:        s.cfg.ContainerUser,
		WorkspaceRoot:        s.identityRoot,
		EngineMode:           s.cfg.Engine.Mode,
	})
}

func (s *sandboxService) resolveTimeout(raw string) (time.Duration, error) {
	timeout := s.cfg.DefaultTimeout
	if raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return 0, errors.Wrapf(err, "invalid timeout %q", raw)
		}
		if parsed <= 0 {
			return 0, errors.Errorf("invalid timeout %q: must be positive", raw)
		}
		timeout = parsed
	}
	if timeout > s.cfg.MaxTimeout {
		timeout = s.cfg.MaxTimeout
	}
	return timeout, nil
}

// resolveWorkdir validates a workspace-relative working directory and returns
// it relative to /workspace.
func resolveWorkdir(workspacePath, workdir string) (string, error) {
	if workdir == "" {
		return "", nil
	}
	resolved, err := workspace.ResolvePath(workspacePath, workdir, false)
	if err != nil {
		return "", errors.Wrapf(err, "invalid workdir %q", workdir)
	}
	rel, err := filepath.Rel(workspacePath, resolved)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve workdir %q", workdir)
	}
	return filepath.ToSlash(rel), nil
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal tool output")
	}
	return string(b), nil
}
