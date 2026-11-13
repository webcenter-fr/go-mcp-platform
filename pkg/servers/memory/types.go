package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
)

// MemoryKind classifies the functional purpose of a stored memory.
//
//nolint:revive // MemoryKind is the documented public name.
type MemoryKind string

const (
	// KindDiscussion is a past conversation summary or Q&A exchange that can
	// directly answer recurring questions.
	KindDiscussion MemoryKind = "discussion"
	// KindPrompt is a refined, reusable prompt template distilled from a session.
	KindPrompt MemoryKind = "prompt"
	// KindProcess is a step-by-step agent workflow or operational procedure to
	// handle a class of prompts or tasks more effectively.
	KindProcess MemoryKind = "process"
	// KindBestPractice is a developer best practice, coding convention,
	// architectural rule, or security/testing pattern.
	KindBestPractice MemoryKind = "best_practice"
	// KindPreference is a user or team preference (e.g. preferred libraries,
	// frameworks, tools, coding style).
	KindPreference MemoryKind = "preference"
	// KindFact is a declarative project, environment, or domain fact.
	KindFact MemoryKind = "fact"
	// KindLearning is a debugging insight, root cause, or pitfall ("X fails because Y").
	KindLearning MemoryKind = "learning"
)

// MemorySource identifies how a memory originated.
//
//nolint:revive // MemorySource is the documented public name.
type MemorySource string

// Memory sources.
const (
	SourceUser        MemorySource = "user"
	SourceAssistant   MemorySource = "assistant"
	SourceObservation MemorySource = "observation"
	SourceSession     MemorySource = "session"
)

// Record is the public representation of a stored memory returned by MCP tools.
type Record struct {
	ID        string            `json:"id"`
	Kind      MemoryKind        `json:"kind"`
	Title     string            `json:"title,omitempty"`
	Content   string            `json:"content"`
	Scope     string            `json:"scope,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Source    MemorySource      `json:"source"`
	SessionID string            `json:"sessionId,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SearchHit pairs a matched Record with its relevance score.
type SearchHit struct {
	Record
	Score float64 `json:"score"`
}

// ConversationMessage is one turn supplied to memory_capture_discussion.
type ConversationMessage struct {
	Role    string `json:"role" validate:"required,oneof=user assistant system tool" jsonschema:"(required) Speaker role: 'user', 'assistant', 'system', or 'tool'"`
	Content string `json:"content" validate:"required,max=16384" jsonschema:"(required) Turn text content (max 16384 chars)"`
}

// KindInfo describes one supported MemoryKind in the memory://kinds resource.
type KindInfo struct {
	Kind        MemoryKind `json:"kind"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Example     string     `json:"example"`
}

// supportedKinds is the canonical kind order used by the catalog and stats.
var supportedKinds = []MemoryKind{
	KindDiscussion,
	KindPrompt,
	KindProcess,
	KindBestPractice,
	KindPreference,
	KindFact,
	KindLearning,
}

// kindCatalog is the static description of every supported kind.
var kindCatalog = []KindInfo{
	{
		Kind:        KindDiscussion,
		Title:       "Discussion",
		Description: "A summarized past conversation exchange or Q&A resolution that can directly answer recurring questions.",
		Example:     "When the ArgoCD sync fails with 'ComparisonError', refresh the application and re-run the sync after the repo server cache expires.",
	},
	{
		Kind:        KindPrompt,
		Title:       "Prompt",
		Description: "A reusable, self-contained prompt template refined from what worked in a session.",
		Example:     "Review this Go diff for error wrapping, context propagation, and table-driven test coverage. Return findings grouped by severity.",
	},
	{
		Kind:        KindProcess,
		Title:       "Process",
		Description: "A step-by-step agent workflow or operational procedure to handle a class of prompts or tasks more effectively.",
		Example:     "1. Reproduce the failing test. 2. Read the stack trace. 3. Add a regression test. 4. Fix the root cause. 5. Run the full suite.",
	},
	{
		Kind:        KindBestPractice,
		Title:       "Best practice",
		Description: "A developer best practice, coding standard, architectural rule, or testing/security pattern.",
		Example:     "Wrap every returned error with emperror.dev/errors and include the operation that failed.",
	},
	{
		Kind:        KindPreference,
		Title:       "Preference",
		Description: "A user or project preference such as preferred libraries, frameworks, tools, or style.",
		Example:     "Prefer table-driven tests with stretchr/testify and avoid new dependencies without discussion.",
	},
	{
		Kind:        KindFact,
		Title:       "Fact",
		Description: "A declarative project, environment, or domain fact.",
		Example:     "The production cluster runs OpenSearch 2.19 behind an internal TLS endpoint.",
	},
	{
		Kind:        KindLearning,
		Title:       "Learning",
		Description: "A debugging insight, root cause, or pitfall (\"X fails because Y\").",
		Example:     "The build fails because the Go module cache is read-only in the distroless image; set GOMODCACHE to a writable path.",
	},
}

// SupportedKinds returns the supported memory kinds in canonical order.
func SupportedKinds() []MemoryKind {
	return append([]MemoryKind(nil), supportedKinds...)
}

// KindCatalog returns the description of every supported memory kind.
func KindCatalog() []KindInfo {
	return append([]KindInfo(nil), kindCatalog...)
}

// NormalizeKind maps a raw category to a supported MemoryKind. It normalizes
// the legacy eino-ext categories "procedure" -> process and "summary" ->
// discussion so records written by an eino-ext MemoryAgent remain readable.
func NormalizeKind(raw string) (MemoryKind, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(KindDiscussion), memoryagent.CategorySummary:
		return KindDiscussion, true
	case string(KindPrompt):
		return KindPrompt, true
	case string(KindProcess), memoryagent.CategoryProcedure:
		return KindProcess, true
	case string(KindBestPractice):
		return KindBestPractice, true
	case string(KindPreference):
		return KindPreference, true
	case string(KindFact):
		return KindFact, true
	case string(KindLearning):
		return KindLearning, true
	default:
		return "", false
	}
}

// DeterministicID returns a content-addressed ID scoped to a tenant. Two
// identities storing identical text produce distinct IDs, and whitespace
// differences do not change the ID.
func DeterministicID(tenantID string, kind MemoryKind, content string) string {
	normalized := strings.Join(strings.Fields(content), " ")
	sum := sha256.Sum256([]byte(tenantID + "\x00" + string(kind) + "\x00" + normalized))
	return hex.EncodeToString(sum[:])[:32]
}

// RecordToDocument projects a Record onto the eino schema.Document shape used
// by every memory backend. The document content is searchable (title, body,
// scope and tags) while the clean body is preserved under raw_content.
func RecordToDocument(rec *Record, tenantID string) *schema.Document {
	if rec == nil {
		return nil
	}
	content := rec.Content
	if rec.Title != "" {
		content = rec.Title + "\n" + rec.Content
	}
	if rec.Scope != "" {
		content += "\n[scope: " + rec.Scope + "]"
	}
	if len(rec.Tags) > 0 {
		content += "\n[tags: " + strings.Join(rec.Tags, ", ") + "]"
	}

	meta := map[string]any{
		"category":    string(rec.Kind),
		"source":      string(rec.Source),
		"session_id":  rec.SessionID,
		"user_id":     tenantID,
		"created_at":  rec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  rec.UpdatedAt.UTC().Format(time.RFC3339),
		"raw_content": rec.Content,
	}
	if rec.Title != "" {
		meta["title"] = rec.Title
	}
	if rec.Scope != "" {
		meta["scope"] = rec.Scope
	}
	if len(rec.Tags) > 0 {
		meta["tags"] = append([]string(nil), rec.Tags...)
	}
	if len(rec.Metadata) > 0 {
		custom := make(map[string]string, len(rec.Metadata))
		for k, v := range rec.Metadata {
			custom[k] = v
		}
		meta["custom_meta"] = custom
	}

	return &schema.Document{
		ID:       rec.ID,
		Content:  content,
		MetaData: meta,
	}
}

// RecordFromDocument reconstructs a Record from a schema.Document. It handles
// the OpenSearch retriever's _id/_score metadata, the raw_content projection,
// and legacy eino-ext categories.
func RecordFromDocument(doc *schema.Document) (*Record, float64) {
	if doc == nil {
		return nil, 0
	}
	rec := &Record{ID: doc.ID}
	meta := doc.MetaData

	if rec.ID == "" {
		if id, ok := meta["_id"].(string); ok {
			rec.ID = id
		}
	}
	if raw, ok := meta["raw_content"].(string); ok {
		rec.Content = raw
	} else {
		rec.Content = doc.Content
	}
	if category, ok := meta["category"].(string); ok {
		if kind, valid := NormalizeKind(category); valid {
			rec.Kind = kind
		}
	}
	if source, ok := meta["source"].(string); ok {
		rec.Source = MemorySource(source)
	}
	if sessionID, ok := meta["session_id"].(string); ok {
		rec.SessionID = sessionID
	}
	if title, ok := meta["title"].(string); ok {
		rec.Title = title
	}
	if scope, ok := meta["scope"].(string); ok {
		rec.Scope = scope
	}
	rec.Tags = stringSlice(meta["tags"])
	rec.Metadata = stringMap(meta["custom_meta"])
	if created, ok := meta["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			rec.CreatedAt = t
		}
	}
	if updated, ok := meta["updated_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			rec.UpdatedAt = t
		}
	}

	score, _ := meta["_score"].(float64)
	return rec, score
}

// stringSlice converts a metadata value to []string, accepting both the
// []string written by RecordToDocument and the []any produced by JSON
// unmarshalling.
func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// stringMap converts a metadata value to map[string]string, accepting both the
// map[string]string written by RecordToDocument and the map[string]any produced
// by JSON unmarshalling.
func stringMap(value any) map[string]string {
	switch v := value.(type) {
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(v))
		for k, item := range v {
			if s, ok := item.(string); ok {
				out[k] = s
			}
		}
		return out
	default:
		return nil
	}
}
