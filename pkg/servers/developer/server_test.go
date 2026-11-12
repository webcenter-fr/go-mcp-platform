package developer

import (
	"context"
	"slices"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubtool "github.com/webcenter-fr/eino-ext/components/tool/github"
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

func TestNewServerToolSurface(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	cfg.GitHub = testGitHubConfig()
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)

	tools := listTools(t, connectInMemory(t, s, nil))
	for _, name := range []string{
		"developer_sandbox_exec",
		"developer_sandbox_info",
		"developer_scaffold_list",
		"developer_scaffold_get",
		"developer_scaffold_apply",
		"github_instance_list",
		"github_repo_clone",
		"github_pr_create",
	} {
		assert.Contains(t, tools, name)
	}

	writeTools := githubtool.WriteToolNames()
	for name, tl := range tools {
		require.NotNil(t, tl.Annotations, name)
		if slices.Contains(writeTools, name) {
			assert.False(t, tl.Annotations.ReadOnlyHint, name)
			require.NotNil(t, tl.Annotations.DestructiveHint, name)
			assert.True(t, *tl.Annotations.DestructiveHint, name)
		} else {
			assert.True(t, tl.Annotations.ReadOnlyHint, name)
		}
	}
}

func TestWriteToolNames(t *testing.T) {
	assert.Nil(t, WriteToolNames(nil))
	assert.Nil(t, WriteToolNames(&Config{}))
	assert.Nil(t, WriteToolNames(&Config{GitHub: &GitHubConfig{ReadOnly: true}}))
	assert.ElementsMatch(t, githubtool.WriteToolNames(), WriteToolNames(&Config{GitHub: testGitHubConfig()}))
}

func TestNewServerWithoutGitHub(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)
	tools := listTools(t, connectInMemory(t, s, nil))
	assert.Contains(t, tools, "developer_sandbox_exec")
	assert.NotContains(t, tools, "github_instance_list")
}

func TestRBACHidesSandboxExec(t *testing.T) {
	id := &mcpserver.Identity{User: "alice"}
	cfg := testConfig(t, &FakeRunner{})
	s, err := NewServer(context.Background(), cfg, &mcpserver.Config{
		LocalIdentity: id,
		Authorization: &mcpserver.AuthzConfig{Rules: []mcpserver.AccessRule{
			{Users: []string{"alice"}, Tools: []string{"developer_sandbox_info"}},
		}},
	})
	require.NoError(t, err)

	cs := connectInMemory(t, s, id)
	tools := listTools(t, cs)
	assert.Contains(t, tools, "developer_sandbox_info")
	assert.NotContains(t, tools, "developer_sandbox_exec")

	_, err = cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "developer_sandbox_exec",
		Arguments: map[string]any{"command": []string{"go", "version"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestGitHubWriteToolRequiresApproval(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	cfg.GitHub = testGitHubConfig()
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)

	cs := connectInMemory(t, s, nil)
	res := callTool(t, cs, "github_pr_create", map[string]any{
		"instance": "corp",
		"owner":    "owner",
		"repo":     "repo",
		"title":    "title",
		"head":     "feature",
		"base":     "main",
	})
	assert.True(t, res.IsError)
	assert.Contains(t, resultText(t, res), "SAFETY GATE")
}

func TestNewServerLazyEngineConnection(t *testing.T) {
	cfg := testConfig(t, nil)
	cfg.Engine.RunnerHost = "tcp://127.0.0.1:1"
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)
	tools := listTools(t, connectInMemory(t, s, nil))
	assert.Contains(t, tools, "developer_sandbox_exec")
}

func TestScaffoldingPromptsAndResources(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)
	cs := connectInMemory(t, s, nil)
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	assert.NotEmpty(t, prompts.Prompts)

	got, err := cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{
		Name:      "developer_bootstrap",
		Arguments: map[string]string{"language": "golang", "projectName": "demo"},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	tc, ok := got.Messages[0].Content.(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "golang")
	assert.Contains(t, tc.Text, "demo")

	resources, err := cs.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	require.NoError(t, err)
	assert.NotEmpty(t, resources.Resources)

	read, err := cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "developer://agents/golang"})
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	assert.Contains(t, read.Contents[0].Text, "AGENTS.md")
}

func TestScaffoldingDisabled(t *testing.T) {
	cfg := testConfig(t, &FakeRunner{})
	disabled := false
	cfg.Scaffolding = &disabled
	s, err := NewServer(context.Background(), cfg, nil)
	require.NoError(t, err)
	cs := connectInMemory(t, s, nil)
	tools := listTools(t, cs)
	assert.NotContains(t, tools, "developer_scaffold_list")

	prompts, err := cs.ListPrompts(context.Background(), &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	assert.Empty(t, prompts.Prompts)
}
