#!/usr/bin/env bash
set -euo pipefail

BINARY_PATH="${1:-./bin/vibebox}"
ENTITLEMENTS_PATH="${ENTITLEMENTS_PATH:-./scripts/vibebox.entitlements.plist}"
CODESIGN_IDENTITY="${CODESIGN_IDENTITY:--}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: signing helper is only supported on macOS" >&2
  exit 1
fi

if ! command -v codesign >/dev/null 2>&1; then
  echo "error: codesign command not found" >&2
  exit 1
fi

if [[ ! -f "${BINARY_PATH}" ]]; then
  echo "error: binary not found: ${BINARY_PATH}" >&2
  exit 1
fi

if [[ ! -f "${ENTITLEMENTS_PATH}" ]]; then
  echo "error: entitlements file not found: ${ENTITLEMENTS_PATH}" >&2
  exit 1
fi

codesign --force --sign "${CODESIGN_IDENTITY}" --entitlements "${ENTITLEMENTS_PATH}" "${BINARY_PATH}"

if ! codesign -d --entitlements - --xml "${BINARY_PATH}" 2>&1 | grep -q "com.apple.security.virtualization"; then
  echo "error: entitlement com.apple.security.virtualization was not found after signing" >&2
  exit 1
fi

echo "signed ${BINARY_PATH} with virtualization entitlement"
