# ADR 0001: one QUIC UDP owner, Pion STUN-only demultiplexing

Date: 2026-09-07. Status: **prototype works on Windows loopback; architecture not frozen**. Dual-NAT and two-machine LAN acceptance remain required.

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

Synchronous initial `ReadNonQUICPacket` with an already-cancelled context initializes the upstream non-QUIC queue before any concurrent operation. This avoids its documented first-read initialization race. The subsequent read pump is the only caller.

## Path and security policy

The low-level endpoint API requires a concrete interface IP and address family. Wildcard binds, multicast, limited broadcast, unspecified and IPv6 link-local addresses are rejected. Loopback requires explicit test configuration. Each endpoint permits at most 32 remote candidates and four explicit STUN URLs. Generation is checked before adding candidates; callers must first verify the signed, authorized session envelope. Pion's remote-IP filter applies the same policy to peer-reflexive discovery. There is no `10/8` blanket exclusion.

Host nomination wait is 0; srflx is 500 ms, using Pion's actual nomination mechanism. Both gather concurrently; a candidate channel enables trickle signaling. A host pair is reported `direct_unknown`, since private addressing or host candidate type does not prove a physical LAN. Srflx pairs are `internet_p2p` (a direct mapped path, not a promise that the test topology is the public Internet). Transport is always `quic`, `relay=false`.

Identity module supplies mutual Ed25519-pinned TLS 1.3. Establish rejects configs without an identity verification callback or client authentication. File operations use neither `DialEarly` nor `ListenEarly`; 0-RTT is disabled. A selected candidate-pair change closes the session and requires a new endpoint/generation; transparent QUIC migration is not claimed. Signaling loss is independent of this endpoint and does not close it.

## Actual evidence, 2026-09-07

`go test ./internal/connectivity ./internal/transport` passed on Windows amd64. Tests cover actual UDP host nomination, authenticated TLS 1.3 QUIC transmission of 1,245,184 bytes, reverse message/receiver SHA256 acknowledgement, wrong pinned key rejection, ICE timeout, stream unblock on peer close, candidate policy/generation, and outgoing STUN datagram/source preservation. These are loopback integration tests, **not two-machine LAN or cross-NAT proof**.

`go test ./internal/transport -run '^$' -bench BenchmarkTransport -benchtime=5x -benchmem` passed, Windows amd64 / Intel Core i9-13980HX / 32 logical processors. Same pinned identity implementation, QUIC configuration, 1 MiB application payload, receiver verification and acknowledgement in both paths; no disk, signaling or race instrumentation.

| Path | ns/op | effective MB/s (decimal) | B/op | allocs/op |
|---|---:|---:|---:|---:|
| Native quic-go | 10,823,960 | 96.88 | 2,410,406 | 4,062 |
| ICE + native quic-go UDP | 10,804,520 | 97.05 | 2,413,068 | 4,053 |

Five iterations are exploratory, not statistically robust. Heap allocations include test `io.ReadAll`, SHA256 acknowledgement handling, and QUIC. CPU time and peak RSS were not measured. No Ethernet/Wi-Fi bandwidth or macOS/Linux optimization claim follows from loopback results. Full file-path performance is a separate benchmark.

Initial `go test -race ./internal/connectivity ./internal/transport` could not start either test process: Windows status `0xc0000139`. This is an environment/runtime failure, not a passed race test or reported data race. Root toolchain investigation may append a later successful command; do not erase the failed attempt.

## Remaining acceptance and limitations

- Real two-host LAN, public IPv6 and controlled dual NAT have not yet run. `tests/natlab` is the reproducible, privileged Linux experiment; it must not be reported as passed without its evidence directory.
- Source-interface behavior is constrained by explicit binds. Multi-interface endpoint selection and ordering belongs in the app coordinator and is not complete merely because this API supports multiple endpoints.
- General route/subnet-directed broadcast rejection requires interface-prefix policy in addition to the currently rejected limited broadcast. Candidate authorization, envelope freshness, message rate limits and per-device active endpoint caps must be enforced by the app/control plane.
- Adapter contract and negative tests continue to expand. No fallback ICE-Conn adapter was attempted because the primary native-UDP integration worked. No alternative production transport, handwritten ICE or relay was introduced.
- Freeze the architecture only after dual-NAT srflx + actual encrypted bytes + failure scenarios + no-relay counters/capture, then integrate and measure the unified file protocol.
