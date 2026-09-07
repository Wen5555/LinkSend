# Acceptance Matrix

This matrix separates source implementation, automated evidence, and real-network validation. A loopback result is never counted as LAN or NAT acceptance.

| Area | Implemented source | Automated/current evidence | Target validation |
|---|---|---|---|
| Identity and trust | Ed25519 identity, invite, fingerprint pinning, revoke | identity/signaling negative tests pass | OOB fingerprint review on two real devices |
| Signaling | SQLite group, HTTP/WSS auth, presence, sessions, candidates, `relay=false` | WSS envelope and replay/session checks pass | HTTPS/WSS deployment with backup/rotation |
| ICE and QUIC | Pion ICE over quic-go-owned UDP socket, TLS 1.3 pinning | Windows loopback host path, timeout/close, wrong pin and reverse encrypted bytes pass | Two-host LAN, IPv6, controlled dual NAT srflx |
| File protocol | Manifest, BLAKE3 chunks, safe root staging, resumable checkpoints | zero/small files, folders, empty dirs, corruption/path/restart unit coverage; app direct transfer passes | Large files and process kill on separate disks |
| CLI | identity, pairing, devices, trust, revoke, diagnostics, send, receive | CLI help and shared app direct integration pass | Two independent real profiles over LAN/NAT |
| Desktop | Wails 2 binding for identity/devices/diagnostics; React shell | GOWORK=off Go checks, frontend checks and Windows Wails build pass | Windows runtime, macOS runtime, consent/progress task UI |
| Operations | Docker/Caddy/STUN-only examples, healthz, redacted diagnostics | compose config and healthz pass locally | Docker engine, HTTPS certificates, backup/restore |
| Race and static checks | No race-specific code bypass | `go vet ./...`, `go test ./...`, and Windows `go test -race ./...` with MinGW 16.2.0 pass | Repeat on CI and target OSes |

Remaining release blockers are real two-machine LAN/NAT evidence, desktop task UI integration, pause/cancel/restart orchestration at app level, and production deployment verification.
