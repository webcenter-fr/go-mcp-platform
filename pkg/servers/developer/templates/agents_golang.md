# AGENTS.md — Go

## Build and test

- Build: `go build ./...`
- Vet: `go vet ./...`
- Test: `go test ./...`
- Lint: `golangci-lint run ./...`

## Conventions

- Dependencies point inward; never import a higher layer from a lower one.
- Wrap errors with `emperror.dev/errors` and include operation context.
- Every exported type and function has a doc comment.
- Table-driven tests; no live external services in unit tests.
- Do not add license banners.

## Security

- Validate every external input.
- Never log secrets or tokens.
- Use argv execution, never a shell string built from user input.
