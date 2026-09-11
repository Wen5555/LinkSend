# LinkSend implementation progress

Updated: 2026-09-11. Current candidate product version: `0.2.0`; protocol version: V1. This is an active implementation, not an accepted product release.

## 2026-09-11 v0.2.0 版本化、总体文档与交付事务（进行中）

- 版本决策：从 `0.1.0` 提升到 `0.2.0`，因为本轮在 1.0 前加入任务 schema 2、暂停/重启恢复、验证后缺块续传、多网卡诊断与连接生命周期修复，属于明显功能扩展；不提升到 `1.0.0`，因为 MASQUERADE-only NAT、物理网络切换、完整原生交互、签名/公证等仍未通过。
- 产品版本与协议分离：`internal/protocol.ProductVersion=0.2.0`；协议 `Version=1`、TLS ALPN `linksend/1` 和 V1 文件帧保持不变。CLI/rendezvous `--version`、`/healthz.version`、capabilities、Wails DTO、Windows/NSIS、macOS plist 和前端 package metadata 统一使用 `0.2.0`。
- 仓库实际使用 Wails 3 `v3.0.0-beta.18`；旧 AGENTS 项目说明中的 Wails 2 不是有效实现状态，不允许据此回迁。
- `.artifacts`、本地 Wails 工具、Task cache、输出目录、WebView2 bootstrapper 和临时远程脚本已加入忽略规则，但没有删除任何既有文件。历史 `0.1.0` Stage/Release 文档保留原事实并标记为历史快照。
- 用户已明确香港入口是测试主站，并授权以后每次版本更新同步服务。每次仍执行 manager resolve→probe→audit-host 与 `inspect → backup → change → verify → rollback-ready`；只操作 `/opt/linksend-lan-test` 的测试服务，不修改防火墙、路由、DNS、代理或正式身份数据。本轮实际部署证据完成后追加。
- 本节开始时仍是 dirty 工作树，基线 HEAD `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`；旧 r2 资产继续标为 `UNCOMMITTED_TEST_SNAPSHOT`，不会放入新 ZIP/DMG 或改写为新提交来源。完成源码提交后从该真实 commit 重新构建并记录 revision、`vcs.modified=false`、大小、SHA256、工具、UTC 时间、签名/公证、原生状态和 workflow/head SHA。
- `0.2.0` 本机预提交门槛：根 `diff --check`、mod verify、全量 test、race、vet、GOWORK=off test/build PASS；P0 立即重连 20 次和真实 QUIC 终态/恢复定向套件 3 次 PASS；Wails bindings 1 service / 27 methods / 14 models，生成后前端 frozen install/typecheck/lint/8 tests/build PASS；桌面独立 mod verify/test/vet/build、Wails production 与 NSIS 3.12 PASS。
- 版本资源复现并修复：Wails 3 beta.18 自动 build-assets 生成的 Windows version info 缺 `FileVersion` 显示字符串，使主 EXE 的 PowerShell VersionInfo 为空；将 fixed file/product version 规范为 `0.2.0.0`、使用 `0409` string table 并补 `FileVersion=0.2.0` 后，主 EXE 与 installer 的 FileVersion/ProductVersion 均实际返回 `0.2.0`。新增跨源码/Wails/安装元数据同步测试防止回退。
- 源码提交 `4b7ccf66f89c9720d9b5970e5d610b453048d69c` 已建立。直接在保留 untracked 的主工作树和共享 Git worktree 构建均被 Go 标为 `vcs.modified=true`，两次资产均拒绝部署；改用独立干净 clone 后 Linux amd64 rendezvous 的 `go version -m` 为该 revision、`vcs.modified=false`，大小 11,309,218 bytes，SHA256 `c549cdbf1bd461d6f583302f96700471000aacba1fed912c66d6e74912f30247`。
- 香港测试主站已按 manager 事务完成 `0.2.0` 同步：备份 `/opt/linksend-lan-test/backups/20260911T101714Z-v0.2.0-4b7ccf6`，旧 PID `352572` → 新 PID `381064`；配置 SHA 与数据库 schema/integrity 不变。公网 health 报告产品 `0.2.0` / 协议 V1；两次完成后立即重连、拒绝后立即完成各传 2 MiB 均 PASS。测试运行时已删除，两条测试设备记录均撤销，额外数据库清理备份为 `/opt/linksend-lan-test/backups/20260911T102329Z-v0.2.0-lifecycle-cleanup`。旧主站立即重试 FAIL 因实际部署复核而关闭。

## 2026-09-11 P0/P1/P2 分阶段可靠性改进（历史 dirty snapshot）

- 基线仍为 `main` / `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`；本节全部结果来自保留既有修改的 dirty working tree。未 reset、clean、提交、推送、创建 Release 或复用旧资产冒充本轮构建。桌面实际为 Wails 3 `v3.0.0-beta.18`。
- P0：同设备信令连接替换时按连接所有权清理旧协商；旧 handler 和旧 generation 不能删除/污染新会话；同时发起以设备 ID 字典序收敛到一个 initiator/session；信令短断不关闭健康 QUIC；`confirmed`、拒绝和 error 使用真实 QUIC 半关闭/有界读取完成终态交付。真实 Pion ICE + quic-go 回归覆盖同时发起（10 次）、旧 handler（20 次）、无响应超时后立即重试、信令断开后继续 QUIC、拒绝/冲突/confirmed 终态；受影响包普通和 race 均 PASS。
- 香港测试主站当时仍运行旧 dirty revision `8baaa1cf1d67a48adbceb32ecb469999361a1e09`，二进制 SHA256 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`；短时间重试仍为产品 `FAIL`，不是外部环境阻塞。本轮新构建 `fef3e3f+dirty` 只在 `hk-main` 的 `127.0.0.1:39117` 隔离实例验证：两次完成后立即重连、拒绝后立即重连均 PASS，每轮 2,097,152 bytes 且接收内容 `cmp` 一致。主站二进制与 PID 前后相同，实验进程、配置、DB、身份和正文已回滚删除。成功 job：`/tmp/codex-ssh/linksend-p0-isolated-healthz-retry-20260911T062201Z`；首次误用 `/v1/health` 的脚本 FAIL 证据保留于 `/tmp/codex-ssh/linksend-p0-isolated-validation-20260911T061946Z`。
- P1：任务快照明确区分 `task_id`、`attempt_id`、`session_id`、ICE generation 与单调 `revision`；旧 attempt 回调不能覆盖新 attempt。状态覆盖 Preparing、AwaitingAcceptance、Transferring、Verifying、Paused、Recovering、Completed、Rejected、Cancelled、Failed，暂停/取消停止新块调度，校验/提交失败不能进入 Completed。
- SQLite 历史升级为 schema 2：snapshot 与私有 recovery metadata 按 revision 原子更新；v1→v2 前用 SQLite `VACUUM INTO` 生成权限为 `0600` 的可读 v1 备份；损坏行进入 `task_quarantine` 且不阻塞启动；写失败撤销 `history_persisted`。旧二进制回滚必须在应用关闭后恢复 `task-history.sqlite.schema-v1-*.bak`，不能直接打开 schema 2。
- 真暂停/重启恢复/缺块续传已接入现有 transfer 协议。恢复绑定 TransferID、manifest digest、chunk size、源路径及内容、接收目录、对端身份/指纹；`ResumeTask` 是正文恢复的显式用户确认，每次生成新 attempt/session/ICE credentials。接收端重哈希 staging，只请求缺失或损坏块；源内容变化和本地 pinned peer 变化在继续正文前失败。
- 真实 QUIC 续传证据：12 MiB 首块后暂停，未损坏时本轮/累计实际发送与接收 12,582,912 bytes、重传 0、唯一验证/提交 12,582,912；损坏首个 4 MiB staging block 时实际发送/接收 16,777,216、重传 4,194,304、唯一验证/提交仍为 12,582,912；完整重启两个 Service 后恢复 8,388,608 bytes，沿用 task/TransferID、使用新 attempt/session，内容一致且无重传。只有这些缺块协商与计数通过后，当前实现才报告 `byte_resume_supported=true`。
- `history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 独立报告：模拟历史写失败时前两者为 false，而进程内 byte resume 实现仍为 true。部分文件提交记录会在每个成功 commit 后 checkpoint；后续文件失败时已提交文件和字节保持可验证，不显示整任务 Completed。
- CLI 新增只读 `status [--task]` 与显式 `resume --task ...`；resume 只拥有当前 CLI 进程启动的 attempt，不宣称能暂停/取消另一个桌面进程。Wails 增加 PauseTask/ResumeTask 并重新生成 bindings；React 按 `can_pause/can_resume` 门控，使用 revision 合并防止迟到轮询覆盖新快照，分别展示实际发送/接收、重传、验证、提交与双方确认；固定测试配对码不再常驻界面。
- P2 网络源码新增可用接口/地址族发现、显式接口优先级与排除、IPv6 link-local 排除、实际 base socket/interface/family、ICE 状态时间线、候选类型、TLS/ALPN、STUN 请求/响应计数和信令 JSON 字节计数。未按网卡名称、私网地址、presence 或 candidate type 推断 LAN/公网，连接方法仍为 `direct_unknown`。已选本地地址消失会废弃旧 endpoint、关闭当前 QUIC，并由任务恢复流程等待双方显式建立新 session。
- `scripts/dual-nat-netns.sh` 已在 `lab-server` 的 rootless 外层 user/mount/network/PID namespace 中真实运行：内部创建两个独立 NAT namespace、重叠 `10.77.0.0/24`、独立 WAN、coturn 4.5.2 STUN-only 与 HTTPS 信令，不使用 loopback 或单 Docker bridge。MASQUERADE-only/状态过滤运行真实返回 `CHECK_TIMEOUT`；这是当前无 relay 情况下的端口受限 NAT 能力 FAIL。显式固定 UDP SNAT/DNAT 的可穿透 NAT 运行完成 8,388,608 bytes QUIC，双方同一 session、内容 SHA256 一致，STUN request/response 为发送侧 16/6、接收侧 20/8，NAT_A/NAT_B 计数 9/7，`relay=false` 且仍为 `direct_unknown`。脱敏包 `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz` SHA256 `ba7447fe3eb603b669db3a76ab7ba2be049d7abeeefdb94b4927161f43f7eebf`；远端身份、私钥、正文、coturn、namespace 和测试根已回滚删除。宿主地址/路由前后相同；宿主 iptables 因无非交互 sudo 不能读取，但实验规则全部位于 rootless 外层 namespace。
- 仍未验收：真实网卡切换/睡眠唤醒、公共 IPv6、未配置固定映射的更多 NAT 类型、Windows/macOS 新版原生文件/目录选择、打开目录、红点/Cmd+Q 按键、退出保护和重启恢复入口。浏览器或窗口创建证据不能替代这些原生操作。
- 最终 Windows 门槛：根模块格式、`git diff --check`、`go mod verify`、普通测试、race、vet、`GOWORK=off` 测试全部 PASS；桌面独立模块 `GOWORK=off` verify/test/vet/build PASS；前端 frozen install/typecheck/lint/8 tests/build PASS；最后一次 Wails production build 确认 EXE 中包含当前 `index-CQOAaBGW.css` 与 `index-Dz-ecqi4.js`。
- 发布脚本复现出 NSIS 默认项目名仍为 `LinkSendTemplate`，会生成错误命名的安装器并把内置 EXE 改名。`apps/desktop/build/windows/nsis/project.nsi` 现显式固定 LinkSend 项目、公司、产品、版本、可执行文件和卸载注册键；NSIS 3.12 重建后输出 `LinkSend-amd64-installer.exe`，7-Zip 确认内含 `LinkSend.exe`。本机已有 `%APPDATA%\\LinkSend.exe` WebView 数据，因此未实际安装/卸载，避免触碰现有用户状态。
- Windows 原生冒烟使用隔离 profile 启动本轮 EXE：进程存活、主窗口句柄非零、Windows close message 返回 true 且进程在 10 秒内退出；随后精确删除测试 identity/SQLite。该结果不替代文件/目录对话框、打开目录或活跃任务退出保护点击验收。
- Mac 修复后包使用 `mac-test-102342413`，按 manager 的 resolve→probe→audit-host 和隔离目录流程执行。158 个明确源文件打包为 `linksend-source-fef3e3f-dirty-r2.tar.gz`，SHA256 `8612b015bdbdacae26d9d3915b95f9d9744d3576fbd280bfbef751faa940e3f8`，排除全部旧 `.artifacts`、bin、node_modules 和临时脚本；根普通/race/vet、桌面 test/vet/build、arm64/amd64 顺序构建均 PASS。r2 构建 job `/tmp/codex-ssh/linksend-mac-rebuild-r2-20260911T085119Z` 的构建步骤完成，但验证脚本把请求架构 `amd64` 与 Mach-O 名称 `x86_64` 直接比较而最终退出 1；修正后的只读验证 `/tmp/codex-ssh/linksend-mac-rebuild-r2-verify-final-20260911T085952Z` 退出 0。该脚本缺陷不改写为构建 PASS 的退出码，也不影响两份已独立核验的 DMG。
- Wails 3 原生退出回调语义已修复：`ShouldQuit` 在无活跃任务时返回 true；`Rejected` 纳入终态；有活跃任务时先阻止原退出事件，由异步原生对话框回调取消任务，等待终态后再调用 `App.Quit`。桌面单元测试 PASS；Windows r2 EXE 的 `WM_CLOSE` 与 10 秒内退出 PASS；Mac arm64 r2 包创建 1 个约 1008×684 的原生窗口，idle quit Apple Event 与进程退出 PASS。由于 macOS Accessibility=false，红点实际点击、Cmd+Q 按键、原生对话框、键盘导航和活跃任务退出保护仍为 `NOT_RUN`，不能由 Apple Event 代替。
- r2 两个 DMG 的只读挂载、bundle id `com.linksend.desktop`、架构和严格 codesign 验证 PASS，签名均为 ad-hoc、无 TeamIdentifier、未公证。Mac 远端 r2 源码、build root、身份和两个原生测试 profile 已由 `/tmp/codex-ssh/linksend-mac-rebuild-r2-cleanup-20260911T090331Z` 清理；用户原有 `/Volumes/LinkSend` 挂载不属于本轮测试，未操作。
- 四个历史 r2 资产位于 `.artifacts/final-20260911/`，均标记 `UNCOMMITTED_TEST_SNAPSHOT` / 基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5` / `vcs_modified=true`。其大小和 SHA256 保留于该目录 `ASSET-MANIFEST.json`；旧资产不能改名或复用为 `0.2.0` 提交构建。后续真实提交、香港部署和新资产在本页最顶部另行记录。
- 2026-09-11T09:24Z 对最终工作树重验：gofmt、`git diff --check`、根 verify/test/race/vet/`GOWORK=off test`、桌面独立 verify/test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 全部退出 0；P0 server 立即重连重复 20 次、真实 QUIC 生命周期/终态重复 3 次及 P1 缺块/重启恢复定向回归再次 PASS。`scripts/dual-nat-netns.sh` 的清理补丁 `bash -n` PASS，运行态证据仍来自上述隔离实测，不冒充重跑。强制 bindings 首次因 Taskfile 子进程 PATH 不含 `wails3` 退出 1，临时加入仓库 `.wails-bin` 后生成 PASS（1 service / 27 methods / 14 models），生成后 TypeScript/lint 再验通过。最终 Windows Wails build 退出 0，EXE SHA256 `c808e31a54c8d4819373902f460ef2d5ffbe28b6603f9ab5353465e908f548b7` 与 r2 staging EXE 相同，并嵌入当前 `index-Dz-ecqi4.js` / `index-CQOAaBGW.css`。

## 2026-09-11 连续验收执行记录

- 环境基线记录于 `.artifacts/acceptance-20260911/baseline.json`；当前工作树原有未跟踪构建/测试产物保持不变。Windows amd64、PowerShell 7.6.5、Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Docker CLI 29.2.1；Wails 3 `v3.0.0-beta.18` 已按锁定版本安装到本地工具目录。
- 根模块 `gofmt`、`git diff --check`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`GOWORK=off go test -count=1 ./...` 均 PASS；桌面模块 GOWORK=off test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 均 PASS。
- Wails `task build ARCH=amd64` 首次因 PATH 未包含 CLI 而失败（环境配置缺陷，原始错误保留在会话记录）；补充 PATH 后重新运行 PASS，并重新生成 TypeScript bindings。生成器发现 `internal/protocol.Capabilities` 的三个历史/恢复字段未同步到 `apps/desktop/frontend/bindings/.../internal/protocol/models.ts`，已更新该绑定文件；未改变协议或数据路径。
- `demo-local` PASS 仅限 loopback-direct（真实 TLS 1.3/QUIC、1 MiB 摘要一致）；`test-nat` 退出码 1，标记 BLOCKED_BY_EXTERNAL_ENV（Windows 无受控 Linux namespace/NAT fixture）。Docker Compose 静态解析 PASS（使用非生产 dummy token），Docker daemon 不可用，运行态 NOT_RUN。
- `hk-main` 与 `nl-highdefense` 使用 codex-ssh-manager 完成 resolve/probe/audit 及扩展只读审计，均 PASS；未执行 sudo、配置变更、防火墙/路由/DNS/代理操作。扩展审计证据中的服务状态、监听端口和配置路径已脱敏记录于 `.artifacts/acceptance-20260911/results.txt`。
- 该连续验收检查点当时尚缺 NSIS 和新 Mac 构建；两项已在本文顶部记录的后续发布准备阶段补齐。双 NAT/IPv6/网络切换/睡眠唤醒及完整原生交互仍为 NOT_RUN 或 BLOCKED_BY_EXTERNAL_ENV；模拟与 loopback 结果没有替代真实双机验收。

## 2026-09-09 Wails 3 migration and Hong Kong production verification

- Wails 3 CLI and Go module fixed at `v3.0.0-beta.18`; desktop entry/service/window/dialog lifecycle and generated TypeScript bindings now use Wails 3. The old v2 module, `wails.json` and `frontend/wailsjs` outputs were removed. Source snapshot and working-tree patch are retained under `.artifacts/pre-wails3-20260909-*`.
- Wails 3 Windows production build completed from the dirty working tree: `apps/desktop/bin/LinkSend.exe`, SHA256 `0DD18B54495B1320518B0E61903B2E9A155DDC7B902ACE320B564CA81BF5B27E`; `go version -m` reports `github.com/wailsapp/wails/v3 v3.0.0-beta.18`. The process remained alive for 4 seconds in a native WebView2 smoke check; no manual native click acceptance was performed.
- Hong Kong `hk-main` was inspected, backed up and updated using the local SSH credential. Candidate rendezvous SHA256 `53445BE88943FBABDAA7AABFAF98E299AD9780B1084DAA764CF8804E48E5A338` is running at `/opt/linksend-lan-test/rendezvous`; backup `/opt/linksend-lan-test/backups/20260909T133335Z-wails3` was verified with SQLite `PRAGMA integrity_check=ok`. `test_pairing_code="orion123"` and an explicit existing group are enabled; generic configs remain off.
- Real Hong Kong HTTPS/WSS control-plane evidence: fixed-code joins for `hk-win-wails3` and `hk-peer-wails3` returned `admin=true` in group `73cde936…`; a dynamic invitation joined `hk-dynamic-wails3` with `admin=false`; reusing that invitation was rejected. After a controlled server restart, health remained `status=ok`, `relay=false`, `transport=quic`, and the group persisted with 8 members/5 admins/0 revoked.
- Real two-way transfers through Hong Kong signaling and direct QUIC on the physical Windows interface `10.234.232.205`: 1 MiB Windows→peer transfer ID `4773432abda776c76882261f55c0b491`, SHA256 `30E14955…`; 2 MiB peer→Windows transfer ID `cf38b13233b0214c7d309e28bf823d0f`, SHA256 `5647F05E…`. Both reported TLS 1.3 (`772`), ALPN `linksend/1`, `relay=false`, host↔host candidates, and matching received hashes.

## Environment and limits

- Windows amd64, PowerShell 7, Go 1.26.5, Node 22.15.0, pnpm 11.19.0.
- The default PATH exposed MinGW-w64 GCC 8.1, which made Windows race binaries exit with `0xc0000139`. Scoop MinGW 16.2.0 is now installed and selected explicitly for race verification.
- Docker 29.2.1 client is installed, but the Linux engine is unavailable.
- Wails 3.0.0-beta.18 is available at `.tools/bin/wails3.exe`; WebView2 is installed, but macOS runtime validation is not available here.
- This round changed only the LinkSend service binary/config on `hk-main`; no unrelated firewall, DNS, proxy or certificate records were modified. At the time of this deployment snapshot no Git commit/push/reset/clean/stash had been performed; the later candidate commits and push are recorded in the 2026-09-10 section below.

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
- Historical Wails 2.15.0 production build passed before migration; the current Wails 3 build is recorded in the 2026-09-09 section above.
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
- Wails 3.0.0-beta.18, React 19.2.8, TypeScript 5.9.3, Vite 7.3.6, pnpm 11.19.0.

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

Verification on Windows amd64: root `gofmt`, `git diff --check`, `go mod verify`, `go test ./...`, `go vet ./...`, `go run ./cmd/devtool check-core` (including race), root and desktop `GOWORK=off` test/build, frontend typecheck/lint/test/build, Wails 3 production build, `demo-local`, and `network-info` passed. A 20x transfer repeat was first interrupted after an erroneous test harness blocked; the corrected transfer package passed its focused run. `test-nat` remains an explicit nonzero not-run result because this Windows host lacks the privileged Linux namespace fixture.

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
- 自动化通过：桌面 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...`；前端 `typecheck`、`lint`、`test`、`build`；Wails 3 production build；`git diff --check`。
- 产物位于 `.artifacts/windows-preview/20260909-0405/`，含 `LinkSend.exe`、`BUILD-INFO.json`、`SHA256SUMS.txt`、中文启动说明、验证记录和浏览器渲染截图；ZIP 位于 `.artifacts/windows-preview/LinkSend-windows-preview-20260909-0405.zip`。
- Wails 原生窗口点击与截图、跨 NAT/IPv6、网络迁移和重启恢复仍未运行，均保持 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV` 边界。
- Windows 启动冒烟：运行 `apps/desktop/build/bin/LinkSend.exe` 后进程保持运行超过 3 秒并由本次检查结束；未进行人工点击和双机传输。

## 2026-09-09 Windows 初版复核修补

- 接收任务快照现在保留已认证 `peer_id`、manifest 内容摘要和条目数量；接收确认信息可在任务区核对发送者、内容和总量。
- `OpenTaskDirectory` 改为按已完成接收任务 ID 校验目录，避免前端传入任意本地路径；桌面接收等待时限固定为 10 分钟，ICE 检查时限为 30 秒。
- 前端任务/远端轮询加入 in-flight 防重叠；操作调用增加 pending 门闩；完成接收任务提供打开目录操作。
- 复核构建：根模块测试/桌面测试与 vet、前端 typecheck/lint/test/build、Wails 3 production build 均通过；交付目录已更新为最后一次构建的二进制和 SHA256。

## 2026-09-09 401 配对错误修复

- 通过当前桌面 profile 的真实身份与香港主站数据库只读比对确认：身份未加入设备组，服务器返回 401 是成员校验拒绝，不是签名算法或 TLS 故障。
- 前端新增后端错误人性化映射：区分“尚未加入设备组”“已加入但无管理员权限”“邀请无效/过期/已使用”，避免把安全拒绝显示成原始 HTTP 错误。
- 新增对应前端回归测试；隔离 profile、密钥和远程检查脚本均已清理。当前身份仍需由现有管理员设备生成一次性邀请后才能加入，客户端不会自动 bootstrap 或绕过管理员权限。

## 2026-09-09 隔离端到端与远端健康复核

- 在独立临时 rendezvous（`127.0.0.1:18887`、独立 SQLite/profile）上完成两个随机身份的 bootstrap、邀请、加入、双向完整指纹信任和 16 MiB 随机文件传输；发送端与接收端独立 SHA256 一致，TLS 1.3、QUIC、`relay=false`。临时 profile、密钥、数据库和大文件已在测试后清理，仅保留本记录摘要。
- 香港主站 `hk-main` 只读探测通过；`https://linksend.oooai.de/healthz` 返回 `status=ok`、`protocol_version=1`、`relay=false`、`transport=quic`；远端 443 由 rendezvous 监听，3478 由 coturn STUN-only 监听。
- 该隔离测试证明 CLI/核心真实链路，不等价于物理 LAN、跨 NAT 或 Wails 原生窗口验收。
- 生产 EXE 使用隔离 `LINKSEND_DATA_DIR` 启动冒烟保持运行超过 3 秒；WebView2 Runtime 152.0.4191.66 已安装。当前会话未进行人工窗口点击，桌面交互仍待现场验收。

## 2026-09-09 Windows Preview 首次配对与退出保护补齐

- `apps/desktop/app.go` 现在区分偏好不存在、有效、损坏、不支持版本和不可读状态；损坏/不支持文件原地保留，远端操作在用户保存修复前被阻止。新增 `PreferencesStatus`、`EffectiveConfig`，明确环境变量 > 已保存偏好 > 默认值及需重启生效边界。
- `internal/app/service.go` 新增基于已认证设备列表的成员/角色状态：`member/admin`、`not_member`、`auth_failed`、`unavailable` 分开表达；合并 401 不再被猜测为确定未入组。前端管理员邀请按钮按服务端角色禁用并解释原因，普通新设备主入口为粘贴邀请。
- `apps/desktop/main.go` 注册 Wails 3 `ShouldQuit`。存在准备、等待确认、连接、传输、校验或取消中的任务时，原生窗口提供继续任务/取消任务并退出；取消清理有界等待，超时保持窗口打开，重复关闭幂等。
- 新增桌面偏好恢复回归测试；根模块、桌面模块（`GOWORK=off`）、前端和 Wails Windows/amd64 production build 均通过，`git diff --check` 通过。
- 最终本地产物：`.artifacts/windows-preview/20260909-1312/` 及对应 ZIP。原生窗口人工点击、原生截图、真实主站管理员邀请和物理双机本轮仍为 `NOT_RUN`/`BLOCKED`，未以浏览器或隔离组结果替代。

## 2026-09-09 配对权限模型调整

- 根据产品决策，邀请生成不再要求管理员角色；任一已配对且未撤销设备都可以生成 32 字节高熵、10 分钟有效、一次性配对码。保留服务端签名认证、设备组隔离和逐对端完整指纹信任。
- 首次建立空设备组仍需使用服务端 bootstrap 入口；本轮没有把服务改成匿名开放注册。
- 更新服务端邀请授权、客户端错误文案、桌面按钮和 signaling 回归测试。管理员字段继续作为服务端事实展示，但不再作为生成配对码的前置条件。

## 2026-09-09 可用性与本地配对改进

- 新增 `docs/IMPROVEMENT-PROMPT.md`，记录全项目检查、配对边界、UI 视觉和验证要求，可作为后续迭代的固定改进提示词。
- `internal/server.Config.TestPairingCode` 默认关闭；本轮香港授权配置可在显式目标组上启用固定码 `orion123`，开发服务仍提供同一固定码。服务端仍要求 Ed25519 注册签名，生产高熵一次性邀请路径不变。
- `internal/store.JoinTestCode` 使用独立兼容路径加入明确设备组并授予管理员，并添加回归测试；公共配置缺少 `test_pairing_group` 时拒绝固定码配置。
- 桌面设备页展示“仅测试使用、未来可能关闭”的固定配对码并支持复制；输入框提示可直接输入 `orion123`。
- `apps/desktop/frontend/src/App.css` 增加 Apple 风格视觉层：系统字体、克制蓝色强调、半透明分组表面、统一圆角与焦点状态、窄屏导航折叠和减少动效规则。
- 本轮验证：根模块 `go test ./...` 通过；桌面模块 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...` 通过；前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（4 tests）、`pnpm run build` 通过；`git diff --check` 通过。
- 未运行：Wails 原生窗口人工点击、Windows↔macOS 新版 UI 验收、跨 NAT/IPv6/网络切换。上述项目仍保持 `NOT_RUN` 边界。

## 2026-09-09 全面产品改进复核

- 增量实现：任务层把接收拒绝、源文件变化、完整性失败、危险路径、磁盘空间/权限和取消映射为稳定协议错误码；Wails 任务 DTO 与 CLI 失败出口只展示中文操作建议，底层 HTTP、文件系统和栈信息保留为内部 cause，不直接进入用户界面。
- 成员资格把信令不可达、版本不兼容和身份拒绝分开；任一已加入且未撤销设备可生成一次性邀请的前端文案与服务端权限保持一致。
- 固定配对码对同一 profile 重试为幂等并授予管理员；生产 `Join` 的高熵、10 分钟、一次性语义未改变，静态入口通过显式目标组控制。
- 直连完成后任务阶段显式进入 `connected`，界面把成员资格、对端 presence、指纹信任和已建立直连分别呈现。新增 favicon 清除浏览器预览的无意义 404。
- 当前 Windows/PowerShell 环境实际通过：`gofmt -l .`、`git diff --check`、根 `go test ./...`、`go vet ./...`、根 `GOWORK=off go test ./...`、根普通及 `GOWORK=off go test -race ./...`、桌面 `GOWORK=off go test/go vet/go build ./...`、前端 typecheck/lint/4 tests/build、Wails 3 production build、`demo-local` 和 Compose 配置解析。Wails 3 EXE 隔离启动后保持运行超过 4 秒，再由本轮检查结束。
- 浏览器行为/视觉检查：实际打开传输、设备、设置三页，并在 700px 窄屏检查导航折叠、单列布局、表单宽度、禁用态和复制入口；这是 WebView 内容渲染证据，不等于 Wails 原生窗口点击验收。
- `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`：本轮未运行新的 Windows↔macOS 双机传输、跨 NAT、IPv6、网络切换、睡眠唤醒、macOS Wails、Windows 原生窗口人工点击、Docker 引擎运行或安装签名。`devtool test-nat` 在 Windows 上按设计非零退出并说明需要 Linux 特权 namespace fixture。
- 下一步：在两端现场执行 Stage 2B 跨 NAT 与新版原生 UI 工作流；之后实现持久任务历史、真正的应用重启恢复和暂停/恢复编排。

## 2026-09-10 Wails 3 候选打包与交付核验

- 任务分支为 `codex/wails3-hk-dmg-20260909`，当前候选 commit 为
  `555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`；本轮提交了 DMG 资源、Windows/Mac 测试说明，
  未修改 `main`、未合并、未发布正式 Release。源码快照仍保留在 `.artifacts/pre-wails3-20260909-*`。
- GitHub Actions run [34387753173](https://github.com/Wen5555/LinkSend/actions/runs/34387753173) 的
  `windows-amd64`、`macos-arm64`、`macos-amd64` 均 `success`。Windows ZIP 和两个 DMG 已下载到
  `.artifacts/ci-34387753173/` 并通过 ZIP 完整性、包内 `SHA256SUMS.txt`、DMG HFS+ 目录、
  `Info.plist`、`CFBundleIdentifier`、Go `GOARCH/GOOS` 与 Wails 依赖核验。
- 最终包 SHA256：Windows ZIP
  `1c43b7576118c5480d139bd691a1c84b8c3d09f67d96f8815c1877c4055f8f80`；macOS arm64 DMG
  `5ff0217fab3b257999eae710f29bc76f08979b37100b2f1b4347546a6d3645a2`；macOS amd64 DMG
  `653c2ceb098c5d81bc4c6f0a351cb20b446772c206888463aec7b34efff73819`。DMG 为内部测试包，
  仅 ad-hoc 签名，不含 Developer ID 或公证。
- Windows ZIP 现在包含真实 `LinkSend.exe` 与 `README-WINDOWS-TEST.txt`；DMG 包含完整
  `LinkSend.app`、`README-MACOS-TEST.txt` 和 Applications 拖拽入口。离线 `go version -m` 确认
  Go 1.26.5、`GOOS=darwin`、`GOARCH=arm64/amd64` 及 Wails `v3.0.0-beta.18`。
- 当前主机实际完成了 Windows 与香港服务的既有真实双向 QUIC 证据；没有新增 Mac 原生窗口、DMG
  挂载、红点/Cmd+Q、跨 NAT、IPv6 或网络切换验收，均标记 `NOT_RUN`/`BLOCKED_BY_EXTERNAL_ENV`。

## 2026-09-11 任务诊断文案与阶段可读性补丁

- 前端任务列表将内部阶段枚举映射为中文可操作文案（发现网络地址、交换连接信息、检查直连路径、建立安全直连、等待接收确认、恢复传输等）；未知未来阶段仍原样保留，便于诊断。
- `humanizeBackendError` 补齐稳定连接、传输、任务操作和桌面边界错误码（包括 `NO_CANDIDATES`、`CHECK_TIMEOUT`、`DISK_FULL`、`UNSAFE_PATH`、`RELAY_NOT_IMPLEMENTED`、`TASK_NOT_RETRYABLE` 等），避免将原始内部错误直接展示给用户。
- 修复测试配对面板 CSS 伪元素重复显示“仅测试使用”文案，并补充阶段与错误码前端回归测试。
- 实际验证：前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（6 tests）、`pnpm run build`；根模块 `gofmt -l .`、`git diff --check`、`go vet ./...`、`GOWORK=off go test ./...`；桌面模块 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`；Wails `v3.0.0-beta.18` production Windows build 均通过。
- `go run ./cmd/devtool demo-local` 实际完成 loopback TLS 1.3/QUIC 文件传输与摘要一致性；该结果仅证明本机链路。Windows↔macOS、跨 NAT、IPv6、网络切换、睡眠唤醒和原生窗口人工点击仍为 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`。
- Wails 最新构建产物 `apps/desktop/bin/LinkSend.exe` 隔离数据目录启动冒烟保持运行超过 4 秒后结束；未进行原生窗口人工点击。

## 2026-09-11 直连路径证据贯通任务界面

- `DirectConfig` 新增本地 evidence 回调，连接成功后把实际 `connection_method`、`transport_protocol` 和 `relay` 结果写入任务快照；未改变协议、ICE、QUIC 或文件数据路径。
- Wails/React 任务列表显示已验证的“局域网直连 / 互联网 P2P / 路径待确认”，避免把在线状态或 host candidate 当作局域网证据。
- 实际验证：根与桌面模块测试、桌面 vet/build、前端 typecheck/lint/6 tests/build、绑定重新生成和 `git diff --check` 均通过。跨设备、跨 NAT、IPv6、网络切换和原生窗口仍未运行。

## 2026-09-11 任务历史与候选构建复核

- `internal/app` 增加 `task-history.sqlite`（SQLite `user_version=1`，含 `schema_version` 等价版本元数据）持久化；启动时损坏记录被隔离，不阻塞应用；未完成任务在重启后标记为 `failed/TASK_INTERRUPTED`，清除旧会话证据并要求用户核对后重新发起；块检查点仍仅由 transfer 层使用，桌面级 byte resume 尚未实现。快照新增单调 `revision`、`history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 与暂停/恢复能力字段。
- 根模块 `go test ./...`、`go test -race ./...`、桌面模块 `GOWORK=off go test/go vet/go build ./...`、前端 frozen install/typecheck/lint/Vitest 6 项/production build 均通过；新增历史持久化和损坏记录回归测试。
- 提交 `9b72334d0fbe5e18649424924e96cf88756c6845` 已推送分支 `codex/wails3-hk-dmg-20260909`。Actions run [34515881198](https://github.com/Wen5555/LinkSend/actions/runs/34513870167) 的 Windows amd64、macOS arm64、macOS amd64 三平台均 success，产物与该提交对应。
- 本机可交付 Windows 便携 ZIP：`apps/desktop/bin/LinkSend-windows-amd64-0.1.0-preview.zip`，包含 EXE、测试说明、BUILD-INFO 和包内 SHA256。NSIS/Wails CLI 未安装，安装器未生成；macOS DMG 仅通过 CI 产出，未在本机挂载或进行原生窗口点击验收。

## 2026-09-11 全面网络提示词执行记录

- 香港主站 `hk-main` 经 codex-ssh-manager 完成只读 `resolve/probe/audit-host`：root、Debian 6.1.0-50-cloud-amd64、x86_64，根分区余量约 78%，`rebootRequired=false`；未修改生产配置。
- 荷兰 VPS `nl-highdefense` 的 SSH 管理器探测在本轮超时，未取得 STUN/TURN 或 NAT 实验运行证据，标记 `BLOCKED_BY_EXTERNAL_ENV`，未执行 sudo 或网络配置变更。
- `go run ./cmd/devtool demo-local` 退出码 0：真实 ICE host candidate、QUIC TLS 1.3、摘要一致（1 MiB），仅作为 loopback 证据，不能替代双机 LAN。
- `go run ./cmd/devtool test-nat` 退出码 1，输出明确为 Windows 缺少受控 Linux namespace/NAT fixture；标记 `BLOCKED_BY_EXTERNAL_ENV`，没有伪造 NAT 成功。
- GitHub Actions run [34523034011](https://github.com/Wen5555/LinkSend/actions/runs/34523034011) 对主分支提交 `a9646f21c3d088036ade6d7f0a44cd0cb4a75c9a` 的 Windows amd64、macOS arm64、macOS amd64 jobs 均 `success`；对应构建附件可从该 run 下载。尚未替换旧候选 release `v0.1.0-preview-e27`（其目标提交不是当前主分支），避免上传不匹配资产。

## 2026-09-11 Windows ↔ Mac 实机联调与修复（阶段性历史记录）

- 本节保留暂停/恢复实现之前的现场事实；当前实现和构建状态以本文顶部“P0/P1/P2 分阶段可靠性改进”为准，不应把本节当作最终能力声明。环境：Windows 物理以太网 `10.234.232.205/16` ↔ Mac 物理 Wi-Fi `10.234.241.3/16`；两端原有 TUN 保持启用。SSH 全部使用 codex-ssh-manager 的 `mac-test-102342413`。完整状态、命令和限制见 [本轮验收与复测步骤](MAC-WINDOWS-VALIDATION-20260911.md)。
- `IMPLEMENTED / PASS`：真实双向 QUIC/TLS 1.3 传输空文件、中文文件、多文件、12 MiB（三 chunk）文件及中文空目录，接收文件 SHA256 匹配；Mac 真实拒绝、权限拒绝、冲突、独立 64 MiB 卷磁盘不足均保留稳定错误分类，权限恢复和实验卷卸载已核验。
- `IMPLEMENTED / PASS`：修复候选诊断泄露 ICE ufrag；修复 QUIC 终态响应因立即 reset 而丢失；冲突/权限/磁盘错误在接收端、发送端、CLI/任务 DTO 和前端一致；移除仅凭 srflx 推断互联网路径的逻辑。前端分别显示历史持久化、重启恢复和字节级续传，移除常驻固定测试配对码。
- 信令协商清理为 `IMPLEMENTED / PASS`（本地/原生 Go 回归），香港现有旧服务短时间重试仍为 `FAIL`。本轮没有部署生产服务；不能以源码修复替代线上验收。
- `IMPLEMENTED / PASS`：Windows 根模块普通/race/vet/独立模块测试，桌面独立模块 test/vet/build，前端 frozen install/typecheck/lint/7 tests/build；Mac Go 1.26.5 根模块 test/race、桌面 test/vet；两端 Wails beta.18 bindings 及主要前端 JS SHA256 一致。
- 原生构建：Windows production EXE、Mac arm64/amd64 DMG 均实际构建；Mac `hdiutil`、`plutil`、`file`、`lipo`、`codesign` 校验通过。隔离 arm64 应用由 CoreGraphics 观察到 1 个真实窗口，未进行原生控件操作。修复 Windows 源码拷贝到 Mac 后的 CRLF shell/HTML 问题，加入 `.gitattributes`。
- 本轮包标记 `UNCOMMITTED_TEST_SNAPSHOT`，基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`，不冒充该提交的干净构建。Windows 为 `UNSIGNED_TEST_BUILD`；Mac `code_signature=ADHOC / distribution_identity=NONE / notarization=NOT_RUN`。本轮无新 PR/合并/Release。
- 该现场阶段当时 `NOT_IMPLEMENTED`：桌面重启恢复、字节级续传、桌面暂停/恢复；这些能力随后已在本文顶部所述源码快照中实现并通过本地真实 QUIC 回归，但尚未做新的物理双机恢复验收。中继仍为 `NOT_IMPLEMENTED`；真实双 NAT、Linux 双机、完整网卡矩阵、原生 UI 缺口仍见验收报告。
- 历史交付勘误：较早的 Windows 便携包曾缺包内 SHA256/准确合并提交来源信息，不能视为满足本轮发布要求；本轮 ZIP 单独记录未提交来源并包含包内校验和。默认 Windows 身份数据目录通常为 `%APPDATA%\LinkSend`（`os.UserConfigDir()`），不是 `%LOCALAPPDATA%`。


