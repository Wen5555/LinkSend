# M1 SQLite 元数据与迁移验证

日期：2026-09-12。基线 M0 提交 `b3d5fc7`，本记录对应其后的工作树实现；尚未据此单独提交或发布。

实现文件：`internal/app/history.go`、`desktop_store.go`、`desktop_store_test.go`；设计与回滚见 [ADR 0004](../adr/0004-device-draft-queue-storage.md)。未改写现有任务快照/恢复字节格式，文件协议 V1 不变。

本次完成 schema 2 → 3 实际 SQLite 迁移：备份生成、受限权限、实际 integrity/schema/tasks 可读性验证和文件 Sync 在迁移前完成；DDL、metadata 版本与 user_version 同事务。设备偏好与草稿使用 CAS，队列包含唯一 request_id、用户明确等待字段与稳定原因码，不含任务历史外键。设备表不复制身份授权。

| 实际命令（PowerShell7，便携 Go1.27.1，GOWORK=off） | 退出码 | 结果 |
|---|---:|---|
| `gofmt -w internal/app/history.go internal/app/desktop_store.go internal/app/desktop_store_test.go` | 0 | 格式化完成 |
| 首次定向 `go test ./internal/app -run TestTaskHistoryMigratesV1AndQuarantinesCorruptRows$ -count=1` | 1 | 当时并行实现尚未定义 queueManager/QueueItem/queueItems，编译未完成；未计为通过 |
| `go test ./internal/app -run 'TestDesktop(Store\|Device\|Draft\|Metadata\|Queue)\|TestTaskHistoryMigrates' -count=1 -v`（表达式实际使用 Go regexp 的 `|`） | 0 | 12 个新增存储顶层用例与既有 schema1 迁移回归 PASS |
| 同范围 `go test -race ... -count=1` | 0 | 含跨独立 DB 连接并发 CAS 的 race 检查 PASS |
| `go test ./internal/app -run 'Test(TaskHistory\|CorruptTaskHistory)' -count=1`（同上，实际 `|`） | 0 | 既有任务历史、损坏隔离、正常/强杀进程和写失败回归 PASS |
| `go vet ./internal/app` | 0 | PASS |
| 增加备份显式 Sync 后 `go test ./internal/app -run 'TestDesktopStore\|TestTaskHistoryMigrates' -count=1`（实际 `|`） | 0 | 迁移与原历史 schema1 路径再次 PASS |

关键真实断言：

- schema 2 原 task snapshot/recovery 在迁移库与备份库中逐字节一致；备份仍为 schema 2，无新增元数据表；重开 schema 3 不重复迁移。
- 第三个新增表遇到实际冲突 view 时，前面新建的 device/draft 表全部回滚，原 schema2、task revision 和备份保留。
- 损坏/错 schema 备份拒绝；未知 schema4 打开失败且文件字节不变。
- 相同别名不同 peer 不共享偏好；同 expected revision 的 12 个并发跨连接写入仅 1 个提交，其余返回 CAS 冲突；UI 不能伪造 last-used。
- 中文、空格、WorkingDir 与 Windows 歧义相对路径均覆盖；未创建的源路径仍可保存草稿，证明草稿保存不依赖打开正文；切换目标/重启/清空后语义保持。
- 4096 项、单路径大小、真实 JSON 总大小限制和无效 UTF-8/控制字符拒绝；超限不截断。
- SQLite query_only 故障下设备、草稿和 last-use 均报错，不返回递增提交 revision；旧数据仍可读取。
- 队列重复 request_id、非数组 JSON 被 SQLite 约束拒绝；默认没有离线等待授权、没有 tasks 外键。

此处不把存储测试冒充真实队列派发、QUIC、Mac、原生入口/通知或安装验收；调度器与跨功能证据由主线程记录。本分工没有提交、推送、打 tag 或发布。
