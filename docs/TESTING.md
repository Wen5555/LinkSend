# Testing

本页适用于产品 `0.2.0`、协议 V1 和 Wails 3 `v3.0.0-beta.18`。测试结果必须同时记录源码 commit、`vcs.modified`、产品/协议版本与实际退出码；旧 `0.1.0` 或 dirty snapshot 结果只能作为历史证据，不能冒充新提交构建。

根模块的单元测试覆盖身份、公钥替换、候选策略、分帧、BLAKE3、危险路径、恢复状态、取消和异常关闭。`tests/integration` 的 `TestLocalDirectDemo` 启动临时 SQLite/WSS 服务，使用两个独立身份执行真实 ICE、TLS 1.3、QUIC 和文件哈希校验。

```powershell
go test ./...
go vet ./...
go run ./cmd/devtool demo-local
go run ./cmd/devtool test-nat
```

`demo-local` 只能证明 loopback direct。Windows 上的 `test-nat` 仍会明确返回未运行；独立 Linux namespace 双 NAT 应运行 `scripts/dual-nat-netns.sh`。相同 bridge 上的容器不能算双 NAT。

`net.Pipe` 测试只覆盖 transport-neutral fallback，不能证明 quic-go 行为。Stage 1B 的真实 loopback 测试现在使用 quic-go v0.62.0 的 client/server stream，覆盖 acceptance read、receive read、ACK wait 和重复 abort；它们不能替代 LAN/NAT 验收。

Candidate exchange 测试覆盖发送失败、接收失败、缺少 `end_of_candidates`、父 context 取消和 malformed message，并使用确定的 done/context 同步点。阶段错误保留 `CANCELLED`、`CANDIDATE_EXCHANGE_TIMEOUT`、`CHECK_TIMEOUT`、`NO_VIABLE_CANDIDATE`、`ICE_FAILED` 和 QUIC handshake 分类，不能统一写成一个连接超时。

传输测试还覆盖恶意 accept/ack 的 verified 上界、受限 peer error、恢复 JSON 尾随数据和未知文件 ID。`linksend send --evidence` 和 `receive --evidence` 会导出实际候选对、共享 socket、STUN 计数和 TLS/ALPN，不导出 ICE credentials、令牌或私钥。

桌面模块必须从 `apps/desktop` 且 `GOWORK=off` 执行，并确认 `go env GOMOD GOWORK` 指向桌面 `go.mod` 和 `off`。前端检查从 `apps/desktop/frontend` 执行。Wails production build、Docker runtime、HTTPS 证书身份验证、coturn STUN Binding/TURN Allocate 负例均需单独记录，不能由 compose config 或 liveness healthcheck 代替。

目标验收还需要两台真实设备、不同网络、IPv6、睡眠唤醒和公网/家庭热点场景。每次运行应保留候选对、基础 socket、路径、传输哈希、信令/STUN 计数和错误阶段。固定 UDP 映射双 NAT 已 PASS；MASQUERADE-only 是产品 `CHECK_TIMEOUT` FAIL，不能写成 not-run 或外部阻塞。

## Stage 2A

真实 Windows↔macOS 局域网验收使用 scripts/stage2a.ps1 与 scripts/stage2a.sh，步骤和证据目录见 docs/STAGE2A.md。

## P0/P1 恢复与生命周期回归

以下测试必须同时跑普通和 race；`net.Pipe` 不能替代其中的真实 ICE + QUIC 用例：

```powershell
go test -count=1 ./internal/server ./internal/signaling ./internal/app ./internal/transport ./internal/transfer
go test -race -count=1 ./internal/server ./internal/signaling ./internal/app ./internal/transport ./internal/transfer
go test -count=1 -run 'TestPauseResumeTransfersOnlyMissingOrDamagedChunksOverRealQUIC|TestRestartRecoveryUsesPersistedTaskAndMissingBlocksOverRealQUIC|TestResumeRejectsChanged' ./internal/app
```

`TestPauseResumeTransfersOnlyMissingOrDamagedChunksOverRealQUIC` 必须断言实际发送/接收、重传、唯一验证和提交字节；`TestRestartRecoveryUsesPersistedTaskAndMissingBlocksOverRealQUIC` 必须重建两个 Service 并证明 task/TransferID 保持、新 attempt/session 生效。源变化和 pinned peer 变化必须在新正文调度前失败。`TestPartialCommitPersistsVerifiableRecord` 覆盖部分文件已提交、后续冲突失败的 checkpoint 边界。

schema 迁移测试必须确认：v1 数据可读、迁移前 `task-history.sqlite.schema-v1-*.bak` 是可打开的 user_version=1 数据库、schema 2 revision guard 生效、损坏行被隔离、写失败撤销持久化能力声明。回滚演练必须关闭所有 LinkSend 进程后再恢复 v1 备份。

## P2 网络实验

`internal/connectivity` 测试覆盖接口优先级/排除、IPv4/IPv6 地址族、IPv6 link-local 拒绝、地址消失触发 endpoint 失效和诊断脱敏。真实双 NAT 使用 `scripts/dual-nat-netns.sh`，只能在 disposable Linux VM 的 root 或 rootless user namespace 内显式设置 `LINKSEND_ISOLATED_LAB=1` 后运行；需要 `iproute2`、iptables、openssl、coturn 和本轮 Linux 二进制。脚本创建两个独立 NAT namespace 与重叠私网，通过独立 WAN 上的 STUN-only/HTTPS 服务传输 8 MiB，并保留 NAT 计数、拓扑、候选、session 和内容摘要。`bash -n` PASS 不是运行态 PASS。

2026-09-11 在 `lab-server` 实际运行两种边界。MASQUERADE-only/状态过滤为 `CHECK_TIMEOUT`，不得改写为外部环境阻塞；固定 UDP SNAT/DNAT 的可穿透 NAT 为 PASS：8,388,608 bytes、SHA256 `9d9094fb6ec5cc17f6bbbd713ea2f4283a60482532ccfc955e051f4d5b5c90c7`、发送 STUN 16/6、接收 STUN 20/8、NAT 计数 9/7、TLS 1.3/ALPN `linksend/1`、`relay=false`、`connection_method=direct_unknown`。成功 manager job 为 `/tmp/codex-ssh/linksend-dual-nat-rootless-r5-20260911T082528Z`；最终回滚 job 为 `/tmp/codex-ssh/linksend-dual-nat-final-rollback-20260911T082920Z`。脱敏证据包位于 `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz`，SHA256 `ba7447fe3eb603b669db3a76ab7ba2be049d7abeeefdb94b4927161f43f7eebf`。宿主地址/路由前后一致；宿主 iptables 无非交互读取权限，不能把该字段记为 PASS，但 rootless 外层 network namespace 保证实验规则不进入宿主网络栈。

2026-09-11 香港隔离 P0 验证使用新 dirty-source Linux 二进制，仅监听 `127.0.0.1:39117`。成功 job `/tmp/codex-ssh/linksend-p0-isolated-healthz-retry-20260911T062201Z` 覆盖两次完成后立即重试、拒绝后立即重试；测试主站二进制/PID 前后相同且实验敏感数据已回滚。首次 `/v1/health` 探测错误的失败 job 必须保留，不能从记录中删除。

## 2026-09-11 最终构建门槛

`0.2.0` 预提交重跑已再次通过根模块 full test/race/vet/GOWORK=off、P0 重复 20 次、真实 QUIC 定向恢复重复 3 次、桌面独立模块、前端 8 tests、bindings 生成、Wails production 和 NSIS package。Windows 主 EXE 与 installer 的 FileVersion/ProductVersion 均读取为 `0.2.0`；仅在 JSON 中出现版本字符串不能替代此检查。首次生成的 Windows metadata 缺 FileVersion 显示字符串，该 FAIL 和修复记录保留在 PROGRESS。

香港测试主站 `0.2.0` 部署后公网复测使用两个隔离 Linux profile 和同一实际 `eth0`：完成→立即完成、拒绝→立即完成均 PASS，每轮 2,097,152 bytes，内容一致；服务 PID/SHA 保持稳定。该结果证明已部署信令生命周期修复，不是双 NAT 或跨平台恢复证明。测试 profile/正文/邀请/CLI 已删除，两条测试成员均撤销；完整 job/备份见 DEPLOY-HK。

Windows 最终顺序实际执行并退出 0：

```powershell
go mod verify
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
$env:GOWORK = 'off'; go test -count=1 ./...

Set-Location apps/desktop
$env:GOWORK = 'off'; go mod verify; go test -count=1 ./...; go vet ./...; go build ./...

Set-Location frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run lint
pnpm run test -- --run
pnpm run build
```

前端与 Wails/桌面编译顺序执行。最后一次 `wails3 task build ARCH=amd64` 使用 Wails `v3.0.0-beta.18`，bindings 为 27 methods / 14 models，EXE 内可检索到最终 dist 资产名。Wails 3 的 `ShouldQuit` 在 idle 时必须返回 true；`TestShouldQuitAllowsEmptyInitializedService` 和完整终态集合测试用于防止该语义再次反转。Windows r2 原生窗口创建、非零句柄、`WM_CLOSE` 接受和 10 秒内退出 PASS；其他原生交互不得据此判定通过。

NSIS 3.12 使用 `wails3 task package ARCH=amd64 INSTALL_SCOPE=user` 构建。7-Zip 检查安装器为 NSIS 3 Unicode 且内含 `LinkSend.exe`。由于卸载脚本会清理现有 WebView 数据路径，而该路径在测试前已存在，真实安装/卸载保持 `NOT_RUN`；不得为冒烟删除现有用户目录。

Mac 使用 `mac-test-102342413` 的 manager resolve/probe/audit-host 后上传 r2 源包。隔离脚本依次运行根普通/race/vet、桌面 test/vet/build，再在两个独立源码副本顺序构建 arm64/amd64 DMG。构建 job `/tmp/codex-ssh/linksend-mac-rebuild-r2-20260911T085119Z` 最终 `exit-code=1`，唯一失败是验证脚本未将请求架构 `amd64` 规范化为 Mach-O 的 `x86_64`；不得把该 job 整体写成退出 0。修正后的只读验证 job `/tmp/codex-ssh/linksend-mac-rebuild-r2-verify-final-20260911T085952Z` 对两份 DMG 的挂载、`plutil`、`lipo`、`codesign --verify --deep --strict` 均 PASS。arm64 r2 原生窗口计数为 1，idle quit Apple Event 和退出 PASS；Accessibility=false，因此红点实际点击、Cmd+Q 按键、对话框和键盘操作仍是 `NOT_RUN`。Mac SDK 26 对 deployment target 11 产生链接器 warning，未造成失败，但发布前仍需在最低支持系统实机检查。

当前资产均使用 `-r2` 文件名；资产和源包的本地 SHA256 位于 `.artifacts/final-20260911/SHA256SUMS.txt`，详细来源/工具/签名状态位于 `ASSET-MANIFEST.json`。`mac-native-r2-result.txt` 记录原生退出结果，SHA256 `eebc76579da4b7a0176e3362e01f9f79eb723f31fcaec3482e2f99629a9a1ee6`。所有资产是未提交 dirty snapshot，不对应 GitHub workflow SHA；同目录无 `-r2` 旧文件不属于当前修复后的资产集合。

最终重验还必须记录工具路径失败本身：直接运行强制 `common:generate:bindings` 时，Taskfile 子进程若 PATH 不含锁定 CLI 会以 `wails3: executable file not found` 退出 1；将仓库 `.wails-bin` 仅加入本次进程 PATH 后重跑才算 PASS。2026-09-11T09:24Z 的重跑处理 1 service / 27 methods / 14 models，随后 typecheck/lint 和 `wails3 task build ARCH=amd64 -f` 均退出 0。最终 `apps/desktop/bin/LinkSend.exe` SHA256 为 `c808e31a54c8d4819373902f460ef2d5ffbe28b6603f9ab5353465e908f548b7`，与 r2 staging EXE 一致；二进制可检索到当前 dist 的 `index-Dz-ecqi4.js` 和 `index-CQOAaBGW.css`。
