# Kube Foundry

A Kubernetes-native software factory that accepts software engineering tasks, routes them to AI coding agents running in isolated sandboxes, and delivers completed work as pull requests.

Submit a `SoftwareTask` custom resource and the operator spins up a sandboxed pod running your chosen AI agent (Claude Code, Codex, or OpenCode). The agent clones your repo, completes the task, and opens a PR.

## How it works

```
kubectl apply -f task.yaml          curl POST /api/v1/tasks
        │                                   │
        ▼                                   ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│ SoftwareTask │────▶│   Operator   │────▶│   Sandbox Pod    │
│     CRD      │     │ (reconciler) │     │  (agent runner)  │
└──────────────┘     └──────┬───────┘     └────────┬─────────┘
                            │                      │
                     ┌──────┴───────┐              ▼
                     │  Skill CRDs  │       ┌──────────┐
                     │ (reusable    │       │  GitHub   │
                     │  config)     │       │    PR     │
                     └──────────────┘       └──────────┘
```

**Components:**

- **SoftwareTask CRD** -- declarative task definition (`kubectl get st`)
- **Skill CRD** -- reusable agent configuration: prompts, files, env vars, init commands, MCP servers (`kubectl get sk`)
- **Operator** -- watches tasks, resolves skills, creates sandbox pods, tracks lifecycle
- **Sandbox Pod** -- isolated container running the selected AI agent
- **Webhook Server** -- REST API and GitHub webhook for programmatic task submission

## Quick links

- [Installation](getting-started/installation.md) -- Install Kube Foundry on your cluster
- [Quickstart](getting-started/quickstart.md) -- Submit your first task in 2 minutes
- [Skills](guides/skills.md) -- Reusable agent configuration
- [MCP Servers](guides/mcp-servers.md) -- Connect tools to agents at runtime
- [SoftwareTask Reference](reference/softwaretask.md) -- Full CRD spec
