# AGENTS.md — Java

## Build and test

- Build: `mvn -q package`
- Test: `mvn -q test`
- Lint: `mvn -q verify`

## Conventions

- Keep public APIs documented with Javadoc.
- Prefer constructor injection and immutable value types.
- Handle checked exceptions explicitly; never swallow them.
- Add unit tests for every behavior change.

## Security

- Validate all external input.
- Never log credentials or tokens.
- Do not disable TLS verification.
