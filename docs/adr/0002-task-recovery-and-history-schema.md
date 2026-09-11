# ADR 0002: logical tasks, revision-guarded history, and verified block recovery

Date: 2026-09-11. Status: **implemented and locally validated; physical network-switch and dual-NAT recovery pending**.

## Decision

LinkSend keeps one logical `task_id` across pause/restart recovery. Every execution has a new `attempt_id`; every direct connection has a new `session_id`; ICE generation is scoped to that session. `revision` monotonically versions the complete public task snapshot. All asynchronous updates and terminal callbacks carry their attempt ID, and persistence accepts only a greater revision, so an old callback cannot overwrite a new attempt.

Public task state is Preparing, AwaitingAcceptance, Transferring, Verifying, Paused, Recovering, Completed, Rejected, Cancelled, or Failed. Internal request phases such as `pause_requested`/`cancel_requested` are visible while cancellation is pending but are not extra success states. Completed requires verified receiver content, successful file commit, sender receipt of completed, and receiver receipt of confirmed.

Control intent has precedence inside an attempt. An ACK/progress callback that arrives after `pause_requested` may advance actual/verified counters for work already performed, but it cannot change the state back to Transferring/Verifying or enable new scheduling. This rule was added after the macOS arm64 packaging CI exposed a late-ACK pause race; the regression now waits on a receiver checkpoint rather than scheduler timing and passes on Windows and the same arm64 runner.

Private recovery metadata is never exposed through WSS or Wails. It binds direction, peer ID/fingerprint, source paths or target directory, TransferID, manifest digest, chunk size, total bytes/file count, sent-chunk bitmap and byte counters. Resume is an explicit user action. Sender rebuilds the manifest and rejects changed source content; receiver matches the recovery identity, rehashes staging/committed records, and requests only missing or damaged blocks.

## Persistence and migration

`task-history.sqlite` schema 2 stores revision, JSON snapshot and private recovery JSON in one revision-guarded row. Startup isolates invalid rows into `task_quarantine` and continues loading valid tasks. A durable write failure revokes `history_persisted`; it does not change the independently tested byte-resume implementation flag.

Before migrating a nonzero older schema, LinkSend executes SQLite `VACUUM INTO` and creates `task-history.sqlite.schema-v<old>-<UTC>.bak` with mode `0600`. v1 rows remain compatible: missing task/attempt identifiers are filled, and records without complete recovery metadata become `failed/TASK_INTERRUPTED` rather than pretending to resume.

Rollback to a v1 binary is manual and backup-based:

1. Close every LinkSend desktop/CLI process using the profile.
2. Preserve the schema 2 database separately for forward recovery.
3. Replace `task-history.sqlite` with the verified `task-history.sqlite.schema-v1-*.bak` from immediately before migration.
4. Start the old binary and verify history before deleting either copy.

An old binary must not open schema 2 directly. WAL/SHM files from a running process must never be copied as a rollback shortcut.

## Byte accounting and commit boundary

`sent_bytes` and `received_bytes` are actual data-plane bytes; `retransmitted_bytes` is the subset previously sent in an older attempt; `verified_bytes` and logical completion count unique receiver-verified content; `committed_bytes/files` advance only after successful filesystem commit. Each committed file writes a digest-bound commit record to the checkpoint. A later file conflict/permission/disk failure leaves earlier commit records verifiable and the task non-Completed.

Local real Pion ICE + quic-go tests cover missing blocks, deliberate staging damage, complete Service restart, source change, pinned peer change, stale attempt callbacks and partial commits. Physical process-kill/network-switch recovery and receiver-committed/sender-unconfirmed behavior remain acceptance targets, not inferred successes.
