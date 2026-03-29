# Installation

## Prerequisites

- Kubernetes cluster (or [kind](https://kind.sigs.k8s.io/) for local dev)
- Helm 3
- An Anthropic API key (for Claude Code), OpenAI API key (for Codex), or both
- A GitHub token with repo access

## Install with Helm

```bash
helm install kube-foundry oci://ghcr.io/kube-foundry/kube-foundry/kube-foundry --version 0.1.0
```

## Create credentials

Create a Kubernetes secret with your API keys:

```bash
kubectl create secret generic factory-creds \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=GITHUB_TOKEN=ghp_...
```

For Codex, use `OPENAI_API_KEY` instead of (or in addition to) `ANTHROPIC_API_KEY`.

## Verify

```bash
kubectl get pods
```

You should see the operator and webhook server running:

```
NAME                                     READY   STATUS    RESTARTS   AGE
kube-foundry-operator-5696b6c4f8-xxxxx   1/1     Running   0          30s
kube-foundry-webhook-59fcc4454-xxxxx     1/1     Running   0          30s
```

## Uninstall

```bash
helm uninstall kube-foundry
```
