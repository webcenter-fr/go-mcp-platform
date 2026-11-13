package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
)

// metaKeyScore is the metadata key carrying the relevance score of a retrieved
// memory. It mirrors the key used by the file and OpenSearch backends.
const metaKeyScore = "_score"

// inMemoryStore is a pure-Go, thread-safe MemoryStore used by the default
// "memory" backend.
type inMemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*schema.Document
	order   []string
}

var _ memoryagent.MemoryStore = (*inMemoryStore)(nil)

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{entries: make(map[string]*schema.Document)}
}

// Store upserts documents. An existing ID keeps its original created_at and is
// not appended to the insertion order a second time.
func (s *inMemoryStore) Store(_ context.Context, docs []*schema.Document, _ ...indexer.Option) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		clone := cloneDocument(doc)
		if existing, ok := s.entries[clone.ID]; ok {
			if created, ok := existing.MetaData["created_at"].(string); ok {
				clone.MetaData["created_at"] = created
			}
		} else {
			s.order = append(s.order, clone.ID)
		}
		s.entries[clone.ID] = clone
		ids = append(ids, clone.ID)
	}
	return ids, nil
}

// Retrieve ranks entries by lexical term overlap, mirroring the file backend.
func (s *inMemoryStore) Retrieve(_ context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.entries) == 0 {
		return nil, nil
	}

	retOpts := retriever.GetCommonOptions(&retriever.Options{}, opts...)
	topK := len(s.entries)
	if retOpts.TopK != nil && *retOpts.TopK > 0 && *retOpts.TopK < topK {
		topK = *retOpts.TopK
	}

	if strings.TrimSpace(query) == "" {
		docs := make([]*schema.Document, 0, topK)
		for _, id := range s.order {
			entry, ok := s.entries[id]
			if !ok {
				continue
			}
			docs = append(docs, cloneDocument(entry))
			if len(docs) >= topK {
				break
			}
		}
		return docs, nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}
	querySet := make(map[string]struct{}, len(queryTerms))
	for _, term := range queryTerms {
		querySet[term] = struct{}{}
	}

	type scoredEntry struct {
		doc   *schema.Document
		score float64
		index int
	}
	matches := make([]scoredEntry, 0, len(s.order))
	for i, id := range s.order {
		entry, ok := s.entries[id]
		if !ok {
			continue
		}
		score := scoreContent(querySet, entry.Content)
		if score <= 0 {
			continue
		}
		matches = append(matches, scoredEntry{doc: entry, score: score, index: i})
	}
	if len(matches) == 0 {
		return nil, nil
	}

	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.score != b.score {
			return a.score > b.score
		}
		aUpdated := metadataTime(a.doc, "updated_at")
		bUpdated := metadataTime(b.doc, "updated_at")
		if !aUpdated.Equal(bUpdated) {
			return aUpdated.After(bUpdated)
		}
		aCreated := metadataTime(a.doc, "created_at")
		bCreated := metadataTime(b.doc, "created_at")
		if !aCreated.Equal(bCreated) {
			return aCreated.After(bCreated)
		}
		return a.index < b.index
	})

	if len(matches) > topK {
		matches = matches[:topK]
	}
	docs := make([]*schema.Document, 0, len(matches))
	for _, match := range matches {
		doc := cloneDocument(match.doc)
		doc.MetaData[metaKeyScore] = match.score
		docs = append(docs, doc)
	}
	return docs, nil
}

// Delete removes a document by ID.
func (s *inMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[id]; !ok {
		return nil
	}
	delete(s.entries, id)
	s.order = removeFromOrder(s.order, id)
	return nil
}

// DeleteByFilter removes documents matching every filter entry.
func (s *inMemoryStore) DeleteByFilter(_ context.Context, filter map[string]any) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deleted := 0
	for id, doc := range s.entries {
		if documentMatchesFilter(doc, filter) {
			delete(s.entries, id)
			s.order = removeFromOrder(s.order, id)
			deleted++
		}
	}
	return deleted, nil
}

// List returns documents in insertion order with pagination.
func (s *inMemoryStore) List(_ context.Context, offset, limit int) ([]*schema.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(s.order)
	}
	docs := make([]*schema.Document, 0, limit)
	for i := offset; i < len(s.order) && i < offset+limit; i++ {
		doc, ok := s.entries[s.order[i]]
		if !ok {
			continue
		}
		docs = append(docs, cloneDocument(doc))
	}
	return docs, nil
}

// Count returns the number of stored documents.
func (s *inMemoryStore) Count(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries), nil
}

// GetDocument returns a copy of the document with the given ID, or nil when it
// does not exist.
func (s *inMemoryStore) GetDocument(_ context.Context, id string) (*schema.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.entries[id]
	if !ok {
		return nil, nil
	}
	return cloneDocument(doc), nil
}

// cloneDocument deep-copies a document and its metadata so callers cannot
// mutate shared state.
func cloneDocument(doc *schema.Document) *schema.Document {
	if doc == nil {
		return nil
	}
	clone := &schema.Document{ID: doc.ID, Content: doc.Content}
	if doc.MetaData != nil {
		clone.MetaData = make(map[string]any, len(doc.MetaData))
		for k, v := range doc.MetaData {
			clone.MetaData[k] = cloneValue(v)
		}
	}
	return clone
}

// cloneValue copies the container types used in document metadata.
func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = cloneValue(item)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return value
	}
}

// metadataTime parses an RFC3339 timestamp from document metadata.
func metadataTime(doc *schema.Document, key string) time.Time {
	if doc == nil || doc.MetaData == nil {
		return time.Time{}
	}
	raw, ok := doc.MetaData[key].(string)
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// documentMatchesFilter reports whether a document matches every filter entry.
func documentMatchesFilter(doc *schema.Document, filter map[string]any) bool {
	for key, want := range filter {
		wantString, ok := want.(string)
		if !ok {
			return false
		}
		got, ok := doc.MetaData[key].(string)
		if !ok || got != wantString {
			return false
		}
	}
	return true
}

// removeFromOrder returns order without id.
func removeFromOrder(order []string, id string) []string {
	result := make([]string, 0, len(order))
	for _, v := range order {
		if v != id {
			result = append(result, v)
		}
	}
	return result
}

// stopwords are query/content tokens that carry no discriminative signal.
var stopwords = map[string]struct{}{
	// English
	"an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "been": {},
	"but": {}, "by": {}, "can": {}, "did": {}, "do": {}, "does": {}, "for": {},
	"from": {}, "had": {}, "has": {}, "have": {}, "if": {}, "in": {}, "into": {},
	"is": {}, "it": {}, "its": {}, "me": {}, "my": {}, "not": {}, "of": {},
	"on": {}, "or": {}, "our": {}, "please": {}, "should": {}, "than": {},
	"that": {}, "the": {}, "their": {}, "them": {}, "then": {}, "there": {},
	"these": {}, "they": {}, "this": {}, "to": {}, "was": {}, "were": {},
	"what": {}, "when": {}, "where": {}, "which": {}, "will": {}, "with": {},
	"would": {}, "you": {}, "your": {},
	// French
	"au": {}, "aux": {}, "avec": {}, "ce": {}, "ces": {}, "cette": {},
	"dans": {}, "de": {}, "des": {}, "du": {}, "elle": {}, "en": {}, "est": {},
	"et": {}, "il": {}, "ils": {}, "je": {}, "la": {}, "le": {}, "les": {},
	"leur": {}, "ma": {}, "mais": {}, "mon": {}, "ne": {}, "nous": {},
	"ou": {}, "par": {}, "pas": {}, "plus": {}, "pour": {}, "que": {},
	"qui": {}, "sa": {}, "se": {}, "ses": {}, "son": {}, "sont": {}, "sur": {},
	"tu": {}, "un": {}, "une": {}, "votre": {}, "vous": {},
}

// tokenize lowercases s, splits it on every rune that is neither a letter nor a
// digit, then drops single-rune tokens and stopwords.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var result []string
	for _, w := range words {
		if utf8.RuneCountInString(w) >= 2 {
			if _, stop := stopwords[w]; !stop {
				result = append(result, w)
			}
		}
	}
	return result
}

// scoreContent returns matched + matched/distinctContentTerms, where matched is
// the number of distinct query terms present in the content.
func scoreContent(queryTerms map[string]struct{}, content string) float64 {
	contentTokens := tokenize(content)
	if len(queryTerms) == 0 || len(contentTokens) == 0 {
		return 0
	}
	contentSet := make(map[string]struct{}, len(contentTokens))
	for _, token := range contentTokens {
		contentSet[token] = struct{}{}
	}
	matched := 0
	for token := range contentSet {
		if _, ok := queryTerms[token]; ok {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) + float64(matched)/float64(len(contentSet))
}
