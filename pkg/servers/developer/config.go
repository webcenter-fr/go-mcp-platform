// Package developer provides an MCP server exposing a credential-free,
// network-restricted Dagger sandbox shell, the GitHub tool family, and
// project-bootstrap scaffolding (AGENTS.md, lifecycle agent specs, opencode
// settings) as tools, MCP prompts and MCP resources.
package developer

import (
	"time"
)

// Config configures the developer MCP server family.
type Config struct {
	// WorkspaceRoot is the absolute host directory under which per-identity
	// workspaces are created: <WorkspaceRoot>/<identityHash>/<owner>/<repo>.
	// It must be an absolute path and must not be "/" or a system directory
	// (enforced by fileutil.ValidateRootDir).
	WorkspaceRoot string `mapstructure:"workspaceRoot" validate:"required" jsonschema:"(required) Absolute host root for per-identity workspaces"`

	// BaseImage is the fallback OCI base image when profile detection finds no
	// marker file. Default "alpine:3.20".
	BaseImage string `mapstructure:"baseImage" validate:"omitempty" jsonschema:"description=Fallback OCI base image (default 'alpine:3.20')"`

	// ImageOverrides maps a profile name (golang, node, python, java, rust,
	// php) to an OCI base image, overriding profile.DefaultImageMap. This is
	// the operator's "set the base image to use" knob from the issue.
	ImageOverrides map[string]string `mapstructure:"imageOverrides" validate:"omitempty" jsonschema:"description=Per-profile base image overrides"`

	// AllowedImages, when non-empty, restricts the base images that may ever be
	// used (after overrides and detection) to this allowlist. A resolved image
	// outside the list is rejected. Empty means no restriction.
	AllowedImages []string `mapstructure:"allowedImages" validate:"omitempty,dive,required" jsonschema:"description=Allowlist of permitted base images (empty means no restriction)"`

	// Egress configures outbound network access from the sandbox. When nil a
	// deny-by-default policy is applied with AllowHosts defaulted to the
	// language package registries (see DefaultEgressAllowHosts).
	Egress *EgressConfig `mapstructure:"egress" validate:"omitempty" jsonschema:"description=Sandbox egress policy"`

	// Engine configures how the Dagger engine is reached.
	Engine EngineConfig `mapstructure:"engine" validate:"required" jsonschema:"(required) Dagger engine connection"`

	// DefaultTimeout is the default per-command timeout. Default 5m.
	DefaultTimeout time.Duration `mapstructure:"defaultTimeout" validate:"omitempty,gte=1000000000" jsonschema:"description=Default command timeout (default '5m')"`

	// MaxTimeout caps the per-call timeout an LLM may request. Default 15m.
	MaxTimeout time.Duration `mapstructure:"maxTimeout" validate:"omitempty,gte=1000000000" jsonschema:"description=Maximum command timeout a caller may request (default '15m')"`

	// MaxOutputBytes caps captured stdout+stderr per command. Output beyond the
	// cap is truncated with an explicit marker. Default 256000 (250 KiB).
	MaxOutputBytes int `mapstructure:"maxOutputBytes" validate:"omitempty,gte=1024" jsonschema:"description=Maximum captured output bytes per command (default 256000)"`

	// MaxConcurrentExecs caps simultaneous sandbox executions across all
	// identities. Default 4. Exceeding it returns ErrSandboxBusy.
	MaxConcurrentExecs int `mapstructure:"maxConcurrentExecs" validate:"omitempty,gte=1" jsonschema:"description=Maximum simultaneous sandbox executions (default 4)"`

	// ContainerUser is the UID[:GID] the command runs as inside the container.
	// Default "1000:1000" — never root, so a container escape attempt starts
	// unprivileged.
	ContainerUser string `mapstructure:"containerUser" validate:"omitempty" jsonschema:"description=Container user (default '1000:1000')"`

	// ExtraBlocklist extends safety.DefaultCommandBlocklist (it does not
	// replace it, unlike shell.Config.Blocklist).
	ExtraBlocklist []string `mapstructure:"extraBlocklist" validate:"omitempty,dive,required" jsonschema:"description=Additional command blocklist regex patterns (extends the defaults)"`

	// WorkspaceTTL is how long an idle per-identity workspace is kept before
	// the GC sweeps it. Default 24h. Zero disables the GC.
	WorkspaceTTL time.Duration `mapstructure:"workspaceTTL" validate:"omitempty,gte=0" jsonschema:"description=Idle workspace TTL before GC (default '24h', 0 disables)"`

	// WorkspaceGCInterval is the GC sweep interval. Default 1h.
	WorkspaceGCInterval time.Duration `mapstructure:"workspaceGCInterval" validate:"omitempty,gte=0" jsonschema:"description=Workspace GC sweep interval (default '1h')"`

	// GitHub configures the GitHub tool family exposed alongside the sandbox.
	// When nil, no GitHub tools are registered (sandbox-only mode).
	GitHub *GitHubConfig `mapstructure:"github" validate:"omitempty" jsonschema:"description=GitHub tool family settings"`

	// Scaffolding enables the AGENTS.md / lifecycle / opencode templates as
	// tools, prompts and resources. Default true.
	Scaffolding *bool `mapstructure:"scaffolding" validate:"omitempty" jsonschema:"description=Expose scaffolding tools, prompts and resources (default true)"`

	// Runner overrides the sandbox execution backend. Nil means the real
	// Dagger runner (lazily connected). Tests inject a fake.
	Runner Runner `mapstructure:"-" json:"-" validate:"-"`

	// DeepAnalyzer is the optional read-only analysis backend. Nil in Phase 1;
	// no analysis tool is registered when nil.
	DeepAnalyzer DeepAnalyzer `mapstructure:"-" json:"-" validate:"-"`
}

// EgressConfig is the sandbox egress policy.
type EgressConfig struct {
	// DefaultDeny denies everything not explicitly allowed. Default true.
	// Setting it false is a dangerous opt-in and logs a startup WARN.
	DefaultDeny *bool `mapstructure:"defaultDeny" validate:"omitempty" jsonschema:"description=Deny all egress except the allowlist (default true)"`
	// AllowHosts are the permitted destination hostnames.
	AllowHosts []string `mapstructure:"allowHosts" validate:"omitempty,dive,required,hostname_rfc1123" jsonschema:"description=Allowed egress hostnames"`
	// AllowCIDRs are the permitted destination CIDRs.
	AllowCIDRs []string `mapstructure:"allowCIDRs" validate:"omitempty,dive,cidr" jsonschema:"description=Allowed egress CIDRs"`
	// AllowLocalNetwork permits RFC1918, link-local and loopback destinations.
	// DANGEROUS: this is what lets a sandboxed command reach an in-cluster
	// Kubernetes API server or a neighbouring Service. Default false and it is
	// NEVER settable from a tool parameter.
	AllowLocalNetwork bool `mapstructure:"allowLocalNetwork" validate:"omitempty" jsonschema:"description=DANGEROUS - allow local/private network egress (default false)"`
}

// EngineConfig configures how the Dagger engine is reached.
type EngineConfig struct {
	// Mode is "sidecar", "cloud" or "dagger-kubernetes".
	//   sidecar            — a Dagger engine container in the same pod; RunnerHost
	//                        points at it (tcp:// or unix://).
	//   cloud              — Dagger Cloud; CloudToken is required.
	//   dagger-kubernetes  — a self-hosted Dagger-Cloud-compatible supervisor
	//                        (github.com/webcenter-fr/dagger-kubernetes);
	//                        CloudURL + CloudToken required and RunnerHost
	//                        defaults to "dagger-cloud://self".
	Mode string `mapstructure:"mode" validate:"required,oneof=sidecar cloud dagger-kubernetes" jsonschema:"(required) Engine mode: 'sidecar', 'cloud' or 'dagger-kubernetes'"`

	// RunnerHost sets _EXPERIMENTAL_DAGGER_RUNNER_HOST for the Dagger CLI
	// session. Required for mode "sidecar"; defaults to "dagger-cloud://self"
	// for mode "dagger-kubernetes"; unused for mode "cloud".
	RunnerHost string `mapstructure:"runnerHost" validate:"omitempty" jsonschema:"description=Dagger runner host (e.g. 'tcp://127.0.0.1:8081', 'dagger-cloud://self')"`

	// CloudURL is the supervisor base URL (DAGGER_CLOUD_URL). Required for mode
	// "dagger-kubernetes".
	CloudURL string `mapstructure:"cloudURL" validate:"required_if=Mode dagger-kubernetes,omitempty,url" jsonschema:"description=Dagger Cloud / supervisor base URL"`

	// CloudTokenFile / CloudTokenEnv are secret references for
	// DAGGER_CLOUD_TOKEN. One is required for modes "cloud" and
	// "dagger-kubernetes".
	CloudTokenFile string `mapstructure:"cloudTokenFile" validate:"omitempty" jsonschema:"description=File containing the Dagger Cloud token"`
	CloudTokenEnv  string `mapstructure:"cloudTokenEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the Dagger Cloud token"`
	// CloudToken is the resolved token. Never set from YAML.
	CloudToken string `mapstructure:"-" json:"-" jsonschema:"-" validate:"-"`

	// EngineVersion pins _EXPERIMENTAL_DAGGER_TAG (e.g. "v0.21.7"). Optional
	// but recommended for cache locality.
	EngineVersion string `mapstructure:"engineVersion" validate:"omitempty" jsonschema:"description=Pinned Dagger engine version (e.g. 'v0.21.7')"`

	// CLIPath is the Dagger CLI binary path set as
	// _EXPERIMENTAL_DAGGER_CLI_BIN. Default "/usr/local/bin/dagger".
	CLIPath string `mapstructure:"cliPath" validate:"omitempty" jsonschema:"description=Dagger CLI binary path (default '/usr/local/bin/dagger')"`

	// RegistryAuth holds registry credentials for pulling private base images,
	// keyed by hostname. These are engine-side pull credentials and are NEVER
	// exposed inside the sandbox container's environment.
	RegistryAuth map[string]RegistryAuthEntry `mapstructure:"registryAuth" validate:"omitempty,dive" jsonschema:"description=Registry pull credentials keyed by hostname"`
}

// RegistryAuthEntry is a registry credential with secret references.
type RegistryAuthEntry struct {
	Username     string `mapstructure:"username" validate:"required" jsonschema:"(required) Registry username"`
	PasswordFile string `mapstructure:"passwordFile" validate:"required_without_all=PasswordEnv Password" jsonschema:"description=File containing the registry password"`
	PasswordEnv  string `mapstructure:"passwordEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the registry password"`
	Password     string `mapstructure:"-" json:"-" jsonschema:"-" validate:"required"`
}

// GitHubConfig configures the GitHub tool family.
type GitHubConfig struct {
	// Instances maps a GitHub instance name to its settings. The instance name
	// is the value of the "instance" tool argument and is the RBAC instance.
	Instances map[string]GitHubInstance `mapstructure:"instances" validate:"required,min=1,dive" jsonschema:"(required) GitHub instances keyed by name"`
	// ReadOnly registers only the non-gated GitHub tools (no commit/push/PR).
	// Default false.
	ReadOnly bool `mapstructure:"readOnly" validate:"omitempty" jsonschema:"description=Expose only read-only GitHub tools (default false)"`
}

// GitHubInstance configures one GitHub instance.
type GitHubInstance struct {
	TokenFile     string        `mapstructure:"tokenFile" validate:"required_without_all=TokenEnv Token" jsonschema:"description=File containing the GitHub token"`
	TokenEnv      string        `mapstructure:"tokenEnv" validate:"omitempty" jsonschema:"description=Environment variable containing the GitHub token"`
	Token         string        `mapstructure:"-" json:"-" jsonschema:"-" validate:"required"`
	BaseURL       string        `mapstructure:"baseURL" validate:"omitempty,url" jsonschema:"description=GitHub Enterprise base URL (empty for github.com)"`
	UploadURL     string        `mapstructure:"uploadURL" validate:"omitempty,url" jsonschema:"description=GHES upload URL"`
	Timeout       time.Duration `mapstructure:"timeout" validate:"omitempty,gte=1000000000" jsonschema:"description=Per-request timeout (default '30s')"`
	TLSSkipVerify bool          `mapstructure:"tlsSkipVerify" validate:"omitempty" jsonschema:"description=Skip TLS certificate verification"`
	MaxCopyBytes  int64         `mapstructure:"maxCopyBytes" validate:"omitempty,gte=1" jsonschema:"description=Maximum bytes for directory copy/move (default 50MB)"`
}

// DefaultEgressAllowHosts are the package registries and source hosts a build
// legitimately needs. Deliberately excludes every cloud metadata endpoint and
// every private range.
var DefaultEgressAllowHosts = []string{
	"proxy.golang.org", "sum.golang.org", "storage.googleapis.com",
	"registry.npmjs.org", "pypi.org", "files.pythonhosted.org",
	"crates.io", "static.crates.io",
	"repo.maven.apache.org", "repo1.maven.org",
	"packagist.org", "repo.packagist.org",
	"github.com", "codeload.github.com", "objects.githubusercontent.com",
	"raw.githubusercontent.com",
}

// applyDefaults fills every default from the configuration table. It must run
// before validate.Struct so validation sees the final values.
func applyDefaults(cfg *Config) {
	if cfg.BaseImage == "" {
		cfg.BaseImage = "alpine:3.20"
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 5 * time.Minute
	}
	if cfg.MaxTimeout == 0 {
		cfg.MaxTimeout = 15 * time.Minute
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
	if cfg.WorkspaceTTL == 0 {
		cfg.WorkspaceTTL = 24 * time.Hour
	}
	if cfg.WorkspaceGCInterval == 0 {
		cfg.WorkspaceGCInterval = time.Hour
	}
	if cfg.Scaffolding == nil {
		enabled := true
		cfg.Scaffolding = &enabled
	}
	if cfg.Egress == nil {
		cfg.Egress = &EgressConfig{}
	}
	if cfg.Egress.DefaultDeny == nil {
		deny := true
		cfg.Egress.DefaultDeny = &deny
	}
	if len(cfg.Egress.AllowHosts) == 0 {
		cfg.Egress.AllowHosts = append([]string(nil), DefaultEgressAllowHosts...)
	}
	if cfg.Engine.CLIPath == "" {
		cfg.Engine.CLIPath = "/usr/local/bin/dagger"
	}
	if cfg.Engine.Mode == "dagger-kubernetes" && cfg.Engine.RunnerHost == "" {
		cfg.Engine.RunnerHost = "dagger-cloud://self"
	}
	if cfg.GitHub != nil {
		for name, instance := range cfg.GitHub.Instances {
			if instance.Timeout == 0 {
				instance.Timeout = 30 * time.Second
			}
			cfg.GitHub.Instances[name] = instance
		}
	}
}
