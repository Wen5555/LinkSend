# M5 内容后端与原生动作边界

2026-09-12，分支 `codex/desktop-content-backend`，在 ec905b9 + M6 47110f8 + M5 wire 44a7435 基础上串行接入统一 M4/M5 接受屏障 b57276d。未修改 App 字段、main、前端或已有 bindings；宿主初始化与实际 UI 由主任务负责。生命周期与迁移决策见 [ADR 0006](../adr/0006-owned-content-lifecycle.md)。

## 已实现接口

| 桌面绑定 | DTO / 行为 |
| --- | --- |
| `CreateContentText({request_id,kind,text})` | text/url 的显式短表单；返回 ContentDraft |
| `CaptureClipboardImage(requestID)` | 经 nativeclipboard 原生适配器读取一次；幂等重试不再读取已变化的剪贴板 |
| `ContentDrafts()` / `DiscardContentDraft(id,revision)` | 独立内容草稿，含 id/snapshot/revision/state/created_at，无正文/绝对路径 |
| `EnqueueContent({request_id,draft_id,draft_revision,peer_id,allow_file_fallback,wait_for_peer,expires_at})` | 消费草稿、创建队列与内容映射单事务提交；返回 QueueItem，可选 Content 元数据 |
| `ContentTask(taskID)` | kind/size/width/height/mode/available/can_preview/can_copy/can_open/can_save |
| `PreviewReceivedText` / `CopyReceivedText` / `OpenReceivedURL` / `SaveReceivedImage` | 只收 task ID；Go 重新验证正文后操作原生系统；返回 task_id/action/state |
| `ContentSettings()` / `SetContentSettings({retain_sent_snapshots})` | 默认 false；活动/可恢复引用始终保留，开关只影响终态发送历史快照 |
| `CleanupContentSnapshots()` | 返回 removed/protected 数量，不删除接收用户文件或外国快照引用 |

宿主在 core/runtime 就绪且 UI 操作接好后调用未绑定的 `initializeContentActions()`。接收 `AcceptNativeContent` 只有该显式启用之后为 true；M4 `IncomingFilesPage.Content/FileFallback` 显示真正协商元数据。文字预览只在原生 Info 弹窗中显示受限内容，不返回文字读取 DTO；打开/预览提交原生请求返回 submitted，复制/保存成功返回 completed。

schema5 新增 content_drafts/content_queue/content_tasks，复用已验证 schema4 备份与事务框架。已有 schema3 迁移回归仍核实 schema3 备份保持不变；增加 schema4→5 的实际备份、表与版本验证。生成内容、排队、恢复、历史重发使用同一受控快照，不把原生内容静默混入普通多文件发送。

## Windows 实际检查

使用 Go 1.27.1、PowerShell 7；每个 Go 命令设置 `GOWORK=off`、`GOTOOLCHAIN=local`。

- 根 `go test -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify` 全部退出 0。
- `go test -race -count=1 ./internal/app ./internal/content ./internal/transfer` 退出 0：app 63.925s、content 1.827s、transfer 7.387s；最后收紧 kind/request ID 边界后 `go test -race -count=1 -run '^TestContent' ./internal/app` 再次退出 0（15.488s）。
- desktop 独立 `go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify` 全部退出 0；最后原生主线程派发版本也重新通过 race/vet/build。

实际覆盖：文字与剪贴板图片创建幂等/并发、草稿重启、限额、入队映射写库失败整事务回滚、活动/恢复/普通文件草稿重叠引用保护、外国引用与被修改正文保留、崩溃遗留 app-content 引用对账、schema4 备份。

真实同机 Pion ICE / pinned TLS 1.3 / QUIC 覆盖 text/URL/PNG 完整队列传输、双方保存同一内容绑定、收到的正文供 Go 复制回调逐字校验、历史重发保持原生内容类型、默认不支持时 0 B 停止和明确文件降级；没有把同机 host candidate 当作物理双机或跨 NAT。

故障注入确认 `ContentAccepted` 写库失败时，双方正文/提交均为 0；恢复存储后同一任务能继续完成。发送中暂停、关闭 Service 与 profile 重建后，经显式 ConfirmQueue 使用同一 task/TransferID/snapshot/content binding、新 attempt 完成；源快照始终保留。此测试发现并修复既有队列问题：活跃恢复阶段的 `recovering` 不能立即映射为 needs_attention，只有 `CanResume=true` 的已停止任务才需要用户再次处理。

原生动作边界测试使用真实安全落盘文件与可观察 Go 回调：未启用原生动作、未双方确认、文件同尺寸改字节均不得复制；未知 URL scheme 不进入 OS 打开回调但可作为文字复制；预览最多 4096 字符；PNG 另存不覆盖现有文件。没有自动改动真实用户剪贴板或打开用户浏览器。

首轮宽范围普通/race 真实失败于旧 M6 测试把最终 schema 硬编码为 4；当前已为 5，因此将“当前版本”断言改为 taskStoreSchema，仍保留原 schema3 备份值与新增 schema4 备份真实性检查。修复后上述回归通过，没有禁用该测试。

## Mac 快照复核与已知基线差异

Mac 源快照 SHA256 `4a94acced613d49a2a29fb8a763b6e69cb6e5341d9df16569fc28c6eed23f2f1`，manager 上传后校验一致。实体 macOS26.5 / arm64、Go1.27.1、`GOWORK=off`、CGo minimum13.0；job `/tmp/codex-ssh/desktop-m5-mac-backend-20260912T073219Z`。

该快照的 core app/content/transfer race 全部通过（28.557s / 1.795s / 5.099s），core vet 与 mod verify 通过。desktop nativeclipboard race 通过，但完整 desktop race 因旧 M2 `TestActivationJournalConcurrentProducersAndRestart` 的 `.lock ENOENT` 失败，20 个并发入口只保留 15 个，job 退出 1，后续 desktop vet/build 未执行。

主任务确认该平台问题已在 **c428f51** 独立修复并通过 Mac 高重复 race；本 M5 隔离基线未包含该提交。此处记录失败，不把已有其他提交的通过结果冒充本快照通过。最后新增的输入 kind/request ID 前置检查由 Windows 目标 race 覆盖，最终合并包仍需按主任务最新源码进行 Mac 复核。M5 原生点击、剪贴板写入、浏览器打开、保存对话框及完整跨设备体验均待主任务集成验收。
