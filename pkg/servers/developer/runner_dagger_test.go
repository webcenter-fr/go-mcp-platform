package developer

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/egress"
)

func TestEngineEnv(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want map[string]string
	}{
		{
			name: "sidecar",
			cfg:  &Config{Engine: EngineConfig{Mode: "sidecar", RunnerHost: "tcp://127.0.0.1:8081", CLIPath: "/usr/local/bin/dagger"}},
			want: map[string]string{
				"_EXPERIMENTAL_DAGGER_CLI_BIN":     "/usr/local/bin/dagger",
				"_EXPERIMENTAL_DAGGER_RUNNER_HOST": "tcp://127.0.0.1:8081",
			},
		},
		{
			name: "cloud",
			cfg:  &Config{Engine: EngineConfig{Mode: "cloud", CloudURL: "https://cloud.example.com", CloudToken: "tok", EngineVersion: "v0.21.7"}},
			want: map[string]string{
				"DAGGER_CLOUD_URL":         "https://cloud.example.com",
				"DAGGER_CLOUD_TOKEN":       "tok",
				"_EXPERIMENTAL_DAGGER_TAG": "v0.21.7",
			},
		},
		{
			name: "dagger-kubernetes",
			cfg:  &Config{Engine: EngineConfig{Mode: "dagger-kubernetes", RunnerHost: "dagger-cloud://self", CloudURL: "https://supervisor.example.com", CloudToken: "tok"}},
			want: map[string]string{
				"_EXPERIMENTAL_DAGGER_RUNNER_HOST": "dagger-cloud://self",
				"DAGGER_CLOUD_URL":                 "https://supervisor.example.com",
				"DAGGER_CLOUD_TOKEN":               "tok",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engineEnv(tt.cfg)
			for key, value := range tt.want {
				assert.Equal(t, value, got[key], key)
			}
		})
	}
}

func TestBuildExecEnvDropsDeniedKeys(t *testing.T) {
	env := buildExecEnv(ExecRequest{Env: map[string]string{
		"OK":           "value",
		"GITHUB_TOKEN": "secret",
		"MY_SECRET":    "secret",
		"HTTP_PROXY":   "http://attacker:3128",
	}})
	assert.Equal(t, map[string]string{"OK": "value"}, env)
}

func TestScrubEnvNames(t *testing.T) {
	names := scrubEnvNames()
	assert.Contains(t, names, "GITHUB_TOKEN")
	assert.Contains(t, names, "KUBECONFIG")
	assert.Contains(t, names, "LD_PRELOAD")
	for _, proxyVar := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
		assert.NotContains(t, names, proxyVar, "proxy variables are set by WithEgressPolicy and must not be scrubbed")
	}
}

func TestDaggerRunnerCloseWithoutConnect(t *testing.T) {
	cfg := testConfig(t, nil)
	applyDefaults(cfg)
	runner, err := NewDaggerRunner(t.Context(), cfg)
	require.NoError(t, err)
	require.NoError(t, runner.Close())
	require.NoError(t, runner.Close())
}

func TestDaggerRunnerE2E(t *testing.T) {
	if os.Getenv("DEVELOPER_DAGGER_E2E") == "" {
		t.Skip("set DEVELOPER_DAGGER_E2E=1 to run against a real Dagger engine")
	}
	cfg := testConfig(t, nil)
	applyDefaults(cfg)
	cfg.Engine.Mode = "sidecar"
	cfg.Engine.RunnerHost = os.Getenv("_EXPERIMENTAL_DAGGER_RUNNER_HOST")
	if cli := os.Getenv("_EXPERIMENTAL_DAGGER_CLI_BIN"); cli != "" {
		cfg.Engine.CLIPath = cli
	}
	runner, err := NewDaggerRunner(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = runner.Close() })

	policy := &egress.Policy{
		AllowHosts:  []string{"github.com"},
		DefaultDeny: true,
	}
	exec := func(command ...string) ExecResult {
		t.Helper()
		result, err := runner.Exec(t.Context(), ExecRequest{
			BaseImage:      "alpine:3.20",
			WorkspacePath:  t.TempDir(),
			Command:        command,
			User:           "1000:1000",
			Egress:         policy,
			Timeout:        2 * time.Minute,
			MaxOutputBytes: 4096,
		})
		require.NoError(t, err)
		return result
	}

	result := exec("echo", "ok")
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "ok")

	result = exec("printenv", "HTTP_PROXY")
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "http://eino-egress:3128", strings.TrimSpace(result.Stdout))
}
