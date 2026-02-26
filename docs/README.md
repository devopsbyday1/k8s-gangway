# Deploying Gangway

Gangway is stateless and straightforward to deploy on Kubernetes. The primary deployment method is the Helm chart; the `docs/yaml/` directory contains reference manifests.

## Helm (recommended)

See the [main README](../README.md#deployment) for full Helm deployment instructions.

## Raw YAML manifests

The `docs/yaml/` directory contains numbered manifests:

| File | Creates |
|------|---------|
| `01-namespace.yaml` | `gangway` namespace |
| `02-config.yaml` | ConfigMap with gangway config |
| `03-deployment.yaml` | Deployment |
| `04-service.yaml` | ClusterIP Service |
| `05-ingress.yaml` | Ingress (networking.k8s.io/v1, cert-manager) |
| `role/rolebinding.yaml` | Example ClusterRoleBinding |

Before applying, replace the `${GANGWAY_HOST}` placeholder in `05-ingress.yaml`, and update `02-config.yaml` with your OIDC provider details.

```bash
kubectl apply -f docs/yaml/01-namespace.yaml
kubectl apply -f docs/yaml/02-config.yaml
kubectl apply -f docs/yaml/03-deployment.yaml
kubectl apply -f docs/yaml/04-service.yaml
kubectl apply -f docs/yaml/05-ingress.yaml

# Create session encryption key
kubectl -n gangway create secret generic gangway-key \
  --from-literal=sessionkey=$(openssl rand -base64 32)
```

## Path Prefix

To host Gangway at a sub-path (e.g. `https://example.com/gangway`), set `httpPath` in the ConfigMap or via the `GANGWAY_HTTP_PATH` environment variable. All redirects will include the configured prefix automatically.

## RBAC

After a user authenticates through Gangway, grant them Kubernetes permissions via a RoleBinding or ClusterRoleBinding. An example is in `docs/yaml/role/rolebinding.yaml` — update the `subjects[].name` field with the user's OIDC identity (the value of the configured `usernameClaim`).

```bash
kubectl apply -f docs/yaml/role/rolebinding.yaml
```

## Identity Provider Configs

- [Auth0](auth0.md)
- [Google](google.md)
- [Dex](dex.md)

## Docker Image

```
ghcr.io/devopsbyday1/k8s-gangway:<tag>
```
