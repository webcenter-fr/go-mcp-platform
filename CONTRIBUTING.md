# Contributing to go-mcp-platform

Thanks for contributing! This document captures repository conventions that are
not obvious from the code alone.

## Project structure

The repository uses a clean-architecture layout:

- `cmd/` — entry points (`cmd/mcp-server`, `cmd/approval-server`). Thin `main`
  packages: parse flags/config, build, run, shut down.
- `internal/` — private implementation: `internal/serverconfig` (binary wiring),
  `internal/approval` (approval service, store backends and the embedded Nuxt
  UI), `internal/logging`, `internal/secretref`.
- `pkg/` — public API: `pkg/mcp` (generic MCP toolkit), `pkg/mcp/approval`
  (approval-service client), `pkg/servers/*` (embeddable per-family servers).
- `helm/go-mcp-platform/` — the Helm chart.
- `.dagger/` — the Dagger CI module. It is a nested Go module and is
  intentionally invisible to the repository's `go build`/`go vet`/golangci-lint.
- `docs/design/` — bilingual technical design documents.

Dependencies point inward and never reverse:

```
cmd/  ->  internal/  ->  pkg/  ->  github.com/webcenter-fr/eino-ext
```

- `pkg/` must not import `internal/` or `cmd/`.
- `internal/` may import `pkg/`.
- Both may import the eino-ext module.
- Never add a reverse dependency from eino-ext to this repository.

### Consuming eino-ext

The platform consumes `github.com/webcenter-fr/eino-ext` as a normal Go module
dependency. The seven packages imported across the boundary are
`components/tool/{kubernetes,argocd,prometheus,grafana}` and
`libs/toolkit/{safety,validate,osclient}`.

For local development against a sibling checkout, create a **gitignored**
`go.work` at the repository root:

```
go 1.26.3

use .
use ../eino-ext
```

Never commit a `replace github.com/webcenter-fr/eino-ext => ../eino-ext`
directive: CI runners have no sibling checkout, and a committed `replace` makes
the published module unusable for downstream consumers.

## Prompts

Prefer keeping LLM prompt text in dedicated Markdown files and embedding them
into the Go package with `//go:embed`, rather than inlining large multi-line
string literals in `.go` source.

This keeps prompts readable, diff-friendly, and editable without touching Go
code, while still compiling them into the binary (no runtime file I/O).

### Conventions

- Store prompt files under a `prompts/` directory inside the component package.
- Use the `.md` extension and one prompt per file.
- Embed each prompt into an exported (or package) `string` variable using
  `//go:embed`, and import the blank `embed` package:

  ```go
  import _ "embed"

  // DefaultSummaryTemplate is the anchored Markdown summary template.
  //
  //go:embed prompts/summary_template.md
  var DefaultSummaryTemplate string
  ```

- Keep the variable doc comment describing what the prompt is for; the file
  itself holds the prompt content.
- If a prompt must be parameterized, embed the static template and assemble the
  dynamic parts in Go (do not build prompts by string-concatenating many small
  embedded fragments).

### Rationale

- Prompts are content, not code: editing them should not require Go expertise.
- `//go:embed` keeps everything in a single binary with no external file
  dependencies at runtime.
- Markdown renders nicely in reviews and editors.

## String formatting

Prefer `fmt.Sprintf` over concatenating strings with `+`. It is more readable,
easier to maintain, and keeps format and arguments clearly separated.

```go
// Preferred
msg := fmt.Sprintf("user %s has %d items", name, count)

// Avoid
msg := "user " + name + " has " + strconv.Itoa(count) + " items"
```

## Components

- Follow the existing component layout: a `Config` struct with `validate` and
  `jsonschema` tags, a `New...` constructor, `emperror.dev/errors` for error
  wrapping, and the shared validation helper.

- Every `New...` constructor MUST validate its config by calling the shared
  helper `github.com/webcenter-fr/eino-ext/libs/toolkit/validate`:

  ```go
  import "github.com/webcenter-fr/eino-ext/libs/toolkit/validate"

  func NewXxx(ctx context.Context, cfg *Config) (*Xxx, error) {
      if cfg == nil {
          cfg = &Config{}
      }
      // ... apply defaults ...
      if err := validate.Struct(cfg); err != nil {
          return nil, err   // already wrapped by the helper
      }
      ...
  }
  ```

- Do NOT instantiate `validator.New()` directly or write ad hoc manual checks
  (`if len(cfg.URLs) == 0`): declare the constraint in the struct tag
  (`validate:"..."`) and let `validate.Struct` enforce it.
- Call `validate.Struct` AFTER applying defaults, so validation runs against
  the final values. Do not use `required` on a field that receives a default
  (use `omitempty,gte=1` instead).

### Constructor Context

- Every `New...` constructor for a component or shared helper that creates
  remote clients (e.g. `NewClient`, `BuildClients`, `newBaseTool`,
  `osclient.New`) MUST accept `ctx context.Context` as its **first** parameter.
- Thread `ctx` through from the top-level constructor down to every
  underlying client-creation call. Even when the underlying library does
  not yet require a context, having it in the signature keeps the API
  future-proof and consistent across the codebase.

- A component is considered complete only when ALL of the following exist:
  1. `xxx.go` with `Config` (tags `validate`+`jsonschema`), `New...`, and a
     compile-time interface check where an interface applies.
  2. `xxx_test.go`: table-driven tests covering the normal case, parameter
     errors, and (with mocks) external dependencies. No test should depend on a
     live external service.
  3. `README.md`: what the component does and a constructor snippet.
  4. A package comment `// Package xxx ...` at the top of the file.

### OpenSearch store

The approval service has one OpenSearch-backed store
(`internal/approval/store/opensearch`). It uses
`github.com/disaster37/opensearch/v4` for the full client (scroll,
delete-by-query, put-mapping, querydsl). Keep the store behind the
`store.ApprovalStore` interface and run the shared `storetest` conformance
suite.

## Containers, Helm and Dagger

- **Images**: `Dockerfile.mcp` and `Dockerfile.approval` pin every base image
  by tag **and** digest. The runtime image is `distroless/static-debian12:nonroot`
  (UID 65532, no shell), so health probes must be HTTP `httpGet`. Images publish
  to `ghcr.io/webcenter-fr/go-mcp-platform/{mcp,approval}`.
- **Helm**: `helm/go-mcp-platform` keeps per-family resources in
  `templates/mcp/` and the approval service in `templates/approval/`.
  `values.yaml` carries `## @param` annotations for
  `readme-generator-for-helm`; `values.schema.json` and `README.md` are
  generated (`make helm-schema`, `make helm-docs`) and checked for drift in CI.
  Keep RBAC read-only by default; `allowWrite`/`allowExec` are dangerous
  opt-ins.
- **Dagger**: the module is pinned to Dagger `v0.21.10` and the
  `disaster37/dagger-library-go@2.0.20` dependencies to commit
  `e205189be6eba617e7bcc870db2e73a04171f38e`. Never call
  `dag.Golang().Ci()`: it runs `gofumpt -w` over the whole repository. Use the
  module's own `lint`/`test` functions instead.

## Documentation

- **Technical design docs** live in `docs/design/` (see `docs/design/README.md`).
  They are written in **English and French** (`*.en.md` / `*.fr.md`) in a
  technical style, and use **mermaid** diagrams for schemas and flows. The
  English version is the reference; keep both in sync.
- **Component-level usage docs** stay in each package's `README.md`.
- When you add or change a component, middleware, callback, or lib covered by a
  design doc, update the doc (both languages) in the same change.

## License Headers

Do **NOT** add license banner comments to source files. The repository already
has a `LICENSE` file at the root, which covers all code. Duplicating it in every
file adds noise and maintenance overhead with no legal benefit.

```go
// DO NOT do this:
/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * ...
 */

// Just start with the package declaration:
package mypackage
```

## Validation

Before opening a pull request:

```bash
go build ./...
go vet ./...
make test
make lint
bash scripts/check_components.sh
```

`make test` is plain `go test -v ./...`. No external test binaries or cluster
assets are needed. The redis, postgresql and opensearch store tests skip cleanly
when `REDIS_ADDR`, `POSTGRES_DSN` and `OPENSEARCH_URL` are unset.

## Naming Conventions

- Follow Go naming conventions: `ToJSON` not `ToJson`, `ID` not `Id`, `URL`
  not `Url`, **`OpenSearch` not `Opensearch`, `GitHub` not `Github`, `GitLab`,
  `PostgreSQL`, `gRPC`, `API`, `HTTP`**. When in doubt, use the official
  casing of the product for all exported identifiers and `GetType()` return
  values.
- Use descriptive names that reveal intent: `ApplicationListOutput` is better than `Output`.
- Keep parameter struct names consistent: `XxxParams` for input, `XxxOutput` for output.

#### Provider and product names in configuration strings

Configuration string values that name a provider, product, or service (e.g.
`Plan`, `Provider`) must use the official full name in lowercase kebab-case, not
an abbreviation:

```go
// Good: official name, full form
Plan: "github-copilot"

// Bad: abbreviation, not the canonical name
Plan: "copilot"
```

This keeps configuration consistent with the official product naming and avoids
ambiguous abbreviations.

### Error Handling

- **Wrap errors** with context using `emperror.dev/errors` to help users debug issues.
- **Validate parameters early** and return clear error messages.
- **Don't panic** in tool implementations; return errors instead.

### Testing

- **Write table-driven tests** for tool invocations with various parameter combinations.
- **Test error cases** including invalid parameters, network failures, and permission errors.
- **Use mocks** for external dependencies to ensure tests are fast and reliable.

## Checklist before PR

- [ ] `go build ./...`, `go vet ./...`, `make test`, `make lint` and
      `bash scripts/check_components.sh` pass.
- [ ] Every new `Config` has `validate`+`jsonschema` tags AND its `New...`
      calls `validate.Struct(cfg)` after defaults.
- [ ] Every new component has: table-driven test, README, and a package
      comment.
- [ ] Every `New...` constructor accepts `ctx context.Context` as its first
      parameter and threads it through to all client creation calls.
- [ ] Naming: acronyms and brands use official casing (`OpenSearch`, `GitHub`,
      `URL`, `ID`, `JSON`).
- [ ] Errors are wrapped with `emperror.dev/errors` (include operation context).
- [ ] No license banner added.
- [ ] Dependencies point inward (`cmd -> internal -> pkg -> eino-ext`); no
      reverse edge.
- [ ] No committed `replace` directive for eino-ext.
- [ ] No duplication of helpers already present in `internal/` or the eino-ext
      module.
