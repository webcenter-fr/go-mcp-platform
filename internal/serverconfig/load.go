package serverconfig

import (
	"strings"

	"emperror.dev/errors"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
	"github.com/webcenter-fr/go-mcp-platform/internal/secretref"
	developermcp "github.com/webcenter-fr/go-mcp-platform/pkg/servers/developer"
)

// Default values applied before validation.
const (
	DefaultTransport = "stdio"
	DefaultAddr      = ":8080"
	DefaultOpsAddr   = ":9090"
	DefaultLogLevel  = "info"
	DefaultLogFormat = "text"
	DefaultTimeout   = "30s"
)

// DefaultDisallowedNamespaces are the namespaces the Kubernetes tools refuse
// to touch unless the configuration overrides them.
var DefaultDisallowedNamespaces = []string{"kube-system", "kube-public", "kube-node-lease"}

// Load reads the YAML configuration at path, applies environment variables and
// CLI overrides, resolves secret references, and validates the result.
//
// Precedence is flags > environment > file > defaults. An empty path means
// "no config file"; a non-empty path that cannot be read is an error.
func Load(path string, overrides map[string]any) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("MCP_PLATFORM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, key := range []string{
		"server", "transport", "addr", "opsAddr",
		"log.level", "log.format",
		"approval.timeout",
		"approval.service.url", "approval.service.serverName",
		"approval.service.receiptAlgorithm", "approval.service.pollInterval",
		"approval.service.requestTimeout", "approval.service.ttl",
		"auth.resourceMetadataURL",
		"developer.engine.mode", "developer.engine.runnerHost",
		"developer.engine.cloudURL", "developer.engine.engineVersion",
		"developer.workspaceRoot",
		"memory.backend", "memory.isolation", "memory.file.dir",
		"memory.opensearch.indexName",
		"memory.embedding.provider", "memory.embedding.model", "memory.embedding.baseURL",
		"memory.llm.provider", "memory.llm.model", "memory.llm.baseURL",
	} {
		_ = v.BindEnv(key)
	}

	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, errors.Wrapf(err, "failed to read MCP config file %q", path)
		}
	}

	for key, value := range overrides {
		v.Set(key, value)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg, func(dc *mapstructure.DecoderConfig) {
		dc.ErrorUnused = true
	}); err != nil {
		return nil, errors.Wrap(err, "failed to decode MCP config")
	}

	applyDefaults(cfg)
	if err := resolveSecrets(cfg); err != nil {
		return nil, err
	}
	if err := validate.Struct(cfg); err != nil {
		return nil, err
	}
	if err := validateFamily(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validateFamily rejects a selected family that is absent or empty. Viper
// drops empty maps, so this is checked on the decoded config rather than with
// a struct tag.
func validateFamily(cfg *Config) error {
	switch cfg.Server {
	case "kubernetes":
		if cfg.Kubernetes == nil || len(cfg.Kubernetes.Clusters) == 0 {
			return errors.New("serverconfig: kubernetes family requires at least one cluster")
		}
	case "argocd":
		if cfg.ArgoCD == nil || len(cfg.ArgoCD.Instances) == 0 {
			return errors.New("serverconfig: argocd family requires at least one instance")
		}
	case "prometheus":
		if cfg.Prometheus == nil || len(cfg.Prometheus.Instances) == 0 {
			return errors.New("serverconfig: prometheus family requires at least one instance")
		}
	case "grafana":
		if cfg.Grafana == nil || len(cfg.Grafana.Instances) == 0 {
			return errors.New("serverconfig: grafana family requires at least one instance")
		}
	case "developer":
		if cfg.Developer == nil {
			return errors.New("serverconfig: developer family is not configured")
		}
		if cfg.Developer.WorkspaceRoot == "" {
			return errors.New("serverconfig: developer family requires workspaceRoot")
		}
		if cfg.Developer.GitHub != nil && len(cfg.Developer.GitHub.Instances) == 0 {
			return errors.New("serverconfig: developer github requires at least one instance")
		}
	case "memory":
		if cfg.Memory == nil {
			return errors.New("serverconfig: memory family is not configured")
		}
		switch cfg.Memory.Backend {
		case "file":
			if cfg.Memory.File == nil || cfg.Memory.File.Dir == "" {
				return errors.New("serverconfig: memory backend 'file' requires file.dir")
			}
		case "opensearch":
			if cfg.Memory.OpenSearch == nil || len(cfg.Memory.OpenSearch.URLs) == 0 {
				return errors.New("serverconfig: memory backend 'opensearch' requires opensearch.urls")
			}
		}
		if _, err := parseOptionalDuration(cfg.Memory.RequestTimeout); err != nil {
			return errors.Wrap(err, "serverconfig: memory requestTimeout")
		}
		if cfg.Memory.Embedding != nil {
			if _, err := parseOptionalDuration(cfg.Memory.Embedding.Timeout); err != nil {
				return errors.Wrap(err, "serverconfig: memory embedding timeout")
			}
		}
		if cfg.Memory.LLM != nil {
			if _, err := parseOptionalDuration(cfg.Memory.LLM.Timeout); err != nil {
				return errors.Wrap(err, "serverconfig: memory llm timeout")
			}
		}
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("transport", DefaultTransport)
	v.SetDefault("addr", DefaultAddr)
	v.SetDefault("opsAddr", DefaultOpsAddr)
	v.SetDefault("log.level", DefaultLogLevel)
	v.SetDefault("log.format", DefaultLogFormat)
}

func applyDefaults(cfg *Config) {
	if cfg.Transport == "" {
		cfg.Transport = DefaultTransport
	}
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}
	if cfg.OpsAddr == "" {
		cfg.OpsAddr = DefaultOpsAddr
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = DefaultLogLevel
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = DefaultLogFormat
	}
	if cfg.Kubernetes != nil {
		for name, cluster := range cfg.Kubernetes.Clusters {
			if len(cluster.DisallowedNamespaces) == 0 {
				cluster.DisallowedNamespaces = append([]string(nil), DefaultDisallowedNamespaces...)
			}
			if cluster.DefaultTimeout == "" {
				cluster.DefaultTimeout = DefaultTimeout
			}
			cfg.Kubernetes.Clusters[name] = cluster
		}
	}
	for _, family := range []*InstanceFamily{cfg.ArgoCD, cfg.Prometheus, cfg.Grafana} {
		if family == nil {
			continue
		}
		for name, instance := range family.Instances {
			if instance.DefaultTimeout == "" {
				instance.DefaultTimeout = DefaultTimeout
			}
			family.Instances[name] = instance
		}
	}
	applyDeveloperDefaults(cfg.Developer)
	applyMemoryDefaults(cfg.Memory)
}

// applyMemoryDefaults mirrors the memory constructor defaults so the loaded
// configuration is validated against the same effective values.
func applyMemoryDefaults(cfg *MemoryFamily) {
	if cfg == nil {
		return
	}
	if cfg.Backend == "" {
		cfg.Backend = "memory"
	}
	if cfg.Isolation == "" {
		cfg.Isolation = "per_identity"
	}
	if cfg.SharedNamespace == "" {
		cfg.SharedNamespace = "shared"
	}
	if cfg.DefaultTopK == 0 {
		cfg.DefaultTopK = 5
	}
	if cfg.MaxTopK == 0 {
		cfg.MaxTopK = 50
	}
	if cfg.MaxContentChars == 0 {
		cfg.MaxContentChars = 16384
	}
	if cfg.MaxQueryChars == 0 {
		cfg.MaxQueryChars = 4096
	}
	if cfg.RequestTimeout == "" {
		cfg.RequestTimeout = "30s"
	}
	if cfg.Prompts == nil {
		enabled := true
		cfg.Prompts = &enabled
	}
	if cfg.OpenSearch != nil {
		if cfg.OpenSearch.IndexName == "" {
			cfg.OpenSearch.IndexName = "eino_agent_memory"
		}
		if cfg.OpenSearch.VectorField == "" {
			cfg.OpenSearch.VectorField = "vector"
		}
		if cfg.OpenSearch.ContentField == "" {
			cfg.OpenSearch.ContentField = "content"
		}
		if cfg.OpenSearch.VectorDimension == 0 {
			cfg.OpenSearch.VectorDimension = 1536
		}
		if cfg.OpenSearch.BatchSize == 0 {
			cfg.OpenSearch.BatchSize = 100
		}
	}
	if cfg.Embedding != nil && cfg.Embedding.Timeout == "" {
		cfg.Embedding.Timeout = "30s"
	}
	if cfg.LLM != nil {
		if cfg.LLM.MaxOutputTokens == 0 {
			cfg.LLM.MaxOutputTokens = 4096
		}
		if cfg.LLM.Timeout == "" {
			cfg.LLM.Timeout = "30s"
		}
	}
}

// applyDeveloperDefaults mirrors the developer constructor defaults so the
// loaded configuration is validated against the same effective values.
func applyDeveloperDefaults(cfg *DeveloperFamily) {
	if cfg == nil {
		return
	}
	if cfg.BaseImage == "" {
		cfg.BaseImage = "alpine:3.20"
	}
	if cfg.DefaultTimeout == "" {
		cfg.DefaultTimeout = "5m"
	}
	if cfg.MaxTimeout == "" {
		cfg.MaxTimeout = "15m"
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 256000
	}
	if cfg.MaxConcurrentExecs == 0 {
		cfg.MaxConcurrentExecs = 4
	}
	if cfg.ContainerUser == "" {
		cfg.ContainerUser = "1000:1000"
	}
	if cfg.WorkspaceTTL == "" {
		cfg.WorkspaceTTL = "24h"
	}
	if cfg.WorkspaceGCInterval == "" {
		cfg.WorkspaceGCInterval = "1h"
	}
	if cfg.Scaffolding == nil {
		enabled := true
		cfg.Scaffolding = &enabled
	}
	if cfg.Egress == nil {
		cfg.Egress = &DeveloperEgress{}
	}
	if cfg.Egress.DefaultDeny == nil {
		deny := true
		cfg.Egress.DefaultDeny = &deny
	}
	if len(cfg.Egress.AllowHosts) == 0 {
		cfg.Egress.AllowHosts = append([]string(nil), developermcp.DefaultEgressAllowHosts...)
	}
	if cfg.Engine.CLIPath == "" {
		cfg.Engine.CLIPath = "/usr/local/bin/dagger"
	}
	if cfg.Engine.Mode == "dagger-kubernetes" && cfg.Engine.RunnerHost == "" {
		cfg.Engine.RunnerHost = "dagger-cloud://self"
	}
	if cfg.GitHub != nil {
		for name, instance := range cfg.GitHub.Instances {
			if instance.Timeout == "" {
				instance.Timeout = "30s"
			}
			cfg.GitHub.Instances[name] = instance
		}
	}
}

func resolveSecrets(cfg *Config) error {
	for _, family := range []*InstanceFamily{cfg.ArgoCD, cfg.Prometheus, cfg.Grafana} {
		if family == nil {
			continue
		}
		for name, instance := range family.Instances {
			token, err := secretref.Resolve(instance.TokenFile, instance.TokenEnv, "", "instance "+name+" token")
			if err != nil {
				return err
			}
			instance.Token = token
			family.Instances[name] = instance
		}
	}

	if cfg.Auth != nil {
		for i := range cfg.Auth.Providers {
			provider := &cfg.Auth.Providers[i]
			if provider.Local == nil {
				continue
			}
			for j := range provider.Local.Tokens {
				token := &provider.Local.Tokens[j]
				value, err := secretref.Resolve(token.TokenFile, token.TokenEnv, "", "local auth token")
				if err != nil {
					return err
				}
				token.Token = value
			}
		}
	}

	if cfg.Developer != nil {
		if cfg.Developer.Engine.CloudTokenFile != "" || cfg.Developer.Engine.CloudTokenEnv != "" {
			token, err := secretref.Resolve(cfg.Developer.Engine.CloudTokenFile, cfg.Developer.Engine.CloudTokenEnv, "", "developer Dagger Cloud token")
			if err != nil {
				return err
			}
			cfg.Developer.Engine.CloudToken = token
		}
		for host, entry := range cfg.Developer.Engine.RegistryAuth {
			password, err := secretref.Resolve(entry.PasswordFile, entry.PasswordEnv, "", "developer registry password for "+host)
			if err != nil {
				return err
			}
			entry.Password = password
			cfg.Developer.Engine.RegistryAuth[host] = entry
		}
		if cfg.Developer.GitHub != nil {
			for name, instance := range cfg.Developer.GitHub.Instances {
				token, err := secretref.Resolve(instance.TokenFile, instance.TokenEnv, "", "developer GitHub token for "+name)
				if err != nil {
					return err
				}
				instance.Token = token
				cfg.Developer.GitHub.Instances[name] = instance
			}
		}
	}

	if cfg.Memory != nil {
		if cfg.Memory.OpenSearch != nil {
			password, err := secretref.Resolve(cfg.Memory.OpenSearch.PasswordFile, cfg.Memory.OpenSearch.PasswordEnv, "", "memory OpenSearch password")
			if err != nil {
				return err
			}
			cfg.Memory.OpenSearch.Password = password
		}
		if cfg.Memory.Embedding != nil {
			apiKey, err := secretref.Resolve(cfg.Memory.Embedding.APIKeyFile, cfg.Memory.Embedding.APIKeyEnv, "", "memory embedding API key")
			if err != nil {
				return err
			}
			cfg.Memory.Embedding.APIKey = apiKey
		}
		if cfg.Memory.LLM != nil {
			apiKey, err := secretref.Resolve(cfg.Memory.LLM.APIKeyFile, cfg.Memory.LLM.APIKeyEnv, "", "memory LLM API key")
			if err != nil {
				return err
			}
			cfg.Memory.LLM.APIKey = apiKey
		}
	}

	if cfg.Approval != nil && cfg.Approval.Service != nil {
		service := cfg.Approval.Service
		serverToken, err := secretref.Resolve(service.ServerTokenFile, service.ServerTokenEnv, "", "approval server token")
		if err != nil {
			return err
		}
		service.ServerToken = serverToken

		receiptSecret, err := secretref.Resolve(service.ReceiptSecretFile, service.ReceiptSecretEnv, "", "approval receipt secret")
		if err != nil {
			return err
		}
		service.ReceiptSecret = receiptSecret

		privateKey, err := secretref.Resolve(service.ReceiptPrivateKeyFile, service.ReceiptPrivateKeyEnv, "", "approval receipt private key")
		if err != nil {
			return err
		}
		service.ReceiptPrivateKeyPEM = privateKey
	}
	return nil
}
