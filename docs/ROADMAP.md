# Roadmap

当前源码产品版本为 `0.3.0`，已发布测试预览仍为 `0.2.0`，协议保持 V1。里程碑状态以源码实现、自动化、物理实机和发布验证分别判定。

- M0 — **IMPLEMENTED / PASS**：两个 Go module、Wails 3、核心文档和 CI 已建立。
- M1 — **PARTIAL**：真实 ICE/QUIC loopback、Windows↔macOS LAN 和独立双 NAT 固定映射正向均 PASS；MASQUERADE-only 仍 `CHECK_TIMEOUT` FAIL，其他 NAT/公网 IPv6未覆盖。
- M2 — **IMPLEMENTED / PASS（自动化与香港测试主站）**：身份、邀请、WSS presence、session/generation/owner、撤销和本地信任已落地；测试主站 `0.2.0` 的完成/拒绝后立即新连接已复核。
- M3 — **IMPLEMENTED / PASS（已覆盖物理双机）**：正式信令上的双向 QUIC 文件/目录传输、摘要和安全错误处理已验证。
- M4 — **IMPLEMENTED / PASS（本地真实 QUIC）/ NOT_RUN（完整物理恢复矩阵）**：多文件/目录、schema 2 历史、暂停、取消、重启恢复、缺块续传和提交记录已接入共享应用服务。
- M5 — **PARTIAL**：Wails 3 绑定、revision 门控和原生 idle 退出 PASS；文件/目录选择、打开目录、活跃任务退出保护、恢复入口、macOS 红点/Cmd+Q 实际操作等仍 NOT_RUN。
- M6 — **PARTIAL**：干净 committed workflow 已生成 Windows ZIP/NSIS 与 macOS arm64/amd64 DMG，SHA256、版本和包级结构可追溯；签名、公证、Windows 安装卸载、Intel Mac 启动和完整原生交互仍待完成。

下一优先级是：消除可运行 NAT FAIL；完成物理断网/强杀/提交边界恢复矩阵；完成原生交互、安装与签名/公证矩阵。Relay、TURN 文件中继、移动/浏览器客户端和自动端口映射仍不在当前版本范围。
