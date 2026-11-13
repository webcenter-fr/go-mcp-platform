package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMCapturerSummarizeAndExtract(t *testing.T) {
	model := &fakeChatModel{responses: []string{
		"the discussion summary",
		"```json\n[" +
			`{"kind":"preference","content":"prefer table tests","confidence":0.9},` +
			`{"kind":"fact","content":"low confidence","confidence":0.2},` +
			`{"kind":"unknown","content":"invalid kind","confidence":0.9}` +
			"]\n```",
	}}
	capturer := newLLMCapturer(&Config{MaxContentChars: DefaultMaxContentChar}, model)

	result, err := capturer.Capture(context.Background(), CaptureRequest{
		Messages: []ConversationMessage{
			{Role: "user", Content: "how should we test?"},
			{Role: "assistant", Content: "with table-driven tests"},
		},
		ExtractTyped: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "llm_extracted", result.Mode)
	assert.Equal(t, "the discussion summary", result.Summary)
	require.Len(t, result.Extracted, 1)
	assert.Equal(t, KindPreference, result.Extracted[0].Kind)
	assert.Equal(t, "prefer table tests", result.Extracted[0].Content)
}

func TestLLMCapturerMalformedExtractionDegrades(t *testing.T) {
	model := &fakeChatModel{responses: []string{
		"the discussion summary",
		"not json at all",
	}}
	capturer := newLLMCapturer(&Config{MaxContentChars: DefaultMaxContentChar}, model)

	result, err := capturer.Capture(context.Background(), CaptureRequest{
		Messages:     []ConversationMessage{{Role: "user", Content: "hello"}},
		ExtractTyped: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "llm_summary", result.Mode)
	assert.Equal(t, "the discussion summary", result.Summary)
	assert.Empty(t, result.Extracted)
}

func TestLLMCapturerSummarizeFailure(t *testing.T) {
	model := &fakeChatModel{err: errors.New("transport down")}
	capturer := newLLMCapturer(&Config{MaxContentChars: DefaultMaxContentChar}, model)

	_, err := capturer.Capture(context.Background(), CaptureRequest{
		Messages: []ConversationMessage{{Role: "user", Content: "hello"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "summarize")
}

func TestLLMCapturerProvidedSummary(t *testing.T) {
	model := &fakeChatModel{responses: []string{"[]"}}
	capturer := newLLMCapturer(&Config{MaxContentChars: DefaultMaxContentChar}, model)

	result, err := capturer.Capture(context.Background(), CaptureRequest{
		Messages:     []ConversationMessage{{Role: "user", Content: "hello"}},
		Summary:      "caller summary",
		ExtractTyped: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "provided_summary", result.Mode)
	assert.Equal(t, "caller summary", result.Summary)
}

func TestTranscriptCapturer(t *testing.T) {
	capturer := &transcriptCapturer{cfg: &Config{MaxContentChars: 100}}
	assert.False(t, capturer.Enabled())

	result, err := capturer.Capture(context.Background(), CaptureRequest{
		Messages: []ConversationMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "transcript", result.Mode)
	assert.Contains(t, result.Summary, "[user]: hello")
	assert.Contains(t, result.Summary, "[assistant]: world")

	result, err = capturer.Capture(context.Background(), CaptureRequest{
		Messages: []ConversationMessage{{Role: "user", Content: "hello"}},
		Summary:  "provided",
	})
	require.NoError(t, err)
	assert.Equal(t, "provided_summary", result.Mode)
	assert.Equal(t, "provided", result.Summary)
}

func TestNewCapturerWithoutLLM(t *testing.T) {
	capturer, err := NewCapturer(context.Background(), &Config{})
	require.NoError(t, err)
	assert.False(t, capturer.Enabled())
}

func TestNewCapturerWithInjectedModel(t *testing.T) {
	capturer, err := NewCapturer(context.Background(), &Config{ChatModel: &fakeChatModel{}})
	require.NoError(t, err)
	assert.True(t, capturer.Enabled())
}
