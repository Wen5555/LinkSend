# Roadmap

N3正文预算和N4托盘快速发送源码候选已加入：动态正文预算、独立quick草稿、稳定Go入队收据及轻量面板；本地QUIC/取消/剪贴板并行与主草稿隔离已验，准确包原生/物理仍另列。当前工作转向同提交准确包与剩余A/B/B2/E实机矩阵。

2026-10-18 A–E开发候选已实现独立LAN端口/刷新恢复、新信令/一致性备份、schema3显式客户端恢复与旧上下文隔离、N1目录预设/恢复v2、任务库11原子多目标批次、N3正文预算及N4独立快速草稿。最新6ec3cc4五项CI/三平台包/四资产来源核验通过；准确Mac补齐N1原生目录/规则与80B三文件集合hash及N4原生目标选择保存；原生N2双目标在线/离线、源变化只确认失败项、子项/活动批次取消与重启/直接恢复屏障，N3活动限速/等待控制/改0完成，quick本机目标禁发与45B同机QUIC，真实Finder Services菜单/后台唤回/去重/重启及110B三文件/中文空目录闭环通过。旧Win原生筛选/双CLI批次/正文预算/picker取消仍绑定6edddad，旧恢复与离线撤权也保留各自版本。后续按用户最新要求优先功能可用、缩减重复测试；物理LAN/性能、OS剪贴板、其余系统入口及完整同提交矩阵仍未验，Win新原生LAN启动策略和JP UDP授权条件保持；Mac原生键盘目标选择已补齐。详见[原生检查点](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)与[唯一TODO](DESKTOP-EXPERIENCE-TODO.md)；下面M0–M6保留各自历史版本边界。

当前源码和测试预发布版本为 `0.4.0`，协议保持 V1。里程碑状态以源码实现、自动化、物理实机和发布验证分别判定。

- M0 — **IMPLEMENTED / PASS**：两个 Go module、Wails 3、核心文档和 CI 已建立。
- M1 — **PARTIAL**：真实 ICE/QUIC loopback、Windows↔macOS LAN 和独立双 NAT 固定映射正向均 PASS；MASQUERADE-only 仍 `CHECK_TIMEOUT` FAIL，其他 NAT/公网 IPv6未覆盖。
- M2 — **IMPLEMENTED / PASS（自动化与香港测试主站）**：身份、邀请、WSS presence、session/generation/owner、撤销和本地信任已落地；测试主站 schema 2 配对与完成/拒绝后立即新连接已复核。
- M3 — **IMPLEMENTED / PASS（已覆盖物理双机）**：正式信令上的双向 QUIC 文件/目录传输、摘要和安全错误处理已验证。
- M4 — **IMPLEMENTED / PASS（本地真实 QUIC）/ NOT_RUN（完整物理恢复矩阵）**：多文件/目录、schema 2 历史、暂停、取消、重启恢复、缺块续传和提交记录已接入共享应用服务。
- M5 — **PARTIAL**：Wails 3 绑定、revision 门控和原生 idle 退出 PASS；文件/目录选择、打开目录、活跃任务退出保护、恢复入口、macOS 红点/Cmd+Q 实际操作等仍 NOT_RUN。
- M6 — **PARTIAL**：干净 committed workflow 已生成 Windows ZIP/NSIS 与 macOS arm64/amd64 DMG，SHA256、版本和包级结构可追溯；签名、公证、Windows 安装卸载、Intel Mac 启动和完整原生交互仍待完成。

下一优先级是：消除可运行 NAT FAIL；完成物理断网/强杀/提交边界恢复矩阵；完成原生交互、安装与签名/公证矩阵。Relay、TURN 文件中继、移动/浏览器客户端和自动端口映射仍不在当前版本范围。
