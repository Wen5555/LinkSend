# Acceptance Matrix

This matrix separates source implementation, automated evidence, and real-network validation. A loopback result is never counted as LAN or NAT acceptance.

| Area | Implemented source | Automated/current evidence | Target validation |
|---|---|---|---|
| Identity and trust | Ed25519 identity, invite, fingerprint pinning, revoke | identity/signaling negative tests pass | OOB fingerprint review on two real devices |
| Signaling | SQLite group, HTTP/WSS auth, presence, sessions, candidates, `relay=false` | WSS envelope and replay/session checks pass | HTTPS/WSS deployment with backup/rotation |
| ICE and QUIC | Pion ICE over quic-go-owned UDP socket, TLS 1.3 pinning, explicit QUIC stream abort boundary and read-only direct evidence | Windows loopback host path, timeout/close, wrong pin, reverse encrypted bytes and real quic-go cancellation lifecycle tests pass | Two-host LAN, IPv6, controlled dual NAT srflx |
| File protocol | Manifest, BLAKE3 chunks, safe root staging, resumable checkpoints | zero/small files, folders, empty dirs, corruption/path/restart unit coverage; app direct transfer passes | Large files and process kill on separate disks |
| CLI | identity, pairing, devices, trust, revoke, diagnostics, send, receive | CLI help and shared app direct integration pass | Two independent real profiles over LAN/NAT |
| Desktop | Wails 3 `v3.0.0-beta.18` service/window binding for identity/devices/diagnostics and in-process task orchestration; React transfer/devices/settings UI | Desktop `GOWORK=off` test/build/vet, frontend typecheck/lint/test/build and Windows Wails 3 production build pass; browser wide/narrow rendering checked | Windows native interaction, macOS runtime, native consent/progress workflow |
| Operations | Docker/Caddy/STUN-only examples, healthz, redacted diagnostics | compose config parses; healthcheck is liveness-only and does not prove TLS identity; coturn runtime is not run | Docker engine, HTTPS certificate positive/negative tests, STUN Binding, TURN Allocate rejection, backup/restore |
| Race and static checks | No race-specific code bypass | `go vet ./...`, `go test ./...`, and Windows `go test -race ./...` with MinGW 16.2.0 pass; Stage 1B affected packages repeated 20 times | Repeat on CI and target OSes |

Current external blockers are cross-NAT/IPv6 evidence, repeated physical Windows↔macOS matrix, macOS native interaction, and persistent pause/resume/restart orchestration. Hong Kong production deployment, fixed-code administrator joins, dynamic invitation, restart persistence, and two-way direct QUIC transfers were verified on 2026-09-09; see `docs/PROGRESS.md` and `docs/DEPLOY-HK.md`.
