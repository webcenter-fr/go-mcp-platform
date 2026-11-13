package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

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

// fakeEmbedder returns a deterministic vector for every input.
type fakeEmbedder struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	vectors := make([][]float64, len(texts))
	for i := range texts {
		vectors[i] = []float64{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

// fakeChatModel returns queued responses and records the prompts it receives.
type fakeChatModel struct {
	mu        sync.Mutex
	responses []string
	err       error
	prompts   []string
}

func (f *fakeChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var prompt strings.Builder
	for _, message := range input {
		prompt.WriteString(message.Content)
		prompt.WriteString("\n")
	}
	f.prompts = append(f.prompts, prompt.String())
	if f.err != nil {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return schema.AssistantMessage("", nil), nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return schema.AssistantMessage(response, nil), nil
}

func (f *fakeChatModel) Stream(_ context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := f.Generate(context.Background(), input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (f *fakeChatModel) lastPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.prompts) == 0 {
		return ""
	}
	return f.prompts[len(f.prompts)-1]
}

// mockOpenSearch is a minimal in-memory OpenSearch REST server.
type mockOpenSearch struct {
	mu       sync.Mutex
	exists   bool
	docs     map[string]map[string]any
	order    []string
	requests int
	failNext bool
}

func newMockOpenSearch(t *testing.T) (*mockOpenSearch, *httptest.Server) {
	t.Helper()
	mock := &mockOpenSearch{docs: make(map[string]map[string]any)}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	return mock, server
}

func (m *mockOpenSearch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++

	if m.failNext {
		m.failNext = false
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case r.Method == http.MethodHead && len(parts) == 1:
		if m.exists {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case r.Method == http.MethodPut && len(parts) == 1:
		m.exists = true
		writeJSON(w, map[string]any{"acknowledged": true})
	case r.Method == http.MethodGet && len(parts) == 2 && parts[1] == "_mapping":
		writeJSON(w, map[string]any{
			parts[0]: map[string]any{
				"mappings": map[string]any{
					"properties": map[string]any{
						"vector": map[string]any{"type": "knn_vector", "dimension": 3},
					},
				},
			},
		})
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "_bulk":
		m.handleBulk(w, r)
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "_search":
		m.handleSearch(w)
	case (r.Method == http.MethodGet || r.Method == http.MethodPost) && len(parts) == 2 && parts[1] == "_count":
		writeJSON(w, map[string]any{"count": len(m.docs)})
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "_doc":
		m.handleGet(w, parts[2])
	case r.Method == http.MethodDelete && len(parts) == 3 && parts[1] == "_doc":
		delete(m.docs, parts[2])
		m.order = removeFromOrder(m.order, parts[2])
		writeJSON(w, map[string]any{"result": "deleted"})
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "_delete_by_query":
		m.handleDeleteByQuery(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *mockOpenSearch) handleBulk(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	items := make([]map[string]any, 0, len(lines)/2)
	for i := 0; i+1 < len(lines); i += 2 {
		var action map[string]map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &action); err != nil {
			continue
		}
		var source map[string]any
		if err := json.Unmarshal([]byte(lines[i+1]), &source); err != nil {
			continue
		}
		id, _ := action["index"]["_id"].(string)
		if id == "" {
			id = fmt.Sprintf("auto-%d", len(m.docs))
		}
		if _, ok := m.docs[id]; !ok {
			m.order = append(m.order, id)
		}
		m.docs[id] = source
		items = append(items, map[string]any{"index": map[string]any{"_id": id, "status": 201}})
	}
	writeJSON(w, map[string]any{"errors": false, "items": items})
}

func (m *mockOpenSearch) handleSearch(w http.ResponseWriter) {
	hits := make([]map[string]any, 0, len(m.order))
	for _, id := range m.order {
		source, ok := m.docs[id]
		if !ok {
			continue
		}
		hits = append(hits, map[string]any{
			"_index":  "eino_agent_memory",
			"_id":     id,
			"_score":  1.0,
			"_source": source,
		})
	}
	writeJSON(w, map[string]any{
		"hits": map[string]any{
			"total": map[string]any{"value": len(hits)},
			"hits":  hits,
		},
	})
}

func (m *mockOpenSearch) handleGet(w http.ResponseWriter, id string) {
	source, ok := m.docs[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"found": false})
		return
	}
	writeJSON(w, map[string]any{
		"_index":  "eino_agent_memory",
		"_id":     id,
		"found":   true,
		"_source": source,
	})
}

func (m *mockOpenSearch) handleDeleteByQuery(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	deleted := 0
	for id, source := range m.docs {
		if matchesMockQuery(source, body) {
			delete(m.docs, id)
			m.order = removeFromOrder(m.order, id)
			deleted++
		}
	}
	writeJSON(w, map[string]any{"deleted": deleted})
}

func matchesMockQuery(source map[string]any, body map[string]any) bool {
	query, _ := body["query"].(map[string]any)
	boolQuery, _ := query["bool"].(map[string]any)
	must, _ := boolQuery["must"].([]any)
	if len(must) == 0 {
		return true
	}
	for _, clause := range must {
		term, _ := clause.(map[string]any)["term"].(map[string]any)
		for key, want := range term {
			if source[key] != want {
				return false
			}
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// allowAuthorizer approves every write-tool execution.
type allowAuthorizer struct{}

func (allowAuthorizer) AuthorizeExecute(context.Context, string, json.RawMessage) error {
	return nil
}
