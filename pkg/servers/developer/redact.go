package developer

import (
	"regexp"
	"strings"
)

// redactedMarker replaces every detected secret.
const redactedMarker = "***REDACTED***"

// secretPatterns are the credential shapes redacted from command output.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),
	regexp.MustCompile(`gh[ousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
}

// redactor replaces configured literal secrets and credential-shaped patterns.
type redactor struct {
	literals []string
}

// newRedactor builds a redactor from the configured GitHub and Dagger Cloud
// tokens. Empty literals are ignored.
func newRedactor(cfg *Config) *redactor {
	var literals []string
	if cfg != nil {
		if cfg.Engine.CloudToken != "" {
			literals = append(literals, cfg.Engine.CloudToken)
		}
		if cfg.GitHub != nil {
			for _, instance := range cfg.GitHub.Instances {
				if instance.Token != "" {
					literals = append(literals, instance.Token)
				}
			}
		}
	}
	return &redactor{literals: literals}
}

// redact applies literal replacement first, then the pattern list.
func (r *redactor) redact(s string) string {
	if r == nil {
		return s
	}
	return redactSecrets(s, r.literals)
}

// redactSecrets replaces every literal secret and credential-shaped pattern.
func redactSecrets(s string, literals []string) string {
	for _, literal := range literals {
		if literal == "" {
			continue
		}
		s = strings.ReplaceAll(s, literal, redactedMarker)
	}
	for _, pattern := range secretPatterns {
		s = pattern.ReplaceAllString(s, redactedMarker)
	}
	return s
}
