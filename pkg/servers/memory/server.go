package memory

import (
	"context"
	"io"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/tool"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// NewServer builds an MCP server exposing the memory family. Construction is
// lazy: no OpenSearch, embedding or LLM connection is opened here.
func NewServer(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, error) {
	srv, closer, err := NewServerWithCloser(ctx, cfg, mcpCfg)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = closer.Close()
	}()
	return srv, nil
}

// NewServerWithCloser is NewServer plus the io.Closer for backend resources.
func NewServerWithCloser(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, io.Closer, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	applyDefaults(cfg)
	if err := validate.Struct(cfg); err != nil {
		return nil, nil, err
	}
	if err := validateBackendConfig(cfg); err != nil {
		return nil, nil, err
	}

	redactor := newRedactor(cfg)
	router := newTenantBackendRouter(cfg)
	capturer, err := NewCapturer(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	if mcpCfg == nil {
		mcpCfg = &mcpserver.Config{}
	}
	if mcpCfg.Name == "" {
		mcpCfg.Name = "memory"
	}
	mcpCfg.InstanceParam = "instance"
	mcpCfg.ToolsFor = func(ctx context.Context, id *mcpserver.Identity) ([]tool.InvokableTool, error) {
		return newMemoryTools(cfg, router, redactor, capturer, resolveTenant(cfg, id))
	}
	mcpCfg.WriteToolsFor = func(context.Context, *mcpserver.Identity) ([]string, error) {
		return WriteToolNames(cfg), nil
	}
	if cfg.Prompts != nil && *cfg.Prompts {
		mcpCfg.PromptsFor = func(context.Context, *mcpserver.Identity) ([]mcpserver.PromptSpec, error) {
			return Prompts(), nil
		}
		mcpCfg.ResourcesFor = func(context.Context, *mcpserver.Identity) ([]mcpserver.ResourceSpec, error) {
			return Resources(), nil
		}
	}

	srv, err := mcpserver.NewServer(ctx, mcpCfg)
	if err != nil {
		_ = router.Close()
		return nil, nil, errors.Wrap(err, "failed to build memory MCP server")
	}
	return srv, router, nil
}

// WriteToolNames returns the gated write-tool names for this family.
// memory_delete is gated only when the operator opts into delete approval.
func WriteToolNames(cfg *Config) []string {
	if cfg == nil || cfg.ReadOnly || !cfg.RequireDeleteApproval {
		return nil
	}
	return []string{"memory_delete"}
}

// resolveTenant derives the tenant key from the authenticated identity. The
// subject is verified server-side and cannot be supplied by a tool argument.
func resolveTenant(cfg *Config, id *mcpserver.Identity) string {
	if cfg.Isolation == IsolationShared {
		return cfg.SharedNamespace
	}
	subject := id.Subject()
	if subject == "" {
		return DefaultLocalTenant
	}
	return subject
}
