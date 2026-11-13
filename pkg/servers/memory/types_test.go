package memory

import (
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordDocumentRoundTrip(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Hour)
	rec := &Record{
		ID:        "abc123",
		Kind:      KindBestPractice,
		Title:     "Wrap errors",
		Content:   "Always wrap errors with context.",
		Scope:     "lang:golang",
		Tags:      []string{"errors", "go"},
		Source:    SourceAssistant,
		SessionID: "session-1",
		CreatedAt: created,
		UpdatedAt: updated,
		Metadata:  map[string]string{"origin": "review"},
	}

	doc := RecordToDocument(rec, "tenant-a")
	require.NotNil(t, doc)
	assert.Contains(t, doc.Content, "Wrap errors")
	assert.Contains(t, doc.Content, "Always wrap errors with context.")
	assert.Contains(t, doc.Content, "[scope: lang:golang]")
	assert.Contains(t, doc.Content, "[tags: errors, go]")
	assert.Equal(t, "tenant-a", doc.MetaData["user_id"])
	assert.Equal(t, "Always wrap errors with context.", doc.MetaData["raw_content"])

	got, score := RecordFromDocument(doc)
	require.NotNil(t, got)
	assert.Zero(t, score)
	assert.Equal(t, rec.ID, got.ID)
	assert.Equal(t, rec.Kind, got.Kind)
	assert.Equal(t, rec.Title, got.Title)
	assert.Equal(t, rec.Content, got.Content)
	assert.Equal(t, rec.Scope, got.Scope)
	assert.Equal(t, rec.Tags, got.Tags)
	assert.Equal(t, rec.Source, got.Source)
	assert.Equal(t, rec.SessionID, got.SessionID)
	assert.True(t, rec.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, rec.UpdatedAt.Equal(got.UpdatedAt))
	assert.Equal(t, rec.Metadata, got.Metadata)
}

func TestRecordFromDocumentIDFallback(t *testing.T) {
	doc := &schema.Document{
		Content: "legacy content",
		MetaData: map[string]any{
			"_id":        "from-metadata",
			"_score":     0.75,
			"category":   "summary",
			"source":     "session",
			"session_id": "s1",
		},
	}
	rec, score := RecordFromDocument(doc)
	require.NotNil(t, rec)
	assert.Equal(t, "from-metadata", rec.ID)
	assert.Equal(t, "legacy content", rec.Content)
	assert.Equal(t, KindDiscussion, rec.Kind)
	assert.Equal(t, 0.75, score)
}

func TestRecordFromDocumentLegacyCategories(t *testing.T) {
	tests := []struct {
		category string
		want     MemoryKind
	}{
		{category: "procedure", want: KindProcess},
		{category: "summary", want: KindDiscussion},
		{category: "fact", want: KindFact},
		{category: "preference", want: KindPreference},
		{category: "learning", want: KindLearning},
	}
	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			rec, _ := RecordFromDocument(&schema.Document{
				ID:       "id",
				Content:  "content",
				MetaData: map[string]any{"category": tt.category},
			})
			require.NotNil(t, rec)
			assert.Equal(t, tt.want, rec.Kind)
		})
	}
}

func TestRecordFromDocumentJSONShapes(t *testing.T) {
	rec, _ := RecordFromDocument(&schema.Document{
		ID:      "id",
		Content: "content",
		MetaData: map[string]any{
			"category":    "prompt",
			"tags":        []any{"a", "b"},
			"custom_meta": map[string]any{"k": "v"},
		},
	})
	require.NotNil(t, rec)
	assert.Equal(t, []string{"a", "b"}, rec.Tags)
	assert.Equal(t, map[string]string{"k": "v"}, rec.Metadata)
}

func TestDeterministicID(t *testing.T) {
	first := DeterministicID("tenant-a", KindFact, "the  sky is blue")
	second := DeterministicID("tenant-a", KindFact, "the sky is blue")
	assert.Equal(t, first, second, "whitespace normalization must produce the same ID")

	assert.NotEqual(t, first, DeterministicID("tenant-b", KindFact, "the sky is blue"))
	assert.NotEqual(t, first, DeterministicID("tenant-a", KindLearning, "the sky is blue"))
	assert.Len(t, first, 32)
}

func TestNormalizeKind(t *testing.T) {
	for _, kind := range SupportedKinds() {
		got, ok := NormalizeKind(string(kind))
		assert.True(t, ok)
		assert.Equal(t, kind, got)
	}
	_, ok := NormalizeKind("unknown")
	assert.False(t, ok)
}

func TestKindCatalog(t *testing.T) {
	catalog := KindCatalog()
	assert.Len(t, catalog, len(SupportedKinds()))
	for _, info := range catalog {
		assert.NotEmpty(t, info.Title)
		assert.NotEmpty(t, info.Description)
		assert.NotEmpty(t, info.Example)
	}
}
