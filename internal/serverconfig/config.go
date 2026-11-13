// Package serverconfig loads the YAML configuration of the deployable MCP
// server, resolves file/environment secret references, and wires the selected
// tool family into a pkg/mcp server.
package serverconfig

import (
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// Config is the top-level MCP server configuration.
type Config struct {
	// Server selects the tool family to expose.
	Server string `mapstructure:"server" validate:"required,oneof=kubernetes argocd prometheus grafana developer memory" jsonschema:"(required) Tool family: kubernetes, argocd, prometheus, grafana, developer or memory"`
	// Transport is the MCP transport: stdio or http.
	Transport string `mapstructure:"transport" validate:"omitempty,oneof=stdio http" jsonschema:"description=MCP transport: 'stdio' or 'http' (default 'stdio')"`
	// Addr is the HTTP listen address for the MCP transport.
	Addr string `mapstructure:"addr" validate:"omitempty" jsonschema:"description=HTTP listen address for the MCP transport (default ':8080')"`
	// OpsAddr is the unauthenticated operations listen address (/healthz,
	// /readyz, /metrics). It is never exposed through a Service.
	OpsAddr string `mapstructure:"opsAddr" validate:"omitempty" jsonschema:"description=Operations listen address for /healthz, /readyz and /metrics (default ':9090')"`
	// Log configures the process logger.
	Log LogConfig `mapstructure:"log" validate:"omitempty" jsonschema:"description=Process logging"`
	// Kubernetes configures the Kubernetes family.
	Kubernetes *KubernetesFamily `mapstructure:"kubernetes" validate:"omitempty" jsonschema:"description=Kubernetes family configuration"`
	// ArgoCD configures the ArgoCD family.
	ArgoCD *InstanceFamily `mapstructure:"argocd" validate:"omitempty" jsonschema:"description=ArgoCD family configuration"`
	// Prometheus configures the Prometheus family.
	Prometheus *InstanceFamily `mapstructure:"prometheus" validate:"omitempty" jsonschema:"description=Prometheus family configuration"`
	// Grafana configures the Grafana family.
	Grafana *InstanceFamily `mapstructure:"grafana" validate:"omitempty" jsonschema:"description=Grafana family configuration"`
	// Developer configures the developer family.
	Developer *DeveloperFamily `mapstructure:"developer" validate:"omitempty" jsonschema:"description=Developer family configuration"`
	// Memory configures the memory family.
	Memory *MemoryFamily `mapstructure:"memory" validate:"omitempty" jsonschema:"description=Memory family configuration"`
	// Auth configures bearer-token authentication for the HTTP transport.
	Auth *AuthConfig `mapstructure:"auth" validate:"omitempty" jsonschema:"description=Bearer-token authentication for the HTTP transport"`
	// Authz configures RBAC.
	Authz *mcpserver.AuthzConfig `mapstructure:"authz" validate:"omitempty" jsonschema:"description=RBAC policy"`
	// Approval configures write-tool approval.
	Approval *ApprovalConfig `mapstructure:"approval" validate:"omitempty" jsonschema:"description=Write-tool approval settings"`
}

// LogConfig configures the process logger.
type LogConfig struct {
	// Level is a logrus level name.
	Level string `mapstructure:"level" validate:"omitempty,oneof=trace debug info warn warning error fatal panic" jsonschema:"description=Log level (default 'info')"`
	// Format is text or json.
	Format string `mapstructure:"format" validate:"omitempty,oneof=text json" jsonschema:"description=Log format: 'text' or 'json' (default 'text')"`
}

// KubernetesFamily configures the Kubernetes tool family.
type KubernetesFamily struct {
	// Clusters maps a cluster name to its connection settings.
	Clusters map[string]ClusterEntry `mapstructure:"clusters" validate:"required,min=1,dive" jsonschema:"(required) Clusters keyed by name"`
}

// ClusterEntry configures one Kubernetes cluster.
type ClusterEntry struct {
	// InCluster uses the pod service account.
	InCluster bool `mapstructure:"inCluster" jsonschema:"description=Use the in-cluster service account"`
	// KubeconfigFile is the path to a kubeconfig file (required unless InCluster).
	KubeconfigFile string `mapstructure:"kubeconfigFile" validate:"required_without=InCluster,omitempty,filepath" jsonschema:"description=Kubeconfig file path (required unless inCluster)"`
	// Context selects a kubeconfig context.
	Context string `mapstructure:"context" jsonschema:"description=Kubeconfig context name"`
	// DisallowedNamespaces are namespaces the tools refuse to touch.
	DisallowedNamespaces []string `mapstructure:"disallowedNamespaces" jsonschema:"description=Namespaces the tools refuse to touch (default kube-system, kube-public, kube-node-lease)"`
	// DefaultTimeout is the default operation timeout (Go duration).
	DefaultTimeout string `mapstructure:"defaultTimeout" validate:"omitempty" jsonschema:"description=Default operation timeout (Go duration, e.g. '30s')"`
}

// InstanceFamily configures a URL/token based tool family.
type InstanceFamily struct {
	// Instances maps an instance name to its connection settings.
	Instances map[string]InstanceEntry `mapstructure:"instances" validate:"required,min=1,dive" jsonschema:"(required) Instances keyed by name"`
}

// InstanceEntry configures one URL/token based instance.
type InstanceEntry struct {
	// URL is the instance base URL.
	URL string `mapstructure:"url" validate:"required,url" jsonschema:"(required) Instance base URL"`
	// TokenFile is a file containing the bearer token.
	TokenFile string `mapstructure:"tokenFile" validate:"required_without=TokenEnv" jsonschema:"description=File containing the bearer token (required unless tokenEnv)"`
	// TokenEnv is the environment variable containing the bearer token.
	TokenEnv string `mapstructure:"tokenEnv" jsonschema:"description=Environment variable containing the bearer token"`
	// Token is the resolved bearer token. Never set from YAML.
	Token string `mapstructure:"-" validate:"required" jsonschema:"-"`
	// TLSSkipVerify disables TLS certificate verification.
	TLSSkipVerify bool `mapstructure:"tlsSkipVerify" jsonschema:"description=Disable TLS certificate verification"`
	// DefaultTimeout is the default request timeout (Go duration).
	DefaultTimeout string `mapstructure:"defaultTimeout" validate:"omitempty" jsonschema:"description=Default request timeout (Go duration, e.g. '30s')"`
}

// AuthConfig mirrors mcpserver.AuthConfig with secret references.
type AuthConfig struct {
	// Providers verify bearer tokens, tried in order.
	Providers []ProviderConfig `mapstructure:"providers" validate:"required,min=1,dive" jsonschema:"(required) Bearer-token providers, tried in order"`
	// ResourceMetadataURL is returned in the WWW-Authenticate header.
	ResourceMetadataURL string `mapstructure:"resourceMetadataURL" validate:"omitempty,url" jsonschema:"description=RFC 9728 resource metadata URL"`
	// RequiredScopes are required on the access token.
	RequiredScopes []string `mapstructure:"requiredScopes" jsonschema:"description=Scopes required on the access token"`
}

// ProviderConfig is a single auth provider.
type ProviderConfig struct {
	// Type is local or oidc.
	Type string `mapstructure:"type" validate:"required,oneof=local oidc" jsonschema:"(required) Provider type: 'local' or 'oidc'"`
	// Local holds local-provider settings.
	Local *LocalProviderConfig `mapstructure:"local" validate:"omitempty" jsonschema:"description=Local provider settings"`
	// OIDC holds OIDC-provider settings.
	OIDC *OIDCProviderConfig `mapstructure:"oidc" validate:"omitempty" jsonschema:"description=OIDC provider settings"`
}

// LocalProviderConfig configures the static-token provider.
type LocalProviderConfig struct {
	// Tokens maps bearer tokens to identities.
	Tokens []LocalToken `mapstructure:"tokens" validate:"required,min=1,dive" jsonschema:"(required) Static bearer tokens mapped to identities"`
}

// LocalToken maps a static bearer token to an identity.
type LocalToken struct {
	// TokenFile is a file containing the bearer token.
	TokenFile string `mapstructure:"tokenFile" validate:"required_without=TokenEnv" jsonschema:"description=File containing the bearer token (required unless tokenEnv)"`
	// TokenEnv is the environment variable containing the bearer token.
	TokenEnv string `mapstructure:"tokenEnv" jsonschema:"description=Environment variable containing the bearer token"`
	// Token is the resolved bearer token. Never set from YAML.
	Token string `mapstructure:"-" validate:"required" jsonschema:"-"`
	// User is the user name for this token.
	User string `mapstructure:"user" validate:"required" jsonschema:"(required) User name for this token"`
	// Groups are the groups for this token.
	Groups []string `mapstructure:"groups" jsonschema:"description=Groups for this token"`
	// Scopes are the scopes granted to this token.
	Scopes []string `mapstructure:"scopes" jsonschema:"description=Scopes granted to this token"`
}

// OIDCProviderConfig configures the OIDC provider.
type OIDCProviderConfig struct {
	// Issuer is the OIDC issuer URL.
	Issuer string `mapstructure:"issuer" validate:"required,url" jsonschema:"(required) OIDC issuer URL"`
	// Audience is the expected audience.
	Audience string `mapstructure:"audience" validate:"required" jsonschema:"(required) Expected audience (aud claim)"`
	// UserClaim is the claim mapped to Identity.User.
	UserClaim string `mapstructure:"userClaim" validate:"omitempty" jsonschema:"description=JWT claim mapped to Identity.User"`
	// SubClaim is the claim mapped to Identity.Sub.
	SubClaim string `mapstructure:"subClaim" validate:"omitempty" jsonschema:"description=JWT claim mapped to Identity.Sub"`
	// GroupsClaim is the claim mapped to Identity.Groups.
	GroupsClaim string `mapstructure:"groupsClaim" validate:"omitempty" jsonschema:"description=JWT claim mapped to Identity.Groups"`
	// JWKSURL overrides the JWKS endpoint.
	JWKSURL string `mapstructure:"jwksURL" validate:"omitempty,url" jsonschema:"description=JWKS endpoint override"`
}

// ApprovalConfig configures write-tool approval.
type ApprovalConfig struct {
	// Timeout is the maximum time to wait for a human decision.
	Timeout string `mapstructure:"timeout" validate:"omitempty" jsonschema:"description=Max time to wait for a human decision (default '5m')"`
	// Service configures the standalone approval service.
	Service *ApprovalServiceConfig `mapstructure:"service" validate:"omitempty" jsonschema:"description=Standalone approval service"`
}

// ApprovalServiceConfig configures the approval service client with secret
// references.
type ApprovalServiceConfig struct {
	// URL is the approval service base URL.
	URL string `mapstructure:"url" validate:"required,url" jsonschema:"(required) Approval service base URL"`
	// ServerName is the MCP server name asserted to the approval service.
	ServerName string `mapstructure:"serverName" validate:"required" jsonschema:"(required) MCP server name asserted to the approval service"`
	// ServerTokenFile is a file containing the per-server bearer token.
	ServerTokenFile string `mapstructure:"serverTokenFile" validate:"required_without=ServerTokenEnv" jsonschema:"description=File containing the per-server bearer token"`
	// ServerTokenEnv is the environment variable containing the per-server bearer token.
	ServerTokenEnv string `mapstructure:"serverTokenEnv" jsonschema:"description=Environment variable containing the per-server bearer token"`
	// ServerToken is the resolved per-server bearer token.
	ServerToken string `mapstructure:"-" validate:"required" jsonschema:"-"`
	// ReceiptSecretFile is a file containing the HS256 receipt secret.
	ReceiptSecretFile string `mapstructure:"receiptSecretFile" jsonschema:"description=File containing the HS256 receipt secret"`
	// ReceiptSecretEnv is the environment variable containing the HS256 receipt secret.
	ReceiptSecretEnv string `mapstructure:"receiptSecretEnv" jsonschema:"description=Environment variable containing the HS256 receipt secret"`
	// ReceiptSecret is the resolved HS256 receipt secret.
	ReceiptSecret string `mapstructure:"-" validate:"required_without=ReceiptPrivateKeyPEM" jsonschema:"-"`
	// ReceiptPrivateKeyFile is a file containing the PEM private key.
	ReceiptPrivateKeyFile string `mapstructure:"receiptPrivateKeyFile" jsonschema:"description=File containing the PEM private key"`
	// ReceiptPrivateKeyEnv is the environment variable containing the PEM private key.
	ReceiptPrivateKeyEnv string `mapstructure:"receiptPrivateKeyEnv" jsonschema:"description=Environment variable containing the PEM private key"`
	// ReceiptPrivateKeyPEM is the resolved PEM private key.
	ReceiptPrivateKeyPEM string `mapstructure:"-" validate:"required_without=ReceiptSecret" jsonschema:"-"`
	// ReceiptKeyID is the JWT kid header.
	ReceiptKeyID string `mapstructure:"receiptKeyID" validate:"omitempty" jsonschema:"description=Receipt JWT key id"`
	// ReceiptAlgorithm is HS256, RS256 or EdDSA.
	ReceiptAlgorithm string `mapstructure:"receiptAlgorithm" validate:"omitempty,oneof=HS256 RS256 EdDSA" jsonschema:"description=Receipt algorithm (default 'HS256')"`
	// PollInterval is the delay between long-poll slices.
	PollInterval string `mapstructure:"pollInterval" validate:"omitempty" jsonschema:"description=Long-poll slice interval (default '250ms')"`
	// RequestTimeout is the per-request HTTP timeout.
	RequestTimeout string `mapstructure:"requestTimeout" validate:"omitempty" jsonschema:"description=Per-request HTTP timeout (default '35s')"`
	// TTL is the approval record TTL requested from the service.
	TTL string `mapstructure:"ttl" validate:"omitempty" jsonschema:"description=Approval record TTL (default '15m')"`
}

// DeveloperFamily configures the developer tool family. It mirrors
// developer.Config but carries secret references instead of resolved values.
type DeveloperFamily struct {
	// WorkspaceRoot is the absolute host root for per-identity workspaces.
	WorkspaceRoot string `mapstructure:"workspaceRoot" validate:"required" jsonschema:"(required) Absolute host root for per-identity workspaces"`
	// BaseImage is the fallback OCI base image.
	BaseImage string `mapstructure:"baseImage" validate:"omitempty" jsonschema:"description=Fallback OCI base image (default 'alpine:3.20')"`
	// ImageOverrides maps a profile name to an OCI base image.
	ImageOverrides map[string]string `mapstructure:"imageOverrides" validate:"omitempty" jsonschema:"description=Per-profile base image overrides"`
	// AllowedImages restricts the base images that may be used.
	AllowedImages []string `mapstructure:"allowedImages" validate:"omitempty,dive,required" jsonschema:"description=Allowlist of permitted base images"`
	// Egress configures sandbox egress.
	Egress *DeveloperEgress `mapstructure:"egress" validate:"omitempty" jsonschema:"description=Sandbox egress policy"`
	// Engine configures the Dagger engine connection.
	Engine DeveloperEngine `mapstructure:"engine" validate:"required" jsonschema:"(required) Dagger engine connection"`
	// DefaultTimeout is the default per-command timeout.
	DefaultTimeout string `mapstructure:"defaultTimeout" validate:"omitempty" jsonschema:"description=Default command timeout (default '5m')"`
	// MaxTimeout caps the per-call timeout.
	MaxTimeout string `mapstructure:"maxTimeout" validate:"omitempty" jsonschema:"description=Maximum command timeout (default '15m')"`
	// MaxOutputBytes caps captured output.
	MaxOutputBytes int `mapstructure:"maxOutputBytes" validate:"omitempty,gte=1024" jsonschema:"description=Maximum captured output bytes (default 256000)"`
	// MaxConcurrentExecs caps simultaneous executions.
	MaxConcurrentExecs int `mapstructure:"maxConcurrentExecs" validate:"omitempty,gte=1" jsonschema:"description=Maximum simultaneous executions (default 4)"`
	// ContainerUser is the UID[:GID] inside the container.
	ContainerUser string `mapstructure:"containerUser" validate:"omitempty" jsonschema:"description=Container user (default '1000:1000')"`
	// ExtraBlocklist extends the default command blocklist.
	ExtraBlocklist []string `mapstructure:"extraBlocklist" validate:"omitempty,dive,required" jsonschema:"description=Additional command blocklist patterns"`
	// WorkspaceTTL is the idle workspace TTL.
	WorkspaceTTL string `mapstructure:"workspaceTTL" validate:"omitempty" jsonschema:"description=Idle workspace TTL (default '24h')"`
	// WorkspaceGCInterval is the GC sweep interval.
	WorkspaceGCInterval string `mapstructure:"workspaceGCInterval" validate:"omitempty" jsonschema:"description=Workspace GC interval (default '1h')"`
	// GitHub configures the GitHub tool family.
	GitHub *DeveloperGitHub `mapstructure:"github" validate:"omitempty" jsonschema:"description=GitHub tool family settings"`
	// Scaffolding enables the scaffolding surface.
	Scaffolding *bool `mapstructure:"scaffolding" validate:"omitempty" jsonschema:"description=Expose scaffolding tools, prompts and resources (default true)"`
}

// DeveloperEgress is the sandbox egress policy.
type DeveloperEgress struct {
	// DefaultDeny denies everything not explicitly allowed.
	DefaultDeny *bool `mapstructure:"defaultDeny" validate:"omitempty" jsonschema:"description=Deny all egress except the allowlist (default true)"`
	// AllowHosts are the permitted destination hostnames.
	AllowHosts []string `mapstructure:"allowHosts" validate:"omitempty,dive,required,hostname_rfc1123" jsonschema:"description=Allowed egress hostnames"`
	// AllowCIDRs are the permitted destination CIDRs.
	AllowCIDRs []string `mapstructure:"allowCIDRs" validate:"omitempty,dive,cidr" jsonschema:"description=Allowed egress CIDRs"`
	// AllowLocalNetwork permits private-network egress. DANGEROUS.
	AllowLocalNetwork bool `mapstructure:"allowLocalNetwork" validate:"omitempty" jsonschema:"description=DANGEROUS - allow local/private network egress"`
}

// DeveloperEngine configures the Dagger engine connection.
type DeveloperEngine struct {
	// Mode is sidecar, cloud or dagger-kubernetes.
	Mode string `mapstructure:"mode" validate:"required,oneof=sidecar cloud dagger-kubernetes" jsonschema:"(required) Engine mode: 'sidecar', 'cloud' or 'dagger-kubernetes'"`
	// RunnerHost is the Dagger runner host.
	RunnerHost string `mapstructure:"runnerHost" validate:"omitempty" jsonschema:"description=Dagger runner host"`
	// CloudURL is the Dagger Cloud / supervisor base URL.
	CloudURL string `mapstructure:"cloudURL" validate:"required_if=Mode dagger-kubernetes,omitempty,url" jsonschema:"description=Dagger Cloud / supervisor base URL"`
	// CloudTokenFile is a file containing the Dagger Cloud token.
	CloudTokenFile string `mapstructure:"cloudTokenFile" validate:"omitempty" jsonschema:"description=File containing the Dagger Cloud token"`
	// CloudTokenEnv is the environment variable containing the Dagger Cloud token.
	CloudTokenEnv string `mapstructure:"cloudTokenEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the Dagger Cloud token"`
	// CloudToken is the resolved token. Never set from YAML.
	CloudToken string `mapstructure:"-" validate:"-" jsonschema:"-"`
	// EngineVersion pins the Dagger engine version.
	EngineVersion string `mapstructure:"engineVersion" validate:"omitempty" jsonschema:"description=Pinned Dagger engine version"`
	// CLIPath is the Dagger CLI binary path.
	CLIPath string `mapstructure:"cliPath" validate:"omitempty" jsonschema:"description=Dagger CLI binary path"`
	// RegistryAuth holds registry pull credentials keyed by hostname.
	RegistryAuth map[string]DeveloperRegistryAuth `mapstructure:"registryAuth" validate:"omitempty,dive" jsonschema:"description=Registry pull credentials keyed by hostname"`
}

// DeveloperRegistryAuth is a registry credential with secret references.
type DeveloperRegistryAuth struct {
	// Username is the registry username.
	Username string `mapstructure:"username" validate:"required" jsonschema:"(required) Registry username"`
	// PasswordFile is a file containing the registry password.
	PasswordFile string `mapstructure:"passwordFile" validate:"required_without_all=PasswordEnv Password" jsonschema:"description=File containing the registry password"`
	// PasswordEnv is the environment variable containing the registry password.
	PasswordEnv string `mapstructure:"passwordEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the registry password"`
	// Password is the resolved password. Never set from YAML.
	Password string `mapstructure:"-" validate:"required" jsonschema:"-"`
}

// DeveloperGitHub configures the GitHub tool family.
type DeveloperGitHub struct {
	// Instances maps a GitHub instance name to its settings.
	Instances map[string]DeveloperGitHubInstance `mapstructure:"instances" validate:"required,min=1,dive" jsonschema:"(required) GitHub instances keyed by name"`
	// ReadOnly exposes only the non-gated GitHub tools.
	ReadOnly bool `mapstructure:"readOnly" validate:"omitempty" jsonschema:"description=Expose only read-only GitHub tools"`
}

// DeveloperGitHubInstance configures one GitHub instance.
type DeveloperGitHubInstance struct {
	// TokenFile is a file containing the GitHub token.
	TokenFile string `mapstructure:"tokenFile" validate:"required_without_all=TokenEnv Token" jsonschema:"description=File containing the GitHub token"`
	// TokenEnv is the environment variable containing the GitHub token.
	TokenEnv string `mapstructure:"tokenEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the GitHub token"`
	// Token is the resolved token. Never set from YAML.
	Token string `mapstructure:"-" validate:"required" jsonschema:"-"`
	// BaseURL is the GitHub Enterprise base URL.
	BaseURL string `mapstructure:"baseURL" validate:"omitempty,url" jsonschema:"description=GitHub Enterprise base URL"`
	// UploadURL is the GHES upload URL.
	UploadURL string `mapstructure:"uploadURL" validate:"omitempty,url" jsonschema:"description=GHES upload URL"`
	// Timeout is the per-request timeout.
	Timeout string `mapstructure:"timeout" validate:"omitempty" jsonschema:"description=Per-request timeout (default '30s')"`
	// TLSSkipVerify disables TLS certificate verification.
	TLSSkipVerify bool `mapstructure:"tlsSkipVerify" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
	// MaxCopyBytes caps directory copy/move size.
	MaxCopyBytes int64 `mapstructure:"maxCopyBytes" validate:"omitempty,gte=1" jsonschema:"description=Maximum bytes for directory copy/move"`
}

// MemoryFamily configures the memory MCP family with file/env secret references.
type MemoryFamily struct {
	Backend               string                   `mapstructure:"backend" validate:"omitempty,oneof=memory file opensearch" jsonschema:"description=Storage backend: 'memory' (default), 'file', or 'opensearch'"`
	Isolation             string                   `mapstructure:"isolation" validate:"omitempty,oneof=per_identity shared" jsonschema:"description=Tenant isolation: 'per_identity' (default) or 'shared'"`
	SharedNamespace       string                   `mapstructure:"sharedNamespace" validate:"omitempty,max=128" jsonschema:"description=Shared namespace when isolation is 'shared'"`
	File                  *MemoryFileBackend       `mapstructure:"file" validate:"omitempty" jsonschema:"description=JSONL file backend settings"`
	OpenSearch            *MemoryOpenSearchBackend `mapstructure:"opensearch" validate:"omitempty" jsonschema:"description=OpenSearch backend settings"`
	Embedding             *MemoryEmbeddingConfig   `mapstructure:"embedding" validate:"omitempty" jsonschema:"description=Embedding provider settings"`
	LLM                   *MemoryLLMConfig         `mapstructure:"llm" validate:"omitempty" jsonschema:"description=Chat model settings for discussion capture"`
	ReadOnly              bool                     `mapstructure:"readOnly" validate:"omitempty" jsonschema:"description=Expose only read-only memory tools"`
	RequireDeleteApproval bool                     `mapstructure:"requireDeleteApproval" validate:"omitempty" jsonschema:"description=Gate memory_delete with human approval"`
	Prompts               *bool                    `mapstructure:"prompts" validate:"omitempty" jsonschema:"description=Expose MCP prompts and resources (default true)"`
	DefaultTopK           int                      `mapstructure:"defaultTopK" validate:"omitempty,gte=1,lte=50" jsonschema:"description=Default search topK (default 5)"`
	MaxTopK               int                      `mapstructure:"maxTopK" validate:"omitempty,gte=1,lte=100" jsonschema:"description=Max search topK (default 50)"`
	MaxContentChars       int                      `mapstructure:"maxContentChars" validate:"omitempty,gte=256,lte=32768" jsonschema:"description=Max characters per memory (default 16384)"`
	MaxQueryChars         int                      `mapstructure:"maxQueryChars" validate:"omitempty,gte=64,lte=16384" jsonschema:"description=Max characters per search query (default 4096)"`
	RequestTimeout        string                   `mapstructure:"requestTimeout" validate:"omitempty" jsonschema:"description=Per-operation timeout (default '30s')"`
	MinScore              *float64                 `mapstructure:"minScore" validate:"omitempty,gte=0" jsonschema:"description=Optional minimum relevance score"`
}

// MemoryFileBackend configures the per-tenant JSONL file store.
type MemoryFileBackend struct {
	Dir string `mapstructure:"dir" validate:"required" jsonschema:"(required) Absolute host directory for JSONL memory files"`
}

// MemoryOpenSearchBackend configures the OpenSearch store.
type MemoryOpenSearchBackend struct {
	URLs               []string `mapstructure:"urls" validate:"required,min=1,dive,required,url" jsonschema:"(required) OpenSearch cluster URLs"`
	Username           string   `mapstructure:"username" validate:"omitempty" jsonschema:"description=Basic auth username"`
	PasswordFile       string   `mapstructure:"passwordFile" validate:"omitempty" jsonschema:"description=File containing the OpenSearch password"`
	PasswordEnv        string   `mapstructure:"passwordEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the OpenSearch password"`
	Password           string   `mapstructure:"-" validate:"-" jsonschema:"-"`
	TLSSkipVerify      bool     `mapstructure:"tlsSkipVerify" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
	IndexName          string   `mapstructure:"indexName" validate:"omitempty,max=255" jsonschema:"description=OpenSearch index name (default 'eino_agent_memory')"`
	VectorField        string   `mapstructure:"vectorField" validate:"omitempty,max=128" jsonschema:"description=knn_vector field name (default 'vector')"`
	ContentField       string   `mapstructure:"contentField" validate:"omitempty,max=128" jsonschema:"description=BM25 content field name (default 'content')"`
	VectorDimension    int      `mapstructure:"vectorDimension" validate:"omitempty,gte=1,lte=8192" jsonschema:"description=knn_vector dimension (default 1536)"`
	Hybrid             bool     `mapstructure:"hybrid" validate:"omitempty" jsonschema:"description=Combine kNN with BM25"`
	K                  int      `mapstructure:"k" validate:"omitempty,gte=1,lte=500" jsonschema:"description=Nearest neighbors K"`
	BatchSize          int      `mapstructure:"batchSize" validate:"omitempty,gte=1,lte=1000" jsonschema:"description=Bulk index batch size"`
	SearchPipeline     string   `mapstructure:"searchPipeline" validate:"omitempty,max=128" jsonschema:"description=Optional search pipeline"`
	Operator           string   `mapstructure:"operator" validate:"omitempty,oneof=or and" jsonschema:"description=BM25 operator ('or' or 'and')"`
	MinimumShouldMatch string   `mapstructure:"minimumShouldMatch" validate:"omitempty,max=128" jsonschema:"description=Optional BM25 minimum_should_match"`
}

// MemoryEmbeddingConfig configures an embedding provider with secret references.
type MemoryEmbeddingConfig struct {
	Provider      string `mapstructure:"provider" validate:"required,oneof=openai github-copilot" jsonschema:"(required) Embedding provider: 'openai' or 'github-copilot'"`
	Model         string `mapstructure:"model" validate:"required,max=256" jsonschema:"(required) Embedding model ID"`
	BaseURL       string `mapstructure:"baseURL" validate:"omitempty,url" jsonschema:"description=Optional provider endpoint URL"`
	APIKeyFile    string `mapstructure:"apiKeyFile" validate:"omitempty" jsonschema:"description=File containing the embedding API key"`
	APIKeyEnv     string `mapstructure:"apiKeyEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the embedding API key"`
	APIKey        string `mapstructure:"-" validate:"-" jsonschema:"-"`
	Timeout       string `mapstructure:"timeout" validate:"omitempty" jsonschema:"description=Request timeout (default '30s')"`
	TLSSkipVerify bool   `mapstructure:"tlsSkipVerify" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
}

// MemoryLLMConfig configures a chat model with secret references.
type MemoryLLMConfig struct {
	Provider        string  `mapstructure:"provider" validate:"required,oneof=ollama openai github-copilot" jsonschema:"(required) Chat model provider: 'ollama', 'openai', or 'github-copilot'"`
	Model           string  `mapstructure:"model" validate:"required,max=256" jsonschema:"(required) Chat model ID"`
	BaseURL         string  `mapstructure:"baseURL" validate:"omitempty,url" jsonschema:"description=Optional provider base URL"`
	APIKeyFile      string  `mapstructure:"apiKeyFile" validate:"omitempty" jsonschema:"description=File containing the LLM API key"`
	APIKeyEnv       string  `mapstructure:"apiKeyEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the LLM API key"`
	APIKey          string  `mapstructure:"-" validate:"-" jsonschema:"-"`
	Temperature     float32 `mapstructure:"temperature" validate:"gte=0,lte=2" jsonschema:"description=Sampling temperature"`
	Thinking        string  `mapstructure:"thinking" validate:"omitempty,oneof=off low medium high" jsonschema:"description=Reasoning effort"`
	MaxOutputTokens int     `mapstructure:"maxOutputTokens" validate:"gte=0,lte=32000" jsonschema:"description=Max output tokens"`
	Timeout         string  `mapstructure:"timeout" validate:"omitempty" jsonschema:"description=LLM request timeout (default '30s')"`
	TLSSkipVerify   bool    `mapstructure:"tlsSkipVerify" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
}
