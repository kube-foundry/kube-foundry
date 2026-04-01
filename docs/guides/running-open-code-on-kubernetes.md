# Running OpenCode on Kubernetes

This guide walks through running OpenCode on Kubernetes using the Kube Foundry operator.

## Prerequisites

- A Kubernetes cluster (EKS, GKE, AKS, kind, etc.)
- `kubectl` and `helm` installed
- An [Anthropic API key](https://console.anthropic.com/)
- A GitHub personal access token with repo permissions

## Install Kube Foundry

```bash
helm repo add kube-foundry https://kube-foundry.github.io/kube-foundry
helm repo update

helm install kube-foundry kube-foundry/kube-foundry \
  --namespace kube-foundry \
  --create-namespace
```

## Create credentials

OpenCode needs an Anthropic API key and a GitHub token:

```bash
kubectl create secret generic factory-creds \
  --namespace kube-foundry \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-your-key-here \
  --from-literal=GITHUB_TOKEN=ghp_your-token-here
```

## Submit a task

Create a `SoftwareTask` with `agent: open-code`:

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: add-healthcheck
spec:
  repo: https://github.com/yourorg/yourapp
  branch: main
  task: "Add a /healthz endpoint that returns 200 OK with a JSON body containing the service version and uptime"
  agent: open-code
  credentials:
    secretRef: factory-creds
```

```bash
kubectl apply -f task.yaml
```

The operator will spin up a sandbox pod, clone your repo, run OpenCode with your task, and open a PR.

## Watch progress

```bash
kubectl get st -w                          # status transitions
kubectl logs -f pod/add-healthcheck-sandbox # agent output
```

Get the PR URL when complete:

```bash
kubectl get st add-healthcheck -o jsonpath='{.status.pullRequestURL}'
```

## Resource limits

Override the defaults (2 CPU, 4Gi memory, 30min timeout):

```yaml
spec:
  agent: open-code
  resources:
    cpu: "4"
    memory: "8Gi"
    timeoutMinutes: 60
```

## MCP servers

Give OpenCode access to external tools via MCP.

Stdio transport (spawns a subprocess):

```yaml
spec:
  agent: open-code
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

Remote transport (HTTP/SSE):

```yaml
spec:
  mcpServers:
    - name: internal-tools
      url: https://mcp.internal.company.com/sse
      headers:
        - name: Authorization
          valueFrom:
            secretKeyRef:
              name: mcp-creds
              key: auth-header
```

## Skills

Reuse configuration across tasks with `Skill` resources:

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: go-project
spec:
  description: "Standard Go project setup"
  files:
    - path: .golangci.yml
      content: |
        linters:
          enable: [govet, errcheck, staticcheck]
  env:
    - name: GOFLAGS
      value: "-count=1"
  init:
    - "go mod download"
    - "make generate"
```

Reference it in your task:

```yaml
spec:
  agent: open-code
  skills:
    - go-project
```

!!! note
    OpenCode does not support skill prompts (the `prompts` field). Use `claude-code` if you need prompt injection. All other skill features work: files, env, init commands, and MCP servers.

## REST API

Submit tasks over HTTP with the webhook server:

```bash
curl -X POST http://<webhook-service>/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/yourorg/yourapp",
    "task": "Refactor the database layer to use connection pooling",
    "agent": "open-code",
    "secretRef": "factory-creds"
  }'
```

## OpenCode vs Claude Code

| | OpenCode | Claude Code |
|---|---|---|
| **License** | Open source | Proprietary |
| **API Key** | `ANTHROPIC_API_KEY` | `ANTHROPIC_API_KEY` |
| **MCP Support** | Yes | Yes |
| **Skill Prompts** | No | Yes |

## Retries

Configure automatic retries on failure:

```yaml
spec:
  agent: open-code
  maxRetries: 2
```

## Next steps

- [Skills guide](skills.md) -- Reusable configuration
- [MCP Servers guide](mcp-servers.md) -- External tool access
- [SoftwareTask reference](../reference/softwaretask.md) -- Full CRD spec
