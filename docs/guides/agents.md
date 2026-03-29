# Agents

Kube Foundry supports three agent runtimes. Each runs in its own container image with an entrypoint that handles git setup, skill injection, agent execution, and PR creation.

## Supported agents

| Agent | Image | API Key Secret Field | MCP Support |
|---|---|---|---|
| `claude-code` | `agent-images/claude-code` | `ANTHROPIC_API_KEY` | Yes |
| `codex` | `agent-images/codex` | `OPENAI_API_KEY` | No |
| `open-code` | `agent-images/open-code` | `ANTHROPIC_API_KEY` | Yes |

## Sandbox lifecycle

Every task goes through the same lifecycle inside the sandbox pod:

1. **Configure git** -- set credentials, author name/email, authenticate with `gh`
2. **Clone repo** -- shallow clone of the specified branch
3. **Create work branch** -- `factory/<taskname>`
4. **Apply skills** -- inject prompts, files, MCP config, run init commands
5. **Run agent** -- execute the AI agent with the task description
6. **Commit and push** -- stage all changes, commit, push to work branch
7. **Create PR** -- open a pull request against the base branch

## Choosing an agent

Set the `agent` field in your SoftwareTask:

```yaml
spec:
  agent: claude-code  # default
```

### Claude Code

Best for complex, multi-file tasks. Supports MCP servers for external tool access. Uses `ANTHROPIC_API_KEY`.

### Codex

OpenAI's coding agent. Uses `OPENAI_API_KEY`. Does not support MCP servers -- a warning is logged if MCP servers are configured.

### OpenCode

Open-source coding agent that uses Anthropic's API. Supports MCP servers. Uses `ANTHROPIC_API_KEY`.

## Credentials

The credentials secret must contain the appropriate API key for the chosen agent:

```bash
# For Claude Code or OpenCode
kubectl create secret generic factory-creds \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=GITHUB_TOKEN=ghp_...

# For Codex
kubectl create secret generic factory-creds \
  --from-literal=OPENAI_API_KEY=sk-... \
  --from-literal=GITHUB_TOKEN=ghp_...
```

## Custom resources

| Resource | Description |
|---|---|
| `resources.cpu` | CPU limit for the sandbox pod (default: `2`) |
| `resources.memory` | Memory limit (default: `4Gi`) |
| `resources.timeoutMinutes` | Max execution time (default: `30`, max: `120`) |
