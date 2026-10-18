# Roadmap

N3正文预算和N4托盘快速发送源码候选已加入：动态正文预算、独立quick草稿、稳定Go入队收据及轻量面板；本地QUIC/取消/剪贴板并行与主草稿隔离已验，准确包原生/物理仍另列。当前工作转向同提交准确包与剩余A/B/B2/E实机矩阵。

2026-10-18 A–E开发候选已推进独立LAN端口/刷新恢复、新信令/一致性备份、显式客户端成员恢复、N1目录预设和N2原子多设备批次。恢复包括schema3旧上下文退休、旧任务/outbox隔离、正常新码重配和独立LAN保留；N1规则保存至草稿/队列及恢复v2，N2将任务库升级11并保持独立子项/取消/重启屏障。6edddad五项CI/三平台包与四资产核验通过，准确Win原生筛选/双CLI批次/正文预算和三个picker取消复验通过；同提交Mac arm64启动通过。B2物理、N4系统入口、准确包最终矩阵及集成继续；新Win原生LAN启动被执行策略拒绝，Mac UI/JP授权缺口保持，详见[原生检查点](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)。当前状态以[唯一TODO](DESKTOP-EXPERIENCE-TODO.md)为准；下面M0–M6保留各自历史版本边界。

当前源码和测试预发布版本为 `0.4.0`，协议保持 V1。里程碑状态以源码实现、自动化、物理实机和发布验证分别判定。

- M0 — **IMPLEMENTED / PASS**：两个 Go module、Wails 3、核心文档和 CI 已建立。
- M1 — **PARTIAL**：真实 ICE/QUIC loopback、Windows↔macOS LAN 和独立双 NAT 固定映射正向均 PASS；MASQUERADE-only 仍 `CHECK_TIMEOUT` FAIL，其他 NAT/公网 IPv6未覆盖。
- M2 — **IMPLEMENTED / PASS（自动化与香港测试主站）**：身份、邀请、WSS presence、session/generation/owner、撤销和本地信任已落地；测试主站 schema 2 配对与完成/拒绝后立即新连接已复核。
- M3 — **IMPLEMENTED / PASS（已覆盖物理双机）**：正式信令上的双向 QUIC 文件/目录传输、摘要和安全错误处理已验证。
- M4 — **IMPLEMENTED / PASS（本地真实 QUIC）/ NOT_RUN（完整物理恢复矩阵）**：多文件/目录、schema 2 历史、暂停、取消、重启恢复、缺块续传和提交记录已接入共享应用服务。
- M5 — **PARTIAL**：Wails 3 绑定、revision 门控和原生 idle 退出 PASS；文件/目录选择、打开目录、活跃任务退出保护、恢复入口、macOS 红点/Cmd+Q 实际操作等仍 NOT_RUN。
- M6 — **PARTIAL**：干净 committed workflow 已生成 Windows ZIP/NSIS 与 macOS arm64/amd64 DMG，SHA256、版本和包级结构可追溯；签名、公证、Windows 安装卸载、Intel Mac 启动和完整原生交互仍待完成。

下一优先级是：消除可运行 NAT FAIL；完成物理断网/强杀/提交边界恢复矩阵；完成原生交互、安装与签名/公证矩阵。Relay、TURN 文件中继、移动/浏览器客户端和自动端口映射仍不在当前版本范围。
