# Running Codex on Kubernetes

This guide walks through running OpenAI's Codex agent on Kubernetes using the Kube Foundry operator.

## Prerequisites

- A Kubernetes cluster (EKS, GKE, AKS, kind, etc.)
- `kubectl` and `helm` installed
- An [OpenAI API key](https://platform.openai.com/)
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

Codex uses an OpenAI API key instead of Anthropic:

```bash
kubectl create secret generic factory-creds \
  --namespace kube-foundry \
  --from-literal=OPENAI_API_KEY=sk-your-key-here \
  --from-literal=GITHUB_TOKEN=ghp_your-token-here
```

## Submit a task

Set `agent: codex` in your SoftwareTask:

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: add-healthcheck
spec:
  repo: https://github.com/yourorg/yourapp
  branch: main
  task: "Add a /healthz endpoint that returns 200 OK with a JSON body containing the service version and uptime"
  agent: codex
  credentials:
    secretRef: factory-creds
```

```bash
kubectl apply -f task.yaml
```

The operator spins up a sandbox pod, clones your repo, runs Codex in full-auto mode, and opens a PR.

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
  agent: codex
  resources:
    cpu: "4"
    memory: "8Gi"
    timeoutMinutes: 60
```

## Skills

Codex supports skill files, environment variables, and init commands. It does **not** support prompts or MCP servers.

```yaml
apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: node-project
spec:
  description: "Standard Node.js project setup"
  files:
    - path: .eslintrc.json
      content: |
        { "extends": "eslint:recommended" }
  env:
    - name: NODE_ENV
      value: "development"
  init:
    - "npm install"
```

Reference it in your task:

```yaml
spec:
  agent: codex
  skills:
    - node-project
```

!!! warning
    Codex does not support MCP servers. If MCP servers are configured on a Codex task, a warning is logged and they are skipped. Use `claude-code` or `open-code` if you need MCP support.

## REST API

Submit tasks over HTTP with the webhook server:

```bash
curl -X POST http://<webhook-service>/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/yourorg/yourapp",
    "task": "Add input validation to all API endpoints",
    "agent": "codex",
    "secretRef": "factory-creds"
  }'
```

## Retries

```yaml
spec:
  agent: codex
  maxRetries: 2
```

## Next steps

- [Skills guide](skills.md) -- Reusable configuration
- [Agents overview](agents.md) -- Compare all supported agents
- [SoftwareTask reference](../reference/softwaretask.md) -- Full CRD spec
