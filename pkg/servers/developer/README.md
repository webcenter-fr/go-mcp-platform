# developer — credential-free developer sandbox MCP server

`pkg/servers/developer` exposes the toolchain a developer agent needs over MCP:

- a **credential-free, network-restricted Dagger sandbox** with a full shell
  (`developer_sandbox_exec`, `developer_sandbox_info`);
- the **GitHub tool family** (20 read/local tools + 12 gated write tools);
- **project bootstrap scaffolding** (`AGENTS.md` per language, lifecycle agent
  specs, opencode settings) as tools, MCP prompts and MCP resources.

The client LLM keeps driving the loop. This server is not an agent runtime.

## Isolation model

The sandbox container holds **no credentials**: no `GITHUB_TOKEN`, no
kubeconfig, no `~/.git-credentials`, no SSH keys, no cloud credentials. GitHub
writes run in the MCP process, under RBAC + dry-run/confirmed + human approval,
never as shell commands with a token in the environment.

Each authenticated identity gets its own workspace root:

```
<WorkspaceRoot>/<sha256(Identity.Subject())[:16]>/default/<owner>/<repo>
```

The subject comes from the verified bearer token, so the model cannot forge it.
`ToolsFor` forces identity-keyed server caching even without RBAC, so two
identities never share a tool set or a workspace.

> **Egress is a soft control.** The pinned eino-ext release does not yet apply
> `WithEgressPolicy` (it is a verified no-op), so `developer_sandbox_info`
> reports `egressEnforced=false` and the server logs a startup WARN. Even once
> the upstream proxy is wired, it only covers tools that honor
> `HTTP_PROXY`/`HTTPS_PROXY`; raw TCP (`psql`, `redis-cli`, `nc`, `kubectl`,
> direct dialers) bypasses it. A hard guarantee requires operator-side
> NetworkPolicy/nftables and a dedicated engine.

## Quick start

```go
srv, err := developer.NewServer(ctx, &developer.Config{
    WorkspaceRoot: "/var/lib/go-mcp-platform/workspaces",
    Engine: developer.EngineConfig{
        Mode:       "sidecar",
        RunnerHost: "tcp://127.0.0.1:8081",
    },
    GitHub: &developer.GitHubConfig{
        Instances: map[string]developer.GitHubInstance{
            "corp": {Token: os.Getenv("GITHUB_TOKEN")},
        },
    },
}, &mcpserver.Config{
    Authorization: authzCfg,
    Approval:      approvalCfg,
})
if err != nil {
    return err
}
```

`NewServer` is lazy: no Dagger engine and no GitHub API call happen at
construction. `NewServerWithRunner` additionally returns an `io.Closer` for the
sandbox backend.

## Tools

| Tool | Write? | Behavior |
|---|---|---|
| `developer_sandbox_exec` | No | Runs an argv in the credential-free sandbox. Returns `SandboxExecOutput` JSON. `dryRun` returns the resolved plan. |
| `developer_sandbox_info` | No | Returns the effective sandbox configuration. No engine contact. |
| `developer_scaffold_list` | No | Lists embedded templates. |
| `developer_scaffold_get` | No | Returns one template body. |
| `developer_scaffold_apply` | No | Writes templates into the caller's workspace (local only). |
| 20 `github_*` read/local tools | No | From `github.NewReadOnlyTools`. |
| 12 `github_*` write tools | **Yes** | From `github.NewAllTools`; gated by RBAC + approval. Skipped when `github.readOnly=true`. |

`developer_sandbox_exec` deliberately has **no** `allowLocalNetwork` and **no**
`profile` parameter: an LLM can never widen egress or choose the base image.
The command is executed as an argv array, never through a shell.

## Prompts and resources

| Prompt | Arguments | Purpose |
|---|---|---|
| `developer_bootstrap` | `language` (required), `projectName` | Bootstrap instruction. |
| `developer_review` | `scope` | Review / simplify persona. |
| `developer_security_audit` | `depth` (`standard`\|`deep`) | Security-review persona. |
| `developer_lifecycle` | `stage` (required) | Stage persona + model tier. |

| Resource | Content |
|---|---|
| `developer://templates` | JSON index of all templates |
| `developer://agents/<language>` | `AGENTS.md` body |
| `developer://lifecycle/workflow` | Lifecycle spec with tiers |
| `developer://opencode/config` | `opencode.json` skeleton |

Prompts and resources are **read** operations: RBAC matches their name against
`AccessRule.Tools` (e.g. `developer_*`), and they fail closed exactly like tools.

## Engine modes

| Mode | Use |
|---|---|
| `sidecar` | A privileged Dagger engine container in the same pod. `runnerHost` points at it. |
| `cloud` | Dagger Cloud. `cloudTokenFile`/`cloudTokenEnv` required. |
| `dagger-kubernetes` | A self-hosted Dagger-Cloud-compatible supervisor. `cloudURL` + token required; `runnerHost` defaults to `dagger-cloud://self`. |

The developer image must carry the Dagger CLI: the SDK's runner-host path execs
the CLI binary, and the distroless MCP image has no CLI. See
`Dockerfile.developer`.

## Operator knobs

- `imageOverrides` — per-profile base image (e.g. `golang:1.23`).
- `allowedImages` — allowlist of every image that may be used after detection
  and overrides.
- `egress.allowHosts` / `egress.allowCIDRs` — egress allowlist.
- `egress.allowLocalNetwork` — **dangerous**; never settable from a tool.
- `extraBlocklist` — extends `safety.DefaultCommandBlocklist` (never replaces).
- `workspaceTTL` / `workspaceGCInterval` — idle workspace GC.
- `maxConcurrentExecs`, `defaultTimeout`, `maxTimeout`, `maxOutputBytes`.

## Not like running locally

- The workspace is **server-side**, populated by `github_repo_clone`; it is not
  the developer's laptop checkout.
- Every exec is one-shot: no watch mode, no REPL, no debugger attach.
- Output is buffered, truncated and redacted; it is not streamed.
- `persistChanges=true` exports the container workspace back to the host,
  excluding `.git` on the export path. Note the workspace is mounted read-write
  into the container, so a compromised container can still modify `.git`
  directly through the mount; the export exclusion only prevents the export
  step from clobbering refs.

## Residual risk

If the MCP client runs on a developer laptop, that client already has the
laptop's `~/.kube/config`, `~/.git-credentials` and SSH keys. This server cannot
prevent that. The guarantee is narrower: **the sandbox we provide holds no
credentials and its egress is restricted; credentials the client process
already possesses are outside this threat model.**
