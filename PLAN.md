# entra-expiry — Build Plan

Go tool that finds Entra ID app registration secrets and certificates that are expiring or expired.
Runs as one-shot `scan` and as a monitored in-cluster `serve` service.

## Rules

- Own tenant only (JOPACK). No client tenants, data, or code.
- Read expiry metadata only — never secret values.
- No credentials in the repo. `az login` locally, workload identity in cluster.
- Graph via plain `net/http` + `azidentity` (no MS Graph SDK).
- Logging: `log/slog` (JSON to stdout).
- Permission: read-only `Application.Read.All`.

## Environment (verified 2026-11-24)

| Item | Value |
|---|---|
| Tenant | JOPACK PRIVATE LIMITED, tenantId `3265dadc-4723-4129-b4ae-14217dc7376f`, user = Global Admin |
| Subscription | "Azure subscription" `b6a593a3-d580-454a-bed8-00bdcc766610` — fresh; only `azure-signup-rg` |
| Key Vault | Dropped — not needed. Graph reads app registrations directly |
| Cluster | New dedicated: RG `rijo-expiry-scanner-rg`, region southindia, Standard_B2s × 1, `--enable-oidc-issuer --enable-workload-identity`. `az aks stop` when idle |
| Toolchain | go 1.24.5, golangci-lint 2.12.2, docker 29, kubectl 1.33, helm 3.18 |

## Phase 1 — Azure setup (`hack/` scripts, idempotent)

1. `hack/01-cluster.sh` — RG `rijo-expiry-scanner-rg` + AKS `entra-expiry-aks` (B2s, southindia, workload identity + OIDC issuer enabled).
2. `hack/02-appreg.sh` — app registration `entra-expiry-scanner`:
   - Graph app role assignment: `Application.Read.All` (Application permission), admin consent granted (Global Admin).
   - Federated identity credential: issuer = AKS OIDC issuer URL, subject = `system:serviceaccount:expiry-scanner:entra-expiry-scanner`.
   - No client secret.
3. `hack/03-fixtures.sh` — dummy apps (real Graph calls; Graph allows custom `endDateTime` on `addPassword` / `keyCredentials`):

   | App | Fixture |
   |---|---|
   | fx-a | secret expires in 5 days |
   | fx-b | secret expires in 25 days |
   | fx-c | self-signed cert (openssl) expires in 45 days |
   | fx-d | secret already expired (10 days ago) |
   | fx-e | healthy — 200 days (proves filtering) |

## Phase 2 — Go tool

```
cmd/entra-expiry/       main, cobra root
internal/graphclient/   paged GET /v1.0/applications, retry on 429/5xx honoring
                        Retry-After, exponential backoff + jitter, context
                        timeouts, %w wrapping, azidentity token
internal/provider/      Provider interface + RealProvider + FakeProvider
internal/owner/         parallel owner lookups, errgroup.SetLimit(8)
internal/report/        expiry evaluation against window, report structs
```

Commands:

- `scan` — `--days N` (default 30), `--json`. Exit 0 even when findings exist.
- `serve` — `--interval 1h`. `/healthz`, `/metrics` (client_golang gauges: expired, expiring, scan duration, scan errors), SIGTERM graceful shutdown.

Auth: `DefaultAzureCredential` locally (az login), workload identity credential in cluster. Credential injected into provider.

## Phase 3 — Packaging

- Multi-stage Dockerfile: `golang:1.24` build → `gcr.io/distroless/static-debian12`, `USER 65532:65532`, `CGO_ENABLED=0`.
- Kustomize manifests: namespace `expiry-scanner`, ServiceAccount annotated with **both** `azure.workload.identity/client-id: <appId>` (injects `AZURE_CLIENT_ID`, per AKS workload-identity docs) and `azure.workload.identity/use: "true"` is a **pod label** requirement — Deployment pod template gets the label `azure.workload.identity/use: "true"`, probes, resource limits.
- Graph token scope must be the v2 format `https://graph.microsoft.com/.default` (raw resource URI fails with WorkloadIdentityCredential).
- Image: GHCR `ghcr.io/rijojohn85/entra-expiry`.

## Phase 4 — CI + ops

- GitHub Actions: `gofmt -l`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, docker build/push on main.
- `hack/stop.sh` / `hack/start.sh` — `az aks stop/start`.

## Learning checkpoints

1. Retry/backoff + context propagation + error wrapping.
2. Interface + fake provider test pattern.
3. errgroup bounded concurrency + `-race`.
4. Workload identity federation token flow.

## Execution order

Phase 1 scripts → Phase 2 (FakeProvider tests first, then live tenant) → Phase 3 → deploy → Phase 4.
Cluster stopped at end of each working session.
