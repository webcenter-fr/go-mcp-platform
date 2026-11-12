package developer

import (
	"context"
	_ "embed"
	"os"
	"sort"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/validate"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/workspace"
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// TemplateKind classifies a scaffolding template.
type TemplateKind string

// Template kinds.
const (
	KindAgents    TemplateKind = "agents"    // AGENTS.md for a language
	KindLifecycle TemplateKind = "lifecycle" // agent lifecycle / step specs
	KindOpenCode  TemplateKind = "opencode"  // opencode settings skeleton
)

// LifecycleStage is one step of the developer agent workflow.
type LifecycleStage string

// Lifecycle stages.
const (
	StageArchitect     LifecycleStage = "architect"
	StageDevelop       LifecycleStage = "develop"
	StageReview        LifecycleStage = "review"
	StageSecurity      LifecycleStage = "security"
	StageDeepSecurity  LifecycleStage = "deep-security"
	StageDeepBugSearch LifecycleStage = "deep-bug-search"
)

// ModelTier is the capability class a lifecycle stage needs. Templates name a
// tier, never a concrete model id: model names change far faster than the
// workflow, and the operator owns the tier -> model mapping.
type ModelTier string

// Model tiers.
const (
	TierStrongReasoning ModelTier = "strong-reasoning"
	TierFastCoding      ModelTier = "fast-coding"
	TierLongContext     ModelTier = "long-context"
)

// Template is one embedded scaffolding template.
type Template struct {
	ID          string       `json:"id"` // e.g. "agents-golang"
	Kind        TemplateKind `json:"kind"`
	Language    string       `json:"language,omitempty"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	TargetPath  string       `json:"targetPath"` // suggested path, e.g. "AGENTS.md"
	MIMEType    string       `json:"mimeType"`
	Body        string       `json:"-"`
}

//go:embed prompts/lifecycle_architect.md
var lifecycleArchitectPrompt string

//go:embed prompts/lifecycle_develop.md
var lifecycleDevelopPrompt string

//go:embed prompts/lifecycle_review.md
var lifecycleReviewPrompt string

//go:embed prompts/lifecycle_security.md
var lifecycleSecurityPrompt string

//go:embed prompts/lifecycle_deep_security.md
var lifecycleDeepSecurityPrompt string

//go:embed prompts/lifecycle_deep_bug_search.md
var lifecycleDeepBugSearchPrompt string

//go:embed prompts/bootstrap.md
var bootstrapPrompt string

//go:embed templates/agents_golang.md
var agentsGolangTemplate string

//go:embed templates/agents_python.md
var agentsPythonTemplate string

//go:embed templates/agents_node.md
var agentsNodeTemplate string

//go:embed templates/agents_java.md
var agentsJavaTemplate string

//go:embed templates/agents_rust.md
var agentsRustTemplate string

//go:embed templates/lifecycle_workflow.md
var lifecycleWorkflowTemplate string

//go:embed templates/opencode_config.json
var openCodeConfigTemplate string

// lifecyclePrompts maps each stage to its embedded persona.
var lifecyclePrompts = map[LifecycleStage]string{
	StageArchitect:     lifecycleArchitectPrompt,
	StageDevelop:       lifecycleDevelopPrompt,
	StageReview:        lifecycleReviewPrompt,
	StageSecurity:      lifecycleSecurityPrompt,
	StageDeepSecurity:  lifecycleDeepSecurityPrompt,
	StageDeepBugSearch: lifecycleDeepBugSearchPrompt,
}

// lifecycleTiers maps each stage to its model tier.
var lifecycleTiers = map[LifecycleStage]ModelTier{
	StageArchitect:     TierStrongReasoning,
	StageDevelop:       TierFastCoding,
	StageReview:        TierStrongReasoning,
	StageSecurity:      TierStrongReasoning,
	StageDeepSecurity:  TierLongContext,
	StageDeepBugSearch: TierLongContext,
}

// Templates returns every embedded template, sorted by ID.
func Templates() []Template {
	templates := []Template{
		{ID: "agents-golang", Kind: KindAgents, Language: "golang", Title: "AGENTS.md for Go", Description: "Go build, test, lint and security conventions.", TargetPath: "AGENTS.md", MIMEType: "text/markdown", Body: agentsGolangTemplate},
		{ID: "agents-python", Kind: KindAgents, Language: "python", Title: "AGENTS.md for Python", Description: "Python build, test, lint and security conventions.", TargetPath: "AGENTS.md", MIMEType: "text/markdown", Body: agentsPythonTemplate},
		{ID: "agents-node", Kind: KindAgents, Language: "node", Title: "AGENTS.md for Node.js", Description: "Node.js build, test, lint and security conventions.", TargetPath: "AGENTS.md", MIMEType: "text/markdown", Body: agentsNodeTemplate},
		{ID: "agents-java", Kind: KindAgents, Language: "java", Title: "AGENTS.md for Java", Description: "Java build, test, lint and security conventions.", TargetPath: "AGENTS.md", MIMEType: "text/markdown", Body: agentsJavaTemplate},
		{ID: "agents-rust", Kind: KindAgents, Language: "rust", Title: "AGENTS.md for Rust", Description: "Rust build, test, lint and security conventions.", TargetPath: "AGENTS.md", MIMEType: "text/markdown", Body: agentsRustTemplate},
		{ID: "lifecycle-workflow", Kind: KindLifecycle, Title: "Developer agent lifecycle", Description: "Architect, develop, review, security and deep-analysis stages with model tiers.", TargetPath: "docs/developer-lifecycle.md", MIMEType: "text/markdown", Body: lifecycleWorkflowTemplate},
		{ID: "opencode-config", Kind: KindOpenCode, Title: "opencode settings skeleton", Description: "A minimal opencode.json mapping model tiers and permissions.", TargetPath: "opencode.json", MIMEType: "application/json", Body: openCodeConfigTemplate},
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].ID < templates[j].ID })
	return templates
}

// TemplateByID returns one template.
func TemplateByID(id string) (Template, bool) {
	for _, tmpl := range Templates() {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return Template{}, false
}

// ScaffoldListParams filters the template catalog.
type ScaffoldListParams struct {
	Kind     string `json:"kind,omitempty" validate:"omitempty,oneof=agents lifecycle opencode" jsonschema:"(optional) Filter by kind: 'agents', 'lifecycle' or 'opencode'"`
	Language string `json:"language,omitempty" validate:"omitempty,alphanum,max=32" jsonschema:"(optional) Filter by language (e.g. 'golang')"`
}

// ScaffoldListOutput is the catalog.
type ScaffoldListOutput struct {
	Templates []Template `json:"templates"`
}

// ScaffoldGetParams selects one template.
type ScaffoldGetParams struct {
	ID string `json:"id" validate:"required,max=64" jsonschema:"(required) Template id from developer_scaffold_list"`
}

// ScaffoldGetOutput is one template with its body.
type ScaffoldGetOutput struct {
	Template Template `json:"template"`
	Body     string   `json:"body"`
}

// ScaffoldApplyParams materializes templates into a workspace.
type ScaffoldApplyParams struct {
	IDs   []string `json:"ids" validate:"required,min=1,dive,required,max=64" jsonschema:"(required) Template ids to write"`
	Owner string   `json:"owner" validate:"required,max=100" jsonschema:"(required) Repository owner selecting the workspace"`
	Repo  string   `json:"repo" validate:"required,max=100" jsonschema:"(required) Repository name selecting the workspace"`
	// Overwrite replaces an existing file. Default false: an existing target is
	// reported as skipped, never silently clobbered.
	Overwrite bool `json:"overwrite,omitempty" jsonschema:"(optional) Overwrite existing files (default false: existing targets are skipped)"`
	DryRun    bool `json:"dryRun,omitempty" jsonschema:"(optional) Report what would be written without writing"`
}

// ScaffoldApplyOutput reports the outcome per template.
type ScaffoldApplyOutput struct {
	Written []string `json:"written"`
	Skipped []string `json:"skipped"` // existed and Overwrite was false
	DryRun  bool     `json:"dryRun,omitempty"`
}

// scaffoldService holds the per-identity scaffolding tool state.
type scaffoldService struct {
	identityRoot string
}

// newScaffoldTools builds the three scaffolding tools for one identity.
func newScaffoldTools(identityRoot string) ([]tool.InvokableTool, error) {
	svc := &scaffoldService{identityRoot: identityRoot}
	listTool, err := utils.InferTool("developer_scaffold_list", scaffoldListDescription, svc.list)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build developer_scaffold_list")
	}
	getTool, err := utils.InferTool("developer_scaffold_get", scaffoldGetDescription, svc.get)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build developer_scaffold_get")
	}
	applyTool, err := utils.InferTool("developer_scaffold_apply", scaffoldApplyDescription, svc.apply)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build developer_scaffold_apply")
	}
	return []tool.InvokableTool{listTool, getTool, applyTool}, nil
}

const scaffoldListDescription = `
** General Purpose **
Lists the embedded developer scaffolding templates (AGENTS.md per language,
lifecycle workflow, opencode settings). Filter by kind and language.
`

const scaffoldGetDescription = `
** General Purpose **
Returns one scaffolding template with its full body. Use the id from
developer_scaffold_list.
`

const scaffoldApplyDescription = `
** General Purpose **
Writes scaffolding templates into the caller's repository workspace. Existing
files are skipped unless overwrite=true. Use dryRun=true to preview. This only
mutates the local workspace, never a remote.
`

func (s *scaffoldService) list(_ context.Context, params *ScaffoldListParams) (string, error) {
	if err := validate.Struct(params); err != nil {
		return "", err
	}
	var out []Template
	for _, tmpl := range Templates() {
		if params.Kind != "" && string(tmpl.Kind) != params.Kind {
			continue
		}
		if params.Language != "" && tmpl.Language != params.Language {
			continue
		}
		out = append(out, tmpl)
	}
	return marshalJSON(ScaffoldListOutput{Templates: out})
}

func (s *scaffoldService) get(_ context.Context, params *ScaffoldGetParams) (string, error) {
	if err := validate.Struct(params); err != nil {
		return "", err
	}
	tmpl, ok := TemplateByID(params.ID)
	if !ok {
		return "", errors.Wrapf(ErrTemplateNotFound, "id %q", params.ID)
	}
	return marshalJSON(ScaffoldGetOutput{Template: tmpl, Body: tmpl.Body})
}

func (s *scaffoldService) apply(_ context.Context, params *ScaffoldApplyParams) (string, error) {
	if err := validate.Struct(params); err != nil {
		return "", err
	}
	repoDir := repoWorkspacePath(s.identityRoot, params.Owner, params.Repo)
	selected := make([]Template, 0, len(params.IDs))
	for _, id := range params.IDs {
		tmpl, ok := TemplateByID(id)
		if !ok {
			return "", errors.Wrapf(ErrTemplateNotFound, "id %q", id)
		}
		selected = append(selected, tmpl)
	}
	out, err := applyTemplates(repoDir, selected, params.Overwrite, params.DryRun)
	if err != nil {
		return "", err
	}
	return marshalJSON(out)
}

// applyTemplates writes templates under repoDir. It is separate from the tool
// so tests can exercise a deliberately malformed target path.
func applyTemplates(repoDir string, templates []Template, overwrite, dryRun bool) (ScaffoldApplyOutput, error) {
	out := ScaffoldApplyOutput{DryRun: dryRun}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return out, errors.Wrapf(err, "failed to create workspace %q", repoDir)
	}
	for _, tmpl := range templates {
		target, err := workspace.ResolvePath(repoDir, tmpl.TargetPath, true)
		if err != nil {
			return out, errors.Wrapf(err, "invalid target path %q for template %q", tmpl.TargetPath, tmpl.ID)
		}
		if _, err := os.Stat(target); err == nil && !overwrite {
			out.Skipped = append(out.Skipped, tmpl.ID)
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return out, errors.Wrapf(err, "failed to stat target %q", target)
		}
		if !dryRun {
			if err := os.WriteFile(target, []byte(tmpl.Body), 0o644); err != nil {
				return out, errors.Wrapf(err, "failed to write template %q to %q", tmpl.ID, target)
			}
		}
		out.Written = append(out.Written, tmpl.ID)
	}
	return out, nil
}

// scaffoldPrompts returns the MCP prompts for the scaffolding surface.
func scaffoldPrompts() []mcpserver.PromptSpec {
	return []mcpserver.PromptSpec{
		{
			Name:        "developer_bootstrap",
			Title:       "Bootstrap the developer experience",
			Description: "Render the bootstrap instruction: which templates to apply and in what order.",
			Arguments: []mcpserver.PromptArgumentSpec{
				{Name: "language", Description: "Project language (e.g. golang)", Required: true},
				{Name: "projectName", Description: "Project name"},
			},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				return bootstrapPrompt + "\n\nLanguage: " + args["language"] + "\nProject: " + args["projectName"] + "\n", nil
			},
		},
		{
			Name:        "developer_review",
			Title:       "Review the change",
			Description: "Review / simplify persona for the review stage.",
			Arguments:   []mcpserver.PromptArgumentSpec{{Name: "scope", Description: "Review scope"}},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				return lifecycleReviewPrompt + "\n\nScope: " + args["scope"] + "\n", nil
			},
		},
		{
			Name:        "developer_security_audit",
			Title:       "Security audit",
			Description: "Security-review persona.",
			Arguments:   []mcpserver.PromptArgumentSpec{{Name: "depth", Description: "standard or deep"}},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				if args["depth"] == "deep" {
					return lifecycleDeepSecurityPrompt, nil
				}
				return lifecycleSecurityPrompt, nil
			},
		},
		{
			Name:        "developer_lifecycle",
			Title:       "Lifecycle stage persona",
			Description: "Return the stage persona and its model tier.",
			Arguments:   []mcpserver.PromptArgumentSpec{{Name: "stage", Description: "architect, develop, review, security, deep-security or deep-bug-search", Required: true}},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				stage := LifecycleStage(args["stage"])
				prompt, ok := lifecyclePrompts[stage]
				if !ok {
					return "", errors.Wrapf(ErrTemplateNotFound, "lifecycle stage %q", args["stage"])
				}
				return prompt + "\n\nModel tier: `" + string(lifecycleTiers[stage]) + "`\n", nil
			},
		},
	}
}

// scaffoldResources returns the MCP resources for the scaffolding surface.
func scaffoldResources() []mcpserver.ResourceSpec {
	resources := []mcpserver.ResourceSpec{{
		URI:         "developer://templates",
		Name:        "developer_templates",
		Title:       "Scaffolding template index",
		Description: "JSON index of every embedded scaffolding template.",
		MIMEType:    "application/json",
		Handler: func(_ context.Context, _ *mcpserver.Identity, _ string) (string, error) {
			return marshalJSON(ScaffoldListOutput{Templates: Templates()})
		},
	}}
	for _, tmpl := range Templates() {
		uri := "developer://" + string(tmpl.Kind) + "/" + tmpl.ID
		switch tmpl.Kind {
		case KindAgents:
			uri = "developer://agents/" + tmpl.Language
		case KindLifecycle:
			uri = "developer://lifecycle/workflow"
		case KindOpenCode:
			uri = "developer://opencode/config"
		}
		resources = append(resources, mcpserver.ResourceSpec{
			URI:         uri,
			Name:        "developer_" + tmpl.ID,
			Title:       tmpl.Title,
			Description: tmpl.Description,
			MIMEType:    tmpl.MIMEType,
			Handler: func(_ context.Context, _ *mcpserver.Identity, _ string) (string, error) {
				return tmpl.Body, nil
			},
		})
	}
	return resources
}
