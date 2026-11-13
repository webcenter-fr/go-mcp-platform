# AGENTS.md — Rust

## Build and test

- Build: `cargo build`
- Test: `cargo test`
- Lint: `cargo clippy -- -D warnings`
- Format: `cargo fmt --check`

## Conventions

- Prefer `Result` over panics in library code.
- Document public items with `///`.
- Keep unsafe blocks minimal and justified.
- Add tests for every behavior change.

## Security

- Validate all external input.
- Never log secrets.
- Do not disable certificate verification.
