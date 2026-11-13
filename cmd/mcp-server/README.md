# cmd/mcp-server — deployable MCP server binary

One binary for every MCP tool family. It selects the family with `--server`,
loads a YAML configuration (see
[`internal/serverconfig`](../../internal/serverconfig)), and serves MCP over
stdio or streamable HTTP.

## Build and run

```bash
go build -o mcp-server ./cmd/mcp-server

# stdio (trusted local process; no listener at all)
./mcp-server --server kubernetes --config /etc/go-mcp-platform/config.yaml

# HTTP + unauthenticated operations port
./mcp-server --server kubernetes --config /etc/go-mcp-platform/config.yaml \
  --transport http --addr :8080 --ops-addr :9090
```

## Flags

| Flag | Purpose |
|---|---|
| `--config` | YAML configuration file |
| `--server` | `kubernetes`, `argocd`, `prometheus`, `grafana`, `developer` or `memory` |
| `--transport` | `stdio` (default) or `http` |
| `--addr` | MCP HTTP listen address (default `:8080`) |
| `--ops-addr` | Operations listen address (default `:9090`) |
| `--log-level` | `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` |
| `--log-format` | `text` or `json` |

Flags override environment variables (`MCP_PLATFORM_*`), which override the YAML
file, which overrides defaults.

## Operations endpoints

The ops listener is **unauthenticated** and must never be exposed through a
Service or Ingress:

- `GET /healthz` — liveness, always 200 once serving.
- `GET /readyz` — readiness, 503 until the server is built.
- `GET /metrics` — Prometheus metrics.

With `--transport stdio` no listener is started at all (no MCP port, no ops
port).

## Shutdown

`SIGINT`/`SIGTERM` cancels the context. The MCP listener drains first, then the
ops listener, with a 10s budget so probes stay answerable during drain.

## Example configuration

See [`testdata/mcp.yaml`](./testdata/mcp.yaml),
[`testdata/developer.yaml`](./testdata/developer.yaml),
[`testdata/memory.yaml`](./testdata/memory.yaml) and the
[`serverconfig` README](../../internal/serverconfig/README.md).
