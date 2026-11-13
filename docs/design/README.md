# Technical design documents

This directory holds the **technical design documents** for go-mcp-platform.
They are written in a technical style, in **English and French**, and use
**mermaid** diagrams for schemas and flows.

## Documents

| Document | English | Français | Covers |
|---|---|---|---|
| Architecture | [`00-architecture.en.md`](./00-architecture.en.md) | [`00-architecture.fr.md`](./00-architecture.fr.md) | The `cmd -> internal -> pkg -> eino-ext` dependency rule, the `pkg` vs `internal` split, the MCP toolkit (adapter, AuthN providers, AuthZ/RBAC, transports, why the LLM cannot self-approve), the MCP handler request path, the module boundary and the entry-point signatures. |
| MCP hard approval | [`01-mcp-hard-approval.en.md`](./01-mcp-hard-approval.en.md) | [`01-mcp-hard-approval.fr.md`](./01-mcp-hard-approval.fr.md) | The standalone out-of-band approval service: topology, approval flow, signed receipt verification, `argsHash` canonicalization, store backends, security controls, and the Nuxt UI build/embedding. |
| Deployment and CI | [`02-deployment-and-ci.en.md`](./02-deployment-and-ci.en.md) | [`02-deployment-and-ci.fr.md`](./02-deployment-and-ci.fr.md) | The deployable MCP/approval binaries, YAML configuration and file/env secrets, container images, the Helm chart and its security guards, and the Dagger CI pipeline. |
| Developer sandbox | [`03-developer-sandbox.en.md`](./03-developer-sandbox.en.md) | [`03-developer-sandbox.fr.md`](./03-developer-sandbox.fr.md) | The credential-free Dagger sandbox, the GitHub tool family, the scaffolding surface, per-identity workspaces, the egress reality, the three engine modes, the lifecycle/tier model and the deferred in-engine Dagger LLM. |
| Memory | [`04-mcp-memory.en.md`](./04-mcp-memory.en.md) | [`04-mcp-memory.fr.md`](./04-mcp-memory.fr.md) | The typed long-term agent memory family: memory taxonomy, storage backends (memory, file, OpenSearch), tenant isolation, secret redaction, MCP tools/prompts/resources, and the lazy OpenSearch/embedding/LLM design. |

## Related projects

- The reusable eino components and shared libraries:
  [`webcenter-fr/eino-ext`](https://github.com/webcenter-fr/eino-ext)
- The pre-split implementation plans live in the eino-ext repository under
  [`.opencode/plans/`](https://github.com/webcenter-fr/eino-ext/tree/main/.opencode/plans).

## Conventions

- **Bilingual:** every design doc exists in English (`*.en.md`) and French
  (`*.fr.md`). The English version is the reference; keep both in sync.
- **Diagrams:** use mermaid (` ```mermaid ` blocks) for architectures, flows,
  sequences, and class diagrams.
- **Code:** identifiers, type names, and code blocks stay in English in both
  versions; only prose and diagram labels are translated.
- **Placement:** design docs live here, in `docs/design/`. Component-level usage
  docs stay in each package's `README.md`.
- **Implementation depth:** each doc ends with an *implementation deep-dives*
  section (EN: `## Implementation deep-dives`, FR: `## Plongées dans
  l'implémentation`) that explains the internal mechanics behind the concepts —
  the code paths, algorithms, and data flow a developer needs to read, modify,
  and debug the code.

## Keeping docs in sync

When you add or change a component, middleware, callback, or lib covered by these
documents, update the matching design doc (both languages) in the same change.
