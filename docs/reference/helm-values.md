# Helm Values

Reference for all configurable values in the Kube Foundry Helm chart.

## Install with custom values

```bash
helm install kube-foundry oci://ghcr.io/kube-foundry/kube-foundry/kube-foundry \
  --version 0.1.0 \
  --set sandbox.defaultCPU=4 \
  --set sandbox.defaultMemory=8Gi
```

Or with a values file:

```bash
helm install kube-foundry oci://ghcr.io/kube-foundry/kube-foundry/kube-foundry \
  --version 0.1.0 \
  -f my-values.yaml
```

## Values

### Operator

| Key | Default | Description |
|---|---|---|
| `operator.image.repository` | `ghcr.io/kube-foundry/kube-foundry/operator` | Operator image |
| `operator.image.tag` | `latest` | Image tag |
| `operator.image.pullPolicy` | `IfNotPresent` | Pull policy |
| `operator.replicas` | `1` | Number of operator replicas |
| `operator.resources.limits.cpu` | `500m` | CPU limit |
| `operator.resources.limits.memory` | `256Mi` | Memory limit |
| `operator.resources.requests.cpu` | `100m` | CPU request |
| `operator.resources.requests.memory` | `128Mi` | Memory request |

### Webhook

| Key | Default | Description |
|---|---|---|
| `webhook.enabled` | `true` | Deploy the webhook server |
| `webhook.image.repository` | `ghcr.io/kube-foundry/kube-foundry/webhook` | Webhook image |
| `webhook.image.tag` | `latest` | Image tag |
| `webhook.replicas` | `1` | Number of webhook replicas |
| `webhook.port` | `8080` | Container port |
| `webhook.service.type` | `ClusterIP` | Service type |
| `webhook.service.port` | `80` | Service port |

### Agents

| Key | Default | Description |
|---|---|---|
| `agent.claudeCode.image.repository` | `ghcr.io/kube-foundry/kube-foundry/agent-claude-code` | Claude Code image |
| `agent.claudeCode.image.tag` | `latest` | Image tag |
| `agent.codex.image.repository` | `ghcr.io/kube-foundry/kube-foundry/agent-codex` | Codex image |
| `agent.codex.image.tag` | `latest` | Image tag |
| `agent.openCode.image.repository` | `ghcr.io/kube-foundry/kube-foundry/agent-open-code` | OpenCode image |
| `agent.openCode.image.tag` | `latest` | Image tag |

### Sandbox defaults

| Key | Default | Description |
|---|---|---|
| `sandbox.defaultCPU` | `"2"` | Default CPU limit for sandbox pods |
| `sandbox.defaultMemory` | `"4Gi"` | Default memory limit |
| `sandbox.defaultTimeoutMinutes` | `30` | Default timeout in minutes |

### Credentials

| Key | Default | Description |
|---|---|---|
| `credentials.secretName` | `factory-creds` | Name of the credentials secret |

### Network policy

| Key | Default | Description |
|---|---|---|
| `networkPolicy.enabled` | `true` | Create network policy for sandbox pods |
| `networkPolicy.allowedEgress` | GitHub + Anthropic CIDRs | Allowed outbound CIDR blocks |

### Other

| Key | Default | Description |
|---|---|---|
| `namespace` | `default` | Namespace to deploy into |
