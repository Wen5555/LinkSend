#!/usr/bin/env sh
set -eu
go version
command -v pnpm >/dev/null 2>&1 && pnpm --version || true
go mod download
(cd apps/desktop && pnpm install --frozen-lockfile)
echo "LinkSend dependencies are ready."
