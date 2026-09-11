# LinkSend

LinkSend（Osend）是由 Go 网络内核、Wails 3 桌面端和自托管信令服务组成的点对点文件传输工具。当前产品版本为 **0.2.0**，文件/控制协议保持 **V1**；产品 SemVer 与协议版本独立演进。

服务器只负责设备组、在线状态、会话和 ICE 候选交换。文件正文始终通过经身份认证的 QUIC 直连传输，不经过 HTTP、WSS、JavaScript IPC 或第三方中继。当前没有 Relay/TURN 文件中继，能力固定为 `relay=false`。

源码仓库：[github.com/Wen5555/LinkSend](https://github.com/Wen5555/LinkSend)。当前 `0.2.0` 候选源码为 `a88553180bd1defac5b236d75fe4dff5046734dd`，三平台 committed 构建见 [Actions run 34596127738](https://github.com/Wen5555/LinkSend/actions/runs/34596127738)，审阅入口为 [PR #6](https://github.com/Wen5555/LinkSend/pull/6)。`0.2.0` 仍是测试候选，不代表生产可用或跨平台完整验收。

版本从 `0.1.0` 提升到 `0.2.0`，因为 1.0 前新增了 schema 2 持久化、暂停/重启恢复、验证后缺块续传、多网卡诊断和连接生命周期能力；协议 wire format 仍为 V1。由于 MASQUERADE-only NAT、完整原生交互、签名/公证等门槛未完成，不提升到 `1.0.0`。候选尚未发布期间的同轮可靠性修复继续构建为 `0.2.0`，不伪造已经发布后的 `0.2.1`。

当前实现包括：

- Ed25519 长期设备身份、邀请配对、本地公钥固定与撤销。
- SQLite 信令控制面、HTTP/WSS 认证、设备组隔离和限流。
- Pion ICE 与 quic-go 的同一 UDP socket 分流，TLS 1.3 双向身份认证。
- manifest、BLAKE3 分块校验、安全 staging、不可覆盖提交与提交记录。
- `task_id`、`attempt_id`、`session_id`、ICE generation 和单调 `revision` 分层状态模型。
- schema 2 任务历史、损坏记录隔离、显式暂停/恢复、重启恢复与验证后缺块续传。
- 自动接口发现、接口优先/排除、IPv6 link-local 排除和脱敏结构化诊断。
- CLI 与 Wails 3 共用 `internal/app` 服务；React 按 revision 丢弃旧快照。

三项能力必须分开理解：`history_persisted` 只说明历史库当前可可靠写入，`restart_recovery_supported` 说明具备重启恢复所需身份和元数据，`byte_resume_supported` 只说明缺块校验、协商、请求和实际字节计数均已通过实现级验证。

截至 2026-09-11，物理 Windows↔macOS 双向 QUIC 传输与摘要、Mac 拒绝/冲突/权限/独立磁盘镜像空间不足均 PASS；Linux 独立双 NAT 的固定 UDP 映射正向场景 PASS，MASQUERADE-only 场景仍为 `CHECK_TIMEOUT` FAIL。committed Windows 包的原生窗口创建与空闲 `WM_CLOSE` PASS；Mac 原生窗口 PASS 仍来自较早 dirty r2 快照，最新 committed DMG 因物理 Mac SSH timeout 未启动，其余原生交互仍是 NOT_RUN。四类候选资产、真实 SHA256 和签名边界见 [v0.2.0 候选记录](docs/RELEASE-v0.2.0-TEST-CANDIDATE.md)，完整状态见 [验收矩阵](docs/ACCEPTANCE.md) 与 [实机报告](docs/MAC-WINDOWS-VALIDATION-20260911.md)。

## 快速开始

需要 Go 1.26.5。核心命令不依赖 Node 或桌面系统库。

```powershell
go run ./cmd/linksend --version
go run ./cmd/rendezvous --version
go run ./cmd/devtool check-core
go run ./cmd/devtool demo-local
```

启动本机开发信令服务：

```powershell
$env:LINKSEND_BOOTSTRAP_TOKEN = "替换为至少 32 字符的高熵令牌"
go run ./cmd/rendezvous --config configs/server.dev.toml
```

另开终端执行首次 bootstrap，并为每个测试客户端使用独立的 `--data-dir` 和接收目录：

```powershell
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback bootstrap --token $env:LINKSEND_BOOTSTRAP_TOKEN --name admin
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback --data-dir .\profile-a send --peer PEER_ID .\file.bin
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback --data-dir .\profile-b receive --dir .\received --auto-accept
```

真实网络应通过 `--bind` 或桌面设置选择实际可用接口，并按部署提供 `--stun stun:host:3478`。不能根据网卡名称、私网地址、在线状态或 candidate 类型直接把路径判断为 LAN/公网；证据不足时显示 `direct_unknown`。

桌面端位于 `apps/desktop`，固定使用 Wails 3 `v3.0.0-beta.18` 和 pnpm 锁文件。开发与构建命令见 [DEVELOPMENT.md](docs/DEVELOPMENT.md)。

## 范围边界

Relay、TURN allocation、移动端、浏览器端、复杂账号平台和自动端口映射不在当前实现中。直连失败会报告失败阶段、稳定错误码和脱敏诊断，不会静默切换到其他正文传输路径。

版本、阶段证据和准确限制见 [PROGRESS.md](docs/PROGRESS.md)。
