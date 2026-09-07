$ErrorActionPreference = "Stop"
go version
if (Get-Command pnpm -ErrorAction SilentlyContinue) { pnpm --version }
go mod download
Push-Location apps/desktop
try { pnpm install --frozen-lockfile } finally { Pop-Location }
Write-Host "LinkSend dependencies are ready."
