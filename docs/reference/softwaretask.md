# SoftwareTask CRD

The `SoftwareTask` CRD is the primary interface for submitting work to Kube Foundry.

## Full example

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: my-task
spec:
  repo: https://github.com/yourorg/yourapp    # required
  branch: main                                  # default: main
  task: "Refactor auth module to use JWT"       # required, natural language
  agent: claude-code                            # claude-code | codex | open-code
  credentials:
    secretRef: factory-creds                    # Secret with API keys + GitHub token
    githubToken: ghp_...                        # optional: inline token (overrides secret)
  gitAuthorName: "Kube Foundry"                 # optional: git commit author
  gitAuthorEmail: "bot@example.com"             # optional: git commit email
  maxRetries: 2                                 # 0-5, default: 1
  skills:                                       # optional: list of Skill resource names
    - go-expert
    - testing
  mcpServers:                                   # optional: task-specific MCP servers
    - name: internal-api
      url: https://mcp.internal.com/sse
      headers:
        - name: Authorization
          valueFrom:
            name: mcp-creds
            key: auth-header
    - name: github
      command: npx
      args: ["-y", "@modelcontextprotocol/server-github"]
  callbackURL: https://hooks.example.com/done   # optional: POST on completion/failure
  resources:
    cpu: "4"
    memory: "8Gi"
    timeoutMinutes: 60
```

## Spec fields

| Field | Type | Default | Description |
|---|---|---|---|
| `repo` | string | **required** | HTTPS URL of the git repository |
| `branch` | string | `main` | Base branch to clone and work from |
| `task` | string | **required** | Natural-language task description |
| `agent` | string | `claude-code` | Agent runtime (`claude-code`, `codex`, `open-code`) |
| `credentials.secretRef` | string | | Secret with `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` and `GITHUB_TOKEN` |
| `credentials.githubToken` | string | | Inline GitHub token (overrides secret) |
| `gitAuthorName` | string | `Bot` | Name used for git commits |
| `gitAuthorEmail` | string | `bot@users.noreply.github.com` | Email used for git commits |
| `maxRetries` | int | `1` | Retry attempts on failure (0-5) |
| `skills` | []string | | Skill resource names to load into the agent |
| `mcpServers` | []MCPServer | | Task-specific MCP servers (see [MCP Servers](../guides/mcp-servers.md)) |
| `callbackURL` | string | | URL to POST when task completes or fails |
| `resources.cpu` | string | `2` | CPU limit for sandbox pod |
| `resources.memory` | string | `4Gi` | Memory limit for sandbox pod |
| `resources.timeoutMinutes` | int | `30` | Max execution time (1-120) |

## Status

```bash
$ kubectl get st
NAME             PHASE       AGENT        REPO                                    AGE
add-login-page   Completed   claude-code  https://github.com/yourorg/yourapp      5m
refactor-auth    Running     claude-code  https://github.com/yourorg/yourapp      1m
```

Phases: `Pending` → `Running` → `Completed` | `Failed`

### Status fields

| Field | Description |
|---|---|
| `phase` | Current phase |
| `podName` | Name of the sandbox pod |
| `startTime` | When the sandbox pod was created |
| `completionTime` | When the task finished |
| `retryCount` | Number of retries so far |
| `pullRequestURL` | URL of the created PR (on success) |
| `message` | Human-readable status message |

## Callback

When `callbackURL` is set, the operator sends a POST request on task completion or failure:

```json
{
  "taskName": "add-login-page",
  "status": "completed",
  "pullRequestUrl": "https://github.com/yourorg/yourapp/pull/42",
  "errorMessage": ""
}
```
