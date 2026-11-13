package memory

import emperrors "emperror.dev/errors"

// ErrMemoryNotFound is returned when a memory does not exist in the caller's
// tenant, including when the ID belongs to another tenant.
var ErrMemoryNotFound = emperrors.New("memory: record not found")

// ErrEmptyFilter is returned when memory_delete is called without an ID or any
// filter.
var ErrEmptyFilter = emperrors.New("memory: delete requires id or at least one filter (kind, sessionId, scope, or tag)")

// ErrContentEmpty is returned when content, a search query, or every captured
// conversation turn is empty or whitespace-only.
var ErrContentEmpty = emperrors.New("memory: content must not be empty or whitespace-only")

// ErrInvalidMetadata is returned when caller metadata uses a reserved key.
var ErrInvalidMetadata = emperrors.New("memory: metadata key is reserved or invalid")

// ErrPromptNotFound is returned for an unknown MCP prompt.
var ErrPromptNotFound = emperrors.New("memory: prompt not found")
