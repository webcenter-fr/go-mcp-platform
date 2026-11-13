package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"emperror.dev/errors"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

//go:embed prompts/distill_session.md
var distillSessionPrompt string

//go:embed prompts/craft_best_prompt.md
var craftBestPromptPrompt string

//go:embed prompts/craft_agent_process.md
var craftAgentProcessPrompt string

//go:embed prompts/record_best_practice.md
var recordBestPracticePrompt string

// Prompts returns the MCP prompts exposed by the memory family.
func Prompts() []mcpserver.PromptSpec {
	return []mcpserver.PromptSpec{
		{
			Name:        "memory_distill_session",
			Title:       "Distill the current session into long-term memory",
			Description: "Review the conversation, store a discussion summary, and record durable preferences, best practices, processes and prompts.",
			Arguments: []mcpserver.PromptArgumentSpec{
				{Name: "scope", Description: "Environment or project scope (e.g. 'repo:org/name')"},
				{Name: "sessionId", Description: "Session correlation ID"},
			},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				return renderPrompt(distillSessionPrompt, args), nil
			},
		},
		{
			Name:        "memory_craft_best_prompt",
			Title:       "Craft and store a reusable prompt",
			Description: "Synthesize a self-contained, reusable prompt template from the current session and store it as a prompt memory.",
			Arguments: []mcpserver.PromptArgumentSpec{
				{Name: "goal", Description: "What the prompt should accomplish", Required: true},
				{Name: "scope", Description: "Environment or project scope"},
			},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				if err := requireArgs(args, "goal"); err != nil {
					return "", err
				}
				return renderPrompt(craftBestPromptPrompt, args), nil
			},
		},
		{
			Name:        "memory_craft_agent_process",
			Title:       "Craft and store an agent process",
			Description: "Write an ordered, verifiable agent workflow for a class of tasks and store it as a process memory.",
			Arguments: []mcpserver.PromptArgumentSpec{
				{Name: "taskType", Description: "The class of task the process handles", Required: true},
				{Name: "scope", Description: "Environment or project scope"},
			},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				if err := requireArgs(args, "taskType"); err != nil {
					return "", err
				}
				return renderPrompt(craftAgentProcessPrompt, args), nil
			},
		},
		{
			Name:        "memory_record_best_practice",
			Title:       "Record best practices and preferences",
			Description: "Capture concrete engineering conventions and preferred libraries/tools from the current session.",
			Arguments: []mcpserver.PromptArgumentSpec{
				{Name: "language", Description: "Programming language or ecosystem"},
				{Name: "topic", Description: "Topic or area of practice"},
			},
			Handler: func(_ context.Context, _ *mcpserver.Identity, args map[string]string) (string, error) {
				return renderPrompt(recordBestPracticePrompt, args), nil
			},
		},
	}
}

// Resources returns the MCP resources exposed by the memory family.
func Resources() []mcpserver.ResourceSpec {
	return []mcpserver.ResourceSpec{{
		URI:         "memory://kinds",
		Name:        "memory_kinds",
		Title:       "Memory kind catalog",
		Description: "JSON catalog of every supported memory kind, its description and a concrete example.",
		MIMEType:    "application/json",
		Handler: func(_ context.Context, _ *mcpserver.Identity, _ string) (string, error) {
			payload, err := json.Marshal(KindCatalog())
			if err != nil {
				return "", errors.Wrap(err, "failed to marshal memory kind catalog")
			}
			return string(payload), nil
		},
	}}
}

// requireArgs rejects a prompt call that omits a required argument.
func requireArgs(args map[string]string, names ...string) error {
	for _, name := range names {
		if strings.TrimSpace(args[name]) == "" {
			return errors.Errorf("memory: prompt argument %q is required", name)
		}
	}
	return nil
}

// renderPrompt substitutes {{name}} placeholders and appends the optional
// scope/session context when supplied.
func renderPrompt(template string, args map[string]string) string {
	rendered := template
	for key, value := range args {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	var context strings.Builder
	if scope := strings.TrimSpace(args["scope"]); scope != "" {
		context.WriteString("\nScope: " + scope + "\n")
	}
	if sessionID := strings.TrimSpace(args["sessionId"]); sessionID != "" {
		context.WriteString("Session ID: " + sessionID + "\n")
	}
	if language := strings.TrimSpace(args["language"]); language != "" {
		context.WriteString("Language: " + language + "\n")
	}
	if topic := strings.TrimSpace(args["topic"]); topic != "" {
		context.WriteString("Topic: " + topic + "\n")
	}
	if context.Len() > 0 {
		rendered += "\n" + context.String()
	}
	return rendered
}
