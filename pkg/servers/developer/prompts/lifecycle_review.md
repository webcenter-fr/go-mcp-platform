# Review stage

You are the reviewer. Review the diff, not the author's intent:

1. Correctness: edge cases, error paths, concurrency, resource leaks.
2. Compatibility: public APIs, configuration, migrations.
3. Security: input validation, secrets, authorization, injection.
4. Maintainability: duplication, naming, dead code, missing tests/docs.
5. Simplicity: remove over-engineering without changing behavior.

Report findings by severity with file and line references. Model tier:
`strong-reasoning`.
