# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in Kube Foundry, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please send a report via email to the repository maintainers or use [GitHub's private vulnerability reporting](https://github.com/kube-foundry/kube-foundry/security/advisories/new).

### What to include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### What to expect

- **Acknowledgment** within 48 hours of your report
- **Status update** within 7 days with an assessment and expected timeline
- **Fix or mitigation** as soon as reasonably possible, depending on severity

We will coordinate disclosure with you and credit you in the release notes (unless you prefer otherwise).

## Security Considerations

Kube Foundry runs AI agents in Kubernetes pods that interact with external services (GitHub, AI APIs). Operators should be aware of the following:

### Secrets management
- **Never commit API keys or tokens** to the repository
- Store secrets using Kubernetes Secrets and reference them via environment variables
- Use short-lived tokens where possible

### Pod isolation
- Agent pods run with restricted security contexts by default
- Network policies limit egress to only required endpoints (GitHub, AI provider APIs)
- Each task runs in its own pod with its own credentials

### RBAC
- The operator requires cluster-level permissions to manage pods and custom resources
- Use the principle of least privilege when configuring service accounts
- Review the RBAC manifests in `config/rbac/` before deploying

### Webhook security
- GitHub webhooks are validated using HMAC-SHA256 signatures
- Always configure a webhook secret in production
