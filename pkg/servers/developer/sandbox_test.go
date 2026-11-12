package developer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/egress"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/profile"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/safety"
)

func newTestSandbox(t *testing.T, cfg *Config, runner Runner) (*sandboxService, string) {
	t.Helper()
	applyDefaults(cfg)
	identityRoot := identityWorkspaceRoot(cfg.WorkspaceRoot, nil)
	require.NoError(t, os.MkdirAll(identityRoot, 0o755))
	blocklist, err := safety.CompileBlocklist(safety.DefaultCommandBlocklist)
	require.NoError(t, err)
	policy := &egress.Policy{
		AllowHosts:  cfg.Egress.AllowHosts,
		DefaultDeny: cfg.Egress.DefaultDeny != nil && *cfg.Egress.DefaultDeny,
	}
	return &sandboxService{
		cfg:          cfg,
		runner:       runner,
		blocklist:    blocklist,
		resolver:     profile.NewResolver(profile.WithImageOverrides(cfg.ImageOverrides)),
		policy:       policy,
		identityRoot: identityRoot,
		redactor:     newRedactor(cfg),
	}, identityRoot
}

func TestSandboxSchemaHasNoEgressOrProfileOverride(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	applyDefaults(cfg)
	tools, err := newSandboxTools(cfg, &FakeRunner{}, nil, profile.NewResolver(), &egress.Policy{}, t.TempDir(), newRedactor(cfg))
	require.NoError(t, err)
	require.Len(t, tools, 2)

	info, err := tools[0].Info(context.Background())
	require.NoError(t, err)
	schema, err := info.ToJSONSchema()
	require.NoError(t, err)
	b, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "allowLocalNetwork")
	assert.NotContains(t, string(b), `"profile"`)
}

func TestSandboxExecParamsValidation(t *testing.T) {
	tests := []struct {
		name   string
		params SandboxExecParams
	}{
		{name: "empty command", params: SandboxExecParams{}},
		{name: "empty argv element", params: SandboxExecParams{Command: []string{"go", ""}}},
		{name: "owner without repo", params: SandboxExecParams{Command: []string{"go"}, Owner: "o"}},
		{name: "repo without owner", params: SandboxExecParams{Command: []string{"go"}, Repo: "r"}},
		{name: "oversized stdin", params: SandboxExecParams{Command: []string{"go"}, Stdin: strings.Repeat("x", 1048577)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t, &FakeRunner{})
			svc, _ := newTestSandbox(t, cfg, &FakeRunner{})
			_, err := svc.exec(context.Background(), &tt.params)
			require.Error(t, err)
		})
	}
}

func TestSandboxExecHappyPath(t *testing.T) {
	runner := &FakeRunner{Respond: func(ExecRequest) (ExecResult, error) {
		return ExecResult{ExitCode: 0, Stdout: "ok", Duration: 10 * time.Millisecond}, nil
	}}
	cfg := testConfig(t, runner)
	svc, identityRoot := newTestSandbox(t, cfg, runner)
	writeFile(t, identityRoot, "go.mod", "module test\n")

	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go", "version"}})
	require.NoError(t, err)

	var result SandboxExecOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "ok", result.Stdout)
	assert.Equal(t, "golang", result.Profile)
	assert.Equal(t, "golang:1.22", result.BaseImage)
	assert.Equal(t, identityRoot, result.Workspace)

	requests := runner.RecordedRequests()
	require.Len(t, requests, 1)
	req := requests[0]
	assert.Equal(t, []string{"go", "version"}, req.Command)
	assert.Equal(t, identityRoot, req.WorkspacePath)
	assert.Equal(t, "1000:1000", req.User)
	assert.Equal(t, "golang:1.22", req.BaseImage)
	assert.Equal(t, "golang", req.Profile)
	assert.NotNil(t, req.Egress)
	assert.Contains(t, req.CacheMounts, "/go/pkg/mod")
}

func TestSandboxExecRepoWorkspace(t *testing.T) {
	runner := &FakeRunner{}
	cfg := testConfig(t, runner)
	svc, identityRoot := newTestSandbox(t, cfg, runner)
	repoDir := repoWorkspacePath(identityRoot, "owner", "repo")
	writeFile(t, repoDir, "go.mod", "module test\n")

	_, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go", "test"}, Owner: "owner", Repo: "repo"})
	require.NoError(t, err)
	requests := runner.RecordedRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, repoDir, requests[0].WorkspacePath)
}

func TestSandboxExecMissingWorkspace(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	svc, _ := newTestSandbox(t, cfg, &FakeRunner{})
	_, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}, Owner: "owner", Repo: "missing"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkspaceMissing)
}

func TestSandboxExecNonZeroExitIsNotAnError(t *testing.T) {
	runner := &FakeRunner{Respond: func(ExecRequest) (ExecResult, error) {
		return ExecResult{ExitCode: 2, Stderr: "compile error"}, nil
	}}
	cfg := testConfig(t, runner)
	svc, _ := newTestSandbox(t, cfg, runner)
	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go", "build"}})
	require.NoError(t, err)
	var result SandboxExecOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, 2, result.ExitCode)
	assert.Equal(t, "compile error", result.Stderr)
}

func TestSandboxExecDryRunDoesNotExecute(t *testing.T) {
	runner := &FakeRunner{}
	cfg := testConfig(t, runner)
	svc, _ := newTestSandbox(t, cfg, runner)
	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go", "version"}, DryRun: true})
	require.NoError(t, err)
	var result SandboxExecOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.True(t, result.DryRun)
	assert.Empty(t, runner.RecordedRequests())
}

func TestSandboxExecTimeoutCapped(t *testing.T) {
	runner := &FakeRunner{}
	cfg := testConfig(t, runner)
	cfg.MaxTimeout = 15 * time.Minute
	svc, _ := newTestSandbox(t, cfg, runner)
	_, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}, Timeout: "1h"})
	require.NoError(t, err)
	requests := runner.RecordedRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, 15*time.Minute, requests[0].Timeout)
}

func TestSandboxExecBlocklistedCommand(t *testing.T) {
	runner := &FakeRunner{}
	cfg := testConfig(t, runner)
	svc, _ := newTestSandbox(t, cfg, runner)
	_, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"rm", "-rf", "/"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	assert.Empty(t, runner.RecordedRequests())
}

func TestSandboxExecEnvRejection(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{name: "github token", env: map[string]string{"GITHUB_TOKEN": "x"}, wantErr: true},
		{name: "kubeconfig", env: map[string]string{"kubeconfig": "x"}, wantErr: true},
		{name: "ld preload", env: map[string]string{"LD_PRELOAD": "x"}, wantErr: true},
		{name: "path", env: map[string]string{"PATH": "x"}, wantErr: true},
		{name: "secret shaped", env: map[string]string{"MY_SECRET": "x"}, wantErr: true},
		{name: "proxy", env: map[string]string{"HTTP_PROXY": "x"}, wantErr: true},
		{name: "safe", env: map[string]string{"ok_VAR": "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &FakeRunner{}
			cfg := testConfig(t, runner)
			svc, _ := newTestSandbox(t, cfg, runner)
			_, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}, Env: tt.env})
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrEnvRejected)
				assert.Empty(t, runner.RecordedRequests())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSandboxExecTruncatesOutput(t *testing.T) {
	runner := &FakeRunner{Respond: func(ExecRequest) (ExecResult, error) {
		return ExecResult{Stdout: strings.Repeat("a", 2000), Stderr: strings.Repeat("b", 2000)}, nil
	}}
	cfg := testConfig(t, runner)
	cfg.MaxOutputBytes = 1024
	svc, _ := newTestSandbox(t, cfg, runner)
	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}})
	require.NoError(t, err)
	var result SandboxExecOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.True(t, result.Truncated)
	assert.Contains(t, result.Stdout, "bytes omitted")
	assert.Contains(t, result.Stderr, "bytes omitted")
}

func TestSandboxExecSanitizesNonUTF8(t *testing.T) {
	runner := &FakeRunner{Respond: func(ExecRequest) (ExecResult, error) {
		return ExecResult{Stdout: string([]byte{0xff, 0xfe, 'o', 'k'})}, nil
	}}
	cfg := testConfig(t, runner)
	svc, _ := newTestSandbox(t, cfg, runner)
	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}})
	require.NoError(t, err)
	var result SandboxExecOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.True(t, utf8.ValidString(result.Stdout))
}

func TestSandboxExecRedactsSecrets(t *testing.T) {
	runner := &FakeRunner{Respond: func(ExecRequest) (ExecResult, error) {
		return ExecResult{Stdout: "token=ghp_012345678901234567890123456789012345"}, nil
	}}
	cfg := testConfig(t, runner)
	svc, _ := newTestSandbox(t, cfg, runner)
	out, err := svc.exec(context.Background(), &SandboxExecParams{Command: []string{"go"}})
	require.NoError(t, err)
	assert.NotContains(t, out, "ghp_012345678901234567890123456789012345")
	assert.Contains(t, out, redactedMarker)
}

func TestSandboxBusy(t *testing.T) {
	runner := &daggerRunner{sem: make(chan struct{}, 1)}
	runner.sem <- struct{}{}
	_, err := runner.Exec(context.Background(), ExecRequest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSandboxBusy)
}

func TestSandboxInfo(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	svc, identityRoot := newTestSandbox(t, cfg, &FakeRunner{})
	out, err := svc.info(context.Background(), &SandboxInfoParams{})
	require.NoError(t, err)
	var result SandboxInfoOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.False(t, result.EgressEnforced)
	assert.False(t, result.CredentialsInSandbox)
	assert.True(t, result.EgressDefaultDeny)
	assert.Equal(t, identityRoot, result.WorkspaceRoot)
	assert.Equal(t, "sidecar", result.EngineMode)
	assert.Contains(t, result.Profiles, "golang")
}

func TestResolveImageAllowedImages(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	applyDefaults(cfg)
	cfg.AllowedImages = []string{"golang:1.22"}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	resolver := profile.NewResolver()
	name, image, err := resolveImage(context.Background(), cfg, resolver, dir)
	require.NoError(t, err)
	assert.Equal(t, "golang", name)
	assert.Equal(t, "golang:1.22", image)

	cfg.AllowedImages = []string{"alpine:3.20"}
	_, _, err = resolveImage(context.Background(), cfg, resolver, dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImageNotAllowed)
}

func TestResolveImageFallbackUsesConfigBaseImage(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	applyDefaults(cfg)
	cfg.BaseImage = "alpine:3.21"
	name, image, err := resolveImage(context.Background(), cfg, profile.NewResolver(), t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "default", name)
	assert.Equal(t, "alpine:3.21", image)
}

func TestResolveWorkdirRelative(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	rel, err := resolveWorkdir(root, "sub")
	require.NoError(t, err)
	assert.Equal(t, "sub", rel)
}
