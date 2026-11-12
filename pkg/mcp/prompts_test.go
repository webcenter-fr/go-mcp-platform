package mcp

import (
	"context"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptsAndResourcesRoundTrip(t *testing.T) {
	s := newTestServer(t, &Config{
		Prompts: []PromptSpec{{
			Name:        "greet",
			Description: "Greets the caller",
			Arguments:   []PromptArgumentSpec{{Name: "name", Required: true}},
			Handler: func(_ context.Context, _ *Identity, args map[string]string) (string, error) {
				return "hello " + args["name"], nil
			},
		}},
		Resources: []ResourceSpec{{
			URI:  "test://greeting",
			Name: "greeting",
			Handler: func(_ context.Context, _ *Identity, uri string) (string, error) {
				return "resource body for " + uri, nil
			},
		}},
	})
	cs := connectInMemory(t, s, nil, nil)
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	require.Len(t, prompts.Prompts, 1)
	assert.Equal(t, "greet", prompts.Prompts[0].Name)

	got, err := cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{Name: "greet", Arguments: map[string]string{"name": "alice"}})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	tc, ok := got.Messages[0].Content.(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, "hello alice", tc.Text)

	resources, err := cs.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	require.NoError(t, err)
	require.Len(t, resources.Resources, 1)
	assert.Equal(t, "text/markdown", resources.Resources[0].MIMEType, "MIME type must default to text/markdown")

	read, err := cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "test://greeting"})
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	assert.Equal(t, "resource body for test://greeting", read.Contents[0].Text)
	assert.Equal(t, "text/markdown", read.Contents[0].MIMEType)
}

func TestPromptsAndResourcesRBAC(t *testing.T) {
	s := newTestServer(t, &Config{
		Prompts: []PromptSpec{{
			Name:    "secret_prompt",
			Handler: func(context.Context, *Identity, map[string]string) (string, error) { return "secret", nil },
		}},
		Resources: []ResourceSpec{{
			URI:     "test://secret",
			Name:    "secret_resource",
			Handler: func(context.Context, *Identity, string) (string, error) { return "secret", nil },
		}},
		Authorization: &AuthzConfig{Rules: []AccessRule{
			{Users: []string{"alice"}, Tools: []string{"allowed_*"}},
		}},
	})
	cs := connectInMemory(t, s, &Identity{User: "alice"}, nil)
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	require.NoError(t, err)
	assert.Empty(t, prompts.Prompts, "prompt must be hidden from tools/list")

	resources, err := cs.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	require.NoError(t, err)
	assert.Empty(t, resources.Resources, "resource must be hidden from resources/list")

	// Not registered for this identity: the SDK fails closed (unknown prompt /
	// resource not found) before any handler runs.
	_, err = cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{Name: "secret_prompt"})
	require.Error(t, err)

	_, err = cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "test://secret"})
	require.Error(t, err)
}

func TestPromptHandlerErrorIsProtocolError(t *testing.T) {
	s := newTestServer(t, &Config{
		Prompts: []PromptSpec{{
			Name: "broken",
			Handler: func(context.Context, *Identity, map[string]string) (string, error) {
				return "", errors.New("boom")
			},
		}},
	})
	cs := connectInMemory(t, s, nil, nil)
	_, err := cs.GetPrompt(context.Background(), &mcpsdk.GetPromptParams{Name: "broken"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestInvalidResourceURIReturnsError(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "empty", uri: ""},
		{name: "scheme-less", uri: "not-a-uri"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(context.Background(), &Config{
				InstanceParam: "cluster",
				Resources: []ResourceSpec{{
					URI:     tt.uri,
					Name:    "bad",
					Handler: func(context.Context, *Identity, string) (string, error) { return "", nil },
				}},
			})
			require.Error(t, err)
		})
	}
}

func TestCapabilitiesAdvertisePromptsAndResources(t *testing.T) {
	s := newTestServer(t, &Config{
		Prompts: []PromptSpec{{
			Name:    "p",
			Handler: func(context.Context, *Identity, map[string]string) (string, error) { return "p", nil },
		}},
		Resources: []ResourceSpec{{
			URI:     "test://r",
			Name:    "r",
			Handler: func(context.Context, *Identity, string) (string, error) { return "r", nil },
		}},
	})
	cs := connectInMemory(t, s, nil, nil)
	caps := cs.InitializeResult().Capabilities
	require.NotNil(t, caps)
	assert.NotNil(t, caps.Prompts)
	assert.NotNil(t, caps.Resources)
}
