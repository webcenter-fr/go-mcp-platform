# Rules

- Always follow the CONTRIBUTING.md
- Run tests with `make test` (plain `go test -v ./...`). No external test
  binaries or cluster assets are needed.
- Use `make lint` (golangci-lint) and `bash scripts/check_components.sh` for
  the other gates, as the CI does.
- Technical design docs live in `docs/design/` in English and French
  (`*.en.md` / `*.fr.md`) with mermaid diagrams. Keep them in sync when you add
  or change a component, middleware, callback, or lib.

## Layering

Dependencies point inward and never reverse:

```
cmd/  ->  internal/  ->  pkg/  ->  github.com/webcenter-fr/eino-ext
```

- `cmd/` holds thin entry points (`cmd/mcp-server`, `cmd/approval-server`).
- `internal/` is private implementation (`serverconfig`, `approval`, `logging`,
  `secretref`).
- `pkg/` is the public API (`pkg/mcp`, `pkg/mcp/approval`, `pkg/servers/*`).
- `pkg/` must not import `internal/` or `cmd/`.
- The platform consumes `github.com/webcenter-fr/eino-ext` as a normal module
  dependency. Never add a reverse dependency from eino-ext to this repository.

## Documentation

- Design documents are in `docs/design/` (see `docs/design/README.md`), written
  in English and French, with mermaid for schemas/flows.
- Component-level usage docs stay in each package's `README.md`.
- When you add or change a component covered by a design doc, update the doc
  (both languages) in the same change.

## Code Quality Standards

When writing or modifying code in this repository, prioritize:

1. **Maintainability**: Minimize code duplication. Extract common patterns into shared helpers under `internal/` (or the eino-ext module when the logic is reusable across repositories). Use generics when the same logic applies to different types.

2. **Comprehensibility**: Write code that is easy for humans to read. Use descriptive names, add clear comments for complex logic, and follow Go naming conventions (e.g., `ToJSON` not `ToJson`, `URL` not `Url`, `OpenSearch` not `Opensearch`, `GitHub` not `Github`, `GitLab`, `PostgreSQL`, `gRPC`, `API`, `HTTP`).

3. **Usability**: This is a deployable platform consumed by operators and MCP clients. Provide:
   - Factory functions to create all servers for a family at once
   - Clear interfaces for tool families
   - Default implementations for common patterns
   - Builder patterns for complex configurations

4. **Interfaces and Implementations**: When a pattern repeats across multiple tools:
   - Define an interface that captures the common behavior
   - Provide a default/generic implementation
   - Allow customization through configuration

5. **Security**: Before implementing tools that execute commands or access external systems:
   - Validate all inputs (URLs, paths, commands)
   - Implement robust blocklists that cannot be easily bypassed
   - Add timeouts for external API calls
   - Redact sensitive data in outputs
   - Test security controls with bypass attempts

## Before Making Changes

- Check if the pattern already exists in `internal/` or in the eino-ext module
- Look for existing interfaces that the new code should implement
- Verify there are no security implications (command injection, SSRF, data leaks)
- Ensure the change improves code clarity and reduces duplication

## Common Pitfalls to Avoid

- **Duplicating helper functions**: If a helper already exists in `internal/` or in the eino-ext module, import it instead of reimplementing.
- **Weak security controls**: Blocklists like `^\s*rm\s` are bypassed with `/bin/rm` or `./rm`. Use word boundaries and multiple patterns.
- **Missing error context**: Always wrap errors with `emperror.dev/errors` to include context about what operation failed.
- **Inconsistent naming**: Follow Go conventions and existing patterns in the codebase.
- **Validation tags without the call**: Adding `validate`/`jsonschema` tags to a `Config` struct but never calling `validate.Struct(cfg)` in the constructor. Every `New...` must call `libs/toolkit/validate.Struct` from the eino-ext module after applying defaults.
- **Component shipped without test or README**: Every component package needs `xxx_test.go` and `README.md`. Without them the component is incomplete.
