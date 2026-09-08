# LinkSend implementation progress

Updated: 2026-09-08. This is an active implementation, not an accepted product release.

## Environment and limits

- Windows amd64, PowerShell 7, Go 1.26.5, Node 22.15.0, pnpm 11.19.0.
- The default PATH exposed MinGW-w64 GCC 8.1, which made Windows race binaries exit with `0xc0000139`. Scoop MinGW 16.2.0 is now installed and selected explicitly for race verification.
- Docker 29.2.1 client is installed, but the Linux engine is unavailable.
- Wails 2.15.0 is available at `.tools/bin/wails.exe`; WebView/macOS runtime validation is not available here.
- No production service, firewall rule, proxy, route, automatic deployment or push was performed.

## 2026-09-08 Hong Kong host connectivity preparation

- The reusable `hk-main` SSH alias was resolved and probed successfully (`root@109.66.88.204`, SSH port `60851`); a read-only host audit reported no failed systemd units and 22% root filesystem usage.
- The remote LinkSend rendezvous process is running from `/opt/linksend-lan-test/rendezvous` with `listen = "0.0.0.0:443"`, the configured origin certificate for `linksend.oooai.de`, and `relay=false`/QUIC capabilities. `GET /healthz` returned `status=ok` through both loopback and the public listener.
- coturn is running in STUN-only mode on UDP `3478`; no TURN allocation or relay port range is configured. nftables permits TCP `443` and UDP `3478` and retains a default-drop input policy.
- Windows reachability checks to `109.66.88.204:443` and `109.66.88.204:3478` succeeded. The origin certificate is self-signed but has SANs `linksend.oooai.de` and `*.oooai.de`; normal clients must use the Cloudflare edge hostname and must not disable certificate verification.
- Earlier check (superseded by the domain recheck below): DNS returned NXDOMAIN for both `linksend.oooai.de` and `stun.oooai.de`. Formal Windows/macOS WSS pairing and transfer testing is therefore pending the user's Cloudflare records: proxied `A linksend.oooai.de -> 109.66.88.204` and DNS-only `A stun.oooai.de -> 109.66.88.204`.

## Version control

The repository is managed with Git on `main`, tracks `https://github.com/Wen5555/LinkSend.git`, and has been pushed without force updates. Local identities, databases, build binaries, toolchains and temporary profiles are excluded by `.gitignore`.

## Milestones

| Milestone | Source implemented | Current evidence | Remaining |
|---|---|---|---|
| M0 | Two Go modules, workspace, Wails template, configs, docs, CI and commands | Root tests/vet/race, CLI/devtool, and desktop GOWORK=off checks pass; desktop CI path corrected in source | CI runner confirmation |
| M1 | Pion ICE + quic-go UDP demux, TLS 1.3 pinning, timeout/close paths | Windows loopback host ICE, encrypted bidirectional QUIC, wrong-pin negative tests, benchmark and `demo-local` pass | Real two-host LAN, public IPv6 and controlled dual NAT not run |
| M2 | Ed25519 identity, invite pairing, SQLite group, WSS auth, presence/session routing, revoke and local trust | signaling package tests and demo control plane pass | Reconnect/backoff and multi-session coordinator |
| M3 | Manifest/BLAKE3/secure receiver plus shared direct session API and CLI send/receive | App service test completes paired profiles through WSS, ICE, TLS 1.3, QUIC and transfer; demo transfers 1 MiB with matching content hashes and no file-sized signaling forwarding | Real two-machine LAN, public IPv6 and cross-NAT |
| M4 | Chunk checkpoint/recovery, safe staging, pause/cancel primitives | transfer unit tests and real quic-go loopback cancellation lifecycle tests pass | Process-kill restart acceptance through app/session coordinator |
| M5 | Thin Wails binding to shared identity/devices/diagnostics | Desktop Go source, frontend checks and Windows Wails production build pass | Windows interactive, macOS runtime and task UI |
| M6 | Deployment examples, explicit STUN-only config, diagnostics and benchmark entry points | Compose config parses; STUN-only runtime and HTTPS identity are not run | Docker daemon, HTTPS certificates, STUN Binding/TURN Allocate negative test, NAT lab and package signing |

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
- `go run ./cmd/devtool check-core` now runs ordinary tests, vet and race tests; it passed with the installed Scoop MinGW 16.2.0 toolchain.

## Stage 1A current scope

- Source changes: coturn 4.6.2 now receives the explicit `stun-only` option; the compose healthcheck is documented as liveness only; desktop CI enters `apps/desktop` and prints `GOMOD/GOWORK`; reserved CLI task commands return `NOT_IMPLEMENTED` without dispatching to unrelated state-changing methods.
- This Stage 1A run must separately record root tests, desktop-module tests, frontend checks and compose parsing. Wails production build, HTTPS certificate validation, coturn runtime and real Windows/macOS LAN/NAT traversal are not implied by those checks. Real quic-go loopback cancellation lifecycle is separately covered by Stage 1B.
- Stage 1A is complete in source and local automated checks; Stage 1B below records the current lifecycle implementation and evidence. Real network acceptance remains separate.

## Stage 1B current scope

- `internal/transport.QUICStream` now keeps quic-go `CancelRead`/`CancelWrite` behind a small transport boundary. Transfer cancellation and protocol failure abort both stream directions; normal completion still uses `Close`.
- Candidate exchange now uses a phase context. The first terminal sender/receiver error cancels its sibling, parent cancellation is classified as `CANCELLED`, and a missing `end_of_candidates` is bounded by a candidate phase budget.
- Signaling establishment, candidate exchange, ICE checks and QUIC handshake retain separate cancellation/timeout boundaries. The existing `CHECK_TIMEOUT` code remains the ICE phase code; signaling and QUIC timeout causes are no longer intentionally collapsed into it.
- Real quic-go loopback tests cover blocked acceptance read, blocked receive read, blocked ACK wait, repeated abort, candidate sibling failure, missing end message, malformed message and parent cancellation. These are lifecycle tests, not LAN/NAT evidence.

## Locked versions

- Go 1.26.5, module minimum 1.26.0.
- Pion ICE v4.4.2, quic-go v0.62.0.
- modernc.org/sqlite v1.58.0, coder/websocket v1.8.15, go-toml/v2 v2.4.3, zeebo/blake3 v0.2.4.
- Wails 2.15.0, React 19.2.8, TypeScript 5.9.3, Vite 7.3.6, pnpm 11.19.0.

## Next concrete actions

1. After the two Cloudflare records resolve, run Windows to macOS real two-machine LAN acceptance with the Hong Kong rendezvous service and STUN. Preserve DirectEvidence, transfer hashes and failure-phase output.
2. Stage 2B: run different-network cross-NAT acceptance without relay.
3. Verify IPv6 and Linux interoperability.
4. Add persistent task orchestration, then Wails task UI, after real-network evidence is recorded.

## 2026-09-08 engineering optimization evidence

- Diagnostics now use a separate redacted identity DTO; health failures expose only a stable code/summary. Direct transfers can export read-only `DirectEvidence` from the selected ICE pair, endpoint counters and negotiated TLS/ALPN through the CLI `--evidence` flag.
- Transfer controls use typed operation constants and semantic validation. Accept/ack verified values are bounded, peer error detail is limited to 4 KiB and sanitized, frame limits reject invalid bounds, and progress rates are finite. Resume state parsing is bounded and rejects trailing JSON/unknown file IDs; fresh staging and checkpoint temporary files have best-effort cleanup. Cancellation persists `Cancelled`.
- ICE failures preserve cancellation, timeout, no-candidate and generic ICE failure categories. Signaling/QUIC wrappers retain underlying causes. Trust files are bounded; SQLite rejects unknown schema versions; server expiry cleanup and network closes avoid holding the mutex during websocket close.
- The root module path and desktop module path are now `github.com/Wen5555/LinkSend` and `/apps/desktop`; CI verifies modules with `GOWORK=off` and `go mod verify`. `devtool network-info` reports interface state/address data without credentials.

Verification on Windows amd64: root `gofmt`, `git diff --check`, `go mod verify`, `go test ./...`, `go vet ./...`, `go run ./cmd/devtool check-core` (including race), root and desktop `GOWORK=off` test/build, frontend typecheck/lint/test/build, Wails 2.15.0 production build, `demo-local`, and `network-info` passed. A 20x transfer repeat was first interrupted after an erroneous test harness blocked; the corrected transfer package passed its focused run. `test-nat` remains an explicit nonzero not-run result because this Windows host lacks the privileged Linux namespace fixture.

## 2026-09-08 domain recheck

- `linksend.oooai.de` resolves to Cloudflare addresses `172.67.208.195` and `104.21.45.36`; `stun.oooai.de` resolves directly to `109.66.88.204`.
- Windows STUN Binding request to UDP 3478 received an 88-byte success response with matching cookie and transaction ID. This is STUN reachability, not ICE/NAT acceptance.
- Public HTTPS `/healthz` returns Cloudflare 525. The source certificate is self-signed Ed25519; corresponding Cloudflare source connections fail with `tls: peer doesn't support any of the certificate's signature algorithms`. Origin-local HTTPS with the origin certificate explicitly trusted returns healthy capabilities and `relay=false`.
- `--server` must use `https://linksend.oooai.de`; the CLI converts HTTPS to WSS internally. Stage 2A commands have been corrected accordingly.
- No server configuration, certificate, firewall, or service was changed. Public pairing/transfer remains `BLOCKED_BY_EXTERNAL_ENV` until TLS is corrected and public HTTPS/WSS are verified.
- Read-only evidence: hk-main `/tmp/codex-ssh/linksend-domain-inspect-20260908T150418Z`.
## 2026-09-08 Cloudflare origin TLS repair

- Using the user-provided Cloudflare credential, an Origin RSA certificate for `linksend.oooai.de` was issued and installed on `hk-main`.
- Existing `/opt/linksend-lan-test/origin.crt`, `origin.key` and `server-public.toml` were backed up to `/root/linksend-lan-test-backups/20260908T151853Z/` before replacement.
- The rendezvous process was restarted with the same configuration. Public `https://linksend.oooai.de/healthz` now returns `status=ok`, `protocol_version=1`, `transport=quic`, `relay=false`.
- STUN remains available at `stun:stun.oooai.de:3478` (DNS-only). The certificate is valid for 7 days and must be renewed before `2026-09-15 15:12 UTC`.
- Verification job logs are retained at `/tmp/codex-ssh/linksend-origin-cert-rotate-20260908T151839Z`; rollback is restoring the recorded backup files and restarting the same rendezvous command.

## 2026-09-09 Stage 2A result

- Real Windows↔macOS LAN transfer passed. A 16 MiB random fixture completed through authenticated QUIC with TLS 1.3, `relay=false`, host candidates `10.234.49.250:62995` and `10.234.232.205:62109`, and 1,680 bytes of observed STUN traffic.
- Receiver and sender SHA256 values matched. Full evidence is recorded in `docs/STAGE2A-20260909.md`; sender output is under `.stage2a/20260909-000044/` and is ignored by Git.
- This closes the Stage 2A same-LAN evidence item. Cross-NAT, IPv6, different-network and desktop UI acceptance remain separate.

## 2026-09-09 Stage 2B readiness

- 加固两端 Stage 2A 入口：SkipBuild 复用并校验显式二进制及 SHA256；脚本固定仓库根目录并限制 role。
- CLI 在 --evidence 模式下对失败也输出结构化结果，stderr 与 result.json 分离；接收端新增独立 --wait-timeout。
- 2B-01 跨 NAT、Linux 双 NAT fixture、IPv6、网络切换和桌面 UI 均未运行，保持待验收。详见 docs/STAGE2B.md。

## 2026-09-09 LAN reliability pass

- 当前分支 main，HEAD e2129b5，工作树含未提交 Stage 2B readiness 加固及本轮文档。
- 本轮实际运行：linksend help/receive help、internal/transfer、internal/app、tests/integration、devtool network-info；Go 测试与网络信息检查通过。用户此前报告的 gofmt/go test/go vet/git diff --check 单独标注为既有证据。
- Windows 物理 LAN 候选为以太网 10.234.232.205/16；198.18.0.1/30 Mihomo 虚拟接口排除。macOS 双向实机重复待现场执行。详见 docs/LAN-VALIDATION.md。

## 2026-09-09 任务接口与 Wails UI

- `internal/app/tasks.go` 新增进程内任务编排：异步发送、接收等待、任务快照、单活动任务忙碌保护、取消请求与确认终态、接收确认/拒绝、失败发送重新发送。任务只存当前进程，未宣称重启恢复；原始失败任务保留，重试生成新本地 ID。
- `internal/app/direct.go` 增加本地阶段观察回调，继续复用既有 `SendFilesDetailed`/`ReceiveOnceDetailed` 真实 ICE、TLS、QUIC 和安全落盘路径。
- `apps/desktop/app.go` 增加薄 Wails 绑定、原生文件/目录选择器和环境变量驱动的 bind/STUN 配置；`frontend/src/App.tsx` 提供传输、设备、设置/诊断三个区域，任务操作连接真实后端快照。
- 自动化：`go test ./...`、`go vet ./...`、前端 `typecheck`/`lint`/`test`/`build` 通过；本轮 Wails 交互运行和 Windows/macOS 新版本双机验收尚未运行。
- 暂缓：跨 NAT、网络迁移、进程重启恢复、持久化历史、并发队列、复杂桌面交互和安装签名。

## 2026-09-09 Windows 初版产品化

- `apps/desktop/app.go` 增加本地偏好原子保存（服务地址、绑定地址、STUN、设备名、接收目录）、真实网卡枚举、加入设备组、完整指纹信任和安全打开接收目录的 Wails 桥接；环境变量仍优先覆盖本地设置，既有身份目录不变。
- `frontend/src/App.tsx` 重做为传输、设备、设置与诊断三页应用壳：发送端只显示等待确认，接收端才显示接受/拒绝；任务轮询与远端状态刷新分离；浏览器桥未注入时显示明确不可用提示。
- `frontend/src/App.css` 使用浅色语义 tokens、侧栏导航、状态条、分组表面、可见焦点和减少动效规则，完成 Playwright 传输/设备/设置三页截图检查。
- 自动化通过：桌面 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...`；前端 `typecheck`、`lint`、`test`、`build`；Wails 2.15.0 production build；`git diff --check`。
- 产物位于 `.artifacts/windows-preview/20260909-0405/`，含 `LinkSend.exe`、`BUILD-INFO.json`、`SHA256SUMS.txt`、中文启动说明、验证记录和浏览器渲染截图；ZIP 位于 `.artifacts/windows-preview/LinkSend-windows-preview-20260909-0405.zip`。
- Wails 原生窗口点击与截图、跨 NAT/IPv6、网络迁移和重启恢复仍未运行，均保持 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV` 边界。
- Windows 启动冒烟：运行 `apps/desktop/build/bin/LinkSend.exe` 后进程保持运行超过 3 秒并由本次检查结束；未进行人工点击和双机传输。

## 2026-09-09 Windows 初版复核修补

- 接收任务快照现在保留已认证 `peer_id`、manifest 内容摘要和条目数量；接收确认信息可在任务区核对发送者、内容和总量。
- `OpenTaskDirectory` 改为按已完成接收任务 ID 校验目录，避免前端传入任意本地路径；桌面接收等待时限固定为 10 分钟，ICE 检查时限为 30 秒。
- 前端任务/远端轮询加入 in-flight 防重叠；操作调用增加 pending 门闩；完成接收任务提供打开目录操作。
- 复核构建：根模块测试/桌面测试与 vet、前端 typecheck/lint/test/build、Wails 2.15.0 production build 均通过；交付目录已更新为最后一次构建的二进制和 SHA256。

## 2026-09-09 隔离端到端与远端健康复核

- 在独立临时 rendezvous（`127.0.0.1:18887`、独立 SQLite/profile）上完成两个随机身份的 bootstrap、邀请、加入、双向完整指纹信任和 16 MiB 随机文件传输；发送端与接收端独立 SHA256 一致，TLS 1.3、QUIC、`relay=false`。临时 profile、密钥、数据库和大文件已在测试后清理，仅保留本记录摘要。
- 香港主站 `hk-main` 只读探测通过；`https://linksend.oooai.de/healthz` 返回 `status=ok`、`protocol_version=1`、`relay=false`、`transport=quic`；远端 443 由 rendezvous 监听，3478 由 coturn STUN-only 监听。
- 该隔离测试证明 CLI/核心真实链路，不等价于物理 LAN、跨 NAT 或 Wails 原生窗口验收。
- 生产 EXE 使用隔离 `LINKSEND_DATA_DIR` 启动冒烟保持运行超过 3 秒；WebView2 Runtime 152.0.4191.66 已安装。当前会话未进行人工窗口点击，桌面交互仍待现场验收。
