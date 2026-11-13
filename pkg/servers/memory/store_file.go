package memory

import (
	"context"
	"sync"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
	memfile "github.com/webcenter-fr/eino-ext/components/agent/memory/file"
)

// fileStore wraps the eino-ext JSONL store with a per-tenant mutex and a
// duplicate-free upsert.
type fileStore struct {
	mu    sync.Mutex
	inner *memfile.Store
}

var _ memoryagent.MemoryStore = (*fileStore)(nil)
var _ documentGetter = (*fileStore)(nil)

// newFileStore opens (or creates) the JSONL store under dir.
func newFileStore(dir string, minScore *float64) (*fileStore, error) {
	inner, err := memfile.NewStore(memfile.Config{Dir: dir, MinScore: minScore})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open memory file store %q", dir)
	}
	return &fileStore{inner: inner}, nil
}

// Store upserts documents. The eino-ext file store appends to its insertion
// order on every Store, so an existing ID is deleted first to avoid duplicate
// entries in List/Retrieve.
func (s *fileStore) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if existing, err := s.getDocumentLocked(ctx, doc.ID); err != nil {
			return ids, err
		} else if existing != nil {
			if created, ok := existing.MetaData["created_at"].(string); ok {
				doc.MetaData["created_at"] = created
			}
			if err := s.inner.Delete(ctx, doc.ID); err != nil {
				return ids, errors.Wrapf(err, "failed to replace memory %q", doc.ID)
			}
		}
		stored, err := s.inner.Store(ctx, []*schema.Document{doc}, opts...)
		if err != nil {
			return ids, errors.Wrapf(err, "failed to store memory %q", doc.ID)
		}
		ids = append(ids, stored...)
	}
	return ids, nil
}

// Retrieve delegates to the eino-ext lexical retriever.
func (s *fileStore) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Retrieve(ctx, query, opts...)
}

// Delete removes a document by ID.
func (s *fileStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Delete(ctx, id)
}

// DeleteByFilter removes documents matching the filter.
func (s *fileStore) DeleteByFilter(ctx context.Context, filter map[string]any) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.DeleteByFilter(ctx, filter)
}

// List returns documents with pagination.
func (s *fileStore) List(ctx context.Context, offset, limit int) ([]*schema.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.List(ctx, offset, limit)
}

// Count returns the number of stored documents.
func (s *fileStore) Count(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Count(ctx)
}

// GetDocument returns the document with the given ID, or nil when absent.
func (s *fileStore) GetDocument(ctx context.Context, id string) (*schema.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getDocumentLocked(ctx, id)
}

// getDocumentLocked scans the in-memory index of the file store. The caller
// must hold s.mu.
func (s *fileStore) getDocumentLocked(ctx context.Context, id string) (*schema.Document, error) {
	docs, err := s.inner.List(ctx, 0, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list memory file store")
	}
	for _, doc := range docs {
		if doc.ID == id {
			return doc, nil
		}
	}
	return nil, nil
}
