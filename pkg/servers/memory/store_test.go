package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
)

type backendFixture struct {
	name     string
	newStore func(t *testing.T) memoryagent.MemoryStore
}

func backendFixtures(t *testing.T) []backendFixture {
	t.Helper()
	return []backendFixture{
		{
			name: "memory",
			newStore: func(*testing.T) memoryagent.MemoryStore {
				return newInMemoryStore()
			},
		},
		{
			name: "file",
			newStore: func(t *testing.T) memoryagent.MemoryStore {
				store, err := newFileStore(t.TempDir(), nil)
				require.NoError(t, err)
				return store
			},
		},
		{
			name: "opensearch",
			newStore: func(t *testing.T) memoryagent.MemoryStore {
				_, server := newMockOpenSearch(t)
				cfg := &Config{
					Backend: BackendOpenSearch,
					OpenSearch: &OpenSearchBackendConfig{
						URLs:            []string{server.URL},
						IndexName:       DefaultOpenSearchIdx,
						VectorField:     "vector",
						ContentField:    "content",
						VectorDimension: 3,
						BatchSize:       DefaultBatchSize,
					},
					Embedder: &fakeEmbedder{},
				}
				return newLazyOpenSearchStore(cfg)
			},
		},
	}
}

func testDocument(id, content, tenant string) *schema.Document {
	now := time.Now().UTC().Format(time.RFC3339)
	return &schema.Document{
		ID:      id,
		Content: content,
		MetaData: map[string]any{
			"category":    "fact",
			"source":      "assistant",
			"session_id":  "s1",
			"user_id":     tenant,
			"created_at":  now,
			"updated_at":  now,
			"raw_content": content,
		},
	}
}

func TestStoreConformance(t *testing.T) {
	for _, fixture := range backendFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			store := fixture.newStore(t)

			ids, err := store.Store(ctx, []*schema.Document{
				testDocument("a", "alpha beta", "tenant"),
				testDocument("b", "gamma delta", "tenant"),
			})
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"a", "b"}, ids)

			count, err := store.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 2, count)

			docs, err := store.List(ctx, 0, 10)
			require.NoError(t, err)
			assert.Len(t, docs, 2)

			hits, err := store.Retrieve(ctx, "alpha", retriever.WithTopK(10))
			require.NoError(t, err)
			require.NotEmpty(t, hits)
			hit, _ := RecordFromDocument(hits[0])
			assert.Equal(t, "a", hit.ID)

			require.NoError(t, store.Delete(ctx, "a"))
			count, err = store.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 1, count)

			deleted, err := store.DeleteByFilter(ctx, map[string]any{"category": "fact"})
			require.NoError(t, err)
			assert.Equal(t, 1, deleted)
		})
	}
}

func TestStoreUpsertDoesNotDuplicate(t *testing.T) {
	for _, fixture := range backendFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			store := fixture.newStore(t)

			first := testDocument("a", "alpha beta", "tenant")
			first.MetaData["created_at"] = "2026-01-01T00:00:00Z"
			_, err := store.Store(ctx, []*schema.Document{first})
			require.NoError(t, err)

			second := testDocument("a", "alpha beta updated", "tenant")
			second.MetaData["created_at"] = "2026-02-01T00:00:00Z"
			_, err = store.Store(ctx, []*schema.Document{second})
			require.NoError(t, err)

			docs, err := store.List(ctx, 0, 10)
			require.NoError(t, err)
			require.Len(t, docs, 1)
		})
	}
}

func TestTenantStoreUpsertPreservesCreatedAt(t *testing.T) {
	for _, fixture := range backendFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := &Config{Backend: BackendMemory}
			tenant := newTenantStore(fixture.newStore(t), "tenant", newRedactor(cfg))

			first, _, err := tenant.Store(ctx, &Record{Kind: KindFact, Content: "alpha"})
			require.NoError(t, err)
			second, upserted, err := tenant.Store(ctx, &Record{ID: first.ID, Kind: KindFact, Content: "alpha updated"})
			require.NoError(t, err)
			assert.True(t, upserted)
			assert.True(t, first.CreatedAt.Equal(second.CreatedAt))
		})
	}
}

func TestFileStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := newFileStore(dir, nil)
	require.NoError(t, err)
	_, err = store.Store(ctx, []*schema.Document{testDocument("a", "alpha beta", "tenant")})
	require.NoError(t, err)

	reopened, err := newFileStore(dir, nil)
	require.NoError(t, err)
	count, err := reopened.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOpenSearchLazyInitAndRetry(t *testing.T) {
	ctx := context.Background()
	mock, server := newMockOpenSearch(t)
	cfg := &Config{
		Backend: BackendOpenSearch,
		OpenSearch: &OpenSearchBackendConfig{
			URLs:            []string{server.URL},
			IndexName:       DefaultOpenSearchIdx,
			VectorField:     "vector",
			ContentField:    "content",
			VectorDimension: 3,
			BatchSize:       DefaultBatchSize,
		},
		Embedder: &fakeEmbedder{},
	}
	store := newLazyOpenSearchStore(cfg)
	assert.Zero(t, mock.requests, "construction must not contact OpenSearch")

	mock.failNext = true
	_, err := store.Count(ctx)
	require.Error(t, err, "the first call must surface the transient failure")

	count, err := store.Count(ctx)
	require.NoError(t, err, "the next call must retry initialization")
	assert.Equal(t, 0, count)
}

func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Backend: BackendMemory}
	router := newTenantBackendRouter(cfg)
	redactor := newRedactor(cfg)

	aliceStore, err := router.storeFor(ctx, "sub-alice")
	require.NoError(t, err)
	bobStore, err := router.storeFor(ctx, "sub-bob")
	require.NoError(t, err)
	alice := newTenantStore(aliceStore, "sub-alice", redactor)
	bob := newTenantStore(bobStore, "sub-bob", redactor)

	aliceRec, _, err := alice.Store(ctx, &Record{Kind: KindFact, Content: "shared secret fact"})
	require.NoError(t, err)
	bobRec, _, err := bob.Store(ctx, &Record{Kind: KindFact, Content: "shared secret fact"})
	require.NoError(t, err)
	assert.NotEqual(t, aliceRec.ID, bobRec.ID, "identical content must produce distinct tenant IDs")

	aliceHits, _, err := alice.Search(ctx, "shared secret fact", nil, "", nil, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, aliceHits, 1)
	assert.Equal(t, aliceRec.ID, aliceHits[0].ID)

	bobHits, _, err := bob.Search(ctx, "shared secret fact", nil, "", nil, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, bobHits, 1)
	assert.Equal(t, bobRec.ID, bobHits[0].ID)

	_, err = bob.Get(ctx, aliceRec.ID)
	require.ErrorIs(t, err, ErrMemoryNotFound)

	_, _, err = bob.Store(ctx, &Record{ID: aliceRec.ID, Kind: KindFact, Content: "overwrite"})
	require.ErrorIs(t, err, ErrMemoryNotFound)

	_, err = bob.Delete(ctx, aliceRec.ID, "", "", "", "", false)
	require.ErrorIs(t, err, ErrMemoryNotFound)

	aliceList, total, err := alice.List(ctx, nil, "", nil, "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, aliceList, 1)
}

func TestTenantStoreConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Backend: BackendMemory}
	router := newTenantBackendRouter(cfg)
	redactor := newRedactor(cfg)
	store, err := router.storeFor(ctx, "tenant")
	require.NoError(t, err)
	tenant := newTenantStore(store, "tenant", redactor)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec, _, storeErr := tenant.Store(ctx, &Record{
				Kind:    KindFact,
				Content: fmt.Sprintf("concurrent fact %d", i),
			})
			if storeErr != nil {
				return
			}
			_, _, _ = tenant.Search(ctx, "concurrent", nil, "", nil, "", 10, nil)
			_, _, _ = tenant.List(ctx, nil, "", nil, "", 0, 10)
			_, _ = tenant.Delete(ctx, rec.ID, "", "", "", "", false)
		}(i)
	}
	wg.Wait()
}
