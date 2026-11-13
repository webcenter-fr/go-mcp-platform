package developer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/safety"
)

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{WorkspaceRoot: t.TempDir(), Engine: EngineConfig{Mode: "sidecar"}}
	applyDefaults(cfg)

	assert.Equal(t, "alpine:3.20", cfg.BaseImage)
	assert.Equal(t, 5*time.Minute, cfg.DefaultTimeout)
	assert.Equal(t, 15*time.Minute, cfg.MaxTimeout)
	assert.Equal(t, 256000, cfg.MaxOutputBytes)
	assert.Equal(t, 4, cfg.MaxConcurrentExecs)
	assert.Equal(t, "1000:1000", cfg.ContainerUser)
	assert.Equal(t, 24*time.Hour, cfg.WorkspaceTTL)
	assert.Equal(t, time.Hour, cfg.WorkspaceGCInterval)
	require.NotNil(t, cfg.Scaffolding)
	assert.True(t, *cfg.Scaffolding)
	require.NotNil(t, cfg.Egress)
	require.NotNil(t, cfg.Egress.DefaultDeny)
	assert.True(t, *cfg.Egress.DefaultDeny)
	assert.Equal(t, DefaultEgressAllowHosts, cfg.Egress.AllowHosts)
	assert.Equal(t, "/usr/local/bin/dagger", cfg.Engine.CLIPath)
}

func TestApplyDefaultsDaggerKubernetesRunnerHost(t *testing.T) {
	cfg := &Config{WorkspaceRoot: t.TempDir(), Engine: EngineConfig{Mode: "dagger-kubernetes", CloudURL: "https://dagger.example.com"}}
	applyDefaults(cfg)
	assert.Equal(t, "dagger-cloud://self", cfg.Engine.RunnerHost)
}

func TestApplyDefaultsGitHubTimeout(t *testing.T) {
	cfg := &Config{
		WorkspaceRoot: t.TempDir(),
		Engine:        EngineConfig{Mode: "sidecar"},
		GitHub:        &GitHubConfig{Instances: map[string]GitHubInstance{"corp": {Token: "t"}}},
	}
	applyDefaults(cfg)
	assert.Equal(t, 30*time.Second, cfg.GitHub.Instances["corp"].Timeout)
}

func TestNewServerValidation(t *testing.T) {
	validRoot := t.TempDir()
	tests := []struct {
		name string
		cfg  *Config
	}{
		{name: "missing workspace root", cfg: &Config{Engine: EngineConfig{Mode: "sidecar"}}},
		{name: "relative workspace root", cfg: &Config{WorkspaceRoot: "relative/path", Engine: EngineConfig{Mode: "sidecar"}}},
		{name: "system workspace root", cfg: &Config{WorkspaceRoot: "/tmp", Engine: EngineConfig{Mode: "sidecar"}}},
		{name: "invalid engine mode", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "nomad"}}},
		{name: "dagger-kubernetes without cloud URL", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "dagger-kubernetes"}}},
		{name: "github without instances", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "sidecar"}, GitHub: &GitHubConfig{}}},
		{name: "max output below 1024", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "sidecar"}, MaxOutputBytes: 512}},
		{name: "bad CIDR", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "sidecar"}, Egress: &EgressConfig{AllowCIDRs: []string{"not-a-cidr"}}}},
		{name: "bad hostname", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "sidecar"}, Egress: &EgressConfig{AllowHosts: []string{"not a host"}}}},
		{name: "sidecar without runner host", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "sidecar"}}},
		{name: "cloud without token", cfg: &Config{WorkspaceRoot: validRoot, Engine: EngineConfig{Mode: "cloud"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.Runner = &FakeRunner{}
			_, _, err := NewServerWithRunner(context.Background(), tt.cfg, nil)
			assert.Error(t, err)
		})
	}
}

func TestExtraBlocklistExtendsDefaults(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	cfg.ExtraBlocklist = []string{`\bcurl\b`}
	_, closer, err := NewServerWithRunner(context.Background(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer.Close() })

	compiled, err := safety.CompileBlocklist(append(append([]string(nil), safety.DefaultCommandBlocklist...), cfg.ExtraBlocklist...))
	require.NoError(t, err)
	assert.Error(t, safety.CheckBlocklist(compiled, []string{"rm", "-rf", "/"}), "default patterns must survive extension")
	assert.Error(t, safety.CheckBlocklist(compiled, []string{"curl", "https://example.com"}), "extra patterns must apply")
}
