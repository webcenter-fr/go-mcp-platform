package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerDefaults(t *testing.T) {
	srv, err := NewServer(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestNewServerConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name: "valid file config",
			cfg:  &Config{Backend: BackendFile, File: &FileBackendConfig{Dir: t.TempDir()}},
		},
		{
			name:    "relative file dir",
			cfg:     &Config{Backend: BackendFile, File: &FileBackendConfig{Dir: "relative/path"}},
			wantErr: "absolute",
		},
		{
			name:    "root file dir",
			cfg:     &Config{Backend: BackendFile, File: &FileBackendConfig{Dir: "/"}},
			wantErr: "system directory",
		},
		{
			name:    "file backend without block",
			cfg:     &Config{Backend: BackendFile},
			wantErr: "file configuration",
		},
		{
			name:    "opensearch without urls",
			cfg:     &Config{Backend: BackendOpenSearch, OpenSearch: &OpenSearchBackendConfig{}},
			wantErr: "urls",
		},
		{
			name: "opensearch hybrid without embedding",
			cfg: &Config{Backend: BackendOpenSearch, OpenSearch: &OpenSearchBackendConfig{
				URLs:   []string{"http://localhost:9200"},
				Hybrid: true,
			}},
			wantErr: "hybrid search requires embedding",
		},
		{
			name:    "default topk exceeds max topk",
			cfg:     &Config{DefaultTopK: 10, MaxTopK: 5},
			wantErr: "defaultTopK",
		},
		{
			name: "embedding unsupported provider",
			cfg: &Config{Embedding: &EmbeddingConfig{
				Provider: "ollama",
				Model:    "nomic-embed-text",
			}},
			wantErr: "provider",
		},
		{
			name: "github-copilot embedding without api key",
			cfg: &Config{Embedding: &EmbeddingConfig{
				Provider: "github-copilot",
				Model:    "text-embedding-3-small",
			}},
			wantErr: "API key",
		},
		{
			name: "invalid llm thinking",
			cfg: &Config{LLM: &LLMConfig{
				Provider: "openai",
				Model:    "gpt-4o-mini",
				Thinking: "extreme",
			}},
			wantErr: "thinking",
		},
		{
			name: "github-copilot llm without api key",
			cfg: &Config{LLM: &LLMConfig{
				Provider: "github-copilot",
				Model:    "gpt-4o",
			}},
			wantErr: "API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(context.Background(), tt.cfg, nil)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	assert.Equal(t, BackendMemory, cfg.Backend)
	assert.Equal(t, IsolationPerIdentity, cfg.Isolation)
	assert.Equal(t, DefaultSharedTenant, cfg.SharedNamespace)
	assert.Equal(t, DefaultTopK, cfg.DefaultTopK)
	assert.Equal(t, DefaultMaxTopK, cfg.MaxTopK)
	assert.Equal(t, DefaultMaxContentChar, cfg.MaxContentChars)
	assert.Equal(t, DefaultMaxQueryChar, cfg.MaxQueryChars)
	assert.Equal(t, DefaultRequestTimeout, cfg.RequestTimeout)
	require.NotNil(t, cfg.Prompts)
	assert.True(t, *cfg.Prompts)
}

func TestApplyDefaultsOpenSearchAndLLM(t *testing.T) {
	cfg := &Config{
		OpenSearch: &OpenSearchBackendConfig{URLs: []string{"http://localhost:9200"}},
		LLM:        &LLMConfig{Provider: "openai", Model: "gpt-4o-mini"},
		Embedding:  &EmbeddingConfig{Provider: "openai", Model: "text-embedding-3-small"},
	}
	applyDefaults(cfg)
	assert.Equal(t, DefaultOpenSearchIdx, cfg.OpenSearch.IndexName)
	assert.Equal(t, "vector", cfg.OpenSearch.VectorField)
	assert.Equal(t, "content", cfg.OpenSearch.ContentField)
	assert.Equal(t, DefaultVectorDim, cfg.OpenSearch.VectorDimension)
	assert.Equal(t, DefaultBatchSize, cfg.OpenSearch.BatchSize)
	assert.Equal(t, 4096, cfg.LLM.MaxOutputTokens)
	assert.Equal(t, DefaultRequestTimeout, cfg.LLM.Timeout)
	assert.Equal(t, DefaultRequestTimeout, cfg.Embedding.Timeout)
}
