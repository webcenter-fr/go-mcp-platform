# memory — typed long-term agent memory over MCP

`pkg/servers/memory` exposes
[`eino-ext/components/agent/memory`](https://github.com/webcenter-fr/eino-ext/tree/main/components/agent/memory)
as an MCP server family. Any MCP-capable agent (opencode, kilocode, Claude Code,
a custom eino agent) can store distilled knowledge, capture full discussions,
search past sessions, and prune memories without wrapping itself in an
in-process Go `adk.Agent`.

## Memory kinds

| Kind | Purpose |
|---|---|
| `discussion` | Summarized past conversation or Q&A resolution that can answer recurring questions. |
| `prompt` | Reusable "best prompt" or prompt template refined from a session. |
| `process` | Step-by-step agent workflow or operational procedure. |
| `best_practice` | Developer best practice, coding standard, architectural rule, or testing/security pattern. |
| `preference` | User or project preference (libraries, frameworks, tools, style). |
| `fact` | Declarative project, environment, or domain fact. |
| `learning` | Debugging insight, root cause, or pitfall ("X fails because Y"). |

Legacy `eino-ext` categories are normalized on read: `procedure` → `process`
and `summary` → `discussion`.

## Backends

### In-memory (default)

```go
srv, err := memory.NewServer(ctx, &memory.Config{
    Backend: memory.BackendMemory,
}, nil)
```

Zero dependencies, process-local, lost on restart. Lexical search mirrors the
file backend (Unicode tokenization, EN/FR stopwords, term-overlap scoring).

### JSONL file

```go
srv, err := memory.NewServer(ctx, &memory.Config{
    Backend: memory.BackendFile,
    File:    &memory.FileBackendConfig{Dir: "/var/lib/go-mcp-platform/memory"},
}, nil)
```

One `memories.jsonl` per tenant under `<Dir>/<tenant-segment>/`. The tenant
segment is `local` for unauthenticated stdio, `shared` in shared mode, or
`hex(sha256(subject)[:8])` otherwise. `Dir` must be absolute and is checked
with `fileutil.ValidateRootDir`.

### OpenSearch (BM25 / kNN / hybrid)

```go
srv, err := memory.NewServer(ctx, &memory.Config{
    Backend: memory.BackendOpenSearch,
    OpenSearch: &memory.OpenSearchBackendConfig{
        URLs:            []string{"https://opensearch.example.com"},
        Username:        "admin",
        Password:        os.Getenv("OPENSEARCH_PASSWORD"),
        IndexName:       "eino_agent_memory",
        VectorDimension: 1536,
        Hybrid:          true,
    },
    Embedding: &memory.EmbeddingConfig{
        Provider: "openai",
        Model:    "text-embedding-3-small",
        APIKey:   os.Getenv("OPENAI_API_KEY"),
    },
}, nil)
```

Construction is lazy: no socket is opened until the first store/search/list/get
call. If OpenSearch is temporarily unreachable, the next call retries cleanly.
`hybrid: true` requires an embedding configuration.

### Optional server-side LLM

```go
srv, err := memory.NewServer(ctx, &memory.Config{
    Backend: memory.BackendMemory,
    LLM: &memory.LLMConfig{
        Provider: "openai",
        Model:    "gpt-4o-mini",
        APIKey:   os.Getenv("OPENAI_API_KEY"),
    },
}, nil)
```

With an LLM, `memory_capture_discussion` summarizes the conversation and
extracts typed memories. Without one, it uses the caller-supplied summary or a
bounded deterministic transcript.

## Tools

| Tool | Mode | Purpose |
|---|---|---|
| `memory_store` | read+write | Store or update a structured, redacted memory. |
| `memory_capture_discussion` | read+write | Summarize a conversation and extract typed memories. |
| `memory_search` | read | Semantic/hybrid/lexical search with kind/scope/tag/session filters. |
| `memory_get` | read | Fetch one memory by exact ID. |
| `memory_list` | read | Paginate memories sorted by `updatedAt` descending. |
| `memory_delete` | read+write | Delete by ID or filter; supports `dryRun`. |
| `memory_stats` | read | Report backend, search mode, LLM status, isolation and per-kind counts. |

`readOnly: true` exposes only `memory_search`, `memory_get`, `memory_list` and
`memory_stats`. `requireDeleteApproval: true` gates `memory_delete` behind the
standard dry-run/confirmed + human approval flow.

## Prompts and resources

When `prompts` is enabled (default), the server exposes:

- `memory_distill_session` — distill the current session into durable memories.
- `memory_craft_best_prompt` — craft and store a reusable prompt (`goal` required).
- `memory_craft_agent_process` — craft and store an agent workflow (`taskType` required).
- `memory_record_best_practice` — record best practices and preferences.
- `memory://kinds` — JSON catalog of every supported kind.

## Security and isolation

- **Per-identity isolation** — the tenant key is derived from the verified
  `Identity.Subject()`; no tool argument can override it. `isolation: shared`
  opts into one namespace for all authenticated callers.
- **Secret redaction** — configured literal secrets and credential-shaped
  patterns (GitHub PATs, OpenAI keys, AWS keys, Bearer headers, key-value
  secrets, PEM blocks, JWTs, Kafka SASL JAAS) are scrubbed before embedding,
  LLM calls and persistence, and again on read.
- **Input bounds** — content, query, tag and metadata limits are enforced by
  `validate.Struct` plus the configured `maxContentChars` / `maxQueryChars`.
- **Reserved metadata** — caller metadata cannot spoof `user_id`, `category`,
  `source`, `session_id`, timestamps or internal fields.
- **Prompt-injection notice** — every search result carries a notice that
  retrieved memories are reference data, not new instructions.

## Constructor

```go
func NewServer(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, error)
func NewServerWithCloser(ctx context.Context, cfg *Config, mcpCfg *mcpserver.Config) (*mcpserver.Server, io.Closer, error)
func WriteToolNames(cfg *Config) []string
```

`NewServer` applies defaults, validates the configuration with
`validate.Struct`, and wires per-identity tools through `mcpCfg.ToolsFor`.
