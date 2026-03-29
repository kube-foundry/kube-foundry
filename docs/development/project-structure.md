# Project Structure

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
├── docs/                      # Documentation (MkDocs)
├── examples/                  # Example SoftwareTask YAMLs
├── config/                    # Kustomize manifests (kubebuilder generated)
│   ├── crd/bases/             # Generated CRDs (do not edit)
│   ├── rbac/                  # Generated RBAC (do not edit)
│   └── samples/               # Sample CRs
├── test/
│   └── e2e/                   # End-to-end tests
├── hack/                      # Build utilities
├── Makefile                   # Build, test, deploy commands
└── mkdocs.yml                 # Documentation config
```

## Key files

| File | Description |
|---|---|
| `api/v1alpha1/softwaretask_types.go` | SoftwareTask CRD schema |
| `api/v1alpha1/skill_types.go` | Skill CRD schema |
| `internal/controller/softwaretask_controller.go` | Main reconciliation logic |
| `webhook/handler.go` | REST API and GitHub webhook handlers |
| `agent-images/*/entrypoint.sh` | Agent sandbox lifecycle scripts |
| `chart/kube-foundry/values.yaml` | Helm chart defaults |
| `config/crd/bases/*.yaml` | Generated CRD manifests |
