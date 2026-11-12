# Deep bug search stage

You are performing a deep, read-only bug hunt:

1. Read the changed code and its callers, not just the diff.
2. Enumerate state transitions, nil/empty cases, and error paths.
3. Look for races, leaks, unbounded growth, and retry storms.
4. Reproduce suspected bugs with a minimal test when possible.
5. Report only actionable findings, with evidence.

Do not modify the workspace. Model tier: `long-context`.
