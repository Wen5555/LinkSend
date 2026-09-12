# LinkSend

LinkSend 是一个面向 Windows 和 macOS 的开源点对点文件传输工具。它由 Go 网络内核、Wails 3 桌面端和可自托管的信令服务组成，优先在局域网内直接发现设备，也支持通过信令服务协调跨网络 P2P 连接。

当前版本为 **v0.4.0 测试预发布**，文件与控制协议保持 **V1**。可从 [GitHub Releases](https://github.com/Wen5555/LinkSend/releases/tag/v0.4.0) 获取 Windows 便携包、Windows 安装程序以及 macOS arm64/amd64 DMG。

## 设计特点

- 同一局域网内使用签名组播、定向广播和受限单播发现；发现设备不等于自动信任。
- 跨网络使用自托管 HTTPS/WSS 信令交换在线状态和 ICE 候选。
- 文件正文只在设备之间通过 Pion ICE + QUIC 直连传输，不经过信令服务、HTTP 上传、JavaScript IPC 或第三方存储。
- QUIC 使用 TLS 1.3 和固定的 Ed25519 设备身份进行双向认证，文件块与完整内容使用 BLAKE3 校验。
- 支持文件、多个文件、目录和空目录，以及暂停、取消、应用重启恢复和验证后的缺块续传。
- 接收端先写入隔离 staging，完整校验后再提交；默认不会覆盖同名文件。
- 桌面应用启动后自动保持接收能力，首次传输需要接收方确认，也可为可信设备启用自动接收。

## v0.4.0 主要变化

- 配对码采用 `ABCD-EFGH` 短码、10 分钟有效期和单次使用语义；相同设备在响应丢失或重复提交时可幂等完成，其他设备复用会被拒绝。
- 配对错误区分无效、过期、已使用和身份冲突，客户端使用服务端相对 TTL 显示倒计时，避免本地时钟偏差造成假过期。
- 新增安全 LAN 发现和 TLS 1.3 临时控制通道；附近新设备只有在双方确认且传输完整完成后才会保存信任。
- 增加已知局域网地址定向探测和手动单地址兜底，不扫描整个子网；校园网或访客网络过滤组播时仍可使用服务端信令路径。
- 信令连接改为后台读泵复用，ICE 候选使用 trickle 交换，响应端提前注册 QUIC listener，并用显式完成确认减少小文件发送延迟。
- 修复 DHCP 旧绑定地址、Windows EFS 原子替换、LAN 信令缺失签名及传输终态过早关闭等问题。
- 接收端已验证字节统计改为 O(1)，恢复位图按批次 checkpoint；崩溃恢复仍会重新哈希 staging 数据。

## 当前限制

本项目仍处于测试预发布阶段。当前没有文件中继或 TURN allocation，能力固定为 `relay=false`；在 UDP 被阻断或 NAT 映射/过滤不允许打洞时，直连会明确失败。macOS 包仅为 ad-hoc 签名且未公证，Windows 包未进行代码签名。公共 IPv6、网络切换、睡眠唤醒、更多真实 NAT 类型以及完整安装/卸载矩阵仍需继续验证。

详细证据和未完成项见 [验收矩阵](docs/ACCEPTANCE.md)、[实现进度](docs/PROGRESS.md) 和 [v0.4.0 发布说明](docs/RELEASE-v0.4.0-TEST-CANDIDATE.md)。

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
