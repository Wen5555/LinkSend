# LinkSend protocol V1 implementation specification

Normative requirements supplied by Zhang Yaowen on 2026-09-07. This file records the complete product scope and acceptance obligations. The current product release line is `0.2.x`; protocol V1 is an independent compatibility contract and is not the same as product SemVer. PROGRESS.md records implementation evidence separately. A pending feature here is not permission to remove it from protocol V1.

## 1. Product scope

Build a Go desktop application whose self-hosted server discovers devices and coordinates authenticated, encrypted peer-to-peer file transfers. LAN: server discovery → verified candidates → authenticated encrypted direct connection → file transfer. Different networks: session signaling → local/STUN candidates → Pion ICE checks and nomination → mutually authenticated QUIC → Internet P2P. Cross-NAT transfer is mandatory in v1, not a future roadmap item. Usable public IPv6 may connect directly. No primary broadcast, mDNS or multicast discovery; offline LAN discovery is optional.

Direct failure must identify stage, stable error code and diagnostic evidence. Relay is explicitly unavailable (`relay=false`). Never substitute HTTP upload/download, object storage, WSS file forwarding, TURN allocations, third-party relay SDKs, WebRTC or TCP while claiming the specified solution. Future encrypted relay is only a capability/method/error reservation. STUN alone is not traversal; UDP blocking, mapping/filtering, routes, firewalls and isolation can prevent direct connectivity. Do not guarantee 100% success or fixed network throughput.

V1 targets Windows/macOS desktop, Linux signaling server and portable CLI; Linux desktop is best effort. Mandatory features: trusted pairing, online devices, single/multiple files, folders including empty directories, receiver confirmation, progress, pause, cancel, connection and application-restart resume, integrity and diagnostics. Excluded: mobile/web clients, SaaS tenancy, payments, OAuth, automatic updates/port mapping. Healthy established P2P must survive temporary signaling outages.

## 2. Technology and implementation discipline

Go core/CLI/server/backend; Wails 3 `v3.0.0-beta.18`, React/TypeScript/Vite; verified stable Pion ICE; compatible pinned quic-go; Go standard Ed25519/TLS/x509/rand; SQLite (evaluate modernc.org/sqlite across platforms); slog; pnpm with one frontend lockfile and no unnecessary workspace. Use mature coturn in STUN-only mode, never write a public STUN server. Recheck official APIs, tagged source/examples/CLI help before implementation; record exact versions rather than unverified `latest`. Do not implement custom ICE, crypto, TLS, QUIC, congestion control or UDP reliability.

Inspect cwd, ancestor/project AGENTS.md, Git and available go/node/pnpm/wails (version/doctor)/docker tools. Empty cwd is the root; preserve all existing user code and keys. Never reset --hard, clean -fd, automatically push/deploy, disable firewall, or change proxy/routes. Missing Docker or WebView dependencies must not stop CLI/unit-test work; document limitations. Every milestone: implement → compile → corresponding tests → repair → update PROGRESS. Not run/unavailable is never passed.

## 3. Repository and developer entry points

Exactly two Go modules: root shared core/network/CLI/server/test tools and apps/desktop for Wails/platform dependencies. Root never imports desktop/WebView/React/Wails; server never imports file sending/receiving. Root `go test ./...` does not test nested desktop. Independent module and GOWORK=off checks are required in CI. The root module path is `github.com/Wen5555/LinkSend`; the desktop module path appends `/apps/desktop` and explicitly replaces the root with `../..` for local development. The workspace includes both without masking independent-module checks. Core builds must not need Node or frontend assets.

Repository layout: cmd/{linksend,rendezvous,devtool}; internal/{protocol,identity,signaling,connectivity,transport,transfer,store,app,server,diagnostics,platform}; apps/desktop with thin app.go/main.go, Wails 3 Taskfiles/build/frontend; tests/{integration,natlab}; configs; deploy/{Dockerfile,compose.yml,Caddyfile,coturn/stun-only.conf}; scripts/bootstrap.{ps1,sh}; docs/{SPEC,ARCHITECTURE,PROTOCOL,SECURITY,DEVELOPMENT,TESTING,PERFORMANCE,PROGRESS,ROADMAP}.md and adr; AGENTS.md/README/.gitignore/.editorconfig/.env.example; core and desktop GitHub Actions. Create meaningful packages as implemented, never empty-directory theater.

When desktop target is absent, install pinned verified Wails CLI, inspect help, create using `wails init -n desktop -t react-ts` from apps. Change generated module path/replacement and pnpm commands using actual Wails configuration. Retain go.mod/go.sum/pnpm-lock.yaml/tool version record and go.work.sum only when generated. Native desktop framework commands must also work.

Implement portable Go devtool dispatch, including `check-core`, `demo-local`, `test-nat`, `bench-transport`, `desktop-dev`, `desktop-build`. Root commands `go run ./cmd/rendezvous --config configs/server.dev.toml` and `go run ./cmd/linksend --help` must run. Unavailable NAT environment returns a nonzero explanatory error and reproducible environment, never success. Bootstrap scripts must not require Bash on Windows.

## 4. M1 architecture gate: real ICE plus QUIC

Pion owns candidate gathering/STUN/checks/roles/nomination/keepalive/failure; quic-go owns handshake/TLS/reliable streams/congestion/file bytes. They do not automatically compose. Prefer real bound UDP endpoint → quic.Transport as unique receive owner → QUIC data to transport and actual non-QUIC read/write API adapter to Pion UDP mux. Verify exact APIs, lifetime, data flow, ownership and limits. Never allow Pion and quic-go to read the underlying socket concurrently or bypass Transport after ownership begins. Pion/STUN still parses/authenticates demultiplexed packets.

Every used candidate must map to a live base address/port/interface/family/socket-owner/generation. A STUN mapping cannot be bound locally, WSS source address does not reveal UDP port, and gathering followed by socket closure/rebinding breaks the path. STUN/checks/QUIC must use the same base endpoint for each path. Multiple NICs/families/sessions may use bounded sockets. Choose srflx's recorded base endpoint, never guess from public address. Verify source interface under wildcard binding; explicitly bind when needed.

An ICE Conn → PacketConn reference route is permitted only after validating datagram boundaries/maximum length, truthful source address, target-enforcing WriteTo, packet attribution during candidate change, deadlines/Close/cancellation/concurrency and copy/buffer/kernel-optimization costs. Never cast net.Conn into net.PacketConn, ignore WriteTo target, or claim a wrapper retains performance. A fixed-pair adapter may close/rebuild on path change; no claim of transparent migration. Keep only the verified minimal production design.

Before expanding UI, demonstrate fixture-pinned identities with real authentication, host connectivity, controlled double-NAT srflx, encrypted bytes, bidirectional messages, timeout/close and no relay. Record selected pair, base socket, actual peer/ports, counts/capture. Benchmark native quic-go same version vs integrated path, throughput/CPU/memory/allocations. ADR 0001 must record versions, alternatives tried, successes/failures/limitations and rationale. Localhost is not LAN or dual NAT. Failure requires diagnosis, not a hand-written ICE fallback or silent scope/transport change.

## 5. Sessions and connectivity

Long-term Ed25519 identity, never IP identity. Presence reports identity/capabilities/state, while actual candidates belong to live per-session generations. Each negotiation includes session_id, initiator/recipient, generation and explicit role. Use Pion roles/nomination; QUIC dialer follows session role. Handle simultaneous initiation, duplicate and stale sessions/messages. Trickle candidates as available; check early, prepare LAN and public paths concurrently. LAN preference must follow supported ICE priority/nomination and be measured. Bound candidates/check concurrency/retries/time/messages.

Shared private prefix/public egress/SSID do not prove LAN. Host may mean VPN/public IPv6/LAN. Report `connection_method=lan_direct|internet_p2p|direct_unknown|relay` separately from `transport_protocol=quic`, with evidence. Unknown classification remains unknown. Respect Ethernet/Wi-Fi/Docker/WSL/VM/Tailscale/VPN/TUN; do not exclude all 10/8 or infer usability from names. Provide interface priority/deny options. IPv6 link-local remote scope cannot reuse peer interface indices; explicitly reject unsupported scoped candidates.

Handle IP/NIC/sleep/Wi-Fi/VPN/NAT mapping changes: cancel stale generations, gather again and reconnect preserving verified blocks. V1 allows restart/resume rather than transparent migration. Signaling failure alone cannot kill healthy data. One STUN response cannot definitively classify NAT; CGNAT is not proof of failure. Do not modify the host network.

## 6. Control plane

Single-instance self-hosting, an internal compatibility membership scope and device-to-device code pairing; no full account platform. The membership scope is not exposed as a product concept. Secure initial service bootstrap; current pairing codes use 40 random bits rendered as `ABCD-EFGH`, expire after 10 minutes, are single-use, rate-limited and stored only as digests. Legacy 43-character invitations remain accepted during the compatibility window. Isolate membership scopes and enforce revocation.

Server responsibilities: authenticate/register/group/presence/WSS/candidate/session/revoke/rate-limit. No file APIs or persistence; filenames/manifests preferably travel inside encrypted P2P. Document implemented API, conceptually healthz; invitations/join; devices/list/delete; ws. Messages cover hello/authenticate/presence/connect_request/connect_response/candidate/end_of_candidates/status/heartbeat/error. Common protocol_version/message_id/session_id/sender/recipient/generation/payload fields, applicable omission only with session constraints.

Unknown optional fields must not crash; unknown critical message/version must fail explicitly. Size limits/replay/expiry/authorization validation, heartbeat expiry cleanup and jittered exponential reconnect with true state resync. No permanent zombie devices. Explicit loopback dev HTTP/WS only; public endpoints require verified HTTPS/WSS certificates. STUN UDP and signaling TCP are separate services. No TURN allocations or relay port range. SQLite volume/health/non-root containers/backup instructions; no unnecessary Redis/Postgres/queues/microservices.

## 7. Trust and security

Entering a valid short-lived pairing code enrolls the device and automatically pins all device keys returned by the authenticated membership service; the desktop does not require manual fingerprint comparison. This deliberately weaker first-pairing model trusts the signaling service at enrollment time, but an already pinned key must never be replaced silently. Never upload private keys. Prefer OS secure storage; document restricted-access fallback. Long-term Ed25519 identity binds QUIC certificate key directly or via validated signature binding, with standard crypto. Mutual TLS validates identity, expected target and authorization. Custom pin verification remains mandatory for the data plane; no unconditional skip-verification/nil verifier. 0-RTT user file operations off.

Critical signals/candidates bind sender/recipient/session/generation/freshness, with clear canonical signature encoding rather than unordered raw JSON. Default receiver consent is shown as an incoming confirmation dialog. The user may persist “always accept” for one paired device; transport identity and file-integrity checks still run. Unauthenticated peers cannot access files. Limit auth time, metadata, rate, candidates and sessions/device. Candidate exchange cannot become an open network scanner: authorized signed sessions, reject invalid/broadcast/multicast, loopback only explicit test; bounded rate/count. Server/STUN may observe addresses and online timing; do not claim no metadata. Logs never contain secrets/tokens/invite secrets/file content; diagnostics redact by default.

## 8. File protocol and integrity

LAN and Internet share transfer logic, independent from Pion. Connectivity establishes path, transport authenticates, transfer owns files. Sequence: authenticated session → proposal → explicit accept → manifest agreement → missing block requests/data → block hash and persistence → full hash → safe commit → bilateral completion. Define IDs, Manifest/FileEntry/Chunk/ResumeState/Progress/Result and stable errors. Relative paths only, file/dir type, size/time, block strategy and content identity. Default blocks 4 MiB configurable; blocks are resume/hash units, never new UDP reliability.

Bound binary framing/control metadata, QUIC streams, parallel files/chunks, buffers/queues/memory. No Base64 file data in JSON/WSS/frontend. Buffer lifetime cannot race reuse. Use reputable BLAKE3 and standard vector tests for block and whole-file hashes. Document precompute/streaming choices. Recovery binds TransferID + manifest digest + chunk strategy + contents + peer; name/mtime/size alone insufficient. Detect source changes during prepare/send and abort/new-task, never mix content.

Write to temporary storage, fully verify then commit, default no overwrite. Distinguish read/enqueued/sent/receiver-verified-and-acknowledged byte counters. Progress speed is based on specified actual acknowledged bytes/time; write success is not receipt.

## 9. Filesystem and persistent recovery

Receive only beneath a user-selected root using current Go root-constrained APIs and product policies; Clean/prefix checking alone is insufficient. Reject traversal/absolute/drive/UNC/ADS/reserved names/case collision/excessive length/symlink/reparse/special files. Handle space exhaustion, conflicts and TOCTOU escape. Symlink and special-file support may be rejected explicitly. Never execute received files.

Persist task/peer/digest/destination/chunk strategy/verified recoverable blocks/state. On resume reauthenticate peer/manifest and revalidate staging files; request missing/corrupted chunks. Cover disconnect/kill/restart/IP change/staging damage/source change. DB marks cannot outrun recoverable disk; checkpoint batching allowed with durability. Pause stops scheduling and preserves state; resume reconfirms; cancel stops network/writes and offers keep/cleanup. Hash/I/O/DB off UI callbacks. Every goroutine/subscription has cancellation and shutdown.

Pause/cancel control intent takes precedence over in-flight progress callbacks. A late acknowledgement may update counters for bytes already sent or verified, but must not reopen scheduling, revert the requested control state, or turn a pause into cancellation.

## 10. Application, CLI and UI

Shared internal/app exposes pairing/devices/send/accept/pause/resume/cancel/events. Wails binding stays thin; no network state machine, file loops or DB transactions in binding. DTOs and throttled progress only, no file bytes through JavaScript. Cancel subscriptions on shutdown, no duplicate old listeners.

CLI must support server config, separate profile/data-dir, identity, pairing, devices, receive/listen, file/folder send, accept/reject, status, resume and diagnostics. Test clients require distinct identity/profile/receive root/ports. Explicit test trust/autoaccept allowed, production defaults unchanged.

UI pages: paired devices, pairing code, incoming consent, tasks, settings, diagnostics. The desktop keeps a background receiver online while the app is open; users never create a separate “start receiving” task. File/folder pickers, optional drag/drop; truthful state/path/progress/rate/ETA/errors. Online does not mean connected, host does not mean LAN, relay unavailable. Render untrusted names as text, bind minimal powers. UI mock tests cannot replace real core product acceptance.

Explicit connection states Idle/Gathering/Signaling/Checking/Nominating/Authenticating/Connected/Reconnecting/Failed/Closed; transfer Preparing/AwaitingAcceptance/Transferring/Verifying/Completed/Paused/Recovering/Cancelled/Failed. Document/test legal transitions including parallel gathering/signaling. Errors distinguish signaling unreachable/offline/unpaired/auth/version/no candidate/check timeout/direct failure/relay unimplemented/rejected/source change/hash/disk/path/cancel. Unknown causes cannot be labeled certainly symmetric NAT or firewall.

## 11. Milestones and acceptance

M0: two modules/workspace/template/config/rules/docs/CI; root compile/help/health; desktop starts when supported.
M1: trusted-fixture network gate from §4, including controlled NAT and native baseline; prerequisite for freezing architecture, not replaced by localhost ping.
M2: production identity/pair/group/WSS/presence/session/generation; two CLI profiles discover; spoof/revoke/key-change negatives.
M3: real single file through production signaling on LAN and cross NAT; authenticated encryption/hash/path/no server file data. Public P2P remains v1.
M4: multiple files/folders/empty dirs/chunks/restart/pause/cancel/safe commit; killed process restart ends with matching hash.
M5: Wails real core; Windows↔macOS where available; separate compiled/tested/actual platform evidence.
M6: Docker/STUN-only/HTTPS/packages/diagnostics/bench/docs; no signing/notarization claim without credentials.

Each stage records changed files, commands, passed/failed/not-run checks and next concrete work. Final handoff distinguishes implemented source, compiled in current environment, automated tests, real target validation, pending verification and unimplemented features. Continue independent work when environments are missing; never pretend the full product is complete.

## 12. Test obligations

Core: gofmt check, go vet/test/race ./...; TS/lint/tests/frontend build; independent desktop and GOWORK=off. Race requires real C toolchain, no blanket CGO disable; throughput without race. Devtool sets env cross-platform. Do not delete failures, suppress lint globally or swallow errors.

Unit/compat/security: versions/unknown optional/critical fields, canonical signatures, key replacement, replay/generation, states, framing/invalid lengths, paths/hash/bitmap/error/cancel/resource bounds/abnormal close; bounded fuzz parsers and paths.

demo-local starts independent signaling and two isolated CLI profiles, pairs/pins, discovers/connects/accepts/transfers/hashes/cleans; identity verification remains real. Fixture keys cannot create a production trust-all backdoor.

NAT lab on Linux namespaces/containers: A→NAT A→public network←NAT B←B with public STUN/signaling, no private bypass route. Same bridge containers are not double NAT. Cases: reachable mapping, UDP block, STUN down, overlapping private networks, changed mapping, timeout, no-relay failure. Capture actual candidates/base endpoints/interface counters/file hash. Privilege boundary explicit; cleanup only owned names/rules, never flush host firewall. Unavailable environment means reproducible code + not-run.

File matrix: zero bytes/Chinese names/empty dirs/multiple/large/many small/conflict/case collision/unsafe path/link/disk exhaustion/block corruption/source mutation/process kill/pause/cancel with no subsequent writes.

No relay proof combines selected pair/endpoints and large transfer signaling/STUN bytes/capture; absence of upload API alone is insufficient. Heartbeats may grow with time, control traffic must not scale with file bytes. Audit dependencies/default endpoints; no implicit public relay.

Real Windows↔macOS matrix: same Wi-Fi, mixed wired/wireless, separate homes, home↔hotspot, TUN on/off, sleep/wake, IP renewal. Only actual runs pass; simulated networks never prove every campus/carrier/firewall.

## 13. Performance

bench-transport compares native quic-go, selected ICE path and full file path. Record versions/OS/CPU/NIC/link/chunk/file size/throughput/CPU/peak memory/alloc/GC/path/errors. Current host results only; Linux GSO cannot stand in for Windows/macOS. Label Mbps/MB/s/MiB/s: 1 Gbps=125 MB/s and 2.5 Gbps=312.5 MB/s before overhead, not promises. Significant integration slowdown requires inspection of copy/buffers/queues/batching/locks/concurrency before language/library replacement. LAN native QUIC/TCP-TLS fallback is future measured work, not cancellation of cross-NAT. UDP mapping does not imply TCP reachability. Throttle real progress, never fake speed.

## 14. Reference entry points

- https://pkg.go.dev/github.com/pion/ice/v4
- https://quic-go.net/docs/quic/transport/
- https://pkg.go.dev/github.com/quic-go/quic-go#Transport
- https://quic-go.net/docs/quic/optimizations/
- https://wails.io/docs/gettingstarted/firstproject/
- https://wails.io/docs/reference/project-config/
- https://go.dev/doc/modules/layout
- https://go.dev/doc/articles/race_detector
- https://go.dev/blog/osroot
- https://www.rfc-editor.org/rfc/rfc8445.html
- https://www.rfc-editor.org/rfc/rfc8489.html

Resolve documentation against selected versions, not copied obsolete examples.
