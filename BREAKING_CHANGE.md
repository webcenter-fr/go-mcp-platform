# Breaking Changes

## Initial import from eino-ext

This repository was extracted from
[`webcenter-fr/eino-ext`](https://github.com/webcenter-fr/eino-ext) at commit
`faf4ab8`. Relative to the pre-split eino-ext tree, the runtime contract changed
as follows:

| Kind | Old | New |
|---|---|---|
| Environment prefix (MCP server) | `EINO_MCP_*` | `MCP_PLATFORM_*` |
| Environment prefix (approval service) | `EINO_APPROVAL_*` | `MCP_APPROVAL_*` |
| Config mount path | `/etc/eino-ext` | `/etc/go-mcp-platform` |
| MCP server identity | `eino-ext-mcp` | `go-mcp-platform-mcp` |
| Helm chart name | `eino-ext` | `go-mcp-platform` |
| Image repositories | `webcenter-fr/eino-ext/{mcp,approval}` | `webcenter-fr/go-mcp-platform/{mcp,approval}` |

The Go import paths also moved:

| Old import path | New import path |
|---|---|
| `github.com/webcenter-fr/eino-ext/libs/mcp/serverconfig` | `github.com/webcenter-fr/go-mcp-platform/internal/serverconfig` |
| `github.com/webcenter-fr/eino-ext/libs/mcp/approval` | `github.com/webcenter-fr/go-mcp-platform/pkg/mcp/approval` |
| `github.com/webcenter-fr/eino-ext/libs/mcp` | `github.com/webcenter-fr/go-mcp-platform/pkg/mcp` |
| `github.com/webcenter-fr/eino-ext/components/mcp` | `github.com/webcenter-fr/go-mcp-platform/pkg/servers` |
| `github.com/webcenter-fr/eino-ext/server/approval/store` | `github.com/webcenter-fr/go-mcp-platform/internal/approval/store` |
| `github.com/webcenter-fr/eino-ext/server/approval` | `github.com/webcenter-fr/go-mcp-platform/internal/approval` |
| `github.com/webcenter-fr/eino-ext/libs/toolkit/logging` | `github.com/webcenter-fr/go-mcp-platform/internal/logging` |
| `github.com/webcenter-fr/eino-ext/libs/toolkit/secretref` | `github.com/webcenter-fr/go-mcp-platform/internal/secretref` |

The eino-ext packages that stay in the library and are imported across the
module boundary are `components/tool/{kubernetes,argocd,prometheus,grafana}` and
`libs/toolkit/{safety,validate,osclient}`.
