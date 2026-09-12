# ADR 0006：内容草稿、受控引用与原生接收动作

日期：2026-09-12。状态：M5 后端已实现，原生动作与完整 UI 由主应用集成验收。基于 schema4 收件箱和 `content_v1` / `acceptance_commit_v1`，保持两个 Go module。

## 决策

内容草稿独立于多文件 `send_drafts`。用户明确输入短文字/链接或明确点击读取当前剪贴板后，Go 创建限额不可变快照；React 获得 `ContentDraft` 与 `Snapshot` 元数据，不回传从文件读取的正文。草稿创建与入队使用不同幂等 request ID；重复请求复用同一快照，不再次读取已经变化的剪贴板。

task-history.sqlite 升级到 schema5，仅增加三张元数据表：`content_drafts` 保存幂等请求摘要、快照信息、revision 与 draft/queued/discarded 状态；`content_queue` 保存 queue ID、快照和明确文件降级选择；`content_tasks` 保存 task ID、源快照身份、实际 native/file 模式与协议内容绑定。既有任务、设备、文件草稿、队列和 schema4 收件箱表保留。升级沿用先 `VACUUM INTO`、完整性检查、Sync 备份再单事务变更；旧程序因更高 schema 明确拒绝打开，不会忽略内容身份继续发送。

入队时普通 send_queue 行、content_queue 映射和消费草稿 revision 在同一 SQLite 事务提交。创建快照先保留 `app-content:` 命名空间引用，再记录 SQLite 草稿；中断最多留下额外受控对象，无法把已排队正文变为无引用。Go content.Store 提供只读、有界的 OwnedObjects 引用清单供崩溃后对账，未知所有者命名空间保持不动。

发送任务通过真实 DirectConfig/SendWithOptions 调用，在经过接受能力与摘要检查后，由 `ContentAccepted` 先保存实际内容解释，再保存私有恢复记录；两次写入都成功才允许既有 `accepted` 屏障放行正文。中间失败或崩溃可留下额外已协商元数据，不能产生成功发送。恢复比较 task 私有身份与 content_tasks，验证源快照持久文件身份、大小和 BLAKE3。曾实际降级的 attempt 恢复时继续普通文件；曾接受原生类型的任务不因对端变化自动切换模式。

接收统一进入 M4 `taskReceiveOptions`，同时要求主应用已显式启用原生动作能力。内容元数据进入 `IncomingFilesPage.Content`，最终目录/选择仍属于 M4 ReceivePlan。内容解释在请求正文前记录；完成、NoContent、拒绝、暂停和恢复继续使用 Go 任务内核。全跳过的内容记录不能变成可复制/打开/保存的项目。

## 接收动作与文件边界

`ContentTask(taskID)` 仅返回类型、尺寸、模式与动作可用性。PreviewReceivedText / CopyReceivedText / OpenReceivedURL / SaveReceivedImage 只接受 task ID；路径从已确认的接收任务和 M6 保存映射中解析，重新检查原 manifest、内容绑定、实际文件身份、大小与完整摘要。未完成、无双方确认、被移动/修改或普通文件降级均不给原生内容动作。

文字预览在原生 Info 对话框显示最多 4096 字符；完整复制通过 Go 调用原生 Clipboard；URL 在每次明确打开时重新限定 http/https，其他 scheme 仍可作为文字预览/复制，永不自动执行。PNG 保存目标来自 Go 原生文件对话框，O_EXCL 不覆盖现有文件，失败时只清理本次创建且文件身份仍一致的目标。结果 DTO 仅为 task/action/state，浏览器启动和预览请求返回 submitted，不声称用户已经浏览或关闭对话框。

所有 Wails 原生 API 依据 beta.18 固定源码：Dialog.Info/SaveFile、Clipboard.SetText、Browser.OpenURL。剪贴板 manager 的惰性创建与写入一起派发到原生主线程，避免并发首次复制竞态。桌面宿主在 core 与 runtime 就绪且 UI 动作接好后调用不绑定的 `initializeContentActions()`；未启用时仍可创建/发送内容，但不宣称具备接收原生内容的能力。

## 保留与清理

默认 `retain_sent_snapshots=false`。活动草稿、队列、暂停/恢复任务必须保留正文；终态的额外发送快照由用户明确清理时移除。打开保留开关后，现存发送历史继续保护快照，以便真实内容重发；历史重发创建新的逻辑队列/任务，沿用内容类型，不静默变成普通文件。已清理的历史内容明确报告不可用。

清理同时锁住内容变更、队列和派发入口；除结构化内容引用外，也检查普通文件草稿/队列/可恢复任务的路径重叠。Store 仅删除验证了所有权、持久 OS 文件身份、摘要和允许文件名的对象，不递归扫描接收目录，不删除收到的用户文件。SQLite 历史只存元数据；用户在接收决策中确认保存的输出文件仍由用户管理，清理发送快照不会替代删除这些文件。

## 验证与范围

真实同机 Pion ICE / pinned TLS 1.3 / QUIC 验证三种内容、默认停止/显式降级、接受写库失败 0 B、恢复和整个 Service 重启后的同一快照/TransferID。原生动作边界在 Go 使用可观察回调验证，不能冒充实体 UI 点击、剪贴板写入或浏览器启动；这些由后续原生验收记录单列。详见 [M5 后端证据](../evidence/DESKTOP-M5-CONTENT-BACKEND.md)。
