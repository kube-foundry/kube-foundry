# MCP Servers

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers provide tools to AI agents at runtime. Kube Foundry supports two transport modes.

## Remote (streamable HTTP / SSE)

Connect to an existing MCP server over HTTP. Use this for shared infrastructure tools, internal APIs, or hosted MCP services.

```yaml
mcpServers:
  - name: internal-tools
    url: https://mcp.internal.company.com/sse
    headers:
      - name: Authorization
        value: "Bearer static-token"          # inline value
      - name: X-API-Key
        valueFrom:                             # or from a Secret
          name: mcp-creds
          key: api-key
```

## stdio (subprocess)

The agent spawns the MCP server as a child process. Use this for tools distributed as npm packages, binaries, or scripts.

```yaml
mcpServers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      - name: GITHUB_PERSONAL_ACCESS_TOKEN
        valueFrom:
          secretKeyRef:
            name: mcp-creds
            key: github-pat
```

## Where to define MCP servers

MCP servers can be defined in two places:

1. **In a Skill** (`spec.mcpServers`) -- shared across all tasks that reference the skill. Use this for team-wide or project-wide tools.

2. **In a SoftwareTask** (`spec.mcpServers`) -- specific to one task. Use this for one-off tools or task-specific services.

When a task references skills that define MCP servers and also defines its own, all servers are merged. Task-level servers are appended after skill-level servers.

## Agent support

| Agent | MCP Support | Config location |
|---|---|---|
| Claude Code | Remote + stdio | `.claude/settings.json` |
| OpenCode | Remote + stdio | `~/.config/opencode/config.json` |
| Codex | Not supported | Warning logged, servers skipped |

## Secret resolution

Header values and environment variables that use `valueFrom` / `secretKeyRef` are resolved by the operator at task creation time. The actual secret values are injected into the pod as serialized JSON, so the agent entrypoint can configure MCP servers without direct access to Kubernetes secrets.
