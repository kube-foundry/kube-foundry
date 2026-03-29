# Webhook Server

The webhook server provides HTTP endpoints for programmatic task submission. It runs as a separate deployment and creates `SoftwareTask` resources in the cluster.

## REST API

### `POST /api/v1/tasks`

Create a task programmatically.

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

### Request fields

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

## GitHub Webhook

### `POST /webhooks/github`

Triggered by GitHub webhook events. Add the `factory:do` label to any GitHub issue, and the webhook server automatically creates a `SoftwareTask` from the issue title and body.

### Setup

1. In your GitHub repo, go to **Settings > Webhooks > Add webhook**.
2. Set the Payload URL to `http://<webhook-service>/webhooks/github`.
3. Set Content type to `application/json`.
4. Optionally set a webhook secret.
5. Select **Issues** events.

### Webhook secret verification

Set the `GITHUB_WEBHOOK_SECRET` environment variable on the webhook deployment to enable signature verification. When set, the server validates the `X-Hub-Signature-256` header on every request.

### Behavior

- Only `labeled` events with the label `factory:do` trigger task creation.
- All other events are acknowledged with 200 OK and ignored.
- Created tasks use defaults: agent `claude-code`, branch `main`, secret `factory-creds`.

## Health check

### `GET /healthz`

Returns 200 OK. Use for liveness/readiness probes.

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `NAMESPACE` | `default` | Namespace for created SoftwareTask resources |
| `PORT` | `8080` | HTTP listen port |
| `GITHUB_WEBHOOK_SECRET` | | Optional secret for GitHub signature verification |
