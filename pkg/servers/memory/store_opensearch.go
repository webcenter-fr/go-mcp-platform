package memory

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	opensearchv4 "github.com/disaster37/opensearch/v4"
	"github.com/disaster37/opensearch/v4/api"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
	memopensearch "github.com/webcenter-fr/eino-ext/components/agent/memory/opensearch"
	"github.com/webcenter-fr/eino-ext/components/model/chatmodel"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/osclient"
)

// lazyOpenSearchStore wraps the eino-ext OpenSearch memory store. Construction
// performs no network I/O: the client, embedder and index are created on the
// first operation and retried on the next call if initialization fails.
type lazyOpenSearchStore struct {
	cfg *Config

	mu           sync.Mutex
	inner        *memopensearch.Store
	client       opensearchv4.Client
	indexName    string
	contentField string
}

var _ memoryagent.MemoryStore = (*lazyOpenSearchStore)(nil)
var _ documentGetter = (*lazyOpenSearchStore)(nil)

func newLazyOpenSearchStore(cfg *Config) *lazyOpenSearchStore {
	return &lazyOpenSearchStore{cfg: cfg}
}

// ensureInit builds the OpenSearch client, optional embedder and store once.
// A failure leaves inner nil so the next call retries cleanly.
func (s *lazyOpenSearchStore) ensureInit(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inner != nil {
		return nil
	}

	embedder := s.cfg.Embedder
	if embedder == nil && s.cfg.Embedding != nil {
		built, err := chatmodel.NewEmbedding(ctx, &chatmodel.EmbeddingConfig{
			Provider:      chatmodel.EmbeddingProvider(s.cfg.Embedding.Provider),
			BaseURL:       s.cfg.Embedding.BaseURL,
			Model:         s.cfg.Embedding.Model,
			Timeout:       s.cfg.Embedding.Timeout,
			TLSSkipVerify: s.cfg.Embedding.TLSSkipVerify,
			APIKey:        s.cfg.Embedding.APIKey,
		})
		if err != nil {
			return errors.Wrap(err, "memory: failed to build embedding provider")
		}
		embedder = built
	}

	client, err := osclient.New(ctx, osclient.Config{
		URLs:          s.cfg.OpenSearch.URLs,
		Username:      s.cfg.OpenSearch.Username,
		Password:      s.cfg.OpenSearch.Password,
		TLSSkipVerify: s.cfg.OpenSearch.TLSSkipVerify,
	}, 30*time.Second)
	if err != nil {
		return errors.Wrap(err, "memory: failed to build OpenSearch client")
	}

	inner, err := memopensearch.NewStore(ctx, &memopensearch.Config{
		URLs:               s.cfg.OpenSearch.URLs,
		Username:           s.cfg.OpenSearch.Username,
		Password:           s.cfg.OpenSearch.Password,
		TLSSkipVerify:      s.cfg.OpenSearch.TLSSkipVerify,
		IndexName:          s.cfg.OpenSearch.IndexName,
		Embedding:          embedder,
		VectorField:        s.cfg.OpenSearch.VectorField,
		ContentField:       s.cfg.OpenSearch.ContentField,
		Hybrid:             s.cfg.OpenSearch.Hybrid,
		K:                  s.cfg.OpenSearch.K,
		VectorDimension:    s.cfg.OpenSearch.VectorDimension,
		BatchSize:          s.cfg.OpenSearch.BatchSize,
		SearchPipeline:     s.cfg.OpenSearch.SearchPipeline,
		Operator:           s.cfg.OpenSearch.Operator,
		MinimumShouldMatch: s.cfg.OpenSearch.MinimumShouldMatch,
		MinScore:           s.cfg.MinScore,
	})
	if err != nil {
		return errors.Wrap(err, "memory: failed to build OpenSearch store")
	}

	s.client = client
	s.inner = inner
	s.indexName = s.cfg.OpenSearch.IndexName
	s.contentField = s.cfg.OpenSearch.ContentField
	return nil
}

// Store indexes documents.
func (s *lazyOpenSearchStore) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	if err := s.ensureInit(ctx); err != nil {
		return nil, err
	}
	return s.inner.Store(ctx, docs, opts...)
}

// Retrieve searches the index.
func (s *lazyOpenSearchStore) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if err := s.ensureInit(ctx); err != nil {
		return nil, err
	}
	return s.inner.Retrieve(ctx, query, opts...)
}

// Delete removes a document by ID.
func (s *lazyOpenSearchStore) Delete(ctx context.Context, id string) error {
	if err := s.ensureInit(ctx); err != nil {
		return err
	}
	return s.inner.Delete(ctx, id)
}

// DeleteByFilter removes documents matching the filter.
func (s *lazyOpenSearchStore) DeleteByFilter(ctx context.Context, filter map[string]any) (int, error) {
	if err := s.ensureInit(ctx); err != nil {
		return 0, err
	}
	return s.inner.DeleteByFilter(ctx, filter)
}

// List returns documents with pagination.
func (s *lazyOpenSearchStore) List(ctx context.Context, offset, limit int) ([]*schema.Document, error) {
	if err := s.ensureInit(ctx); err != nil {
		return nil, err
	}
	return s.inner.List(ctx, offset, limit)
}

// Count returns the number of documents in the index.
func (s *lazyOpenSearchStore) Count(ctx context.Context) (int, error) {
	if err := s.ensureInit(ctx); err != nil {
		return 0, err
	}
	return s.inner.Count(ctx)
}

// GetDocument fetches one document by ID with an O(1) OpenSearch GET.
func (s *lazyOpenSearchStore) GetDocument(ctx context.Context, id string) (*schema.Document, error) {
	if err := s.ensureInit(ctx); err != nil {
		return nil, err
	}
	result, err := s.client.Document().Get(ctx, &api.GetRequest{Index: s.indexName, Id: id})
	if err != nil {
		return nil, errors.Wrapf(err, "memory: failed to get OpenSearch document %q", id)
	}
	if !result.Found {
		return nil, nil
	}
	src := map[string]any{}
	if len(result.Source) > 0 {
		if err := json.Unmarshal(result.Source, &src); err != nil {
			return nil, errors.Wrapf(err, "memory: failed to decode OpenSearch document %q", id)
		}
	}
	content, _ := src[s.contentField].(string)
	meta := make(map[string]any, len(src))
	for k, v := range src {
		if k == s.contentField {
			continue
		}
		meta[k] = v
	}
	return &schema.Document{ID: result.Id, Content: content, MetaData: meta}, nil
}
