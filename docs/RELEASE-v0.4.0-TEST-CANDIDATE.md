# LinkSend v0.4.0 test prerelease

`v0.4.0` 是协议 V1 上的测试预发布，不是稳定版或生产可用声明。相对 `v0.3.0`，本次发布重点改善首次配对、局域网发现、连续发送及时性和传输终态可靠性。

## Changes

- 配对码改为可读的 `ABCD-EFGH` 形式，10 分钟有效、单次使用；相同 Ed25519 身份可幂等重试，其他身份复用会被拒绝。
- 新增无效、过期、已使用和身份冲突错误码，以及基于服务端相对 TTL 的客户端倒计时。
- 新增签名 UDP 组播、每接口定向广播、受限单播和已知地址探测；不会扫描整个子网。
- LAN 控制通道强制 TLS 1.3 双向身份验证，并复用现有签名 ICE envelope、Pion ICE 和 QUIC 数据面。
- 附近陌生设备只有在发送方主动选择、接收方确认且传输完成后才会保存设备 pin。
- 认证 WSS 使用单后台读泵和连接复用；ICE 候选改为 trickle 交换，响应端提前注册 QUIC listener。
- 完成流程加入 `confirmed_ack`，并把 QUIC draining 移出用户可见完成路径，减少接收确认延迟。
- 修复纯 IP/DHCP 旧绑定、Windows EFS 原子替换、LAN envelope 未签名及连接终态过早关闭等问题。
- 接收端已验证字节统计改为 O(1)，恢复位图按 8 块或 500ms checkpoint；恢复时仍重新哈希 staging。
- 桌面端显示附近设备、LAN/信令可用性、配对码倒计时和单地址发现兜底，并防止重复操作。

## Security and data path

文件正文不会经过 HTTPS/WSS 信令、JavaScript IPC、对象存储或第三方中继。服务器只处理身份、配对、在线状态、会话和 ICE 候选。数据面使用固定设备身份的 TLS 1.3 QUIC，文件块和完整文件使用 BLAKE3 校验，接收内容在验证前保留于隔离 staging。

## Known limitations

`v0.4.0` 仍为测试预发布。当前没有文件中继，UDP 被阻断或 NAT 映射/过滤不允许打洞时会失败；macOS 包仅 ad-hoc 签名且未公证，Windows 包未代码签名。公共 IPv6、物理网络切换/睡眠唤醒、更多 NAT 类型、完整安装卸载和全量原生交互仍未完成。发布不得把这些项目改写为 PASS。

## Validation

- Windows 根模块普通测试、race、vet、`GOWORK=off` 和真实 loopback ICE/TLS 1.3/QUIC 摘要校验通过。
- Wails 桌面独立模块 verify/test/vet/build、前端 typecheck/lint/11 tests/build 和 Windows Wails/NSIS production package 通过；本地产物 FileVersion/ProductVersion 均为 `0.4.0`。
- 物理 Windows→Mac 的受限单播发现、mTLS 控制通道和停止后台接收后的并行准备/ICE/QUIC 精确诊断路径通过；新版原生确认弹窗完整人工点击仍未标记 PASS。
- 香港测试主站已部署 `0.4.0`，公网 health、schema 2、数据库完整性、短码 TTL、同身份幂等与其他身份复用拒绝通过。rendezvous 已迁移为独立 systemd service，Origin 证书真实续期、定时器和 443 复核通过。

## Release assets

GitHub Actions 从发布 tag 的干净 checkout 生成并上传：

- Windows amd64 便携 ZIP（包含 `LinkSend.exe` 和测试说明）；
- Windows amd64 NSIS 安装程序；
- macOS arm64 DMG；
- macOS amd64 DMG；
- 每个平台的 `BUILD-INFO.txt` 和 `SHA256SUMS.txt`。

Release 页面中的资产、哈希和 workflow 运行记录是分发证据；本地 `.artifacts` 中的 dirty snapshot 不得作为正式 Release 资产。
