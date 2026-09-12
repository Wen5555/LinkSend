# M6 收件箱后端、历史索引与安全清理

更新：2026-09-12。独立分支 `codex/desktop-inbox`，基于已提交 `3feb03c`；未复制主工作区的未提交文件。

本阶段实现 Go 后端和真实数据库/文件系统路径。桌面 Inbox 页、原生 reveal、M4 统一 PlanChanged 接线和跨功能 UI 验收由主任务继续完成；本记录不把后端测试等同于六项功能原生验收。

## 数据与查询

- task store schema **3 → 4**。沿用迁移前 `VACUUM INTO`、备份 schema/integrity 核验与文件同步；新建任务筛选索引、完整 manifest/文件名表、FTS5 trigram、暂存所有权登记和历史删除标记。
- M4 实际 QUIC 联调发现旧 historyDB 每次打开都做 DDL/metadata 写事务，引发 `SQLITE_BUSY` 和接收计划持久化失败。schema 已是当前版本时改为只读核验版本后返回；根据已核实的 modernc/sqlite 1.58.0 `conn.go`/`tx.go` 使用 `_txlock=immediate`，在事务开始等待写锁，避免读锁升级立即 BUSY。DSN 使用正确转义的文件 URI，并为每次连接设置 busy timeout。
- 支持 peer、send/receive、状态集合、起止日期、文件名搜索。每页默认 50、最多 100；游标绑定筛选条件，按固定开始时间和 task ID 做 keyset 分页，拒绝把旧游标用于新筛选。无任意 SQL、文件路径或排序表达式输入。
- 3 字符以上的文件名使用 SQLite FTS5 trigram；1–2 字符使用数据库内 `instr` 回退，仍仅返回有界结果。名称先 Unicode NFC + case fold，中文和清单前三项之后的文件名均可检索。旧历史没有完整 manifest 时仅保留已有摘要搜索，不能声称迁移恢复了从未保存的文件名。
- 启动只把最近 200 个终态任务及未结束任务恢复到内存。Workspace 最多返回 200 项，活跃任务优先；完整历史由 Inbox 分页查询，私人 recovery、网络诊断和绝对路径不进入 Inbox DTO。

## 稳定 API

```go
Inbox(ctx, InboxQuery{PeerID, Direction, States, After, Before, Search, Cursor, Limit}) (InboxPage, error)
InboxFiles(ctx, taskID, cursor, limit) (InboxFilesPage, error)
ResolveInboxFile(ctx, taskID, fileID) (InboxLocation, error)
ResendInbox(ctx, ResendInboxRequest{TaskID, RequestID, WaitForPeer}) (QueueItem, error)
ForgetInboxRecords([]InboxRecordRef{{TaskID, Revision}}) error
CleanupInboxStaging(ctx, limit) (InboxCleanupResult, error)
```

`ResolveInboxFile` 只接受不透明 task ID 和清单 file ID，并从 Go 的持久计划映射取得实际路径。只允许已完成、双方确认的接收任务；重新检查目录、普通文件/目录类型、symlink 和长度，移动/删除返回 `INBOX_FILE_MOVED_OR_DELETED`。`InboxLocation.Path` 标记 `json:"-"`，由 Go 原生 reveal 使用；没有给 JS 任意路径打开接口。此定位检查不重算整个文件哈希，不声称内容重新验签。

`ResendInbox` 是新的逻辑发送。先重建原 manifest 核对源是否变化，再经 Enqueue 的二次内容摘要检查和派发前校验；原有 chunk 布局不要求与新队列布局相同。相同 request ID 取得同一新队列项；确认丢失后的重试不会因后来剪贴板/源变化再排入另一份内容。接收历史不隐式“发回给原发送者”，仅发送历史提供此操作；恢复旧任务继续使用既有 ResumeTask。

## 统一接收 hook

```go
// 已持久化 TaskSnapshot.ManifestDigest 后，可记录完整 offer（含后来拒绝的请求）：
s.IndexInboxManifest(ctx, taskID, manifest)

// 同一个、串行的 PlanChanged callback：
persistCurrentTaskPlan(plan) // M4 App 的最终目录与 plan/selection digest
return s.RegisterInboxReceivePlan(taskID, manifest, plan)
```

`RegisterInboxReceivePlan` 只能在 OpenReceiver 已持久化本计划之后调用。它核对任务/peer/manifest/plan digest、读取真正的 stage 目录及 `.part` 文件身份，并把完整名称、选中最终路径和暂存登记写入同一个 DB 事务。调用失败必须传播，阻止 accept、正文请求或新目标名 commit。发送与恢复发送的完整 manifest 索引已直接接入 `tasks.go`，接收侧由 M4 App 作者接入统一 callback，避免双方覆盖 hook。

## 删除与清理边界

- Forget 只删除本地历史/文件名索引，使用期望 revision；同步删除内存任务视图并断开已结束队列的 task 引用。活跃/暂停/可恢复失败/待处理队列引用均拒绝删除。
- 删除标记长期保留不透明 task ID；即使更高 revision 的迟到 observer 保存，也不能重新插入已忘记的任务。没有通过“仅从内存删掉”伪装可靠清理。
- owned staging 单独清理。只访问由 PlanChanged 登记的 root + transfer ID，比较 Windows volume/file index/creation time 或 Unix dev/ino。未知文件、外来替换 `.part`、目录替换、symlink、损坏 checkpoint 都保留。
- 清理同时检查本任务、同一 transfer 的新接收任务、其他活跃/暂停/可恢复来源、队列和草稿引用；序列化新派发/恢复，防止检查后启动新写入者。每次最多 100 项，默认 20。
- 不递归删除，不扫描用户接收目录，不删除任何最终接收路径。只 unlink 已登记的 stage 链接，已完成的目标硬链接及用户后来移动的文件保持原样。
- M5 内容快照仍由其自己的引用 Store 管理；Forget 不释放或删除任何仍被任务/草稿/历史正文保留选项引用的 snapshot。损坏/未知暂存、清理中断后的不完整目录会返回问题并保留，不能宣称无条件自动回收。

## 实际验证

Windows PowerShell 7 / Go 1.27.1：

```powershell
go test ./internal/app -run TestInbox -count=1 -v
$env:GOWORK = 'off'
go test -race ./internal/app -run 'TestInbox|TestTaskHistory|TestDesktopStore|TestQueue' -count=1
go test ./internal/app -count=1
go vet ./internal/app
# desktop 模块：
go test ./...
go vet ./...
```

已 PASS：新 Inbox 用例、完整 app 普通测试、针对性 app/history/schema/queue race、根 `GOWORK=off go test ./...`，以及 desktop 独立普通测试和 vet。后续合并 M4/M5 接线后仍需完整复核。

实际 SQLite 万条 fixture：按设备、方向、状态和日期选出 43 行，7 次 keyset 分页耗时约 **2.69 ms**；`EXPLAIN QUERY PLAN` 确认使用 peer/date 索引。该数字只描述本机内存热缓存的测试查询，不是生产延迟保证。

文件系统回归实际创建 Receiver、写块、完成 hard-link commit，再登记/定位/清理；核对最终接收字节不变。另覆盖清单末尾中文文件、注入样式查询、游标错用、无界请求、schema3 备份、450 行启动只恢复 200 项、删除后迟到高 revision、防止恢复记录/队列引用被忘记、外来同名暂存、未知用户文件、草稿引用、暂停、同 transfer 新接收器、移动文件和重新发送的幂等新队列。

新增真实多连接锁测试：第一个连接持有写事务时，当前 schema 的 historyDB 打开仍能完成只读核验；第二个读后写事务在 Begin 等待，释放第一把写锁后正常完成，不发生即时锁升级错误。

初轮测试编译发现一个缺失的 fmt/json import 和测试调用 SaveDraft 少传 workingDir，修正后重新运行通过；未作为成功结果掩盖。

## 剩余联调

- M4 接收 callback 的实际接线后复跑“选择/保留两份/重启/定位/清理”的完整 QUIC → App 路径。
- M5 内容类型/引用接入后的历史保留开关，以及明确的复制文字、手动 URL 打开和图片保存。
- 桌面原生 reveal 与收件箱分页 UI、真实双平台操作和 M6 里程碑包验收。
