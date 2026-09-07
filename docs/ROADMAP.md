# Roadmap

- M0：两个 Go module、Wails 模板、核心文档和 CI，已基本完成。
- M1：真实 ICE/QUIC loopback 关卡已完成；双 NAT srflx 和两机 LAN 仍是架构冻结条件。
- M2：身份、邀请、WSS presence、会话 generation、撤销和本地信任已落地，应用编排仍需扩展。
- M3：正式控制面建立端到端单文件传输，当前由 `demo-local` 证明的 loopback 路径先行。
- M4：多文件、文件夹、重启续传、暂停/取消和桌面任务状态待应用编排接入。
- M5：Wails 共享服务绑定已开始，Windows/macOS 实机矩阵待目标设备。
- M6：Docker、HTTPS/Caddy、STUN-only、诊断导出和性能报告待部署环境。

Relay、TURN 文件中继、移动端、浏览器端、UPnP/NAT-PMP/PCP 和云端暂存只保留能力错误，不在本期实现。
