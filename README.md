# go-mcp-platform

A deployable MCP platform: one MCP server per eino tool family (Kubernetes,
ArgoCD, Prometheus, Grafana, Developer) plus a standalone out-of-band
human-approval service with an embedded Nuxt UI. It provides provider-based authentication
(local static tokens + OIDC), granular RBAC, and signed-receipt approval for
write tools.

The reusable eino components and shared libraries live in
[`webcenter-fr/eino-ext`](https://github.com/webcenter-fr/eino-ext); this
repository consumes them as a Go module dependency.

## Layout

Dependencies point inward and never reverse:

```
cmd/  ->  internal/  ->  pkg/  ->  github.com/webcenter-fr/eino-ext
```

- `cmd/mcp-server/` — deployable MCP server binary (one binary, five families).
- `cmd/approval-server/` — standalone approval-service binary.
- `internal/serverconfig/` — YAML configuration and binary wiring.
- `internal/approval/` — approval service, store backends and the embedded Nuxt
  UI (`internal/approval/ui`).
- `internal/logging/`, `internal/secretref/` — private helpers.
- `pkg/mcp/` — public generic MCP toolkit (auth, RBAC, approval, transports).
- `pkg/mcp/approval/` — public approval-service client.
- `pkg/servers/` — public embeddable per-family servers, including the
  credential-free developer sandbox (`pkg/servers/developer`).
- `helm/go-mcp-platform/` — Helm chart.
- `.dagger/` — Dagger CI module (Go gates, UI, images, Helm, publish).
- `docs/design/` — bilingual technical design documents.

## Quick start

MCP server:

```bash
go run ./cmd/mcp-server --server kubernetes --config config.yaml
go run ./cmd/mcp-server --server grafana --config config.yaml --transport http
go run ./cmd/mcp-server --server developer --config config.yaml --transport http
```

Approval service:

```bash
go run ./cmd/approval-server --config approval.yaml
```

Configuration uses YAML with file/environment secret references. See
[`internal/serverconfig`](./internal/serverconfig) and
[`cmd/mcp-server`](./cmd/mcp-server) for the MCP side, and
[`internal/approval`](./internal/approval) and
[`cmd/approval-server`](./cmd/approval-server) for the approval side.

## Container images

Images publish to:

- `ghcr.io/webcenter-fr/go-mcp-platform/mcp`
- `ghcr.io/webcenter-fr/go-mcp-platform/approval`
- `ghcr.io/webcenter-fr/go-mcp-platform/developer` (adds the pinned Dagger CLI)

Build them locally with:

```bash
make docker-build-mcp
make docker-build-approval
make docker-build-developer
```

## Helm chart

```bash
helm lint helm/go-mcp-platform
helm template t helm/go-mcp-platform
helm install go-mcp-platform helm/go-mcp-platform
```

The chart defaults to read-only Kubernetes RBAC, authenticated MCP, and the
file-backed approval store. `allowWrite` and `allowExec` are dangerous opt-ins.
The developer family is disabled by default (`mcpServers.developer.enabled=false`)
because it needs a Dagger engine; in `sidecar` mode that engine is a privileged
container in the same pod.

## Development

```bash
go build ./...
go vet ./...
make test
make lint
bash scripts/check_components.sh
```

CI runs the same gates through Dagger:

```bash
dagger call ci --dry-run --version 0.0.0-dev export --path ./ci-out
```

### Local development against eino-ext

The platform consumes `github.com/webcenter-fr/eino-ext` as a normal module
dependency. To develop against a sibling checkout, create a **gitignored**
`go.work` at the repository root:

```
go 1.26.3

use .
use ../eino-ext
```

Never commit a `replace github.com/webcenter-fr/eino-ext => ../eino-ext`
directive: CI runners have no sibling checkout, and a committed `replace` makes
the published module unusable for downstream consumers.

See the [architecture design doc](./docs/design/00-architecture.en.md), the
[developer sandbox design doc](./docs/design/03-developer-sandbox.en.md) and
[`CONTRIBUTING.md`](./CONTRIBUTING.md).
