#!/usr/bin/env bash
# Triggered by GitHub Actions over SSH on every push to main.
# Pulls the latest source, rebuilds the core image, and restarts the stack.

set -euo pipefail

REPO_DIR=${REPO_DIR:-/opt/billionmail}
BRANCH=${BRANCH:-main}

cd "$REPO_DIR"

echo ">>> Fetching latest code from origin/$BRANCH"
git fetch --all --prune
git reset --hard "origin/$BRANCH"

echo ">>> Rebuilding and restarting stack"
docker compose pull --ignore-buildable
docker compose up -d --build

echo ">>> Pruning dangling images"
docker image prune -f

echo ">>> Status"
sleep 8
docker compose ps
