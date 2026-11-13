package memory

import (
	"regexp"
	"strings"
)

// redactedMarker replaces every detected secret.
const redactedMarker = "***REDACTED***"

// secretPattern is one credential shape and the replacement template that
// preserves any non-secret prefix captured by the pattern.
type secretPattern struct {
	re          *regexp.Regexp
	replacement string
}

// secretPatterns are the credential shapes redacted from memory content.
var secretPatterns = []secretPattern{
	{re: regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`gh[ousr]_[A-Za-z0-9]{36,}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer\s+)[^\s]+`), replacement: "${1}" + redactedMarker},
	{re: regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api_?key)(\s*[:=]\s*)([^\s,;"']+)`), replacement: "${1}${2}" + redactedMarker},
	{re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), replacement: redactedMarker},
	{re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), replacement: redactedMarker},
	{re: regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), replacement: redactedMarker},
	{re: regexp.MustCompile(`(?i)(sasl\.jaas\.config\s*=\s*)[^\n;]+`), replacement: "${1}" + redactedMarker},
}

// redactor replaces configured literal secrets and credential-shaped patterns.
type redactor struct {
	literals []string
}

// newRedactor builds a redactor from the configured OpenSearch, embedding and
// LLM secrets. Empty literals are ignored.
func newRedactor(cfg *Config) *redactor {
	var literals []string
	if cfg != nil {
		if cfg.OpenSearch != nil && cfg.OpenSearch.Password != "" {
			literals = append(literals, cfg.OpenSearch.Password)
		}
		if cfg.Embedding != nil && cfg.Embedding.APIKey != "" {
			literals = append(literals, cfg.Embedding.APIKey)
		}
		if cfg.LLM != nil && cfg.LLM.APIKey != "" {
			literals = append(literals, cfg.LLM.APIKey)
		}
	}
	return &redactor{literals: literals}
}

// redact applies literal replacement first, then the pattern list. It reports
// whether the input changed.
func (r *redactor) redact(s string) (string, bool) {
	if r == nil {
		return s, false
	}
	return redactSecrets(s, r.literals)
}

// redactRecord scrubs every free-text field of a record in place.
func (r *redactor) redactRecord(rec *Record) bool {
	if r == nil || rec == nil {
		return false
	}
	changed := false
	if v, ok := r.redact(rec.Content); ok {
		rec.Content = v
		changed = true
	}
	if v, ok := r.redact(rec.Title); ok {
		rec.Title = v
		changed = true
	}
	if v, ok := r.redact(rec.Scope); ok {
		rec.Scope = v
		changed = true
	}
	for i, tag := range rec.Tags {
		if v, ok := r.redact(tag); ok {
			rec.Tags[i] = v
			changed = true
		}
	}
	for k, value := range rec.Metadata {
		if v, ok := r.redact(value); ok {
			rec.Metadata[k] = v
			changed = true
		}
	}
	return changed
}

// redactSecrets replaces every literal secret and credential-shaped pattern.
func redactSecrets(s string, literals []string) (string, bool) {
	original := s
	for _, literal := range literals {
		if literal == "" {
			continue
		}
		s = strings.ReplaceAll(s, literal, redactedMarker)
	}
	for _, pattern := range secretPatterns {
		s = pattern.re.ReplaceAllString(s, pattern.replacement)
	}
	return s, s != original
}
