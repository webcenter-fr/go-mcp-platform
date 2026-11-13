# Developer agent lifecycle

| Stage | Purpose | Model tier |
|---|---|---|
| `architect` | Understand the request, map the code, propose a plan | `strong-reasoning` |
| `develop` | Implement the approved plan with tests | `fast-coding` |
| `review` | Review correctness, compatibility, security, simplicity | `strong-reasoning` |
| `security` | Threat-model the change and verify controls | `strong-reasoning` |
| `deep-security` | Wide read-only security analysis | `long-context` |
| `deep-bug-search` | Wide read-only bug hunt | `long-context` |

Templates name a **tier**, never a concrete model id. The operator maps tiers to
models, so the workflow survives model churn.

## Rules

1. The architect does not edit files.
2. The developer keeps the tree compiling and tests passing.
3. The reviewer reports findings; the developer fixes them.
4. Security stages are read-only.
5. Every stage reports the gates it ran and any it skipped.
