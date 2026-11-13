package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

func newTestServer(t *testing.T, cfg *Config, mcpCfg *mcpserver.Config) *mcpserver.Server {
	t.Helper()
	srv, err := NewServer(context.Background(), cfg, mcpCfg)
	require.NoError(t, err)
	return srv
}

func decodeOutput[T any](t *testing.T, res *mcpsdk.CallToolResult) T {
	t.Helper()
	require.False(t, res.IsError, resultText(t, res))
	var out T
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &out))
	return out
}

func TestMemoryStoreAllKinds(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	for _, kind := range SupportedKinds() {
		t.Run(string(kind), func(t *testing.T) {
			res := callTool(t, cs, "memory_store", map[string]any{
				"kind":    string(kind),
				"content": "content for " + string(kind),
			})
			out := decodeOutput[MemoryStoreOutput](t, res)
			assert.Equal(t, kind, out.Record.Kind)
			assert.False(t, out.Upserted)
		})
	}
}

func TestMemoryStoreDeterministicUpsert(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	first := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "the sky is blue",
	}))
	second := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "the  sky is blue",
	}))
	assert.Equal(t, first.Record.ID, second.Record.ID)
	assert.False(t, first.Upserted)
	assert.True(t, second.Upserted)
}

func TestMemoryStoreExplicitIDUpdate(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	first := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "first",
	}))
	assert.False(t, first.Upserted)
	second := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"id": first.Record.ID, "kind": "fact", "content": "second",
	}))
	assert.True(t, second.Upserted)
	assert.Equal(t, "second", second.Record.Content)
}

func TestMemoryStoreValidation(t *testing.T) {
	cfg := &Config{Backend: BackendMemory, MaxContentChars: 256}
	srv := newTestServer(t, cfg, nil)
	cs := connectInMemory(t, srv, nil)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "empty content", args: map[string]any{"kind": "fact", "content": "   "}, want: "content"},
		{name: "oversized content", args: map[string]any{"kind": "fact", "content": strings.Repeat("x", 300)}, want: "exceeding"},
		{name: "reserved metadata", args: map[string]any{"kind": "fact", "content": "ok", "metadata": map[string]any{"user_id": "spoof"}}, want: "reserved"},
		{name: "invalid kind", args: map[string]any{"kind": "unknown", "content": "ok"}, want: "kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, cs, "memory_store", tt.args)
			require.True(t, res.IsError)
			assert.Contains(t, strings.ToLower(resultText(t, res)), strings.ToLower(tt.want))
		})
	}
}

func TestMemoryStoreRedactsSecrets(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	out := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind":    "fact",
		"content": "token ghp_012345678901234567890123456789012345",
	}))
	assert.True(t, out.Redacted)
	assert.NotContains(t, out.Record.Content, "ghp_012345678901234567890123456789012345")
}

func TestMemoryCaptureDiscussionWithLLM(t *testing.T) {
	model := &fakeChatModel{responses: []string{
		"the discussion summary",
		`[{"kind":"preference","content":"prefer table tests","confidence":0.9}]`,
	}}
	srv := newTestServer(t, &Config{Backend: BackendMemory, ChatModel: model}, nil)
	cs := connectInMemory(t, srv, nil)

	out := decodeOutput[MemoryCaptureDiscussionOutput](t, callTool(t, cs, "memory_capture_discussion", map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "how should we test?"},
			{"role": "assistant", "content": "with table-driven tests"},
		},
	}))
	assert.Equal(t, "llm_extracted", out.Mode)
	assert.Equal(t, "the discussion summary", out.Discussion.Content)
	require.Len(t, out.Extracted, 1)
	assert.Equal(t, KindPreference, out.Extracted[0].Kind)
}

func TestMemoryCaptureDiscussionWithoutLLM(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	out := decodeOutput[MemoryCaptureDiscussionOutput](t, callTool(t, cs, "memory_capture_discussion", map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "world"},
		},
	}))
	assert.Equal(t, "transcript", out.Mode)
	assert.Contains(t, out.Discussion.Content, "[user]: hello")
}

func TestMemoryCaptureDiscussionValidation(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	res := callTool(t, cs, "memory_capture_discussion", map[string]any{"messages": []map[string]any{}})
	require.True(t, res.IsError)

	res = callTool(t, cs, "memory_capture_discussion", map[string]any{
		"messages": []map[string]any{{"role": "user", "content": "   "}},
	})
	require.True(t, res.IsError)
	assert.Contains(t, strings.ToLower(resultText(t, res)), "content")
}

func TestMemoryCaptureDiscussionOversizedSummary(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory, MaxContentChars: 256}, nil)
	cs := connectInMemory(t, srv, nil)

	res := callTool(t, cs, "memory_capture_discussion", map[string]any{
		"messages": []map[string]any{{"role": "user", "content": "hello"}},
		"summary":  strings.Repeat("x", 300),
	})
	require.True(t, res.IsError)
	assert.Contains(t, strings.ToLower(resultText(t, res)), "exceeding")
}

func TestMemoryCaptureRedactsBeforeLLM(t *testing.T) {
	model := &fakeChatModel{responses: []string{"summary", "[]"}}
	srv := newTestServer(t, &Config{Backend: BackendMemory, ChatModel: model}, nil)
	cs := connectInMemory(t, srv, nil)

	out := decodeOutput[MemoryCaptureDiscussionOutput](t, callTool(t, cs, "memory_capture_discussion", map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "my token is ghp_012345678901234567890123456789012345"},
		},
	}))
	assert.True(t, out.Redacted)
	assert.NotContains(t, model.lastPrompt(), "ghp_012345678901234567890123456789012345")
}

func TestMemorySearchFilters(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	store := func(kind, content, scope, session string, tags []string) {
		args := map[string]any{"kind": kind, "content": content, "scope": scope, "sessionId": session}
		if len(tags) > 0 {
			args["tags"] = tags
		}
		res := callTool(t, cs, "memory_store", args)
		require.False(t, res.IsError, resultText(t, res))
	}
	store("fact", "alpha beta", "repo:a", "s1", []string{"go"})
	store("preference", "alpha gamma", "repo:b", "s2", []string{"python"})

	out := decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "kinds": []string{"fact"},
	}))
	require.Len(t, out.Hits, 1)
	assert.Equal(t, KindFact, out.Hits[0].Kind)
	assert.Equal(t, searchNotice, out.Notice)

	out = decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "scope": "repo:b",
	}))
	require.Len(t, out.Hits, 1)
	assert.Equal(t, "repo:b", out.Hits[0].Scope)

	out = decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "tags": []string{"go"},
	}))
	require.Len(t, out.Hits, 1)
	assert.Equal(t, "repo:a", out.Hits[0].Scope)

	out = decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "sessionId": "s2",
	}))
	require.Len(t, out.Hits, 1)
	assert.Equal(t, "s2", out.Hits[0].SessionID)
}

func TestMemorySearchTopKAndMinScore(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory, DefaultTopK: 1, MaxTopK: 1}, nil)
	cs := connectInMemory(t, srv, nil)

	for _, content := range []string{"alpha one", "alpha two"} {
		res := callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": content})
		require.False(t, res.IsError)
	}

	out := decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "topK": 10,
	}))
	assert.Len(t, out.Hits, 1, "topK must be capped by maxTopK")

	out = decodeOutput[MemorySearchOutput](t, callTool(t, cs, "memory_search", map[string]any{
		"query": "alpha", "minScore": 1000,
	}))
	assert.Empty(t, out.Hits)
}

func TestMemorySearchBlankQuery(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	res := callTool(t, cs, "memory_search", map[string]any{"query": "   "})
	require.True(t, res.IsError)
	assert.Contains(t, strings.ToLower(resultText(t, res)), "content")
}

func TestMemoryGet(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "alpha",
	}))
	out := decodeOutput[MemoryGetOutput](t, callTool(t, cs, "memory_get", map[string]any{"id": stored.Record.ID}))
	assert.Equal(t, stored.Record.ID, out.Record.ID)

	res := callTool(t, cs, "memory_get", map[string]any{"id": "missing"})
	require.True(t, res.IsError)
}

func TestMemoryGetCrossTenant(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	alice := connectInMemory(t, srv, &mcpserver.Identity{User: "alice", Sub: "sub-alice"})
	bob := connectInMemory(t, srv, &mcpserver.Identity{User: "bob", Sub: "sub-bob"})

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, alice, "memory_store", map[string]any{
		"kind": "fact", "content": "alice secret",
	}))
	res := callTool(t, bob, "memory_get", map[string]any{"id": stored.Record.ID})
	require.True(t, res.IsError)
}

func TestMemoryListPaginationAndOrdering(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	for _, content := range []string{"first", "second", "third"} {
		res := callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": content})
		require.False(t, res.IsError)
	}

	out := decodeOutput[MemoryListOutput](t, callTool(t, cs, "memory_list", map[string]any{
		"offset": 0, "limit": 2,
	}))
	assert.Equal(t, 3, out.Total)
	assert.Len(t, out.Records, 2)
	assert.Equal(t, 2, out.Count)

	out = decodeOutput[MemoryListOutput](t, callTool(t, cs, "memory_list", map[string]any{
		"offset": 2, "limit": 2,
	}))
	assert.Len(t, out.Records, 1)
}

func TestMemoryListFilters(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": "a", "tags": []string{"go"}})
	callTool(t, cs, "memory_store", map[string]any{"kind": "preference", "content": "b", "tags": []string{"python"}})

	out := decodeOutput[MemoryListOutput](t, callTool(t, cs, "memory_list", map[string]any{
		"kinds": []string{"preference"},
	}))
	require.Len(t, out.Records, 1)
	assert.Equal(t, KindPreference, out.Records[0].Kind)

	out = decodeOutput[MemoryListOutput](t, callTool(t, cs, "memory_list", map[string]any{
		"tags": []string{"go"},
	}))
	require.Len(t, out.Records, 1)
	assert.Equal(t, KindFact, out.Records[0].Kind)
}

func TestMemoryDelete(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "alpha", "tags": []string{"go"},
	}))
	out := decodeOutput[MemoryDeleteOutput](t, callTool(t, cs, "memory_delete", map[string]any{
		"id": stored.Record.ID,
	}))
	assert.Equal(t, 1, out.Deleted)

	res := callTool(t, cs, "memory_delete", map[string]any{})
	require.True(t, res.IsError)
	assert.Contains(t, strings.ToLower(resultText(t, res)), "filter")
}

func TestMemoryDeleteByFilterAndDryRun(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": "a", "tags": []string{"go"}})
	callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": "b", "tags": []string{"go"}})

	dry := decodeOutput[MemoryDeleteOutput](t, callTool(t, cs, "memory_delete", map[string]any{
		"tag": "go", "dryRun": true,
	}))
	assert.True(t, dry.DryRun)
	assert.Len(t, dry.IDs, 2)

	list := decodeOutput[MemoryListOutput](t, callTool(t, cs, "memory_list", map[string]any{}))
	assert.Equal(t, 2, list.Total, "dry run must not delete")

	deleted := decodeOutput[MemoryDeleteOutput](t, callTool(t, cs, "memory_delete", map[string]any{
		"tag": "go",
	}))
	assert.Equal(t, 2, deleted.Deleted)
}

func TestMemoryDeleteApprovalGate(t *testing.T) {
	cfg := &Config{Backend: BackendMemory, RequireDeleteApproval: true}
	srv := newTestServer(t, cfg, &mcpserver.Config{ExecutionAuthorizer: allowAuthorizer{}})
	cs := connectInMemory(t, srv, nil)

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, cs, "memory_store", map[string]any{
		"kind": "fact", "content": "alpha",
	}))

	res := callTool(t, cs, "memory_delete", map[string]any{"id": stored.Record.ID})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(t, res), "SAFETY GATE")

	out := decodeOutput[MemoryDeleteOutput](t, callTool(t, cs, "memory_delete", map[string]any{
		"id": stored.Record.ID, "confirmed": true,
	}))
	assert.Equal(t, 1, out.Deleted)
}

func TestMemoryStats(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": "a"})
	callTool(t, cs, "memory_store", map[string]any{"kind": "fact", "content": "b"})
	callTool(t, cs, "memory_store", map[string]any{"kind": "preference", "content": "c"})

	out := decodeOutput[MemoryStatsOutput](t, callTool(t, cs, "memory_stats", map[string]any{}))
	assert.Equal(t, string(BackendMemory), out.Backend)
	assert.Equal(t, "lexical", out.SearchMode)
	assert.Equal(t, 3, out.TotalMemories)
	assert.Equal(t, 2, out.ByKind[string(KindFact)])
	assert.Equal(t, 1, out.ByKind[string(KindPreference)])
	assert.Equal(t, SupportedKinds(), out.SupportedKinds)
}

func TestMemoryStatsEmptyArguments(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	cs := connectInMemory(t, srv, nil)

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "memory_stats"})
	require.NoError(t, err)
	out := decodeOutput[MemoryStatsOutput](t, res)
	assert.Equal(t, string(BackendMemory), out.Backend)
}

func TestMemoryStoreForeignIDRejected(t *testing.T) {
	srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
	alice := connectInMemory(t, srv, &mcpserver.Identity{User: "alice", Sub: "sub-alice"})
	bob := connectInMemory(t, srv, &mcpserver.Identity{User: "bob", Sub: "sub-bob"})

	stored := decodeOutput[MemoryStoreOutput](t, callTool(t, alice, "memory_store", map[string]any{
		"kind": "fact", "content": "alice only",
	}))
	res := callTool(t, bob, "memory_store", map[string]any{
		"id": stored.Record.ID, "kind": "fact", "content": "overwrite",
	})
	require.True(t, res.IsError)
}

func TestMemoryDeleteByEachFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter map[string]any
	}{
		{name: "kind", filter: map[string]any{"kind": "fact"}},
		{name: "sessionId", filter: map[string]any{"sessionId": "s1"}},
		{name: "scope", filter: map[string]any{"scope": "repo:a"}},
		{name: "tag", filter: map[string]any{"tag": "go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, &Config{Backend: BackendMemory}, nil)
			cs := connectInMemory(t, srv, nil)
			callTool(t, cs, "memory_store", map[string]any{
				"kind": "fact", "content": "alpha", "scope": "repo:a", "sessionId": "s1", "tags": []string{"go"},
			})
			out := decodeOutput[MemoryDeleteOutput](t, callTool(t, cs, "memory_delete", tt.filter))
			assert.Equal(t, 1, out.Deleted)
		})
	}
}
