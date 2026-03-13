# k8s-gangway

[![CI](https://github.com/devopsbyday1/k8s-gangway/actions/workflows/ci.yml/badge.svg)](https://github.com/devopsbyday1/k8s-gangway/actions/workflows/ci.yml)

> _(noun): An opening in the bulwark of the ship to allow passengers to board or leave the ship._

A modernized fork of [vmware-archive/gangway](https://github.com/vmware-archive/gangway) — a web application that enables OIDC authentication flows for Kubernetes cluster access. The upstream project was archived in 2020; this fork brings it up to date with current Go, security, and Kubernetes standards.

**What it does:** Gangway lets users self-configure their `kubectl` credentials in a few clicks by walking them through an OIDC browser flow and presenting ready-to-run `kubectl` commands.

![gangway screenshot](docs/images/screenshot.png)

## Changes from upstream

- Go 1.23, module path `github.com/devopsbyday1/k8s-gangway`
- `dgrijalva/jwt-go` (abandoned, CVE-affected) replaced with `coreos/go-oidc/v3` — full JWKS-based token verification
- All dependencies updated (oauth2, crypto, client-go, gorilla, etc.)
- `ioutil` replaced with `os`/`io` throughout
- Template embedding via native `//go:embed` (replaces `esc` codegen tool)
- Dockerfile: `golang:1.23-alpine` + `debian:12-slim` (was Stretch + Debian 9 EOL)
- Generated kubeconfig uses an exec credential helper script (replaces deprecated `auth-provider: oidc`, removed in kubectl 1.30) — auto-refreshes via refresh_token with no extra tools
- `commandline` page: Option 1 downloads a self-contained helper script (set-and-forget); Option 2 is a static token fallback
- Ingress manifest updated to `networking.k8s.io/v1` with `pathType: Prefix`
- cert-manager annotation updated to `cert-manager.io/cluster-issuer`
- Travis CI replaced with GitHub Actions (CI + multi-arch release to ghcr.io)
- Helm chart added (`helm/gangway/`)
- Image: `ghcr.io/devopsbyday1/k8s-gangway`

## How It Works

Kubernetes supports [OpenID Connect Tokens](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens) as a user authentication mechanism. Gangway acts as an OIDC client that:

1. Redirects the user to the upstream Identity Provider (IdP)
2. Receives the authorization code callback
3. Exchanges the code for an ID token (stored in an encrypted session cookie)
4. Presents the user with `kubectl` commands and a downloadable kubeconfig

The user's credentials are never shared with Gangway; it only handles the OAuth2 authorization code flow.

<p align="center">
    <img src="docs/images/gangway-sequence-diagram.png" width="600px" />
</p>

## kubectl Credential Setup

After authenticating through gangway, the commandline page presents two options for configuring `kubectl`.

### Option 1 — Set and forget (recommended)

The commandline page serves a self-contained shell script from the `/helper.sh` endpoint. Once downloaded, it handles token refresh automatically with no extra tools.

1. Download the helper script (it embeds your current tokens):

   ```bash
   curl -sf "https://gangway.example.com/helper.sh" -o ~/.kube/gangway-<cluster>.sh && chmod 700 ~/.kube/gangway-<cluster>.sh
   ```

2. Configure kubectl to use it:

   ```bash
   kubectl config set-credentials "<username>@<cluster>" \
       --exec-api-version=client.authentication.k8s.io/v1 \
       --exec-command=/bin/sh \
       --exec-arg=-c \
       --exec-arg="exec $HOME/.kube/gangway-<cluster>.sh" \
       --exec-interactive-mode=Never
   ```

   The commandline page shows the exact commands with all values filled in.

How it works:

- The script stores tokens in `~/.kube/.gangway-<cluster>-tokens.json`
- On each `kubectl` call it checks the id-token expiry from the JWT
- If still valid, returns the cached token immediately
- If expired, calls the OIDC token endpoint with the refresh_token, stores the new tokens, and returns the new id-token
- Requires only `curl` and `base64` — universally available on Linux/macOS, no additional installs needed
- When the IdP SSO session eventually expires (typically weeks/months), the script prints a message asking the user to re-authenticate via gangway

### Option 2 — Static token

The commandline page also shows a simple `--token=<id-token>` option. This works immediately but the token expires at the IdP's configured TTL (typically 1 hour), after which `kubectl` returns `Unauthorized` until the user re-authenticates via gangway.

Use this option only when Option 1 is not available (e.g. no shell on the client machine).

## Kubernetes API Server Configuration

The API server must be configured for OIDC:

```bash
kube-apiserver \
  --oidc-issuer-url="https://your-idp.example.com/" \
  --oidc-client-id=<your-client-id> \
  --oidc-username-claim=email \
  --oidc-groups-claim=groups
```

See the [Kubernetes authentication docs](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#configuring-the-api-server) for full details.

## Configuration

Gangway is configured via a YAML file (pass with `-config`) and/or environment variables prefixed with `GANGWAY_`.

| Field | Env var | Required | Description |
|-------|---------|----------|-------------|
| `clusterName` | `GANGWAY_CLUSTER_NAME` | yes | Display name for the cluster |
| `issuerURL` | `GANGWAY_ISSUER_URL` | recommended | OIDC issuer URL for token verification via JWKS discovery |
| `authorizeURL` | `GANGWAY_AUTHORIZE_URL` | yes | OIDC authorization endpoint |
| `tokenURL` | `GANGWAY_TOKEN_URL` | yes | OIDC token endpoint |
| `clientID` | `GANGWAY_CLIENT_ID` | yes | OAuth2 client ID |
| `clientSecret` | `GANGWAY_CLIENT_SECRET` | yes* | OAuth2 client secret |
| `redirectURL` | `GANGWAY_REDIRECT_URL` | yes | Callback URL (must match IdP config) |
| `apiServerURL` | `GANGWAY_APISERVER_URL` | yes | Kubernetes API server URL |
| `sessionSecurityKey` | `GANGWAY_SESSION_SECURITY_KEY` | yes | Key used to encrypt session cookies |
| `scopes` | `GANGWAY_SCOPES` | no | OAuth2 scopes (default: `openid profile email offline_access`) |
| `usernameClaim` | `GANGWAY_USERNAME_CLAIM` | no | JWT claim to use as username (default: `nickname`) |
| `clusterCAPath` | `GANGWAY_CLUSTER_CA_PATH` | no | Path to cluster CA cert |
| `httpPath` | `GANGWAY_HTTP_PATH` | no | URL path prefix (e.g. `/gangway`) |
| `serveTLS` | `GANGWAY_SERVE_TLS` | no | Serve TLS directly (default: false) |

> **Note on `issuerURL`:** When set, gangway performs full OIDC token verification (signature, issuer, audience, expiry) using the provider's JWKS endpoint. Without it, token claims are parsed without signature verification — acceptable for closed environments but not recommended for production.

Example config file:

```yaml
clusterName: my-cluster
issuerURL: https://accounts.google.com
authorizeURL: https://accounts.google.com/o/oauth2/auth
tokenURL: https://oauth2.googleapis.com/token
clientID: my-client-id
redirectURL: https://gangway.example.com/callback
apiServerURL: https://k8s-api.example.com
scopes:
  - openid
  - profile
  - email
  - offline_access
usernameClaim: email
```

## Deployment

### Helm (recommended)

```bash
helm install gangway helm/gangway/ \
  --namespace gangway --create-namespace \
  --set config.clusterName=my-cluster \
  --set config.issuerURL=https://your-idp.example.com \
  --set config.authorizeURL=https://your-idp.example.com/authorize \
  --set config.tokenURL=https://your-idp.example.com/token \
  --set config.clientID=my-client-id \
  --set config.clientSecret=my-client-secret \
  --set config.redirectURL=https://gangway.example.com/callback \
  --set config.apiServerURL=https://k8s-api.example.com \
  --set config.sessionSecurityKey=$(openssl rand -base64 32) \
  --set ingress.enabled=true \
  --set ingress.host=gangway.example.com
```

For production, store secrets in an existing Kubernetes Secret and reference it:

```bash
kubectl -n gangway create secret generic gangway-secrets \
  --from-literal=clientSecret=<secret> \
  --from-literal=sessionSecurityKey=$(openssl rand -base64 32)

helm install gangway helm/gangway/ \
  --set existingSecret=gangway-secrets \
  ...
```

### Raw manifests

Reference manifests are in `docs/yaml/`. Apply in order:

```bash
# Replace ${GANGWAY_HOST} in 05-ingress.yaml first
kubectl apply -f docs/yaml/01-namespace.yaml
kubectl apply -f docs/yaml/02-config.yaml
kubectl apply -f docs/yaml/03-deployment.yaml
kubectl apply -f docs/yaml/04-service.yaml
kubectl apply -f docs/yaml/05-ingress.yaml

# Create session encryption key secret
kubectl -n gangway create secret generic gangway-key \
  --from-literal=sessionkey=$(openssl rand -base64 32)
```

## Build

Requirements: Go 1.23+

```bash
git clone https://github.com/devopsbyday1/k8s-gangway
cd k8s-gangway
go build ./...
go test ./...
```

Build the container image:

```bash
docker build -t ghcr.io/devopsbyday1/k8s-gangway:latest .
```

## Docker Image

```
ghcr.io/devopsbyday1/k8s-gangway:<tag>
```

Images are published automatically on tag push via GitHub Actions (linux/amd64 and linux/arm64).

## Identity Provider Configs

See `docs/` for IdP-specific setup guides:

- [Auth0](docs/auth0.md)
- [Google](docs/google.md)
- [Dex](docs/dex.md)

## License

Apache 2.0 — see [LICENSE](LICENSE).
