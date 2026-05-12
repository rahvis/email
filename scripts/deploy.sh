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

echo ">>> Health checks"
for path in / /login /overview; do
	body=$(curl -fsSL --max-time 20 "http://127.0.0.1${path}")
	if ! grep -q "PING2" <<<"$body"; then
		echo "Health check failed: ${path} did not include PING2"
		exit 1
	fi
	echo "OK http://127.0.0.1${path}"
done

curl -fsS --max-time 20 http://127.0.0.1/api/languages/get >/tmp/billionmail-languages-health.json
echo "OK http://127.0.0.1/api/languages/get"
