# Local Development

## Prerequisites

- Go 1.25+
- Docker
- [kind](https://kind.sigs.k8s.io/)
- Helm 3
- kubectl

## Quick start

The `dev` target handles everything:

```bash
make dev
```

This creates a Kind cluster, builds all images, loads them, and installs the Helm chart.

## Step by step

```bash
make kind-create          # create Kind cluster
make docker-build-all     # build all images
make kind-load            # load images into Kind
make helm-install         # install via Helm
```

## Create credentials and test

```bash
kubectl create secret generic factory-creds \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=GITHUB_TOKEN=ghp_...

kubectl apply -f examples/task-basic.yaml
kubectl get st -w
```

## Running tests

```bash
make test                 # unit tests (uses envtest)
make test-e2e             # e2e tests (creates Kind cluster automatically)
```

## Linting

```bash
make lint                 # run golangci-lint with custom plugins
make lint-fix             # auto-fix what's possible
```

## Regenerating after type changes

After editing `api/v1alpha1/*_types.go` or kubebuilder markers:

```bash
make manifests            # regenerate CRDs and RBAC
make generate             # regenerate DeepCopy methods
```

## Cleanup

```bash
make helm-uninstall       # remove Helm release
make kind-delete          # delete Kind cluster
```
