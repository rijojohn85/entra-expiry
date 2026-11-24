#!/usr/bin/env bash
# Creates dummy app registrations ("fixtures") with secrets/certs expiring at
# controlled dates, so the scanner has data to find.
#
#   fx-a  secret expires in  +5 days
#   fx-b  secret expires in +25 days
#   fx-c  cert   expires in +45 days (self-signed)
#   fx-d  secret expired 10 days ago
#   fx-e  healthy: secret expires in +200 days
#
# Graph lets addPassword/keyCredentials carry a custom endDateTime — that is
# how we manufacture near-expiry data without waiting 24 months.
#
# The tool never reads secret VALUES; secrets created here are throwaway and
# never echoed to the terminal or written to disk.
# Idempotent: apps that already exist are left alone.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE_DIR="$REPO_ROOT/.state"
mkdir -p "$STATE_DIR"
GRAPH="https://graph.microsoft.com/v1.0"

iso_after()  { date -u -d "+$1 days"  +%Y-%m-%dT%H:%M:%SZ; }
iso_before() { date -u -d "$1 days ago" +%Y-%m-%dT%H:%M:%SZ; }

# object id (not appId) is what per-app Graph calls need.
app_object_id() {
  az ad app list --display-name "$1" --query "[0].id" -o tsv 2>/dev/null || true
}

# add_password OBJECT_ID DISPLAY_NAME END_DATE
# POSTs /applications/{id}/addPassword with a custom endDateTime.
add_password() {
  local obj_id="$1" name="$2" end="$3"
  az rest --method POST \
    --url "$GRAPH/applications/$obj_id/addPassword" \
    --headers Content-Type=application/json \
    --body "{\"passwordCredential\":{\"displayName\":\"$name\",\"endDateTime\":\"$end\"}}" \
    --query "secretText" -o none # secretText is dropped, never printed
}

# create_app NAME -> echoes object id
create_app() {
  local name="$1"
  local obj_id
  obj_id=$(app_object_id "$name")
  if [ -n "$obj_id" ] && [ "$obj_id" != "null" ]; then
    echo "app exists: $name" >&2
    echo "$obj_id"
    return
  fi
  obj_id=$(az ad app create --display-name "$name" --query id -o tsv)
  echo "app created: $name" >&2
  echo "$obj_id"
}

# --- fx-a: secret expiring in 5 days ---
OBJ=$(create_app "fx-a-expiring-secret")
add_password "$OBJ" "fx-a" "$(iso_after 5)"

# --- fx-b: secret expiring in 25 days ---
OBJ=$(create_app "fx-b-expiring-secret")
add_password "$OBJ" "fx-b" "$(iso_after 25)"

# --- fx-c: cert expiring in 45 days ---
OBJ=$(create_app "fx-c-expiring-cert")
if [ -n "$OBJ" ] && [ "$OBJ" != "null" ]; then
  CERT="$STATE_DIR/fx-c-cert.pem"
  if [ ! -s "$CERT" ]; then
    openssl req -x509 -newkey rsa:2048 -nodes -sha256 \
      -subj "/CN=fx-c-expiring-cert" -days 45 \
      -keyout "$STATE_DIR/fx-c-key.pem" -out "$CERT"
  fi
  # Idempotent: skip PATCH if the app already has keyCredentials. Re-PATCHing
  # with recomputed dates fails because Graph validates endDateTime against the
  # certificate's actual notBefore/notAfter (InvalidKeyEndDate).
  HAS_KEY=$(az ad app show --id "$OBJ" --query "length(keyCredentials)" -o tsv)
  if [ "$HAS_KEY" != "0" ]; then
    echo "fx-c cert already uploaded" >&2
  else
    # keyCredentials key is base64 DER, not PEM. Send the cert's REAL validity
    # dates: docs (applications-how-to-add-certificate) say endDateTime must be
    # <= 1 year after startDateTime, and Graph cross-checks with the cert.
    CERT_START=$(openssl x509 -in "$CERT" -noout -startdate | cut -d= -f2-)
    CERT_END=$(openssl x509 -in "$CERT" -noout -enddate | cut -d= -f2-)
    START_ISO=$(date -u -d "$CERT_START" +%Y-%m-%dT%H:%M:%SZ)
    END_ISO=$(date -u -d "$CERT_END" +%Y-%m-%dT%H:%M:%SZ)
    DER_B64=$(openssl x509 -in "$CERT" -outform DER | base64 -w0)
    az rest --method PATCH \
      --url "$GRAPH/applications/$OBJ" \
      --headers Content-Type=application/json \
      --body "{\"keyCredentials\":[{\"displayName\":\"fx-c\",\"type\":\"AsymmetricX509Cert\",\"usage\":\"Verify\",\"key\":\"$DER_B64\",\"startDateTime\":\"$START_ISO\",\"endDateTime\":\"$END_ISO\"}]}" \
      -o none
    echo "fx-c cert uploaded ($START_ISO -> $END_ISO)" >&2
  fi
fi

# --- fx-d: secret already expired (will be, within ~2 minutes) ---
# NOTE: Graph rejects a past endDateTime on addPassword (InvalidKeyEndDate).
# Trick: create with a 2-minute window; it becomes permanently "expired"
# right after creation, which is exactly what the scanner should find.
OBJ=$(create_app "fx-d-expired-secret")
add_password "$OBJ" "fx-d" "$(date -u -d '+2 minutes' +%Y-%m-%dT%H:%M:%SZ)"

# --- fx-e: healthy, expires in 200 days ---
OBJ=$(create_app "fx-e-healthy")
add_password "$OBJ" "fx-e" "$(iso_after 200)"

echo "fixtures done. state files: $STATE_DIR"
