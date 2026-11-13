package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactSecrets(t *testing.T) {
	redactor := newRedactor(&Config{
		OpenSearch: &OpenSearchBackendConfig{Password: "opensearch-literal"},
		Embedding:  &EmbeddingConfig{APIKey: "embedding-literal"},
		LLM:        &LLMConfig{APIKey: "llm-literal"},
	})

	tests := []struct {
		name    string
		input   string
		changed bool
		absent  string
	}{
		{name: "literal opensearch", input: "password is opensearch-literal", changed: true, absent: "opensearch-literal"},
		{name: "literal embedding", input: "key=embedding-literal", changed: true, absent: "embedding-literal"},
		{name: "literal llm", input: "token llm-literal", changed: true, absent: "llm-literal"},
		{name: "github pat", input: "ghp_012345678901234567890123456789012345", changed: true, absent: "ghp_012345678901234567890123456789012345"},
		{name: "github fine grained", input: "github_pat_0123456789012345678901", changed: true, absent: "github_pat_0123456789012345678901"},
		{name: "openai key", input: "sk-012345678901234567890123", changed: true, absent: "sk-012345678901234567890123"},
		{name: "aws key", input: "AKIA0123456789ABCDEF", changed: true, absent: "AKIA0123456789ABCDEF"},
		{name: "bearer", input: "Authorization: Bearer abcdef123456", changed: true, absent: "abcdef123456"},
		{name: "password kv", input: "password=hunter2", changed: true, absent: "hunter2"},
		{name: "pem", input: "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----", changed: true, absent: "MIIE"},
		{name: "jwt", input: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop", changed: true, absent: "eyJhbGciOiJIUzI1NiJ9"},
		{name: "sasl jaas", input: "sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required;", changed: true, absent: "PlainLoginModule"},
		{name: "clean", input: "the sky is blue", changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := redactor.redact(tt.input)
			assert.Equal(t, tt.changed, changed)
			if tt.absent != "" {
				assert.NotContains(t, got, tt.absent)
			}
			if !tt.changed {
				assert.Equal(t, tt.input, got)
			}
		})
	}
}

func TestRedactRecord(t *testing.T) {
	redactor := newRedactor(&Config{LLM: &LLMConfig{APIKey: "secret-token"}})
	rec := &Record{
		Title:    "token secret-token",
		Content:  "password=hunter2",
		Scope:    "secret-token",
		Tags:     []string{"secret-token"},
		Metadata: map[string]string{"k": "secret-token"},
	}
	assert.True(t, redactor.redactRecord(rec))
	assert.NotContains(t, rec.Title, "secret-token")
	assert.NotContains(t, rec.Content, "hunter2")
	assert.NotContains(t, rec.Scope, "secret-token")
	assert.NotContains(t, rec.Tags[0], "secret-token")
	assert.NotContains(t, rec.Metadata["k"], "secret-token")
}
