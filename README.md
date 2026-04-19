# Auto-ConfigMap-Restart

A lightweight Kubernetes operator that automatically triggers a rolling restart of your Deployments whenever a watched ConfigMap changes.

No more manual `kubectl rollout restart` after config updates.

## How it works

You create a `ConfigWatcher` resource that maps a ConfigMap to one or more Deployments. The operator watches for ConfigMap changes and patches the Deployment pod template — triggering a standard Kubernetes rolling restart.

```
ConfigMap updated  →  operator detects change  →  Deployment rolling restart
```

## Prerequisites

- Kubernetes 1.24+
- kubectl

## Installation

**Install the CRD:**

```bash
kubectl apply -f config/crd/bases/apps.example.com_configwatchers.yaml
```

**Run the operator locally (for development):**

```bash
go run ./cmd/main.go
```

**Build and deploy to a cluster:**

```bash
make docker-build docker-push IMG=ghcr.io/ori-iro-ori/auto-configmap-restart:latest
make deploy IMG=ghcr.io/ori-iro-ori/auto-configmap-restart:latest
```

## Usage

Create a `ConfigWatcher` in the same namespace as your ConfigMap and Deployments:

```yaml
apiVersion: apps.example.com/v1alpha1
kind: ConfigWatcher
metadata:
  name: my-watcher
  namespace: default
spec:
  configMapName: my-app-config
  deployments:
    - my-app
    - my-worker
```

```bash
kubectl apply -f my-watcher.yaml
```

From now on, any change to `my-app-config` will automatically trigger a rolling restart of `my-app` and `my-worker`.

## Check status

```bash
kubectl get configwatcher my-watcher -o yaml
```

```yaml
status:
  lastRestartedAt: "2026-04-19T11:06:44Z"
  lastSyncedResourceVersion: "591"
  message: Restarted 2 deployment(s)
```

## Spec reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `configMapName` | string | yes | Name of the ConfigMap to watch |
| `deployments` | []string | yes | Deployments to restart when the ConfigMap changes |

The `ConfigWatcher` must be in the same namespace as the ConfigMap and Deployments it references.

## How the restart works

The operator patches `kubectl.kubernetes.io/restartedAt` on the Deployment's pod template with a value derived from the current timestamp and the ConfigMap's `ResourceVersion`. This guarantees a unique value on every change and triggers a rolling update — identical to `kubectl rollout restart`.

## License

Apache 2.0
