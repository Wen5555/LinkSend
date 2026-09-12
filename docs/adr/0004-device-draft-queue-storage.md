# ADR 0004：设备偏好、发送草稿与队列复用任务历史库

日期：2026-09-12。状态：M1 存储基础已实现，调度、界面和完整原生验收分别记录。

## 背景

桌面六项能力需要在重启后保留本地设备命名、发送草稿与排队意图。现有 `task-history.sqlite` 为 schema 2，已经保存任务快照、私有恢复元数据和 revision。另建一个“信任库”会造成身份权限有两个所有者；另建队列数据库也会使入队、草稿清理与历史关联难以事务化。

## 决策

继续使用一个本地 SQLite 文件 `task-history.sqlite`，升级为 schema 3。`historyDB` 是唯一 schema 初始化/迁移入口，`openDesktopStore` 通过它打开同一文件。每个连接池最多一个连接，`busy_timeout=5000`；任务历史和元数据可以各持连接池。M0 profile 内核锁保证同一 profile 一个应用写入者；SQLite 事务与 CAS 仍保护应用内并发和重入。

新增三个实际使用的表：

| 表 | 字段与约束 | 所有者 |
|---|---|---|
| `device_profiles` | `peer_id` 主键；`alias`、`my_device`、`pinned`、`position`、`receive_directory`、`last_used_at`、正整数 `revision`；布尔列限制 0/1，position 非负 | app 本地偏好 |
| `send_drafts` | `id` 主键；`source_paths` 必须是 JSON 数组、`peer_id`、正整数 `revision`、`updated_at` | app 草稿 |
| `send_queue` | `id` 主键、唯一 `request_id`、`peer_id`、JSON 数组 `source_paths`、`source_digest`、`state`、`position`、`task_id`、`expires_at`、`last_error`、`wait_for_peer`、正整数 `revision`、`created_at`、`updated_at` | Go scheduler |

设备排序索引为 `(pinned DESC, position, peer_id)`；队列派发索引为 `(state, position, created_at, id)`，对端索引为 `(peer_id, state)`。队列没有引用 tasks 的外键：清理历史不能隐式删除排队意图或用户文件。队列状态机由调度器定义，DDL 不硬编码会妨碍后续状态演进的状态枚举。`last_error` 只保存稳定原因码，`wait_for_peer` 记录用户是否明确选择等待离线设备。

设备配置只含本机显示与目录偏好，**不含公钥 pin、拒绝记录或 auto_accept**。收藏或“我的设备”标签不授予身份信任或免确认。相同别名但不同 `peer_id` 永远是不同记录；修改目录不修改已经建立的 ReceivePlan。

## API 与并发语义

`desktopStore` 在 `internal/app` 内共享；导出的 `DeviceProfile` / `SendDraft` 只是类型化 DTO。设备接口为 `DeviceProfiles`、`DeviceProfile`、`SaveDeviceProfile`、`TouchDevice`；草稿接口为 `Draft`、`Drafts`、`SaveDraft`。队列事务直接复用同一 store 的 DB，调度器只维护一份状态机。

`SaveDeviceProfile` 和 `SaveDraft` 的输入 `Revision` 是 expected revision：0 只能创建，已有记录必须精确匹配。插入冲突或旧 revision 返回 `METADATA_REVISION_CONFLICT`，成功提交后才返回递增 revision；数据库只读、空间不足、锁或其他 SQL 错误不会伪装成保存成功。查询缺失记录返回 `METADATA_NOT_FOUND`；输入格式或限额错误返回 `METADATA_INVALID`。

`LastUsedAt` 不接受偏好表单写入。只有真实交互后调用 `TouchDevice`，该操作可为首次真实使用创建默认偏好记录，并递增 revision。使用固定 9 位小数的 UTC RFC3339 时间格式，使 SQL 字典序可准确拒绝迟到的旧事件；重复/旧时间不会倒退时间或制造新 revision。revision 达 SQLite int64 上限时明确报告冲突。

设备标识要求 64 位小写十六进制；别名最多 128 个 Unicode 字符、512 UTF-8 bytes，不含控制字符。目录为空表示继承全局值，否则要求当前平台绝对路径并通过现有目录 `stat`；实际接收前仍必须验证目的目录、权限、容量和安全落盘条件，这不是保存成功保证。

## 草稿与路径边界

默认草稿 ID 为 `main`，保留后续多草稿的可能。用户显式编辑即保存元数据；保存草稿或读取列表都不会派发任务。

- 路径上限 4096 项，每项 32768 UTF-8 bytes，编码后的路径数组最多 1 MiB。计入 JSON 编码后的大小，失败不截断后报告全部加入。
- 相对路径必须携带发起进程的绝对 WorkingDir，按该目录解析。Windows `C:relative` 或只带根斜线的歧义路径拒绝；不把应用自己的当前目录冒充发起目录。
- 词法 clean、去重并保留首次出现顺序；Windows 对归一化路径进行大小写去重，其他平台保留大小写。此阶段不通过跟随 symlink 或读取文件正文判断同一文件。
- 草稿允许保存暂时不存在的源路径，以便显示和修复；入队与实际发送必须重新验证文件可用性和源摘要。此处不会创建、读取或复制源文件正文。
- 目标切换不会隐式清空 Paths；旧 revision 的表单不能覆盖后续编辑。空草稿持久为 `[]`，不是 SQL NULL 或 JSON null。

本地路径和接收绝对目录不进入网络协议；文件/图片正文仍不得经 JavaScript IPC 或信令服务。

## 迁移、失败与回滚

从 schema 1/2 升级时，先执行 `VACUUM INTO` 生成带时间戳的 `task-history.sqlite.schema-vN-*.bak`，设置受限文件权限，再实际打开备份并执行 `PRAGMA query_only=ON`、`integrity_check`、schema 核对及 tasks 可读性查询。验证后显式执行文件 `Sync`，使备份持久化不依赖源连接未来的 synchronous 配置，成功后才能提交新 schema。[SQLite 官方 VACUUM 说明](https://www.sqlite.org/lang_vacuum.html) 于本次 HTTP 200 核实：源 synchronous 为 NORMAL/FULL 时 SQLite 自己也会 flush 输出；显式 Sync 是额外保证，不声称 SQLite 一律不 flush。任何一步失败都停止迁移并返回 `TASK_STORE_BACKUP_FAILED`，不创建新 schema。

备份验证通过后，在同一事务中执行旧 recovery 列兼容迁移、新表/索引 DDL、metadata 版本写入与 `PRAGMA user_version=3`。任一 DDL 失败全部回滚，原 tasks snapshot/recovery 字节不重写。已是 schema 3 的正常重开不再备份或执行新 DDL。未知更高 schema 返回 `TASK_STORE_VERSION`，不尝试降级。

回滚必须先退出应用，确认 profile 锁释放，保留当前 schema 3 数据库及其可能的 WAL/SHM，再恢复经过核验的迁移前 schema 2 备份，最后启动旧版本。旧程序会拒绝 schema 3，不能仅换旧 EXE 后强行降低 `user_version`。迁移后新增的设备/草稿/队列记录不会出现在旧备份里，回滚前必须明确这一数据时间点。回滚只针对应用数据库，不移动或删除已接收的用户文件。

## 验证边界

存储回归覆盖 schema 2 实际迁移、任务与 recovery 字节保留、备份可读性、重复打开、事务中途 DDL 失败回滚、损坏/错版备份、更高 schema 无修改拒绝；设备覆盖重启、别名同名异身份隔离、跨连接并发 CAS、迟到 last-use、revision 极限、目录与文本验证；草稿覆盖中文空格、WorkingDir、路径去重、目标切换、缺失源、重启、空草稿、大小限制和陈旧 revision；数据库只读故障验证无成功提交声称；队列表验证唯一幂等键、JSON 数组、默认等待权限和无历史外键。

这些是存储测试，不等于队列已完成实际派发、真实 QUIC 已复测或原生 UI 已验收。具体命令和结果记录于 M1 evidence。
