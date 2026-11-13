package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
)

// maxScanDocs bounds a full scan of a backend that has no server-side filter.
const maxScanDocs = 10000

// documentGetter is implemented by backends that can fetch one document by ID
// without scanning the whole store.
type documentGetter interface {
	GetDocument(ctx context.Context, id string) (*schema.Document, error)
}

// tenantBackendRouter creates and caches one backend store per tenant. The
// OpenSearch backend is shared across tenants because isolation is enforced by
// the user_id metadata and tenant-scoped IDs.
type tenantBackendRouter struct {
	cfg *Config

	mu         sync.Mutex
	stores     map[string]memoryagent.MemoryStore
	opensearch *lazyOpenSearchStore
}

func newTenantBackendRouter(cfg *Config) *tenantBackendRouter {
	return &tenantBackendRouter{cfg: cfg, stores: make(map[string]memoryagent.MemoryStore)}
}

// storeFor returns the backend store for a tenant, creating it on first use.
func (r *tenantBackendRouter) storeFor(ctx context.Context, tenantID string) (memoryagent.MemoryStore, error) {
	if r.cfg.Store != nil {
		return r.cfg.Store, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if store, ok := r.stores[tenantID]; ok {
		return store, nil
	}

	var store memoryagent.MemoryStore
	switch r.cfg.Backend {
	case BackendMemory:
		store = newInMemoryStore()
	case BackendFile:
		dir := filepath.Join(r.cfg.File.Dir, tenantDirSegment(tenantID))
		file, err := newFileStore(dir, r.cfg.MinScore)
		if err != nil {
			return nil, err
		}
		store = file
	case BackendOpenSearch:
		if r.opensearch == nil {
			r.opensearch = newLazyOpenSearchStore(r.cfg)
		}
		store = r.opensearch
	default:
		return nil, errors.Errorf("memory: unsupported backend %q", r.cfg.Backend)
	}

	r.stores[tenantID] = store
	return store, nil
}

// Close releases backend resources. The current backends hold no long-lived
// handles, so it is a no-op kept for the NewServerWithCloser contract.
func (r *tenantBackendRouter) Close() error {
	return nil
}

// tenantDirSegment maps a tenant key to a safe directory name. The local and
// shared tenants use fixed names; every other tenant uses a SHA-256 digest so
// untrusted subject characters never reach the filesystem.
func tenantDirSegment(tenantID string) string {
	switch tenantID {
	case DefaultLocalTenant:
		return DefaultLocalTenant
	case DefaultSharedTenant:
		return DefaultSharedTenant
	default:
		sum := sha256.Sum256([]byte(tenantID))
		return hex.EncodeToString(sum[:8])
	}
}

// tenantStore enforces tenant ownership, filtering and pagination on top of a
// backend store.
type tenantStore struct {
	inner    memoryagent.MemoryStore
	tenantID string
	redactor *redactor
}

func newTenantStore(inner memoryagent.MemoryStore, tenantID string, redactor *redactor) *tenantStore {
	return &tenantStore{inner: inner, tenantID: tenantID, redactor: redactor}
}

// Store upserts a record in the tenant namespace. It returns the stored record
// and whether an existing record was updated.
func (s *tenantStore) Store(ctx context.Context, rec *Record) (*Record, bool, error) {
	if rec == nil {
		return nil, false, errors.New("memory: record is nil")
	}
	s.redactor.redactRecord(rec)

	now := time.Now().UTC().Truncate(time.Second)
	explicitID := rec.ID != ""
	if !explicitID {
		rec.ID = DeterministicID(s.tenantID, rec.Kind, rec.Content)
	}

	existing, err := s.getDocument(ctx, rec.ID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil && explicitID {
		return nil, false, errors.Wrapf(ErrMemoryNotFound, "memory %q", rec.ID)
	}
	upserted := false
	if existing != nil {
		if docTenant(existing) != s.tenantID {
			return nil, false, errors.Wrapf(ErrMemoryNotFound, "memory %q", rec.ID)
		}
		upserted = true
		if created, ok := existing.MetaData["created_at"].(string); ok {
			if t, parseErr := time.Parse(time.RFC3339, created); parseErr == nil {
				rec.CreatedAt = t
			}
		}
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now

	doc := RecordToDocument(rec, s.tenantID)
	if _, err := s.inner.Store(ctx, []*schema.Document{doc}); err != nil {
		return nil, false, errors.Wrapf(err, "failed to store memory %q", rec.ID)
	}
	return rec, upserted, nil
}

// Get returns one record owned by the tenant.
func (s *tenantStore) Get(ctx context.Context, id string) (*Record, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil || docTenant(doc) != s.tenantID {
		return nil, errors.Wrapf(ErrMemoryNotFound, "memory %q", id)
	}
	rec, _ := RecordFromDocument(doc)
	s.redactor.redactRecord(rec)
	return rec, nil
}

// Search retrieves and filters records, overfetching so post-filtering still
// fills topK in a multi-tenant index.
func (s *tenantStore) Search(
	ctx context.Context,
	query string,
	kinds []MemoryKind,
	scope string,
	tags []string,
	sessionID string,
	topK int,
	minScore *float64,
) ([]SearchHit, int, error) {
	fetchK := topK * 5
	if fetchK < 50 {
		fetchK = 50
	}
	if fetchK > 200 {
		fetchK = 200
	}

	docs, err := s.inner.Retrieve(ctx, query, retriever.WithTopK(fetchK))
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to search memories")
	}

	hits := make([]SearchHit, 0, len(docs))
	for _, doc := range docs {
		if docTenant(doc) != s.tenantID {
			continue
		}
		rec, score := RecordFromDocument(doc)
		s.redactor.redactRecord(rec)
		if !matchesFilters(rec, kinds, scope, tags, sessionID) {
			continue
		}
		if minScore != nil && score < *minScore {
			continue
		}
		hits = append(hits, SearchHit{Record: *rec, Score: score})
	}

	total := len(hits)
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, total, nil
}

// List returns the tenant's records sorted by updatedAt descending.
func (s *tenantStore) List(
	ctx context.Context,
	kinds []MemoryKind,
	scope string,
	tags []string,
	sessionID string,
	offset, limit int,
) ([]Record, int, error) {
	records, err := s.ownedRecords(ctx)
	if err != nil {
		return nil, 0, err
	}

	filtered := make([]Record, 0, len(records))
	for _, rec := range records {
		if matchesFilters(rec, kinds, scope, tags, sessionID) {
			filtered = append(filtered, *rec)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].UpdatedAt.Equal(filtered[j].UpdatedAt) {
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		}
		if !filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})

	total := len(filtered)
	if offset >= total {
		return []Record{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

// Delete removes records selected by ID or by a non-empty filter. A dry run
// returns the matching IDs without deleting anything.
func (s *tenantStore) Delete(
	ctx context.Context,
	id, kind, sessionID, scope, tag string,
	dryRun bool,
) ([]string, error) {
	var ids []string
	if id != "" {
		doc, err := s.getDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		if doc == nil || docTenant(doc) != s.tenantID {
			return nil, errors.Wrapf(ErrMemoryNotFound, "memory %q", id)
		}
		ids = []string{id}
	} else {
		records, err := s.ownedRecords(ctx)
		if err != nil {
			return nil, err
		}
		for _, rec := range records {
			if kind != "" && string(rec.Kind) != kind {
				continue
			}
			if sessionID != "" && rec.SessionID != sessionID {
				continue
			}
			if scope != "" && rec.Scope != scope {
				continue
			}
			if tag != "" && !slices.Contains(rec.Tags, tag) {
				continue
			}
			ids = append(ids, rec.ID)
		}
	}

	if dryRun {
		return ids, nil
	}
	for _, memoryID := range ids {
		if err := s.inner.Delete(ctx, memoryID); err != nil {
			return ids, errors.Wrapf(err, "failed to delete memory %q", memoryID)
		}
	}
	return ids, nil
}

// Stats returns the tenant's total record count and per-kind counts.
func (s *tenantStore) Stats(ctx context.Context) (int, map[string]int, error) {
	records, err := s.ownedRecords(ctx)
	if err != nil {
		return 0, nil, err
	}
	byKind := make(map[string]int, len(supportedKinds))
	for _, rec := range records {
		byKind[string(rec.Kind)]++
	}
	return len(records), byKind, nil
}

// ownedRecords scans the backend and returns the tenant's records.
func (s *tenantStore) ownedRecords(ctx context.Context) ([]*Record, error) {
	docs, err := s.scanAll(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]*Record, 0, len(docs))
	for _, doc := range docs {
		if docTenant(doc) != s.tenantID {
			continue
		}
		rec, _ := RecordFromDocument(doc)
		s.redactor.redactRecord(rec)
		records = append(records, rec)
	}
	return records, nil
}

// scanAll pages through the backend up to maxScanDocs.
func (s *tenantStore) scanAll(ctx context.Context) ([]*schema.Document, error) {
	var all []*schema.Document
	for offset := 0; offset < maxScanDocs; offset += 100 {
		docs, err := s.inner.List(ctx, offset, 100)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list memories")
		}
		all = append(all, docs...)
		if len(docs) < 100 {
			break
		}
	}
	return all, nil
}

// getDocument fetches one raw document, using the backend's O(1) getter when
// available and falling back to a scan otherwise.
func (s *tenantStore) getDocument(ctx context.Context, id string) (*schema.Document, error) {
	if getter, ok := s.inner.(documentGetter); ok {
		return getter.GetDocument(ctx, id)
	}
	docs, err := s.scanAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if doc.ID == id {
			return doc, nil
		}
	}
	return nil, nil
}

// docTenant returns the user_id metadata of a document.
func docTenant(doc *schema.Document) string {
	if doc == nil || doc.MetaData == nil {
		return ""
	}
	tenant, _ := doc.MetaData["user_id"].(string)
	return tenant
}

// matchesFilters applies the kind, scope, tag and session filters.
func matchesFilters(rec *Record, kinds []MemoryKind, scope string, tags []string, sessionID string) bool {
	if len(kinds) > 0 && !slices.Contains(kinds, rec.Kind) {
		return false
	}
	if scope != "" && !strings.Contains(strings.ToLower(rec.Scope), strings.ToLower(scope)) {
		return false
	}
	if sessionID != "" && rec.SessionID != sessionID {
		return false
	}
	for _, tag := range tags {
		if !slices.Contains(rec.Tags, tag) {
			return false
		}
	}
	return true
}
