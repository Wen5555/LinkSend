# ADR 0003：桌面 profile 所有权、本机拒绝与保存退出

日期：2026-09-12。状态：M0 实现；跨平台结果见阶段证据，不据此宣称六项完成。

## 决策

继续使用两个 Go module、Wails 3 beta.18、Pion ICE、quic-go 与协议 V1。
依赖精确升级与官方证据见 [M0 依赖报告](../evidence/DESKTOP-M0-DEPENDENCIES.md)。
不为 Wails 原生能力迁移框架，也不改变文件正文路径。

`internal/app.New` 在读取/创建身份及 SQLite 前持有 profile 的 OS 排他锁。
Windows 使用 LockFileEx，Unix 使用 flock；锁文件 `.profile.lock` 保留，退出不删除。
正常退出及强杀由内核释放锁；PID 文件、删除“陈旧锁”或仅 SQLite busy_timeout 无法代替所有权。
同 profile 第二个 CLI/GUI 服务拒绝 `PROFILE_IN_USE`。M2 的 Wails 单实例转交参数发生在
ServiceStartup 前；不同 profile 可并行，同 profile 的 CLI 不绕过 GUI 所有权。

本机信任的唯一所有者仍是 `identity/trust.json`。格式 schema 1 增加 `denied_peers`，
以完整身份 ID 记录拒绝与时间；撤销原子移除 pin、免确认和历史 LAN 地址，同时写入拒绝。
本地拒绝先于服务端 Revoke，所以网络失败不会让本机继续自动接收；服务端失败仍作为错误报告。
成员同步跳过拒绝，其他有效设备仍可用。LAN 新连接、WSS 对端查询、新发送、接收确认和恢复
统一检查拒绝；写入失败如实返回，读取失败拒绝授权。

解除拒绝是显式本机操作，只移除拒绝，不恢复旧 pin 或免确认；后续配对或 LAN 确认独立进行。
别名/固定/我的设备元数据将在 M1 存入任务库的版本化元数据表，不复制身份授权到第二个数据库。

退出先建立新派发屏障，再为活动任务设置保存意图，取消网络工作并等待 owners 结束。
迟到进度不能覆盖暂停、取消或保存意图。可恢复任务保留逻辑 task/attempt、验证块和恢复身份，
重启后等待用户确认；尚无恢复元数据的准备任务报告中断，不伪称可以续传。
`ShutdownContext` 超时只返回尚未清理的状态，清理结束前仍持有 profile 锁。
原生退出增加“保存并退出”，保留用户主动“取消任务并退出”的独立选择。

## 迁移与回滚

首次 trust 写变更前创建可读、受限的 `trust.json.pre-schema-1`。已有 legacy peers 保持可读；
支持未知可选字段，未来 schema 明确拒绝。迁移备份存在后主文件丢失/损坏时不自动恢复
旧 `.previous`，避免把拒绝前的 pin/auto_accept 复活。

M0 不改变任务历史 schema 2 或文件帧；M1 元数据与 M4 接收计划须独立迁移和 ADR。
旧程序不理解拒绝记录及 profile 锁，不能与新版使用同一目录或直接打开新 profile。
回滚步骤：关闭所有新旧进程，备份当前完整 profile；只在隔离目录恢复迁移前 trust/SQLite
及旧偏好后启动旧程序。该恢复会撤回后续本地拒绝，必须重新审查授权；不要覆盖当前数据。
恢复块 checkpoint/fsync 的顺序保持由 transfer 层控制。

## 验收边界

自动测试覆盖 OS 锁竞争、子进程退出/强杀锁释放、拒绝与迁移、源/身份恢复负例及真实 QUIC
在已验证块后保存退出。窗口/托盘/通知、Finder Services、安装卸载属于各阶段原生验收，
不得由编译或 loopback 结果代替。网络矩阵的既有 MASQUERADE-only FAIL 保留。
