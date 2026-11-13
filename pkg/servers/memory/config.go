// Package memory provides an MCP server family exposing typed long-term agent
// memory (discussions, prompts, processes, best practices, preferences, facts
// and learnings) backed by an in-memory, JSONL-file or OpenSearch store.
package memory

import (
	"time"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
	"github.com/webcenter-fr/eino-ext/components/model/chatmodel"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/fileutil"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
)

// BackendType selects the persistence engine.
type BackendType string

// Backend types.
const (
	BackendMemory     BackendType = "memory"
	BackendFile       BackendType = "file"
	BackendOpenSearch BackendType = "opensearch"
)

// IsolationMode controls how memories are partitioned across authenticated callers.
type IsolationMode string

const (
	// IsolationPerIdentity scopes memories to Identity.Subject() (default).
	IsolationPerIdentity IsolationMode = "per_identity"
	// IsolationShared shares one namespace across all authenticated callers on this server.
	IsolationShared IsolationMode = "shared"
)

// Default configuration constants.
const (
	DefaultBackend        = BackendMemory
	DefaultIsolation      = IsolationPerIdentity
	DefaultSharedTenant   = "shared"
	DefaultLocalTenant    = "local"
	DefaultTopK           = 5
	DefaultMaxTopK        = 50
	DefaultMaxContentChar = 16384
	DefaultMaxQueryChar   = 4096
	DefaultRequestTimeout = 30 * time.Second
	DefaultOpenSearchIdx  = "eino_agent_memory"
	DefaultVectorDim      = 1536
	DefaultBatchSize      = 100
)

// Config configures the memory MCP server family.
type Config struct {
	// Backend selects the persistence store: "memory" (default), "file", or "opensearch".
	Backend BackendType `mapstructure:"backend" json:"backend" validate:"required,oneof=memory file opensearch" jsonschema:"(required) Storage backend: 'memory', 'file', or 'opensearch' (default 'memory')"`

	// Isolation controls tenant partitioning: "per_identity" (default) or "shared".
	Isolation IsolationMode `mapstructure:"isolation" json:"isolation" validate:"required,oneof=per_identity shared" jsonschema:"Tenant isolation mode: 'per_identity' (default) or 'shared'"`

	// SharedNamespace is the tenant key used when Isolation == "shared". Default "shared".
	SharedNamespace string `mapstructure:"sharedNamespace" json:"sharedNamespace,omitempty" validate:"omitempty,max=128" jsonschema:"description=Tenant namespace when isolation is 'shared' (default 'shared')"`

	// File configures the JSONL file backend (required when Backend == "file").
	File *FileBackendConfig `mapstructure:"file" json:"file,omitempty" validate:"omitempty" jsonschema:"description=JSONL file backend settings (required when backend is 'file')"`

	// OpenSearch configures the OpenSearch backend (required when Backend == "opensearch").
	OpenSearch *OpenSearchBackendConfig `mapstructure:"opensearch" json:"opensearch,omitempty" validate:"omitempty" jsonschema:"description=OpenSearch backend settings (required when backend is 'opensearch')"`

	// Embedding configures optional vector embeddings for OpenSearch kNN / hybrid search.
	Embedding *EmbeddingConfig `mapstructure:"embedding" json:"embedding,omitempty" validate:"omitempty" jsonschema:"description=Optional embedding provider for OpenSearch vector/hybrid search"`

	// LLM configures optional server-side conversation summarization and typed extraction.
	LLM *LLMConfig `mapstructure:"llm" json:"llm,omitempty" validate:"omitempty" jsonschema:"description=Optional chat model for server-side discussion summarization and typed memory extraction"`

	// ReadOnly exposes only read tools (memory_search, memory_get, memory_list, memory_stats).
	ReadOnly bool `mapstructure:"readOnly" json:"readOnly,omitempty" jsonschema:"description=Expose only read-only memory tools (default false)"`

	// RequireDeleteApproval marks memory_delete as a gated write tool requiring dryRun/confirmed + human approval.
	RequireDeleteApproval bool `mapstructure:"requireDeleteApproval" json:"requireDeleteApproval,omitempty" jsonschema:"description=Require human approval (elicitation or approval service) for memory_delete (default false)"`

	// Prompts enables the MCP prompt and resource surface. Default true.
	Prompts *bool `mapstructure:"prompts" json:"prompts,omitempty" jsonschema:"description=Expose memory distillation MCP prompts and memory://kinds resource (default true)"`

	// DefaultTopK is the default result limit for memory_search. Default 5.
	DefaultTopK int `mapstructure:"defaultTopK" json:"defaultTopK" validate:"gte=1,lte=50" jsonschema:"description=Default topK for memory_search (default 5)"`

	// MaxTopK caps per-call topK in memory_search. Default 50.
	MaxTopK int `mapstructure:"maxTopK" json:"maxTopK" validate:"gte=1,lte=100" jsonschema:"description=Maximum topK a caller may request (default 50)"`

	// MaxContentChars caps stored content length in runes. Default 16384, max 32768.
	MaxContentChars int `mapstructure:"maxContentChars" json:"maxContentChars" validate:"gte=256,lte=32768" jsonschema:"description=Maximum characters per memory content (default 16384)"`

	// MaxQueryChars caps search query length in runes. Default 4096, max 16384.
	MaxQueryChars int `mapstructure:"maxQueryChars" json:"maxQueryChars" validate:"gte=64,lte=16384" jsonschema:"description=Maximum characters for a search query (default 4096)"`

	// RequestTimeout bounds store and LLM operations. Default 30s.
	RequestTimeout time.Duration `mapstructure:"requestTimeout" json:"requestTimeout" validate:"gte=1000000000" jsonschema:"description=Per-operation timeout (default '30s')"`

	// MinScore sets a global default minimum relevance score for search hits. Optional.
	MinScore *float64 `mapstructure:"minScore" json:"minScore,omitempty" validate:"omitempty,gte=0" jsonschema:"description=Optional minimum relevance score threshold"`

	// Programmatic injection hooks for Go embedders and unit tests (never set from YAML).
	Store     memoryagent.MemoryStore `mapstructure:"-" json:"-" jsonschema:"-" validate:"-"`
	Embedder  embedding.Embedder      `mapstructure:"-" json:"-" jsonschema:"-" validate:"-"`
	ChatModel model.BaseChatModel     `mapstructure:"-" json:"-" jsonschema:"-" validate:"-"`
}

// FileBackendConfig configures the per-tenant JSONL file store.
type FileBackendConfig struct {
	// Dir is the absolute host root directory under which per-tenant JSONL stores live.
	Dir string `mapstructure:"dir" json:"dir" validate:"required" jsonschema:"(required) Absolute host root directory for JSONL memory files"`
}

// OpenSearchBackendConfig configures the OpenSearch store.
type OpenSearchBackendConfig struct {
	URLs               []string `mapstructure:"urls" json:"urls" validate:"required,min=1,dive,required,url" jsonschema:"(required) OpenSearch cluster URLs"`
	Username           string   `mapstructure:"username" json:"username,omitempty" validate:"omitempty" jsonschema:"description=Basic auth username"`
	Password           string   `mapstructure:"-" json:"-" jsonschema:"-" validate:"omitempty"`
	TLSSkipVerify      bool     `mapstructure:"tlsSkipVerify" json:"tlsSkipVerify,omitempty" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
	IndexName          string   `mapstructure:"indexName" json:"indexName,omitempty" validate:"omitempty,max=255" jsonschema:"description=OpenSearch index name (default 'eino_agent_memory')"`
	VectorField        string   `mapstructure:"vectorField" json:"vectorField,omitempty" validate:"omitempty,max=128" jsonschema:"description=knn_vector field name (default 'vector')"`
	ContentField       string   `mapstructure:"contentField" json:"contentField,omitempty" validate:"omitempty,max=128" jsonschema:"description=BM25 text field name (default 'content')"`
	VectorDimension    int      `mapstructure:"vectorDimension" json:"vectorDimension,omitempty" validate:"omitempty,gte=1,lte=8192" jsonschema:"description=knn_vector dimension when creating index (default 1536)"`
	Hybrid             bool     `mapstructure:"hybrid" json:"hybrid,omitempty" validate:"omitempty" jsonschema:"description=Combine kNN vector search with BM25 text match"`
	K                  int      `mapstructure:"k" json:"k,omitempty" validate:"omitempty,gte=1,lte=500" jsonschema:"description=Nearest neighbors K for kNN queries"`
	BatchSize          int      `mapstructure:"batchSize" json:"batchSize,omitempty" validate:"omitempty,gte=1,lte=1000" jsonschema:"description=Bulk index batch size (default 100)"`
	SearchPipeline     string   `mapstructure:"searchPipeline" json:"searchPipeline,omitempty" validate:"omitempty,max=128" jsonschema:"description=Optional OpenSearch RRF/hybrid search pipeline"`
	Operator           string   `mapstructure:"operator" json:"operator,omitempty" validate:"omitempty,oneof=or and" jsonschema:"description=BM25 match operator: 'or' (default) or 'and'"`
	MinimumShouldMatch string   `mapstructure:"minimumShouldMatch" json:"minimumShouldMatch,omitempty" validate:"omitempty,max=128" jsonschema:"description=Optional BM25 minimum_should_match"`
}

// EmbeddingConfig configures an eino-ext chatmodel.NewEmbedding provider.
// Note: provider uses official lowercase kebab-case names ("openai", "github-copilot").
// Ollama or vLLM OpenAI-compatible endpoints use provider "openai" with BaseURL set.
type EmbeddingConfig struct {
	Provider      string        `mapstructure:"provider" json:"provider" validate:"required,oneof=openai github-copilot" jsonschema:"(required) Embedding provider: 'openai' or 'github-copilot'"`
	Model         string        `mapstructure:"model" json:"model" validate:"required,max=256" jsonschema:"(required) Embedding model ID (e.g. 'text-embedding-3-small')"`
	BaseURL       string        `mapstructure:"baseURL" json:"baseURL,omitempty" validate:"omitempty,url" jsonschema:"description=Optional provider endpoint URL (use for self-hosted/Ollama '/v1')"`
	APIKey        string        `mapstructure:"-" json:"-" jsonschema:"-" validate:"omitempty"`
	Timeout       time.Duration `mapstructure:"timeout" json:"timeout,omitempty" validate:"omitempty,gte=0" jsonschema:"description=Embedding request timeout (default '30s')"`
	TLSSkipVerify bool          `mapstructure:"tlsSkipVerify" json:"tlsSkipVerify,omitempty" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
}

// LLMConfig configures an eino-ext chatmodel.New chat model for discussion capture.
type LLMConfig struct {
	Provider        string        `mapstructure:"provider" json:"provider" validate:"required,oneof=ollama openai github-copilot" jsonschema:"(required) Chat model provider: 'ollama', 'openai', or 'github-copilot'"`
	Model           string        `mapstructure:"model" json:"model" validate:"required,max=256" jsonschema:"(required) Chat model ID"`
	BaseURL         string        `mapstructure:"baseURL" json:"baseURL,omitempty" validate:"omitempty,url" jsonschema:"description=Optional provider base URL"`
	APIKey          string        `mapstructure:"-" json:"-" jsonschema:"-" validate:"omitempty"`
	Temperature     float32       `mapstructure:"temperature" json:"temperature,omitempty" validate:"gte=0,lte=2" jsonschema:"description=Sampling temperature (0..2)"`
	Thinking        string        `mapstructure:"thinking" json:"thinking,omitempty" validate:"omitempty,oneof=off low medium high" jsonschema:"description=Reasoning effort: 'off', 'low', 'medium', or 'high'"`
	MaxOutputTokens int           `mapstructure:"maxOutputTokens" json:"maxOutputTokens,omitempty" validate:"gte=0,lte=32000" jsonschema:"description=Maximum output tokens (default 4096)"`
	Timeout         time.Duration `mapstructure:"timeout" json:"timeout,omitempty" validate:"omitempty,gte=0" jsonschema:"description=LLM request timeout (default '30s')"`
	TLSSkipVerify   bool          `mapstructure:"tlsSkipVerify" json:"tlsSkipVerify,omitempty" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
}

// applyDefaults fills every default from the configuration table. It must run
// before validate.Struct so validation sees the final values.
func applyDefaults(cfg *Config) {
	if cfg.Backend == "" {
		cfg.Backend = DefaultBackend
	}
	if cfg.Isolation == "" {
		cfg.Isolation = DefaultIsolation
	}
	if cfg.SharedNamespace == "" {
		cfg.SharedNamespace = DefaultSharedTenant
	}
	if cfg.DefaultTopK == 0 {
		cfg.DefaultTopK = DefaultTopK
	}
	if cfg.MaxTopK == 0 {
		cfg.MaxTopK = DefaultMaxTopK
	}
	if cfg.MaxContentChars == 0 {
		cfg.MaxContentChars = DefaultMaxContentChar
	}
	if cfg.MaxQueryChars == 0 {
		cfg.MaxQueryChars = DefaultMaxQueryChar
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.Prompts == nil {
		enabled := true
		cfg.Prompts = &enabled
	}
	if cfg.OpenSearch != nil {
		if cfg.OpenSearch.IndexName == "" {
			cfg.OpenSearch.IndexName = DefaultOpenSearchIdx
		}
		if cfg.OpenSearch.VectorField == "" {
			cfg.OpenSearch.VectorField = "vector"
		}
		if cfg.OpenSearch.ContentField == "" {
			cfg.OpenSearch.ContentField = "content"
		}
		if cfg.OpenSearch.VectorDimension == 0 {
			cfg.OpenSearch.VectorDimension = DefaultVectorDim
		}
		if cfg.OpenSearch.BatchSize == 0 {
			cfg.OpenSearch.BatchSize = DefaultBatchSize
		}
	}
	if cfg.Embedding != nil && cfg.Embedding.Timeout == 0 {
		cfg.Embedding.Timeout = DefaultRequestTimeout
	}
	if cfg.LLM != nil {
		if cfg.LLM.MaxOutputTokens == 0 {
			cfg.LLM.MaxOutputTokens = 4096
		}
		if cfg.LLM.Timeout == 0 {
			cfg.LLM.Timeout = DefaultRequestTimeout
		}
	}
}

// validateBackendConfig enforces the cross-field invariants that struct tags
// cannot express.
func validateBackendConfig(cfg *Config) error {
	if cfg.DefaultTopK > cfg.MaxTopK {
		return errors.Errorf("memory: defaultTopK (%d) must not exceed maxTopK (%d)", cfg.DefaultTopK, cfg.MaxTopK)
	}

	if cfg.Backend == BackendFile && cfg.Store == nil {
		if cfg.File == nil {
			return errors.New("memory: backend 'file' requires a file configuration block")
		}
		if err := validate.Struct(cfg.File); err != nil {
			return err
		}
		if err := fileutil.ValidateRootDir(cfg.File.Dir); err != nil {
			return errors.Wrap(err, "memory: invalid file.dir")
		}
	}

	if cfg.Backend == BackendOpenSearch && cfg.Store == nil {
		if cfg.OpenSearch == nil {
			return errors.New("memory: backend 'opensearch' requires an opensearch configuration block")
		}
		if err := validate.Struct(cfg.OpenSearch); err != nil {
			return err
		}
		if cfg.OpenSearch.Hybrid && cfg.Embedding == nil && cfg.Embedder == nil {
			return errors.New("memory: hybrid search requires embedding configuration")
		}
	}

	if cfg.Embedding != nil {
		if err := validate.Struct(cfg.Embedding); err != nil {
			return err
		}
		if cfg.Embedding.Provider == string(chatmodel.CopilotEmbeddingProvider) && cfg.Embedding.APIKey == "" {
			return errors.New("memory: embedding provider 'github-copilot' requires an API key")
		}
	}

	if cfg.LLM != nil {
		if err := validate.Struct(cfg.LLM); err != nil {
			return err
		}
		if _, err := chatmodel.ParseThinkingLevel(cfg.LLM.Thinking); err != nil {
			return errors.Wrap(err, "memory: invalid llm.thinking")
		}
		if cfg.LLM.Provider == string(chatmodel.CopilotProvider) && cfg.LLM.APIKey == "" {
			return errors.New("memory: llm provider 'github-copilot' requires an API key")
		}
	}

	return nil
}
