package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"emperror.dev/errors"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/sirupsen/logrus"

	memoryagent "github.com/webcenter-fr/eino-ext/components/agent/memory"
	"github.com/webcenter-fr/eino-ext/components/model/chatmodel"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/strutil"
)

// extractTypedMemoriesPrompt is the system prompt for server-side typed
// extraction.
//
//go:embed prompts/extract_typed_memories.md
var extractTypedMemoriesPrompt string

// Capturer summarizes a conversation and optionally extracts typed memories.
type Capturer interface {
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResult, error)
	Enabled() bool
}

// CaptureRequest is the input of a capture operation.
type CaptureRequest struct {
	Messages     []ConversationMessage
	Summary      string
	ExtractTyped bool
}

// ExtractedItem is one typed memory proposed by the extraction LLM.
type ExtractedItem struct {
	Kind       MemoryKind `json:"kind"`
	Title      string     `json:"title,omitempty"`
	Content    string     `json:"content"`
	Scope      string     `json:"scope,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Source     string     `json:"source,omitempty"`
	Confidence float64    `json:"confidence"`
}

// CaptureResult is the output of a capture operation.
type CaptureResult struct {
	Mode      string
	Summary   string
	Extracted []ExtractedItem
}

// NewCapturer builds the capture implementation. A configured chat model (or
// LLM config) enables server-side summarization and typed extraction; otherwise
// the deterministic transcript fallback is used.
func NewCapturer(ctx context.Context, cfg *Config) (Capturer, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.ChatModel != nil {
		return newLLMCapturer(cfg, cfg.ChatModel), nil
	}
	if cfg.LLM != nil {
		built, err := chatmodel.New(ctx, &chatmodel.Config{
			Provider:        chatmodel.Provider(cfg.LLM.Provider),
			BaseURL:         cfg.LLM.BaseURL,
			Model:           cfg.LLM.Model,
			Temperature:     cfg.LLM.Temperature,
			Thinking:        chatmodel.ThinkingLevel(cfg.LLM.Thinking),
			MaxOutputTokens: cfg.LLM.MaxOutputTokens,
			Timeout:         cfg.LLM.Timeout,
			TLSSkipVerify:   cfg.LLM.TLSSkipVerify,
			APIKey:          cfg.LLM.APIKey,
		})
		if err != nil {
			return nil, errors.Wrap(err, "memory: failed to build chat model")
		}
		return newLLMCapturer(cfg, built), nil
	}
	return &transcriptCapturer{cfg: cfg}, nil
}

// llmCapturer uses an eino-ext Extractor for summarization and a dedicated
// prompt for typed extraction.
type llmCapturer struct {
	cfg       *Config
	model     model.BaseChatModel
	extractor *memoryagent.Extractor
}

func newLLMCapturer(cfg *Config, chatModel model.BaseChatModel) *llmCapturer {
	return &llmCapturer{
		cfg:       cfg,
		model:     chatModel,
		extractor: memoryagent.NewExtractor(chatModel),
	}
}

// Enabled reports that server-side LLM capture is available.
func (c *llmCapturer) Enabled() bool {
	return c.model != nil
}

// Capture summarizes the conversation and, when requested, extracts typed
// memories. A malformed extraction response degrades to the summary instead of
// failing the whole capture.
func (c *llmCapturer) Capture(ctx context.Context, req CaptureRequest) (*CaptureResult, error) {
	result := &CaptureResult{}

	if summary := strings.TrimSpace(req.Summary); summary != "" {
		result.Summary = summary
		result.Mode = "provided_summary"
	} else {
		messages := make([]*schema.Message, 0, len(req.Messages))
		for _, message := range req.Messages {
			messages = append(messages, &schema.Message{
				Role:    schema.RoleType(message.Role),
				Content: message.Content,
			})
		}
		summary, err := c.extractor.Summarize(ctx, messages, "")
		if err != nil {
			return nil, errors.Wrap(err, "failed to summarize discussion")
		}
		result.Summary = strings.TrimSpace(summary)
		result.Mode = "llm_summary"
	}

	if req.ExtractTyped {
		extracted, err := c.extractTyped(ctx, req.Messages)
		if err != nil {
			logrus.WithError(err).Warn("memory: typed extraction failed; storing the discussion summary only")
		} else if len(extracted) > 0 {
			result.Extracted = extracted
			result.Mode = "llm_extracted"
		}
	}

	return result, nil
}

// extractTyped asks the chat model for typed memories and filters low-confidence
// or invalid items.
func (c *llmCapturer) extractTyped(ctx context.Context, messages []ConversationMessage) ([]ExtractedItem, error) {
	var transcript strings.Builder
	for _, message := range messages {
		fmt.Fprintf(&transcript, "[%s]: %s\n", message.Role, strutil.Truncate(message.Content, 2000, "..."))
	}

	response, err := c.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(extractTypedMemoriesPrompt),
		schema.UserMessage(transcript.String()),
	})
	if err != nil {
		return nil, errors.Wrap(err, "typed extraction LLM call")
	}

	block := strutil.ExtractJSONBlock(response.Content)
	if block == "" {
		return nil, nil
	}
	var items []ExtractedItem
	if err := json.Unmarshal([]byte(block), &items); err != nil {
		return nil, errors.Wrap(err, "parse typed extraction JSON")
	}

	filtered := make([]ExtractedItem, 0, len(items))
	for _, item := range items {
		if item.Confidence < 0.7 || strings.TrimSpace(item.Content) == "" {
			continue
		}
		kind, ok := NormalizeKind(string(item.Kind))
		if !ok {
			continue
		}
		item.Kind = kind
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// transcriptCapturer is the deterministic fallback used when no server-side LLM
// is configured.
type transcriptCapturer struct {
	cfg *Config
}

// Enabled reports that server-side LLM capture is unavailable.
func (c *transcriptCapturer) Enabled() bool {
	return false
}

// Capture uses the caller-supplied summary or a bounded transcript.
func (c *transcriptCapturer) Capture(_ context.Context, req CaptureRequest) (*CaptureResult, error) {
	if summary := strings.TrimSpace(req.Summary); summary != "" {
		return &CaptureResult{Mode: "provided_summary", Summary: summary}, nil
	}
	return &CaptureResult{
		Mode:    "transcript",
		Summary: deterministicTranscript(req.Messages, c.cfg.MaxContentChars),
	}, nil
}

// deterministicTranscript renders a bounded, deterministic transcript.
func deterministicTranscript(messages []ConversationMessage, maxChars int) string {
	var transcript strings.Builder
	used := 0
	for _, message := range messages {
		line := fmt.Sprintf("[%s]: %s\n", message.Role, strutil.Truncate(message.Content, 2000, "..."))
		lineLength := utf8.RuneCountInString(line)
		if maxChars > 0 {
			remaining := maxChars - used
			if remaining <= 0 {
				break
			}
			if lineLength > remaining {
				transcript.WriteString(strutil.Truncate(line, remaining, "..."))
				break
			}
		}
		transcript.WriteString(line)
		used += lineLength
	}
	return strings.TrimSpace(transcript.String())
}
