#!/usr/bin/env bash
# Creates the resource group and AKS cluster for entra-expiry.
# Idempotent: skips whatever already exists.
set -euo pipefail

RG="rijo-expiry-scanner-rg"
CLUSTER="entra-expiry-aks"
LOCATION="southindia"
NODE_COUNT=1
# B2s/B2s_v2 blocked: subscription size allowlist excludes plain B2s, and Bsv2
# family quota is 0/0 (self-serve increase not offered on this sub). D2s_v6 is
# allowed and has StandardDsv6Family quota (0/10). Stop the cluster when idle.
NODE_SIZE="Standard_D2s_v6"

need_login() {
  az account show >/dev/null 2>&1 || { echo "not logged in: run 'az login'" >&2; exit 1; }
}

# --- resource group ---
if az group show -n "$RG" >/dev/null 2>&1; then
  echo "rg exists: $RG"
else
  az group create -n "$RG" -l "$LOCATION" -o none
  echo "rg created: $RG"
fi

# --- AKS cluster ---
if az aks show -g "$RG" -n "$CLUSTER" >/dev/null 2>&1; then
  echo "cluster exists: $CLUSTER"
  STATE=$(az aks show -g "$RG" -n "$CLUSTER" --query provisioningState -o tsv)
  echo "cluster state: $STATE"
  # deallocated cluster: start it
  if az aks show -g "$RG" -n "$CLUSTER" --query powerState.code -o tsv | grep -q Stopped; then
    echo "cluster stopped; starting..."
    az aks start -g "$RG" -n "$CLUSTER" >/dev/null 2>&1 &
  fi
else
  echo "creating cluster $CLUSTER ($NODE_SIZE x$NODE_COUNT, $LOCATION)..."
  az aks create \
    -g "$RG" -n "$CLUSTER" -l "$LOCATION" \
    --node-count "$NODE_COUNT" \
    --node-vm-size "$NODE_SIZE" \
    --enable-oidc-issuer \
    --enable-workload-identity \
    --network-plugin azure \
    --tier free \
    -o none
  echo "cluster created"
fi

# --- record OIDC issuer for later scripts ---
ISSUER=$(az aks show -g "$RG" -n "$CLUSTER" --query oidcIssuerProfile.issuerUrl -o tsv)
if [ -n "$ISSUER" ] && [ "$ISSUER" != "null" ]; then
  mkdir -p "$(dirname "$0")/../.state"
  echo "$ISSUER" > "$(dirname "$0")/../.state/oidc-issuer-url"
  echo "oidc issuer: $ISSUER"
else
  echo "warning: no OIDC issuer URL (cluster may be provisioning)" >&2
fi
