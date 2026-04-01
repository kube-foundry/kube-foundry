# Running Claude Code on Kubernetes

This guide walks through running Claude Code on Kubernetes using the Kube Foundry operator.

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

```bash
kubectl create secret generic factory-creds \
  --namespace kube-foundry \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-your-key-here \
  --from-literal=GITHUB_TOKEN=ghp_your-token-here
```

## Submit a task

Claude Code is the default agent, so no `agent` field is required:

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: add-healthcheck
spec:
  repo: https://github.com/yourorg/yourapp
  branch: main
  task: "Add a /healthz endpoint that returns 200 OK with a JSON body containing the service version and uptime"
  credentials:
    secretRef: factory-creds
```

```bash
kubectl apply -f task.yaml
```

The operator spins up a sandbox pod, clones your repo, runs Claude Code with the task, and opens a PR.

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
  resources:
    cpu: "4"
    memory: "8Gi"
    timeoutMinutes: 60
```

## Skills

Claude Code supports the full Skill spec including prompts. Prompts are written to `.claude/skills/{name}.md` inside the workspace.

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: go-project
spec:
  description: "Standard Go project setup"
  prompts:
    - name: go-conventions
      content: |
        Follow standard Go conventions. Use table-driven tests.
        Run `go vet` before committing.
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
  skills:
    - go-project
```

## MCP servers

Claude Code supports both stdio and remote MCP servers. Config is written to `.claude/settings.json`.

Stdio transport:

```yaml
spec:
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

Remote transport:

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

## REST API

Submit tasks over HTTP with the webhook server:

```bash
curl -X POST http://<webhook-service>/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/yourorg/yourapp",
    "task": "Refactor the database layer to use connection pooling",
    "secretRef": "factory-creds"
  }'
```

No `agent` field needed -- Claude Code is the default.

## Retries

```yaml
spec:
  maxRetries: 2
```

## Next steps

- [Skills guide](skills.md) -- Reusable configuration
- [MCP Servers guide](mcp-servers.md) -- External tool access
- [SoftwareTask reference](../reference/softwaretask.md) -- Full CRD spec
