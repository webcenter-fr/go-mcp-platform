package developer

import (
	"context"
	"io"
	"regexp"
	"slices"

	emperrors "emperror.dev/errors"
	"github.com/cloudwego/eino/components/tool"
	"github.com/sirupsen/logrus"

	githubtool "github.com/webcenter-fr/eino-ext/components/tool/github"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/egress"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/fileutil"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/profile"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/safety"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/workspace"
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// NewServer builds an MCP server exposing the developer sandbox, the GitHub
// tool family and the scaffolding surface. Construction is lazy: no Dagger
// engine and no GitHub API call happen here.
func NewServer(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, error) {
	srv, closer, err := NewServerWithRunner(ctx, cfg, mcpCfg)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = closer.Close()
	}()
	return srv, nil
}

// NewServerWithRunner is NewServer plus the io.Closer for the sandbox backend,
// so a host can release engine resources on shutdown.
func NewServerWithRunner(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, io.Closer, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	applyDefaults(cfg)
	if err := validate.Struct(cfg); err != nil {
		return nil, nil, err
	}
	if err := validateEngine(cfg); err != nil {
		return nil, nil, err
	}
	if err := fileutil.ValidateRootDir(cfg.WorkspaceRoot); err != nil {
		return nil, nil, emperrors.Wrap(err, "WorkspaceRoot")
	}

	blocklist, err := safety.CompileBlocklist(append(
		slices.Clone(safety.DefaultCommandBlocklist), cfg.ExtraBlocklist...))
	if err != nil {
		return nil, nil, emperrors.Wrap(err, "failed to compile command blocklist")
	}

	resolver := profile.NewResolver(profile.WithImageOverrides(cfg.ImageOverrides))

	runner := cfg.Runner
	if runner == nil {
		runner, err = NewDaggerRunner(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
	}

	policy := &egress.Policy{
		AllowHosts:        append([]string(nil), cfg.Egress.AllowHosts...),
		AllowCIDRs:        append([]string(nil), cfg.Egress.AllowCIDRs...),
		AllowLocalNetwork: cfg.Egress.AllowLocalNetwork,
		DefaultDeny:       cfg.Egress.DefaultDeny != nil && *cfg.Egress.DefaultDeny,
	}
	if cfg.Egress.AllowLocalNetwork {
		logrus.Warn("developer: egress.allowLocalNetwork=true — sandboxed commands may reach private networks, the Kubernetes API server and cloud metadata endpoints")
	}
	if !policy.DefaultDeny {
		logrus.Warn("developer: egress.defaultDeny=false — sandboxed commands may reach any destination")
	}
	if _, ok := runner.(egressEnforcer); ok {
		logrus.Warn("developer: egress is enforced through an HTTP proxy (soft control): tools that ignore HTTP_PROXY/HTTPS_PROXY (raw TCP: psql, redis-cli, nc, kubectl, direct dialers) bypass it; operator-side NetworkPolicy/nftables is required for a hard guarantee")
	}

	redactor := newRedactor(cfg)
	scaffolding := cfg.Scaffolding != nil && *cfg.Scaffolding

	if mcpCfg == nil {
		mcpCfg = &mcpserver.Config{}
	}
	if mcpCfg.Name == "" {
		mcpCfg.Name = "developer"
	}
	mcpCfg.InstanceParam = "instance"
	mcpCfg.ToolsFor = func(ctx context.Context, id *mcpserver.Identity) ([]tool.InvokableTool, error) {
		return buildToolsForIdentity(ctx, cfg, runner, blocklist, resolver, policy, redactor, scaffolding, id)
	}
	mcpCfg.WriteToolsFor = func(context.Context, *mcpserver.Identity) ([]string, error) {
		return WriteToolNames(cfg), nil
	}
	if scaffolding {
		mcpCfg.PromptsFor = func(context.Context, *mcpserver.Identity) ([]mcpserver.PromptSpec, error) {
			return scaffoldPrompts(), nil
		}
		mcpCfg.ResourcesFor = func(context.Context, *mcpserver.Identity) ([]mcpserver.ResourceSpec, error) {
			return scaffoldResources(), nil
		}
	}

	layout := &workspace.Layout{Root: cfg.WorkspaceRoot, SessionKey: workspace.DefaultSessionKey}
	layout.StartGC(ctx, cfg.WorkspaceTTL, cfg.WorkspaceGCInterval)

	srv, err := mcpserver.NewServer(ctx, mcpCfg)
	if err != nil {
		_ = runner.Close()
		return nil, nil, err
	}
	return srv, runner, nil
}

// validateEngine enforces the mode-specific engine requirements that cannot be
// expressed with struct tags alone.
func validateEngine(cfg *Config) error {
	switch cfg.Engine.Mode {
	case "sidecar":
		if cfg.Engine.RunnerHost == "" {
			return emperrors.New("developer: engine.mode=sidecar requires engine.runnerHost")
		}
	case "cloud", "dagger-kubernetes":
		if cfg.Engine.CloudToken == "" {
			return emperrors.Errorf("developer: engine.mode=%s requires a Dagger Cloud token (cloudTokenFile or cloudTokenEnv)", cfg.Engine.Mode)
		}
	}
	return nil
}

// WriteToolNames returns the gated write-tool names for this family.
//
// developer_sandbox_exec is intentionally NOT a write tool: it executes in a
// disposable, credential-free container and cannot reach a production system.
// developer_scaffold_apply is intentionally NOT a write tool either: like
// github_file_write it only mutates the local workspace, never a remote.
// The gated set is exactly the GitHub API/commit write tools.
func WriteToolNames(cfg *Config) []string {
	if cfg == nil || cfg.GitHub == nil || cfg.GitHub.ReadOnly {
		return nil
	}
	return githubtool.WriteToolNames()
}

// buildToolsForIdentity builds the tool set bound to one identity's workspace.
func buildToolsForIdentity(
	ctx context.Context,
	cfg *Config,
	runner Runner,
	blocklist []*regexp.Regexp,
	resolver *profile.Resolver,
	policy *egress.Policy,
	redactor *redactor,
	scaffolding bool,
	id *mcpserver.Identity,
) ([]tool.InvokableTool, error) {
	identityRoot := identityWorkspaceRoot(cfg.WorkspaceRoot, id)

	sandboxTools, err := newSandboxTools(cfg, runner, blocklist, resolver, policy, identityRoot, redactor)
	if err != nil {
		return nil, err
	}
	tools := append([]tool.InvokableTool(nil), sandboxTools...)

	if scaffolding {
		scaffoldTools, err := newScaffoldTools(identityRoot)
		if err != nil {
			return nil, err
		}
		tools = append(tools, scaffoldTools...)
	}

	if cfg.GitHub != nil {
		configs := make(githubtool.Configs, len(cfg.GitHub.Instances))
		for name, instance := range cfg.GitHub.Instances {
			configs[name] = githubtool.Config{
				Token:          instance.Token,
				BaseURL:        instance.BaseURL,
				UploadURL:      instance.UploadURL,
				CloneDir:       identityRoot,
				Timeout:        instance.Timeout,
				TLSSkipVerify:  instance.TLSSkipVerify,
				RequireSession: false,
				MaxCopyBytes:   instance.MaxCopyBytes,
			}
		}
		var githubTools []tool.InvokableTool
		if cfg.GitHub.ReadOnly {
			githubTools, err = githubtool.NewReadOnlyTools(ctx, configs)
		} else {
			githubTools, err = githubtool.NewAllTools(ctx, configs)
		}
		if err != nil {
			return nil, emperrors.Wrap(err, "failed to build GitHub tools")
		}
		tools = append(tools, githubTools...)
	}

	return tools, nil
}
