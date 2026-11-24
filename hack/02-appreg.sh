#!/usr/bin/env bash
# Creates the entra-expiry-scanner app registration, grants it Application.Read.All,
# and wires a federated identity credential to the AKS OIDC issuer.
# Idempotent. Prints the appId; no client secrets are created.
set -euo pipefail

APP_NAME="entra-expiry-scanner"
FIC_NAME="aks-entra-expiry"
SUBJECT="system:serviceaccount:expiry-scanner:entra-expiry-scanner"
GRAPH_APP_ID="00000003-0000-0000-c000-000000000000" # Microsoft Graph service principal appId

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE_DIR="$REPO_ROOT/.state"
mkdir -p "$STATE_DIR"

# --- get or create app registration ---
APP_ID=$(az ad app list --display-name "$APP_NAME" --query "[0].appId" -o tsv 2>/dev/null || true)
if [ -n "$APP_ID" ] && [ "$APP_ID" != "null" ]; then
  echo "app registration exists: $APP_ID"
else
  APP_ID=$(az ad app create --display-name "$APP_NAME" --query appId -o tsv)
  echo "app registration created: $APP_ID"
fi
echo "$APP_ID" > "$STATE_DIR/app-id"

# --- ensure its service principal exists ---
SP_ID=$(az ad sp list --id "$APP_ID" --query "[0].id" -o tsv 2>/dev/null || true)
if [ -z "$SP_ID" ] || [ "$SP_ID" = "null" ]; then
  SP_ID=$(az ad sp create --id "$APP_ID" --query id -o tsv)
  echo "service principal created: $SP_ID"
fi

# --- grant Application.Read.All (Application role) + implicit admin consent ---
GRAPH_SP_ID=$(az ad sp list --filter "appId eq '$GRAPH_APP_ID'" --query "[0].id" -o tsv)
APP_ROLE_ID=$(az rest --method GET \
  --url "https://graph.microsoft.com/v1.0/servicePrincipals/$GRAPH_SP_ID/appRoles" \
  --query "value[?value=='Application.Read.All' && contains(allowedMemberTypes, 'Application')].id | [0]" \
  -o tsv)
if [ -z "$APP_ROLE_ID" ]; then
  echo "could not resolve Application.Read.All app role" >&2
  exit 1
fi

ASSIGNED=$(az rest --method GET \
  --url "https://graph.microsoft.com/v1.0/servicePrincipals/$GRAPH_SP_ID/appRoleAssignedTo" \
  --query "value[?appRoleId=='$APP_ROLE_ID' && principalId=='$SP_ID'].id | [0]" -o tsv)
if [ -n "$ASSIGNED" ] && [ "$ASSIGNED" != "null" ]; then
  echo "Application.Read.All already granted"
else
  az rest --method POST \
    --url "https://graph.microsoft.com/v1.0/servicePrincipals/$GRAPH_SP_ID/appRoleAssignedTo" \
    --headers Content-Type=application/json \
    --body "{\"principalId\":\"$SP_ID\",\"resourceId\":\"$GRAPH_SP_ID\",\"appRoleId\":\"$APP_ROLE_ID\"}" \
    -o none
  echo "Application.Read.All granted (admin consent)"
fi

# --- federated identity credential bound to AKS OIDC issuer ---
ISSUER_FILE="$STATE_DIR/oidc-issuer-url"
if [ ! -s "$ISSUER_FILE" ]; then
  echo "missing $ISSUER_FILE — run 01-cluster.sh first" >&2
  exit 1
fi
ISSUER=$(cat "$ISSUER_FILE")

FIC_EXISTS=$(az ad app federated-credential list --id "$APP_ID" \
  --query "[?name=='$FIC_NAME'] | length(@)" -o tsv 2>/dev/null || echo 0)
if [ "$FIC_EXISTS" != "0" ]; then
  echo "federated credential exists: $FIC_NAME"
else
  az ad app federated-credential create --id "$APP_ID" --parameters "$(cat <<EOF
{
  "name": "$FIC_NAME",
  "issuer": "$ISSUER",
  "subject": "$SUBJECT",
  "description": "AKS workload identity for entra-expiry-scanner",
  "audiences": ["api://AzureADTokenExchange"]
}
EOF
)" -o none
  echo "federated credential created: $FIC_NAME -> $SUBJECT"
  # Docs (workload-identity-deploy-cluster): FIC takes a few seconds to
  # propagate; immediate token exchanges can fail until the cache refreshes.
  echo "waiting 30s for FIC propagation..."
  sleep 30
fi

echo "done. appId: $APP_ID"
