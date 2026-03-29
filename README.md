# Kube Foundry

A Kubernetes-native software factory that accepts software engineering tasks, routes them to AI coding agents running in isolated sandboxes, and delivers completed work as pull requests.

Submit a `SoftwareTask` custom resource and the operator spins up a sandboxed pod running your chosen AI agent (Claude Code, Codex, or OpenCode). The agent clones your repo, completes the task, and opens a PR.

## Architecture

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

## Quickstart

### Prerequisites

- Kubernetes cluster (or [kind](https://kind.sigs.k8s.io/) for local dev)
- Helm 3
- Docker
- An Anthropic API key (for Claude Code), OpenAI API key (for Codex), or both
- A GitHub token with repo access

### Install

```bash
helm install kube-foundry ./chart/kube-foundry
```

### Create credentials

```bash
kubectl create secret generic factory-creds \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=GITHUB_TOKEN=ghp_...
```

### Submit a task

```yaml
# task.yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: add-login-page
spec:
  repo: https://github.com/yourorg/yourapp
  branch: main
  task: "Add a login page with email/password auth"
  credentials:
    secretRef: factory-creds
```

```bash
kubectl apply -f task.yaml
```

### Watch it work

```bash
kubectl get st -w                          # watch status transitions
kubectl logs -f pod/add-login-page-sandbox # watch agent output
```

A PR will appear on your repo when the task completes.

## SoftwareTask Reference

The `SoftwareTask` CRD is the primary interface for submitting work.

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

### Spec fields

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
| `mcpServers` | []MCPServer | | Task-specific MCP servers (see [MCP Servers](#mcp-servers)) |
| `callbackURL` | string | | URL to POST when task completes or fails |
| `resources.cpu` | string | `2` | CPU limit for sandbox pod |
| `resources.memory` | string | `4Gi` | Memory limit for sandbox pod |
| `resources.timeoutMinutes` | int | `30` | Max execution time (1-120) |

### Status

```bash
$ kubectl get st
NAME             PHASE       AGENT        REPO                                    AGE
add-login-page   Completed   claude-code  https://github.com/yourorg/yourapp      5m
refactor-auth    Running     claude-code  https://github.com/yourorg/yourapp      1m
```

Phases: `Pending` -> `Running` -> `Completed` | `Failed`

Status fields include `podName`, `startTime`, `completionTime`, `retryCount`, `pullRequestURL`, and `message`.

### Callback

When `callbackURL` is set, the operator sends a POST request on task completion or failure:

```json
{
  "taskName": "add-login-page",
  "namespace": "default",
  "status": "Completed",
  "pullRequestURL": "https://github.com/yourorg/yourapp/pull/42",
  "message": "Task completed successfully"
}
```

## Skills

Skills are reusable configuration bundles defined as Kubernetes custom resources. They let you standardize agent behavior across tasks by providing prompts, files, environment variables, init commands, and MCP servers.

A task can reference multiple skills. The operator resolves all referenced skills and merges their configuration into the sandbox pod before the agent starts.

### Skill CRD

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: go-expert
spec:
  description: "Go development best practices and project setup"

  # Prompts are written to .claude/skills/{name}.md in the workspace.
  # This is the primary way to give the agent domain-specific instructions.
  prompts:
    - name: go-conventions
      content: |
        You are an expert Go developer. Follow these conventions:
        - Use Go 1.25+ idioms
        - Always run `go vet ./...` before committing
        - Write table-driven tests
        - Use structured logging with slog

  # Files injected into the workspace before the agent starts.
  # Content can be inline or referenced from a ConfigMap.
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

  # MCP servers available to the agent (see MCP Servers section).
  mcpServers:
    - name: internal-tools
      url: https://mcp.internal.company.com/sse
      headers:
        - name: Authorization
          valueFrom:
            name: mcp-creds
            key: auth-header
```

### Using skills in a task

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

The operator fetches each referenced Skill, resolves any ConfigMap or Secret references, and injects the merged configuration into the sandbox pod via environment variables (`SKILL_PROMPTS`, `SKILL_FILES`, `SKILL_MCP_SERVERS`, `SKILL_INIT_COMMANDS`, plus any custom env vars).

### Listing skills

```bash
$ kubectl get sk
NAME         DESCRIPTION                                    AGE
go-expert    Go development best practices and project...   2d
testing      Standard testing patterns and CI config        1d
```

## MCP Servers

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers provide tools to AI agents at runtime. Kube Foundry supports two transport modes:

### Remote (streamable HTTP / SSE)

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

### stdio (subprocess)

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

### Where to define MCP servers

MCP servers can be defined in two places:

1. **In a Skill** (`spec.mcpServers`) -- shared across all tasks that reference the skill. Use this for team-wide or project-wide tools.

2. **In a SoftwareTask** (`spec.mcpServers`) -- specific to one task. Use this for one-off tools or task-specific services.

When a task references skills that define MCP servers and also defines its own, all servers are merged. Task-level servers are appended after skill-level servers.

### Agent support

| Agent | MCP Support | Config location |
|---|---|---|
| Claude Code | Remote + stdio | `.claude/settings.json` |
| OpenCode | Remote + stdio | `~/.config/opencode/config.json` |
| Codex | Not supported | Warning logged, servers skipped |

### Secret resolution

Header values and environment variables that use `valueFrom` / `secretKeyRef` are resolved by the operator at task creation time. The actual secret values are injected into the pod as serialized JSON, so the agent entrypoint can configure MCP servers without direct access to Kubernetes secrets.

## Webhook Server

The webhook server provides HTTP endpoints for programmatic task submission. It runs as a separate deployment and creates `SoftwareTask` resources in the cluster.

### REST API

**`POST /api/v1/tasks`** -- Create a task programmatically.

```bash
curl -X POST http://<webhook-service>/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/yourorg/yourapp",
    "task": "Add dark mode support",
    "branch": "main",
    "agent": "claude-code",
    "secretRef": "factory-creds",
    "skills": ["go-expert", "testing"],
    "mcpServers": [
      {
        "name": "internal-tools",
        "url": "https://mcp.internal.com/sse"
      }
    ],
    "maxRetries": 2,
    "callbackURL": "https://hooks.example.com/done",
    "gitAuthorName": "Factory Bot",
    "gitAuthorEmail": "bot@example.com"
  }'
```

Response:

```json
{
  "name": "task-1711612800000",
  "namespace": "default",
  "message": "Task created. Watch with: kubectl get st task-1711612800000 -w"
}
```

#### Request fields

| Field | Type | Default | Description |
|---|---|---|---|
| `repo` | string | **required** | HTTPS URL of the git repository |
| `task` | string | **required** | Natural-language task description |
| `branch` | string | `main` | Base branch |
| `agent` | string | `claude-code` | Agent runtime |
| `secretRef` | string | `factory-creds` | Credentials secret name |
| `githubToken` | string | | Inline GitHub token |
| `skills` | []string | | Skill resource names |
| `mcpServers` | []object | | Task-specific MCP servers |
| `maxRetries` | int | | Max retry attempts |
| `callbackURL` | string | | Completion callback URL |
| `gitAuthorName` | string | | Git commit author name |
| `gitAuthorEmail` | string | | Git commit author email |

### GitHub Webhook

**`POST /webhooks/github`** -- Triggered by GitHub webhook events.

Add the `factory:do` label to any GitHub issue, and the webhook server automatically creates a `SoftwareTask` from the issue title and body.

**Setup:**

1. In your GitHub repo, go to Settings > Webhooks > Add webhook.
2. Set the Payload URL to `http://<webhook-service>/webhooks/github`.
3. Set Content type to `application/json`.
4. Optionally set a webhook secret.
5. Select "Issues" events.

**Webhook secret verification:**

Set the `GITHUB_WEBHOOK_SECRET` environment variable on the webhook deployment to enable signature verification. When set, the server validates the `X-Hub-Signature-256` header on every request.

**Behavior:**

- Only `labeled` events with the label `factory:do` trigger task creation.
- All other events are acknowledged with 200 OK and ignored.
- Created tasks use defaults: agent `claude-code`, branch `main`, secret `factory-creds`.

### Health check

**`GET /healthz`** -- Returns 200 OK. Use for liveness/readiness probes.

### Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `NAMESPACE` | `default` | Namespace for created SoftwareTask resources |
| `PORT` | `8080` | HTTP listen port |
| `GITHUB_WEBHOOK_SECRET` | | Optional secret for GitHub signature verification |

## Agents

Kube Foundry supports three agent runtimes. Each runs in its own container image with an entrypoint that handles git setup, skill injection, agent execution, and PR creation.

| Agent | Image | API Key Secret Field | MCP Support |
|---|---|---|---|
| `claude-code` | `agent-images/claude-code` | `ANTHROPIC_API_KEY` | Yes |
| `codex` | `agent-images/codex` | `OPENAI_API_KEY` | No |
| `open-code` | `agent-images/open-code` | `ANTHROPIC_API_KEY` | Yes |

### Agent sandbox lifecycle

1. **Configure git** -- set credentials, author name/email, authenticate with `gh`
2. **Clone repo** -- shallow clone of the specified branch
3. **Create work branch** -- `factory-<taskname>-<timestamp>`
4. **Apply skills** -- inject prompts, files, MCP config, run init commands
5. **Run agent** -- execute the AI agent with the task description
6. **Commit and push** -- stage all changes, commit, push to work branch
7. **Create PR** -- open a pull request against the base branch

## Local Development

```bash
# Full local dev cycle (creates kind cluster, builds images, deploys)
make dev

# Or step by step:
make kind-create          # create kind cluster
make docker-build-all     # build all images
make kind-load            # load images into kind
make helm-install         # install via Helm

# Create credentials and submit a test task
kubectl create secret generic factory-creds \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=GITHUB_TOKEN=ghp_...
kubectl apply -f examples/task-basic.yaml
```

### Run tests

```bash
make test                 # unit tests (uses envtest)
make test-e2e             # e2e tests (requires kind cluster)
```

### Regenerate after changes

```bash
# After editing *_types.go or kubebuilder markers:
make manifests            # regenerate CRDs and RBAC
make generate             # regenerate DeepCopy methods
```

## Project Structure

```
├── api/v1alpha1/              # CRD type definitions (SoftwareTask, Skill)
├── internal/controller/       # Operator reconciler
├── cmd/                       # Operator entrypoint
├── webhook/                   # REST API + GitHub webhook server
├── agent-images/
│   ├── claude-code/           # Claude Code agent container
│   ├── codex/                 # Codex agent container
│   └── open-code/             # OpenCode agent container
├── chart/
│   └── kube-foundry/          # Helm chart
├── examples/                  # Example SoftwareTask YAMLs
├── config/                    # Kustomize manifests (kubebuilder generated)
│   ├── crd/bases/             # Generated CRDs (do not edit)
│   ├── rbac/                  # Generated RBAC (do not edit)
│   └── samples/               # Sample CRs
├── test/
│   └── e2e/                   # End-to-end tests
└── Makefile
```

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
