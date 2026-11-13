package developer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// testConfig returns a valid config backed by runner and a temporary workspace.
func testConfig(t *testing.T, runner Runner) *Config {
	t.Helper()
	return &Config{
		WorkspaceRoot: t.TempDir(),
		Engine:        EngineConfig{Mode: "sidecar", RunnerHost: "tcp://127.0.0.1:8081"},
		Runner:        runner,
	}
}

// testGitHubConfig returns a GitHub config with one dummy instance.
func testGitHubConfig() *GitHubConfig {
	return &GitHubConfig{Instances: map[string]GitHubInstance{
		"corp": {Token: "ghp_012345678901234567890123456789012345"},
	}}
}

// connectInMemory connects the per-identity server to an in-memory client.
func connectInMemory(t *testing.T, s *mcpserver.Server, id *mcpserver.Identity) *mcpsdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv, err := s.ServerFor(ctx, id)
	require.NoError(t, err)
	ct, st := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func listTools(t *testing.T, cs *mcpsdk.ClientSession) map[string]*mcpsdk.Tool {
	t.Helper()
	res, err := cs.ListTools(context.Background(), &mcpsdk.ListToolsParams{})
	require.NoError(t, err)
	tools := make(map[string]*mcpsdk.Tool, len(res.Tools))
	for _, tl := range res.Tools {
		tools[tl.Name] = tl
	}
	return tools
}

func callTool(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func resultText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])
	return tc.Text
}

// writeFile writes a file under dir, creating parents.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
