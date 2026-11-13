package memory

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptsAndResources(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	names := make([]string, 0, len(prompts.Prompts))
	for _, prompt := range prompts.Prompts {
		names = append(names, prompt.Name)
	}
	assert.ElementsMatch(t, []string{
		"memory_distill_session",
		"memory_craft_best_prompt",
		"memory_craft_agent_process",
		"memory_record_best_practice",
	}, names)

	got, err := cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{
		Name:      "memory_craft_best_prompt",
		Arguments: map[string]string{"goal": "review a Go diff", "scope": "lang:golang"},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	text, ok := got.Messages[0].Content.(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "review a Go diff")
	assert.Contains(t, text.Text, "lang:golang")

	_, err = cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{Name: "memory_craft_best_prompt"})
	require.Error(t, err)

	resources, err := cs.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	require.NoError(t, err)
	require.Len(t, resources.Resources, 1)
	assert.Equal(t, "memory://kinds", resources.Resources[0].URI)

	read, err := cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "memory://kinds"})
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	var catalog []KindInfo
	require.NoError(t, json.Unmarshal([]byte(read.Contents[0].Text), &catalog))
	assert.Len(t, catalog, len(SupportedKinds()))
}

func TestPromptsDisabled(t *testing.T) {
	disabled := false
	srv := newTestServer(t, &Config{Backend: BackendMemory, Prompts: &disabled}, nil)
	cs := connectInMemory(t, srv, nil)
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	assert.Empty(t, prompts.Prompts)

	resources, err := cs.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	require.NoError(t, err)
	assert.Empty(t, resources.Resources)
}
