# LinkSend Desktop Shell

This Wails 3 module (`v3.0.0-beta.18`) is the desktop shell for LinkSend. It exposes the shared
Go identity, device, and redacted diagnostics services to the React frontend.
ICE, QUIC, file reads/writes, BLAKE3 verification, and transfer state remain
in the root Go module; file bytes must never cross Wails JavaScript IPC.

## Development

From this directory, install the frontend dependencies and start Wails:

```powershell
Set-Location D:\apps\Osend\apps\desktop\frontend
pnpm install --frozen-lockfile

Set-Location ..
wails3 dev -config ./build/config.yml
```

`D:\apps\Osend\.tools\bin\wails3.exe` is the checked project-local Wails 3
entry point on Windows and is used by the Taskfile commands.

## Build

```powershell
Set-Location D:\apps\Osend\apps\desktop
wails3 task build ARCH=amd64
```

## Module Boundary

The repository intentionally has two Go modules: the root module contains the
portable core and this nested module contains Wails and native desktop
dependencies. `go.work` enables local shared-core development. CI also runs
this module with `GOWORK=off` so its explicit `replace github.com/Wen5555/LinkSend =>
../..` dependency is independently verified.

Run the desktop checks from `apps/desktop`; do not add network or filesystem
transfer logic to React or Wails bindings.
