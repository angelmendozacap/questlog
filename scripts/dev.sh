#!/usr/bin/env bash
# One-command local stack: Postgres + Keycloak + both APIs + both apps.
set -euo pipefail
cd "$(dirname "$0")/.."

docker compose -f deploy/docker-compose.yml up --build "$@"
