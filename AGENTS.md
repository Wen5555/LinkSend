# LinkSend implementation rules

- Read docs/SPEC.md and docs/PROGRESS.md before changing code.
- Go core and Wails 2 desktop are separate modules; test both, including GOWORK=off.
- V1 includes LAN and cross-NAT P2P. Relay is not implemented; relay=false.
- Use verified Pion ICE and quic-go APIs. Never invent a simplified ICE implementation.
- File bytes must never pass through signaling, JavaScript IPC, or third-party relays.
- Prove the network prototype before expanding the UI. Localhost is not a dual-NAT test.
- Never disable identity verification or fabricate test/network results.
- Protocol changes require documentation and compatibility/security tests.
- Record actual commands, outcomes, limitations and next steps at each milestone.
- Preserve user files; no automatic push, production deployment or host firewall changes.
- Default communication language: Chinese. Windows development shell: PowerShell 7.
