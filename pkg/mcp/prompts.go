package mcp

import (
	"context"
	"net/url"
	"time"

	emperrors "emperror.dev/errors"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/safety"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
)

// PromptSpec declares an MCP prompt exposed by a family.
type PromptSpec struct {
	Name        string               `validate:"required" jsonschema:"(required) Prompt name"`
	Title       string               `validate:"omitempty" jsonschema:"description=Human-readable prompt title"`
	Description string               `validate:"omitempty" jsonschema:"description=What the prompt is for"`
	Arguments   []PromptArgumentSpec `validate:"omitempty,dive" jsonschema:"description=Prompt template arguments"`
	Handler     PromptHandlerFunc    `json:"-" validate:"required"`
}

// PromptArgumentSpec declares one prompt argument.
type PromptArgumentSpec struct {
	Name        string `validate:"required" jsonschema:"(required) Argument name"`
	Description string `validate:"omitempty" jsonschema:"description=Argument description"`
	Required    bool   `validate:"omitempty" jsonschema:"description=Whether the argument is mandatory"`
}

// PromptHandlerFunc renders a prompt for an identity. args holds the client
// supplied template arguments (always strings per the MCP spec).
type PromptHandlerFunc func(ctx context.Context, id *Identity, args map[string]string) (string, error)

// ResourceSpec declares an MCP resource exposed by a family.
type ResourceSpec struct {
	URI         string              `validate:"required,uri" jsonschema:"(required) Absolute resource URI (must have a scheme)"`
	Name        string              `validate:"required" jsonschema:"(required) Resource name"`
	Title       string              `validate:"omitempty" jsonschema:"description=Human-readable resource title"`
	Description string              `validate:"omitempty" jsonschema:"description=What the resource contains"`
	MIMEType    string              `validate:"omitempty" jsonschema:"description=Resource MIME type (default 'text/markdown')"`
	Handler     ResourceHandlerFunc `json:"-" validate:"required"`
}

// ResourceHandlerFunc returns the resource body for an identity.
type ResourceHandlerFunc func(ctx context.Context, id *Identity, uri string) (string, error)

// addPrompts registers the configured prompts on srv, filtered by RBAC.
func (s *Server) addPrompts(ctx context.Context, srv *mcpsdk.Server, id *Identity) error {
	specs := s.cfg.Prompts
	if s.cfg.PromptsFor != nil {
		var err error
		if specs, err = s.cfg.PromptsFor(ctx, id); err != nil {
			return emperrors.Wrap(err, "failed to build per-identity prompts")
		}
	}
	for _, spec := range specs {
		if err := validate.Struct(&spec); err != nil {
			return emperrors.Wrapf(err, "invalid prompt %q", spec.Name)
		}
		if s.authz != nil && s.authz.Authorize(id, spec.Name, "", false) != nil {
			continue
		}
		srv.AddPrompt(promptFromSpec(spec), s.makePromptHandler(spec))
	}
	return nil
}

// addResources registers the configured resources on srv, filtered by RBAC.
func (s *Server) addResources(ctx context.Context, srv *mcpsdk.Server, id *Identity) error {
	specs := s.cfg.Resources
	if s.cfg.ResourcesFor != nil {
		var err error
		if specs, err = s.cfg.ResourcesFor(ctx, id); err != nil {
			return emperrors.Wrap(err, "failed to build per-identity resources")
		}
	}
	for _, spec := range specs {
		if err := validate.Struct(&spec); err != nil {
			return emperrors.Wrapf(err, "invalid resource %q", spec.Name)
		}
		// AddResource panics on an unparseable or scheme-less URI; validate
		// before registering so a bad spec is a returned error, not a panic.
		u, err := url.Parse(spec.URI)
		if err != nil {
			return emperrors.Wrapf(err, "invalid resource URI %q", spec.URI)
		}
		if u.Scheme == "" {
			return emperrors.Errorf("invalid resource URI %q: an absolute URI with a scheme is required", spec.URI)
		}
		if s.authz != nil && s.authz.Authorize(id, spec.Name, "", false) != nil {
			continue
		}
		srv.AddResource(resourceFromSpec(spec), s.makeResourceHandler(spec))
	}
	return nil
}

func promptFromSpec(spec PromptSpec) *mcpsdk.Prompt {
	args := make([]*mcpsdk.PromptArgument, 0, len(spec.Arguments))
	for _, arg := range spec.Arguments {
		args = append(args, &mcpsdk.PromptArgument{
			Name:        arg.Name,
			Description: arg.Description,
			Required:    arg.Required,
		})
	}
	return &mcpsdk.Prompt{
		Name:        spec.Name,
		Title:       spec.Title,
		Description: spec.Description,
		Arguments:   args,
	}
}

func resourceFromSpec(spec ResourceSpec) *mcpsdk.Resource {
	mimeType := spec.MIMEType
	if mimeType == "" {
		mimeType = "text/markdown"
	}
	return &mcpsdk.Resource{
		URI:         spec.URI,
		Name:        spec.Name,
		Title:       spec.Title,
		Description: spec.Description,
		MIMEType:    mimeType,
	}
}

func (s *Server) makePromptHandler(spec PromptSpec) mcpsdk.PromptHandler {
	return func(ctx context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		id := s.identityFromRequestExtra(req.Extra)
		if s.authz != nil {
			if err := s.authz.Authorize(id, spec.Name, "", false); err != nil {
				return nil, err
			}
		}
		text, err := spec.Handler(ctx, id, req.Params.Arguments)
		if err != nil {
			return nil, emperrors.Wrapf(err, "failed to render prompt %q", spec.Name)
		}
		s.audit(ctx, safety.AuditEvent{
			Timestamp: time.Now(), ToolName: "prompt:" + spec.Name, CallID: uuid.NewString(),
			Phase: safety.PhaseRead, Metadata: identityMetadata(id),
		})
		return &mcpsdk.GetPromptResult{
			Description: spec.Description,
			Messages: []*mcpsdk.PromptMessage{{
				Role:    mcpsdk.Role("user"),
				Content: &mcpsdk.TextContent{Text: text},
			}},
		}, nil
	}
}

func (s *Server) makeResourceHandler(spec ResourceSpec) mcpsdk.ResourceHandler {
	return func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		id := s.identityFromRequestExtra(req.Extra)
		if s.authz != nil {
			if err := s.authz.Authorize(id, spec.Name, "", false); err != nil {
				return nil, err
			}
		}
		text, err := spec.Handler(ctx, id, req.Params.URI)
		if err != nil {
			return nil, emperrors.Wrapf(err, "failed to read resource %q", req.Params.URI)
		}
		s.audit(ctx, safety.AuditEvent{
			Timestamp: time.Now(), ToolName: "resource:" + req.Params.URI, CallID: uuid.NewString(),
			Phase: safety.PhaseRead, Metadata: identityMetadata(id),
		})
		mimeType := spec.MIMEType
		if mimeType == "" {
			mimeType = "text/markdown"
		}
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: mimeType,
				Text:     text,
			}},
		}, nil
	}
}

// identityFromRequestExtra resolves the caller identity from a server request's
// transport metadata, falling back to the configured local identity.
func (s *Server) identityFromRequestExtra(extra *mcpsdk.RequestExtra) *Identity {
	var ti *auth.TokenInfo
	if extra != nil {
		ti = extra.TokenInfo
	}
	return s.identityOrLocal(identityFromTokenInfo(ti))
}
