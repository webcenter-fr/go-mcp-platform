# AGENTS.md — Node.js

## Build and test

- Install: `npm ci`
- Test: `npm test`
- Lint: `npm run lint`
- Build: `npm run build`

## Conventions

- Use TypeScript for new code.
- Keep public APIs typed and documented.
- Prefer `async`/`await` with explicit error handling.
- Add tests for every behavior change.

## Security

- Never pass untrusted input to `child_process.exec`.
- Validate URLs and paths before use.
- Never commit tokens or `.env` files.
