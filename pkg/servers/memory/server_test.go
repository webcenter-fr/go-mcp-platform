package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

func TestNewServerToolSurface(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	tools := listTools(t, connectInMemory(t, srv, nil))

	for _, name := range []string{
		"memory_store",
		"memory_capture_discussion",
		"memory_search",
		"memory_get",
		"memory_list",
		"memory_delete",
		"memory_stats",
	} {
		assert.Contains(t, tools, name)
	}
	for name, tl := range tools {
		require.NotNil(t, tl.Annotations, name)
		assert.True(t, tl.Annotations.ReadOnlyHint, name)
	}
}

func TestReadOnlySurface(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory, ReadOnly: true}, nil)
	tools := listTools(t, connectInMemory(t, srv, nil))

	assert.ElementsMatch(t, []string{
		"memory_search",
		"memory_get",
		"memory_list",
		"memory_stats",
	}, mapKeys(tools))
}

func TestRBACHidesWriteTools(t *testing.T) {
	id := &mcpserver.Identity{User: "alice"}
	srv := newTestServer(t, &Config{Backend: BackendMemory}, &mcpserver.Config{
		LocalIdentity: id,
		Authorization: &mcpserver.AuthzConfig{Rules: []mcpserver.AccessRule{
			{Users: []string{"alice"}, Tools: []string{"memory_search"}},
		}},
	})
	tools := listTools(t, connectInMemory(t, srv, id))
	assert.ElementsMatch(t, []string{"memory_search"}, mapKeys(tools))
}

func TestLazyOpenSearchStartup(t *testing.T) {
	cfg := &Config{
		Backend: BackendOpenSearch,
		OpenSearch: &OpenSearchBackendConfig{
			URLs:            []string{"http://127.0.0.1:1"},
			IndexName:       DefaultOpenSearchIdx,
			VectorField:     "vector",
			ContentField:    "content",
			VectorDimension: 3,
			BatchSize:       DefaultBatchSize,
		},
	}
	srv := newTestServer(t, cfg, nil)
	tools := listTools(t, connectInMemory(t, srv, nil))
	assert.Contains(t, tools, "memory_search")
}

func TestPerIdentityIsolation(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	alice := connectInMemory(t, srv, &mcpserver.Identity{User: "alice", Sub: "sub-alice"})
	bob := connectInMemory(t, srv, &mcpserver.Identity{User: "bob", Sub: "sub-bob"})

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, alice, "memory_store", map[string]any{
		"kind": "fact", "content": "alice only",
	}))
	require.NotEmpty(t, stored.Record.ID)

	bobList := decodeOutput[MemoryListOutput](t, callTool(t, bob, "memory_list", map[string]any{}))
	assert.Zero(t, bobList.Total)

	aliceList := decodeOutput[MemoryListOutput](t, callTool(t, alice, "memory_list", map[string]any{}))
	assert.Equal(t, 1, aliceList.Total)
}

func TestWriteToolNames(t *testing.T) {
	assert.Nil(t, WriteToolNames(nil))
	assert.Nil(t, WriteToolNames(&Config{}))
	assert.Nil(t, WriteToolNames(&Config{ReadOnly: true, RequireDeleteApproval: true}))
	assert.Nil(t, WriteToolNames(&Config{RequireDeleteApproval: false}))
	assert.Equal(t, []string{"memory_delete"}, WriteToolNames(&Config{RequireDeleteApproval: true}))
}

func TestResolveTenant(t *testing.T) {
	assert.Equal(t, DefaultLocalTenant, resolveTenant(&Config{}, nil))
	assert.Equal(t, "sub-alice", resolveTenant(&Config{}, &mcpserver.Identity{User: "alice", Sub: "sub-alice"}))
	assert.Equal(t, "alice", resolveTenant(&Config{}, &mcpserver.Identity{User: "alice"}))
	assert.Equal(t, "shared", resolveTenant(&Config{Isolation: IsolationShared, SharedNamespace: "shared"}, &mcpserver.Identity{User: "alice"}))
}

func TestNewServerWithCloser(t *testing.T) {
	srv, closer, err := NewServerWithCloser(context.Background(), &Config{Backend: BackendMemory}, nil)
	require.NoError(t, err)
	assert.NotNil(t, srv)
	require.NotNil(t, closer)
	assert.NoError(t, closer.Close())
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
