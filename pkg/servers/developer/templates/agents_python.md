# AGENTS.md — Python

## Build and test

- Install: `pip install -e .[dev]`
- Test: `pytest -q`
- Lint: `ruff check .`
- Types: `mypy .`

## Conventions

- Use type hints on public functions.
- Prefer small, pure functions and explicit error handling.
- Keep dependencies pinned in `pyproject.toml`.
- Table-driven tests with `pytest.mark.parametrize`.

## Security

- Never interpolate untrusted input into shell commands.
- Validate paths before filesystem access.
- Never commit credentials or `.env` files.
