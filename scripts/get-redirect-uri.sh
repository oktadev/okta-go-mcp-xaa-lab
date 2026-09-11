#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${CODESPACE_NAME:-}" || -z "${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-}" ]]; then
  echo "Not running inside a GitHub Codespace (CODESPACE_NAME / GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN not set)." >&2
  exit 1
fi

APP_URL="https://${CODESPACE_NAME}-8080.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN}"

echo "App URL:                  ${APP_URL}"
echo "Redirect URI:              ${APP_URL}/callback"
echo "Post-Logout Redirect URI:  ${APP_URL}/logged-out"
