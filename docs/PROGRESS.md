# LinkSend implementation progress

Updated: 2026-09-07. This is an active implementation, not an accepted product release.

## Environment and limits

- Windows amd64, PowerShell 7, Go 1.26.5, Node 22.15.0, pnpm 11.19.0.
- The default PATH exposed MinGW-w64 GCC 8.1, which made Windows race binaries exit with `0xc0000139`. Scoop MinGW 16.2.0 is now installed and selected explicitly for race verification.
- Docker 29.2.1 client is installed, but the Linux engine is unavailable.
- Wails 2.15.0 is available at `.tools/bin/wails.exe`; WebView/macOS runtime validation is not available here.
- No production service, firewall rule, proxy, route, automatic deployment or push was performed.

## Milestones

| Milestone | Source implemented | Current evidence | Remaining |
|---|---|---|---|
| M0 | Two Go modules, workspace, Wails template, configs, docs, CI and commands | `go test ./...` passes; CLI/devtool compile | Independent desktop and GOWORK=off checks recorded below |
| M1 | Pion ICE + quic-go UDP demux, TLS 1.3 pinning, timeout/close paths | Windows loopback host ICE, encrypted bidirectional QUIC, wrong-pin negative tests, benchmark and `demo-local` pass | Real two-host LAN, public IPv6 and controlled dual NAT not run |
| M2 | Ed25519 identity, invite pairing, SQLite group, WSS auth, presence/session routing, revoke and local trust | signaling package tests and demo control plane pass | Reconnect/backoff and multi-session coordinator |
| M3 | Manifest/BLAKE3/secure receiver plus shared direct session API and CLI send/receive | App service test completes paired profiles through WSS, ICE, TLS 1.3, QUIC and transfer; demo transfers 1 MiB with matching content hashes and no file-sized signaling forwarding | Real two-machine LAN, public IPv6 and cross-NAT |
| M4 | Chunk checkpoint/recovery, safe staging, pause/cancel primitives | transfer unit tests pass | Process-kill restart acceptance through app/session coordinator |
| M5 | Thin Wails binding to shared identity/devices/diagnostics | Desktop Go source compiles in workspace | Independent desktop/GOWORK=off and Windows/macOS runtime |
| M6 | Deployment examples, STUN-only config, diagnostics and benchmark entry points | Source and docs present | Docker daemon, HTTPS certificates, NAT lab and package signing |

## Commands run in this milestone

Passed:

```text
gofmt -w internal/signaling/client_test.go internal/app/service.go internal/app/demo.go cmd/linksend/main.go cmd/rendezvous/main.go cmd/devtool/main.go apps/desktop/app.go
go test ./...
go test ./internal/signaling
go run ./cmd/devtool demo-local
```

`demo-local` reported authenticated TLS 1.3, `relay=false`, a direct loopback candidate pair, 1,048,576 file bytes and about 3,171 signaling-forwarded bytes. ICE library close logs mention closed sockets during cleanup; they do not change the result.

Not run or blocked:

- The old GCC 8.1 race attempt exited with `0xc0000139` before tests; it is retained as a toolchain failure, not a race result.
- Real two-machine LAN, cross-NAT srflx, public IPv6, Windows to macOS and Linux namespace NAT: no target topology/privilege available.
- Docker compose and production HTTPS: Docker Linux engine/certificates unavailable.
- Independent desktop module and frontend checks were run below; macOS runtime and interactive desktop acceptance remain pending.

Additional verification completed after the initial update:

- `go vet ./...`: passed.
- `GOWORK=off go test ./...` at the root: passed.
- With Scoop MinGW 16.2.0 selected via `CC`, `CXX` and `PATH`, `go test -race ./...` passed across all root packages and integration tests.
- `apps/desktop` with `GOWORK=off`: `go test ./...` and `go build ./...` passed.
- `apps/desktop/frontend`: `pnpm run typecheck`, `pnpm run lint`, `pnpm run test` (3 tests) and `pnpm run build` passed.
- Wails 2.15.0 production build passed and produced `apps/desktop/build/bin/LinkSend.exe` on Windows amd64.
- After shared direct-session changes, Wails production build was rerun and passed again.
- `go run ./cmd/devtool test-nat` correctly returned nonzero with an explicit Windows/Linux-privilege explanation.
- `go run ./cmd/devtool bench-transport` passed on this Windows host: native 102.18 MB/s, ICE integration 98.87 MB/s for one exploratory iteration; this is loopback evidence only.
- `docker compose -f deploy/compose.yml config` passed with an explicit bootstrap token; the Docker engine itself remains unavailable, so image build/start was not run.
- `internal/app.TestDirectServiceSendAndReceive` passed: two profiles used the shared `ConnectDirect`/`AcceptDirect` API and `SendFiles`/`ReceiveOnce` API for an authenticated transfer.

## Locked versions

- Go 1.26.5, module minimum 1.26.0.
- Pion ICE v4.4.2, quic-go v0.62.0.
- modernc.org/sqlite v1.58.0, coder/websocket v1.8.15, go-toml/v2 v2.4.3, zeebo/blake3 v0.2.4.
- Wails 2.15.0, React 19.2.8, TypeScript 5.9.3, Vite 7.3.6, pnpm 11.19.0.

## Next concrete actions

1. Connect Wails transfer task UI to the shared direct session API and expose consent/progress events.
2. Add a privileged Linux NAT fixture without flushing host firewall state.
3. Add reconnect/backoff and multi-session coordination while preserving verified chunks.
4. Re-run the full file matrix after app-level pause/resume/cancel wiring.
