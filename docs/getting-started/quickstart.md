# Quickstart

After [installing](installation.md) Kube Foundry and creating your credentials secret, you can submit your first task.

## Submit a task

Create a file called `task.yaml`:

```yaml
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

Apply it:

```bash
kubectl apply -f task.yaml
```

## Watch it work

```bash
kubectl get st -w                          # watch status transitions
kubectl logs -f pod/add-login-page-sandbox # watch agent output
```

The task will progress through phases: `Pending` → `Running` → `Completed`.

## Check the result

When the task completes, a PR will appear on your repo:

```bash
kubectl get st add-login-page -o jsonpath='{.status.pullRequestURL}'
```

## Using a different agent

By default, tasks use Claude Code. To use Codex or OpenCode:

```yaml
spec:
  agent: codex       # or: open-code
  credentials:
    secretRef: factory-creds  # must contain OPENAI_API_KEY for codex
```

## Using the REST API

You can also submit tasks via HTTP:

```bash
curl -X POST http://<webhook-service>/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/yourorg/yourapp",
    "task": "Add dark mode support"
  }'
```

## Next steps

- [Skills](../guides/skills.md) -- Standardize agent behavior across tasks
- [MCP Servers](../guides/mcp-servers.md) -- Give agents access to external tools
- [SoftwareTask Reference](../reference/softwaretask.md) -- Full spec reference
