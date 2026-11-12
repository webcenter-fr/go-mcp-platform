package developer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		literals []string
		want     string
	}{
		{name: "literal github token", input: "token=ghp_literal", literals: []string{"ghp_literal"}, want: "token=" + redactedMarker},
		{name: "literal dagger token", input: "DAGGER_CLOUD_TOKEN=dag_literal", literals: []string{"dag_literal"}, want: "DAGGER_CLOUD_TOKEN=" + redactedMarker},
		{name: "github pat pattern", input: "ghp_012345678901234567890123456789012345", want: redactedMarker},
		{name: "github oauth pattern", input: "gho_012345678901234567890123456789012345", want: redactedMarker},
		{name: "github fine-grained pat", input: "github_pat_0123456789012345678901", want: redactedMarker},
		{name: "aws access key", input: "AKIA0123456789ABCDEF", want: redactedMarker},
		{name: "private key header", input: "-----BEGIN RSA PRIVATE KEY-----", want: redactedMarker},
		{name: "jwt", input: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop", want: redactedMarker},
		{name: "no secret", input: "hello world", want: "hello world"},
		{name: "overlapping literal and pattern", input: "ghp_012345678901234567890123456789012345", literals: []string{"ghp_012345678901234567890123456789012345"}, want: redactedMarker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactSecrets(tt.input, tt.literals))
		})
	}
}

func TestNewRedactorCollectsTokens(t *testing.T) {
	cfg := &Config{
		Engine: EngineConfig{CloudToken: "cloud-secret"},
		GitHub: &GitHubConfig{Instances: map[string]GitHubInstance{
			"corp": {Token: "github-secret"},
		}},
	}
	r := newRedactor(cfg)
	assert.Equal(t, redactedMarker, r.redact("cloud-secret"))
	assert.Equal(t, redactedMarker, r.redact("github-secret"))
}
