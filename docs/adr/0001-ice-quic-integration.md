# ADR 0001: one QUIC UDP owner, Pion STUN-only demultiplexing

Date: 2026-09-07. Status: **physical LAN and an explicitly mapped independent dual-NAT case pass; architecture not frozen**. Public IPv6, network switching and additional NAT behaviors remain required.

## Version and primary-source audit

Go 1.26.5; `github.com/pion/ice/v4 v4.4.2`; `github.com/quic-go/quic-go v0.62.0`. Read the downloaded, exact-version `transport.go`, `agent_options.go`, `udp_mux.go`, `udp_mux_universal.go`, `gather.go`, `agent_handlers.go` and the following official references:

- https://quic-go.net/docs/quic/transport/
- https://pkg.go.dev/github.com/pion/ice/v4@v4.4.2
- https://pkg.go.dev/github.com/quic-go/quic-go@v0.62.0#Transport

Pion `AgentConfig` is deprecated in this version. Implementation uses `NewAgentWithOptions`, `WithUDPMux`, `WithUDPMuxSrflx`, `WithUrls`, `WithNetworkTypes`, candidate types host/srflx, and mDNS disabled. There is no TURN URL and no relay candidate gathering. Pion's `UniversalUDPMuxDefault` validates STUN transaction IDs/server sources for srflx discovery; the ICE agent authenticates connectivity checks and performs nomination.

## Data direction and ownership

```
one explicitly bound *net.UDPConn per endpoint/generation
  -> quic.Transport (sole UDP reader and writer)
     -> QUIC / TLS 1.3 / streams / file layer
     -> ReadNonQUICPacket -> bounded STUN PacketConn -> UniversalUDPMux -> Pion
        Pion -> bounded STUN PacketConn -> quic.Transport.WriteTo
```

QUIC receives the real `*net.UDPConn`, retaining its supported operating-system UDP optimizations. ICE `Conn` is intentionally not used for application bytes. `Dial`/`Accept` establishes Pion controlling/controlled roles; selected candidate pair determines the QUIC destination. QUIC client is always the ICE controlling peer. Host and srflx candidates share the same live mux and base socket. A srflx address is never bound locally. The endpoint is kept alive for the entire session.

The adapter permits only decoded, size-limited STUN Binding datagrams. Application data, TURN allocation and other non-QUIC messages are rejected. Its sole read pump preserves the actual packet source and copies STUN only into bounded 32-entry queues. Maximum accepted STUN packet is 4096 bytes; maximum UDP read buffer prevents silent truncation. Pion still performs protocol authentication after this syntactic validation. QUIC's own non-QUIC receive queue is 32 packets in this version; sustained STUN floods may drop checks. QUIC file packets do not pass through these queues or copies.

Read/write deadlines and cancellation are adapter-local, never applied to the base socket where they could interrupt QUIC. A queued STUN write owns a copied buffer. A write already passed to the operating system can complete even if its caller times out; this is restricted to bounded STUN control data. Endpoint close cancels adapter operations, closes mux/agent/QUIC/socket, and joins adapter workers. There is no second reader on the UDP socket.

At the stream layer, `internal/transport.QUICStream` is the only quic-go-specific lifecycle adapter exposed to transfer orchestration. Normal completion calls `Stream.Close` (send-direction FIN); cancellation and protocol failure call `CancelRead` and `CancelWrite` with a local application error code. This does not change UDP ownership or close signaling sessions used by other work.

Synchronous initial `ReadNonQUICPacket` with an already-cancelled context initializes the upstream non-QUIC queue before any concurrent operation. This avoids its documented first-read initialization race. The subsequent read pump is the only caller.

## Path and security policy

The low-level endpoint accepts either a concrete interface IP or an empty bind. Empty binds discover currently-up safe unicast interface addresses and select one deterministically using exact, user-supplied interface priority/exclusion lists; no network nature is inferred from interface names, private prefixes, presence or candidate type. Wildcard binds, multicast, limited broadcast, unspecified and IPv6 link-local addresses are rejected. Loopback requires explicit test configuration. Each endpoint permits at most 32 remote candidates and four explicit STUN URLs. Generation is checked before adding candidates; callers must first verify the signed, authorized session envelope. Pion's remote-IP filter applies the same policy to peer-reflexive discovery. There is no `10/8` blanket exclusion.

Host nomination wait is 0; srflx is 500 ms, using Pion's actual nomination mechanism. Both gather concurrently; a candidate channel enables trickle signaling. Candidate types alone are reported `direct_unknown`. During the 2026-09-11 Windows/macOS test, a local srflx candidate was nominated while the actual QUIC destination remained the peer's private host address. The former srflx-to-`internet_p2p` inference was therefore removed. Physical LAN/Internet route classification remains PARTIAL pending independent route evidence. Transport is always `quic`, `relay=false`.

Identity module supplies mutual Ed25519-pinned TLS 1.3. Establish rejects configs without an identity verification callback or client authentication. File operations use neither `DialEarly` nor `ListenEarly`; 0-RTT is disabled. A selected candidate-pair change closes the session and requires a new endpoint/generation; transparent QUIC migration is not claimed. Signaling loss is independent of this endpoint and does not close it.

The chosen interface/address is monitored while the endpoint lives. If that exact address disappears or the nominated pair changes, the endpoint signals `PathChanged`; transport closes the old QUIC session and task orchestration moves an eligible task to Recovering. Resume creates a new session, ICE credentials and endpoint after explicit user confirmation. This is reconnection with verified block recovery, not transparent QUIC migration.

Structured evidence now records the actual base socket, exact interface label, address family, allowlisted candidate descriptions/types, bounded ICE state timeline, TLS/ALPN, `relay=false`, STUN request/response counts, and signaling JSON byte counts. It does not retain ufrag/password, tokens, private keys, candidate extensions or file bytes. Endpoint close no longer uses a fixed sleep; adapter workers and the local-address monitor have explicit completion joins, and affected race tests pass.

## Actual evidence, 2026-09-07

`go test ./internal/connectivity ./internal/transport` passed on Windows amd64. Tests cover actual UDP host nomination, authenticated TLS 1.3 QUIC transmission of 1,245,184 bytes, reverse message/receiver SHA256 acknowledgement, wrong pinned key rejection, ICE timeout, stream unblock on peer close, candidate policy/generation, and outgoing STUN datagram/source preservation. These are loopback integration tests, **not two-machine LAN or cross-NAT proof**.

`go test ./internal/transport -run '^$' -bench BenchmarkTransport -benchtime=5x -benchmem` passed, Windows amd64 / Intel Core i9-13980HX / 32 logical processors. Same pinned identity implementation, QUIC configuration, 1 MiB application payload, receiver verification and acknowledgement in both paths; no disk, signaling or race instrumentation.

| Path | ns/op | effective MB/s (decimal) | B/op | allocs/op |
|---|---:|---:|---:|---:|
| Native quic-go | 10,823,960 | 96.88 | 2,410,406 | 4,062 |
| ICE + native quic-go UDP | 10,804,520 | 97.05 | 2,413,068 | 4,053 |

Five iterations are exploratory, not statistically robust. Heap allocations include test `io.ReadAll`, SHA256 acknowledgement handling, and QUIC. CPU time and peak RSS were not measured. No Ethernet/Wi-Fi bandwidth or macOS/Linux optimization claim follows from loopback results. Full file-path performance is a separate benchmark.

Initial `go test -race ./internal/connectivity ./internal/transport` could not start either test process: Windows status `0xc0000139`. This is an environment/runtime failure, not a passed race test or reported data race. Root toolchain investigation may append a later successful command; do not erase the failed attempt.

## Controlled dual-NAT evidence, 2026-09-11

`lab-server` ran the Linux amd64 CLI and rendezvous from the current dirty source inside a rootless outer user/mount/network/PID namespace. Inside it, the fixture created two independent NAT namespaces with overlapping `10.77.0.0/24` LANs and a separate `198.18.0.0/15` WAN; coturn was STUN-only and the HTTPS rendezvous used a one-run CA. A MASQUERADE-only/state-filtered run ended in `CHECK_TIMEOUT`. The positive run added explicit per-peer UDP SNAT/DNAT mappings and completed 8,388,608 encrypted file bytes with equal SHA256, TLS 1.3, ALPN `linksend/1`, matching session IDs, real STUN and NAT counters, `relay=false`, and `connection_method=direct_unknown`.

This is real independent dual NAT, not loopback or one Docker bridge, but the positive case is intentionally scoped to a NAT with known endpoint-independent port mapping. It does not prove symmetric/endpoint-dependent NAT traversal; the negative result remains a product capability FAIL while relay is not implemented. The sanitized archive is `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz`, SHA256 `ba7447fe3eb603b669db3a76ab7ba2be049d7abeeefdb94b4927161f43f7eebf`. All experiment namespaces, processes, identities, private keys, file bodies and user-local coturn files were deleted after local archive verification.

## Remaining acceptance and limitations

- Real two-host LAN and the explicitly mapped controlled dual-NAT case have evidence; public IPv6, physical network switching and additional unmapped/endpoint-dependent NAT types remain unverified or FAIL. `scripts/dual-nat-netns.sh` is the reproducible Linux root/user-namespace experiment and must not be generalized beyond the archived topology and counters.
- The current coordinator selects one base socket per endpoint. Explicit ordering/exclusion and reconnection are implemented, but simultaneous multi-path QUIC migration is intentionally not claimed.
- General route/subnet-directed broadcast rejection requires interface-prefix policy in addition to the currently rejected limited broadcast. Candidate authorization, envelope freshness, message rate limits and per-device active endpoint caps must be enforced by the app/control plane.
- Adapter contract and negative tests continue to expand. No fallback ICE-Conn adapter was attempted because the primary native-UDP integration worked. No alternative production transport, handwritten ICE or relay was introduced.
- Freeze the architecture only after public IPv6, physical network switching and a documented product decision for endpoint-dependent NAT failure (relay, explicit unsupported status, or another reviewed in-scope mechanism). The current mapped dual-NAT srflx/QUIC bytes and MASQUERADE-only failure are evidence, not universal reachability.
