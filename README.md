# LinkSend

LinkSend 是 Go 核心、Wails 2 桌面端和自托管信令服务组成的点对点文件传输工具。服务器只负责设备组、在线状态、会话和候选交换；文件字节通过经过身份认证的 QUIC 直连传输，当前没有 Relay、TURN 文件中继或 HTTP 上传接口。

源码仓库：[github.com/Wen5555/LinkSend](https://github.com/Wen5555/LinkSend)。当前版本是开发验收阶段，真实双 NAT 和跨平台实机结果以验收矩阵为准。

当前代码已经具备：

- Ed25519 长期设备身份、邀请配对和本地公钥固定。
- SQLite 信令控制面、HTTP/WSS 认证、设备撤销和限流。
- Pion ICE 与 quic-go 的同一 UDP socket 分流，TLS 1.3 双向身份认证。
- manifest、BLAKE3 分块校验、安全 staging、断点恢复和安全提交。
- `demo-local` 真实驱动 WSS、ICE、QUIC 和文件协议。
- CLI `send` / `receive` 已调用共享直连编排；接收端默认要求交互确认，测试才使用 `--auto-accept`。
- `send --evidence` 和 `receive --evidence` 可输出实际选中的直连路径、STUN 计数和 TLS/ALPN，不含 ICE credential 或令牌。

当前已验证的是 Windows loopback 直连。真实两机 LAN、公网 IPv6、受控双 NAT、Windows 与 macOS 互通以及 Linux NAT 实验仍待目标环境运行，不能由本地演示替代。

## 快速开始

需要 Go 1.26.5。核心命令不依赖 Node 或桌面系统库。

```powershell
go run ./cmd/linksend --help
go run ./cmd/devtool check-core
go run ./cmd/devtool demo-local
```

启动本机开发信令服务：

```powershell
$env:LINKSEND_BOOTSTRAP_TOKEN = "替换为至少 32 字符的高熵令牌"
go run ./cmd/rendezvous --config configs/server.dev.toml
```

另开终端执行首次 bootstrap：

```powershell
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback bootstrap --token $env:LINKSEND_BOOTSTRAP_TOKEN --name admin
```

其余 CLI 命令见 `go run ./cmd/linksend --help`。每个测试客户端必须使用独立的 `--data-dir` 和接收目录。

发送端需要先用 `devices` 取得并通过 `trust` 固定对端指纹：

```powershell
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback --data-dir .\profile-a send --peer PEER_ID .\file.bin
go run ./cmd/linksend --server http://127.0.0.1:8787 --allow-insecure-loopback --data-dir .\profile-b receive --dir .\received --auto-accept
```

真实网络请为 `--bind` 提供具体本地接口地址，并按部署情况提供 `--stun stun:host:3478`；不提供 bind 时只允许显式 loopback 开发模式。

桌面端位于 `apps/desktop`，使用 pnpm 和 Wails 2。传输页、设备页和设置/诊断页已接入进程内任务服务，可发起真实发送、准备接收、查看快照、确认/拒绝、取消和失败后重新发送。任务仅在当前进程内管理，重启恢复和完整历史尚未实现。

## 范围边界

Relay、TURN allocation、移动端、浏览器端、复杂账号平台和自动端口映射不在当前实现中。直连失败时必须显示阶段、稳定错误码和诊断信息，不能偷偷改用第三方中继。

版本和阶段证据见 [`docs/PROGRESS.md`](docs/PROGRESS.md)。
