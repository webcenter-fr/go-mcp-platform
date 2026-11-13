package serverconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webcenter-fr/go-mcp-platform/internal/secretref"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	return writeFile(t, t.TempDir(), "config.yaml", content)
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("", map[string]any{
		"server": "kubernetes",
		"kubernetes.clusters.in-cluster.inCluster": true,
	})
	require.NoError(t, err)
	assert.Equal(t, DefaultTransport, cfg.Transport)
	assert.Equal(t, DefaultAddr, cfg.Addr)
	assert.Equal(t, DefaultOpsAddr, cfg.OpsAddr)
	assert.Equal(t, DefaultLogLevel, cfg.Log.Level)
	assert.Equal(t, DefaultLogFormat, cfg.Log.Format)
	require.Contains(t, cfg.Kubernetes.Clusters, "in-cluster")
	assert.Equal(t, DefaultDisallowedNamespaces, cfg.Kubernetes.Clusters["in-cluster"].DisallowedNamespaces)
	assert.Equal(t, DefaultTimeout, cfg.Kubernetes.Clusters["in-cluster"].DefaultTimeout)
}

func TestLoadPrecedence(t *testing.T) {
	path := writeConfig(t, "server: kubernetes\naddr: \":1111\"\nkubernetes:\n  clusters:\n    in-cluster:\n      inCluster: true\n")

	t.Setenv("MCP_PLATFORM_ADDR", ":2222")
	cfg, err := Load(path, nil)
	require.NoError(t, err)
	assert.Equal(t, ":2222", cfg.Addr, "environment must override the file")

	cfg, err = Load(path, map[string]any{"addr": ":3333"})
	require.NoError(t, err)
	assert.Equal(t, ":3333", cfg.Addr, "flags must override the environment")
}

func TestLoadSecretFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := writeFile(t, dir, "token", "s3cr3t\r\n")
	path := writeConfig(t, "server: argocd\nargocd:\n  instances:\n    prod:\n      url: https://argocd.example.com\n      tokenFile: "+tokenFile+"\n")

	cfg, err := Load(path, nil)
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", cfg.ArgoCD.Instances["prod"].Token)
}

func TestLoadSecretFileWinsOverEnv(t *testing.T) {
	dir := t.TempDir()
	tokenFile := writeFile(t, dir, "token", "from-file\n")
	path := writeConfig(t, "server: grafana\ngrafana:\n  instances:\n    prod:\n      url: https://grafana.example.com\n      tokenFile: "+tokenFile+"\n      tokenEnv: TEST_GRAFANA_TOKEN\n")
	t.Setenv("TEST_GRAFANA_TOKEN", "from-env")

	cfg, err := Load(path, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-file", cfg.Grafana.Instances["prod"].Token)
}

func TestLoadSecretErrors(t *testing.T) {
	dir := t.TempDir()
	empty := writeFile(t, dir, "empty", "\n")
	oversize := writeFile(t, dir, "oversize", strings.Repeat("x", secretref.MaxBytes+1))

	tests := []struct {
		name    string
		token   string
		wantErr string
	}{
		{name: "missing file", token: filepath.Join(dir, "missing"), wantErr: "failed to read"},
		{name: "empty file", token: empty, wantErr: "is empty"},
		{name: "oversize file", token: oversize, wantErr: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, "server: argocd\nargocd:\n  instances:\n    prod:\n      url: https://argocd.example.com\n      tokenFile: "+tt.token+"\n")
			_, err := Load(path, nil)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
		})
	}
}

func TestLoadUnknownKey(t *testing.T) {
	path := writeConfig(t, "server: kubernetes\nunknownKey: true\nkubernetes:\n  clusters:\n    in-cluster:\n      inCluster: true\n")
	_, err := Load(path, nil)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknownkey")
}

func TestLoadValidation(t *testing.T) {
	t.Setenv("TOKEN", "x")
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "missing server", content: "transport: http\n", wantErr: "server"},
		{name: "unknown server", content: "server: nomad\n", wantErr: "server"},
		{name: "invalid transport", content: "server: kubernetes\ntransport: grpc\n", wantErr: "transport"},
		{name: "invalid url", content: "server: argocd\nargocd:\n  instances:\n    prod:\n      url: not-a-url\n      tokenEnv: TOKEN\n", wantErr: "url"},
		{name: "zero instances", content: "server: argocd\nargocd:\n  instances: {}\n", wantErr: "instance"},
		{name: "missing token", content: "server: argocd\nargocd:\n  instances:\n    prod:\n      url: https://argocd.example.com\n", wantErr: "token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content), nil)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
		})
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.yaml")
}

func TestBuildFamilies(t *testing.T) {
	dir := t.TempDir()
	tokenFile := writeFile(t, dir, "token", "token\n")
	kubeconfig := writeFile(t, dir, "kubeconfig", `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: dummy
`)

	tests := []struct {
		name    string
		content string
	}{
		{name: "kubernetes", content: "server: kubernetes\nkubernetes:\n  clusters:\n    prod:\n      kubeconfigFile: " + kubeconfig + "\n      context: test\n"},
		{name: "argocd", content: "server: argocd\nargocd:\n  instances:\n    prod:\n      url: https://argocd.example.com\n      tokenFile: " + tokenFile + "\n"},
		{name: "prometheus", content: "server: prometheus\nprometheus:\n  instances:\n    prod:\n      url: https://prometheus.example.com\n      tokenFile: " + tokenFile + "\n"},
		{name: "grafana", content: "server: grafana\ngrafana:\n  instances:\n    prod:\n      url: https://grafana.example.com\n      tokenFile: " + tokenFile + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(writeConfig(t, tt.content), nil)
			require.NoError(t, err)
			srv, err := Build(context.Background(), cfg)
			require.NoError(t, err)
			assert.NotNil(t, srv)
		})
	}
}

func TestBuildUnknownFamily(t *testing.T) {
	_, err := Build(context.Background(), &Config{Server: "nomad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MCP server family")
}

func TestBuildMissingFamily(t *testing.T) {
	_, err := Build(context.Background(), &Config{Server: "argocd"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestOpsHandler(t *testing.T) {
	readyErr := assert.AnError
	handler := OpsHandler(func() error { return readyErr })

	check := func(path string, want int) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, want, rec.Code, path)
	}

	check("/healthz", http.StatusOK)
	check("/readyz", http.StatusServiceUnavailable)
	check("/metrics", http.StatusOK)

	readyErr = nil
	check("/readyz", http.StatusOK)
}

func TestLoadEnvOnlyServer(t *testing.T) {
	t.Setenv("MCP_PLATFORM_SERVER", "argocd")
	t.Setenv("TEST_ARGOCD_TOKEN", "token")
	path := writeConfig(t, "argocd:\n  instances:\n    prod:\n      url: https://argocd.example.com\n      tokenEnv: TEST_ARGOCD_TOKEN\n")

	cfg, err := Load(path, nil)
	require.NoError(t, err)
	assert.Equal(t, "argocd", cfg.Server)
}

func TestLoadDeveloperConfig(t *testing.T) {
	cfg, err := Load("testdata/developer.yaml", nil)
	require.NoError(t, err)
	require.NotNil(t, cfg.Developer)
	assert.Equal(t, "/var/lib/go-mcp-platform/workspaces", cfg.Developer.WorkspaceRoot)
	assert.Equal(t, "alpine:3.20", cfg.Developer.BaseImage)
	assert.Equal(t, "5m", cfg.Developer.DefaultTimeout)
	assert.Equal(t, "15m", cfg.Developer.MaxTimeout)
	assert.Equal(t, 256000, cfg.Developer.MaxOutputBytes)
	assert.Equal(t, 4, cfg.Developer.MaxConcurrentExecs)
	assert.Equal(t, "1000:1000", cfg.Developer.ContainerUser)
	assert.Equal(t, "24h", cfg.Developer.WorkspaceTTL)
	assert.Equal(t, "1h", cfg.Developer.WorkspaceGCInterval)
	require.NotNil(t, cfg.Developer.Scaffolding)
	assert.True(t, *cfg.Developer.Scaffolding)
	require.NotNil(t, cfg.Developer.Egress)
	require.NotNil(t, cfg.Developer.Egress.DefaultDeny)
	assert.True(t, *cfg.Developer.Egress.DefaultDeny)
	assert.Equal(t, "/usr/local/bin/dagger", cfg.Developer.Engine.CLIPath)
	require.NotNil(t, cfg.Developer.GitHub)
	assert.Equal(t, "ghp_012345678901234567890123456789012345", cfg.Developer.GitHub.Instances["corp"].Token)
	assert.Equal(t, "30s", cfg.Developer.GitHub.Instances["corp"].Timeout)
}

func TestLoadDeveloperValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "missing developer block", content: "server: developer\n", wantErr: "developer"},
		{name: "empty workspace root", content: "server: developer\ndeveloper:\n  workspaceRoot: \"\"\n  engine:\n    mode: sidecar\n", wantErr: "workspaceRoot"},
		{name: "github without instances", content: "server: developer\ndeveloper:\n  workspaceRoot: /var/lib/workspaces\n  engine:\n    mode: sidecar\n  github:\n    readOnly: true\n", wantErr: "instance"},
		{name: "unknown developer key", content: "server: developer\ndeveloper:\n  workspaceRoot: /var/lib/workspaces\n  engine:\n    mode: sidecar\n  unknownKey: true\n", wantErr: "unknownkey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content), nil)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
		})
	}
}

func TestLoadDeveloperSecrets(t *testing.T) {
	dir := t.TempDir()
	githubToken := writeFile(t, dir, "github-token", "gh-token\n")
	cloudToken := writeFile(t, dir, "cloud-token", "cloud-token\n")
	registryPassword := writeFile(t, dir, "registry-password", "registry-password\n")
	content := "server: developer\n" +
		"developer:\n" +
		"  workspaceRoot: /var/lib/workspaces\n" +
		"  engine:\n" +
		"    mode: cloud\n" +
		"    cloudTokenFile: " + cloudToken + "\n" +
		"    registryAuth:\n" +
		"      registry:\n" +
		"        username: user\n" +
		"        passwordFile: " + registryPassword + "\n" +
		"  github:\n" +
		"    instances:\n" +
		"      corp:\n" +
		"        tokenFile: " + githubToken + "\n"
	cfg, err := Load(writeConfig(t, content), nil)
	require.NoError(t, err)
	assert.Equal(t, "cloud-token", cfg.Developer.Engine.CloudToken)
	assert.Equal(t, "registry-password", cfg.Developer.Engine.RegistryAuth["registry"].Password)
	assert.Equal(t, "gh-token", cfg.Developer.GitHub.Instances["corp"].Token)
}

func TestBuildDeveloper(t *testing.T) {
	cfg, err := Load("testdata/developer.yaml", nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := Build(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestLoadMemoryConfig(t *testing.T) {
	cfg, err := Load("testdata/memory.yaml", nil)
	require.NoError(t, err)
	require.NotNil(t, cfg.Memory)
	assert.Equal(t, "file", cfg.Memory.Backend)
	assert.Equal(t, "per_identity", cfg.Memory.Isolation)
	assert.Equal(t, "shared", cfg.Memory.SharedNamespace)
	assert.Equal(t, "/var/lib/go-mcp-platform/memory", cfg.Memory.File.Dir)
	assert.True(t, cfg.Memory.RequireDeleteApproval)
	require.NotNil(t, cfg.Memory.Prompts)
	assert.True(t, *cfg.Memory.Prompts)
	assert.Equal(t, 5, cfg.Memory.DefaultTopK)
	assert.Equal(t, 50, cfg.Memory.MaxTopK)
	assert.Equal(t, 16384, cfg.Memory.MaxContentChars)
	assert.Equal(t, 4096, cfg.Memory.MaxQueryChars)
	assert.Equal(t, "30s", cfg.Memory.RequestTimeout)
}

func TestLoadMemoryEnvOverrides(t *testing.T) {
	t.Setenv("MCP_PLATFORM_MEMORY_BACKEND", "memory")
	t.Setenv("MCP_PLATFORM_MEMORY_ISOLATION", "shared")
	cfg, err := Load("testdata/memory.yaml", nil)
	require.NoError(t, err)
	assert.Equal(t, "memory", cfg.Memory.Backend)
	assert.Equal(t, "shared", cfg.Memory.Isolation)
}

func TestLoadMemorySecrets(t *testing.T) {
	dir := t.TempDir()
	password := writeFile(t, dir, "password", "opensearch-password\n")
	embeddingKey := writeFile(t, dir, "embedding-key", "embedding-key\n")
	llmKey := writeFile(t, dir, "llm-key", "llm-key\n")
	content := "server: memory\n" +
		"memory:\n" +
		"  backend: opensearch\n" +
		"  opensearch:\n" +
		"    urls: [https://opensearch.example.com]\n" +
		"    passwordFile: " + password + "\n" +
		"  embedding:\n" +
		"    provider: openai\n" +
		"    model: text-embedding-3-small\n" +
		"    apiKeyFile: " + embeddingKey + "\n" +
		"  llm:\n" +
		"    provider: openai\n" +
		"    model: gpt-4o-mini\n" +
		"    apiKeyFile: " + llmKey + "\n"
	cfg, err := Load(writeConfig(t, content), nil)
	require.NoError(t, err)
	assert.Equal(t, "opensearch-password", cfg.Memory.OpenSearch.Password)
	assert.Equal(t, "embedding-key", cfg.Memory.Embedding.APIKey)
	assert.Equal(t, "llm-key", cfg.Memory.LLM.APIKey)
}

func TestLoadMemoryValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "missing memory block", content: "server: memory\n", wantErr: "memory"},
		{name: "file without dir", content: "server: memory\nmemory:\n  backend: file\n", wantErr: "file.dir"},
		{name: "opensearch without urls", content: "server: memory\nmemory:\n  backend: opensearch\n", wantErr: "opensearch.urls"},
		{name: "invalid request timeout", content: "server: memory\nmemory:\n  requestTimeout: not-a-duration\n", wantErr: "requestTimeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content), nil)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
		})
	}
}

func TestBuildMemory(t *testing.T) {
	cfg, err := Load("testdata/memory.yaml", nil)
	require.NoError(t, err)
	srv, err := Build(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, srv)
}
