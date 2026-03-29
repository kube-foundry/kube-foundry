# Skill CRD

The `Skill` CRD defines reusable agent configuration that can be shared across tasks.

## Full example

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: go-expert
spec:
  description: "Go development best practices"

  prompts:
    - name: conventions
      content: |
        You are an expert Go developer.
        Write table-driven tests. Use slog for logging.

  files:
    - path: .golangci.yml
      content: |
        linters:
          enable: [govet, errcheck, staticcheck]
    - path: docs/style-guide.md
      configMapRef:
        name: team-docs
        key: go-style-guide

  env:
    - name: GOFLAGS
      value: "-count=1"

  init:
    - "go mod download"
    - "make generate"

  mcpServers:
    - name: internal-tools
      url: https://mcp.internal.company.com/sse
      headers:
        - name: Authorization
          valueFrom:
            name: mcp-creds
            key: auth-header
```

## Spec fields

| Field | Type | Description |
|---|---|---|
| `description` | string | Human-readable description (shown in `kubectl get sk`) |
| `prompts` | []SkillPrompt | Written to `.claude/skills/{name}.md` in the workspace |
| `prompts[].name` | string | Filename (without extension) |
| `prompts[].content` | string | Prompt content |
| `files` | []SkillFile | Files injected into the workspace |
| `files[].path` | string | Path relative to workspace root |
| `files[].content` | string | Inline file content |
| `files[].configMapRef` | object | Reference to a ConfigMap key (alternative to inline content) |
| `env` | []EnvVar | Environment variables added to the sandbox container |
| `init` | []string | Shell commands run before the agent starts |
| `mcpServers` | []MCPServer | MCP servers available to the agent |

## Listing skills

```bash
$ kubectl get sk
NAME         DESCRIPTION                                    AGE
go-expert    Go development best practices and project...   2d
testing      Standard testing patterns and CI config        1d
```
