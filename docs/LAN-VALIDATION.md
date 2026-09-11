# LAN validation matrix

## 当前证据边界

2026-09-11 已在物理 Windows（以太网）↔macOS arm64（Wi-Fi）完成双向 QUIC 文件组传输，双方实际发送/接收、唯一验证和提交均为 12,582,949 bytes，摘要一致；Mac 端拒绝、冲突、权限拒绝和独立磁盘镜像 ENOSPC 也已 PASS。该结果是混合有线/无线同站点实测，不等价于所有 LAN、跨公网、网络切换或双 NAT。

## 已验证

- Windows→Mac 与 Mac→Windows 文件组、中文路径和 SHA256：PASS。
- 接收前明确确认、拒绝、冲突不覆盖、权限拒绝、磁盘不足：PASS。
- TLS 1.3 / ALPN `linksend/1`、pinned peer、session/generation、base socket 和候选证据：PASS。
- 任务状态/错误语义、实际字节与逻辑完成量：自动化 PASS；现场旧证据缺少后来新增的完整 task/attempt/revision 字段，不能倒填。

## 尚未验证

- 物理两机的暂停、强杀、重启、损坏 staging、receiver committed 但 sender unconfirmed 和部分文件提交后的恢复矩阵：NOT_RUN。
- Wi-Fi/以太网切换、DHCP/IP 变化、睡眠唤醒、VPN/TUN 开关和全局 IPv6：NOT_RUN。
- 完整 Wails 原生交互：NOT_RUN；原生窗口创建和 idle 退出不代表文件选择器或恢复入口已验收。

每次实机运行必须保存独立 run ID、两端 binary SHA256/产品版本/commit、网络快照、脱敏连接证据、stdout/stderr/退出码、任务快照、字节计数和最终哈希。每个客户端使用新的 `--data-dir` 与接收目录；不得根据网卡名、私网地址或 candidate 类型推断网络性质，证据不足保持 `direct_unknown`。

macOS 通过已配置的 `codex-ssh-manager` alias `mac-test-102342413` 操作，顺序为 resolve → probe → audit-host。远端实验遵循 inspect → backup → change → verify → rollback，只操作隔离测试目录、进程和磁盘镜像。
