# Skills

Skills are reusable configuration bundles defined as Kubernetes custom resources. They let you standardize agent behavior across tasks by providing prompts, files, environment variables, init commands, and MCP servers.

A task can reference multiple skills. The operator resolves all referenced skills and merges their configuration into the sandbox pod before the agent starts.

## Defining a Skill

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: go-expert
spec:
  description: "Go development best practices and project setup"

  # Prompts are written to .claude/skills/{name}.md in the workspace.
  prompts:
    - name: go-conventions
      content: |
        You are an expert Go developer. Follow these conventions:
        - Use Go 1.25+ idioms
        - Always run `go vet ./...` before committing
        - Write table-driven tests
        - Use structured logging with slog

  # Files injected into the workspace before the agent starts.
  files:
    - path: .golangci.yml
      content: |
        linters:
          enable: [govet, errcheck, staticcheck]
    - path: docs/style-guide.md
      configMapRef:
        name: team-docs
        key: go-style-guide

  # Environment variables passed to the agent container.
  env:
    - name: GOFLAGS
      value: "-count=1"

  # Shell commands run after clone, before the agent starts.
  init:
    - "go mod download"
    - "make generate"

  # MCP servers available to the agent.
  mcpServers:
    - name: internal-tools
      url: https://mcp.internal.company.com/sse
      headers:
        - name: Authorization
          valueFrom:
            name: mcp-creds
            key: auth-header
```

## Using skills in a task

Reference skills by name in the `skills` field:

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: fix-auth-bug
spec:
  repo: https://github.com/yourorg/yourapp
  task: "Fix the race condition in the session handler"
  skills:
    - go-expert
    - testing
  credentials:
    secretRef: factory-creds
```

## How skills are applied

The operator fetches each referenced Skill, resolves any ConfigMap or Secret references, and injects the merged configuration into the sandbox pod:

| Skill field | Env variable | Description |
|---|---|---|
| `prompts` | `SKILL_PROMPTS` | Written to `.claude/skills/{name}.md` |
| `files` | `SKILL_FILES` | Written to the specified paths in the workspace |
| `env` | _(direct injection)_ | Added as container env vars |
| `init` | `SKILL_INIT_COMMANDS` | Run as shell commands before the agent starts |
| `mcpServers` | `SKILL_MCP_SERVERS` | Configured in the agent's MCP settings |

## Listing skills

```bash
kubectl get sk
```

```
NAME         DESCRIPTION                                    AGE
go-expert    Go development best practices and project...   2d
testing      Standard testing patterns and CI config        1d
```
