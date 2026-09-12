# M5 本地内容快照与原生图片读取底座

更新：2026-09-12。基于 `2859e6d` 的独立分支 `codex/desktop-content-snapshots`。
本记录仅证明内容存储和平台 adapter；尚未接入 App、绑定、内容协议或最终接收界面，不能据此把 S04 标成完成。

## 已实现行为

- `internal/content` 在 profile 的 `content-snapshots` 下保存文字、URL 和规范化 PNG。返回的 `Snapshot` 只有 ID、类型、大小、BLAKE3、媒体类型、尺寸、创建时间；不含路径、正文、Base64 或图片预览。
- 正文先以独占创建方式写入、同步，然后登记 schema 1 元数据。元数据记录随机 store owner、随机 snapshot ID、正文的真实 OS file identity 及引用集合。`OwnedPath` 重新核对文件身份、长度和摘要；仅相同哈希的外来替换文件不会被当成本应用所有。
- Store 使用 OS 文件锁防止两个写入者。引用更新先同步临时文件，再原子替换元数据；同一引用重复 Retain/Release 幂等。已完整写入但尚未 rename 的本对象临时记录可丢弃并继续；损坏或外来的临时记录保持不动并报错，不猜测恢复。
- `Cleanup` 只删除零引用、所有权和正文校验通过、且目录内只有本对象文件的快照。无递归删除；未知文件、被修改的正文、外来同内容文件、坏元数据、symlink 均保留。活跃、暂停和可恢复任务必须继续持有引用。
- `apps/desktop/nativeclipboard` 的 Windows 实现使用同一 OS 线程上的 `OpenClipboard`/`CloseClipboard`，读取注册 PNG、CF_DIBV5 或 CF_DIB，并立即复制到 Go 私有内存。支持 24/32 位 RGB、常规 bitfields、透明度和 top-down/bottom-up；不支持的压缩、调色板或嵌入 profile 明确拒绝。
- macOS 实现使用 NSPasteboard + ImageIO，支持 PNG/TIFF，在解码前检查 ImageIO 尺寸，并使用有界 CGDataConsumer 编码 PNG。剪贴板变化返回明确错误。Linux desktop 当前返回不支持，不能伪称通用 Wails 图片 API。
- 两个平台都只读用户主动请求的当前图片。图片字节和像素不会进入 JavaScript IPC；通知是否成功与读取结果分开处理。

## 集成 API 与引用顺序

```go
store, err := content.OpenStore(profileDirectory)
snapshot, err := store.CreateText(ctx, content.Text, userText, "draft:<id>")
snapshot, err := nativeclipboard.CaptureImageSnapshot(ctx, store, "draft:<id>")
err = store.Retain(snapshot.ID, "task:<logical-task-id>")
path, err := store.OwnedPath(ctx, snapshot.ID) // 仅交给 Go sender
err = store.Release(snapshot.ID, "draft:<id>")
removedIDs, err := store.Cleanup(ctx)
```

创建即要求 owner reference。先 Retain，再持久化草稿/队列对快照的引用；先持久化移除草稿/任务引用，再 Release。数据库操作失败应释放本次新加引用；不确定结果先查该事务是否完成。进程中断最多留下额外引用，不能让清理先于有效队列任务。暂停和应用重启恢复沿用逻辑 task reference，不因为 attempt 改变就释放。

历史正文保留开关仍由 App 层落实。默认不保留正文时，只有在任务不再活跃、暂停或可恢复且草稿也已移除后才 Release；保留选项需要独立历史引用。仅将隐藏目录名作为垃圾判断不符合此 API 的所有权约定。

`ValidateURL` 只允许完整 http/https URL，拒绝凭据、控制字符、前后空白及其他 scheme；此函数从不打开链接。接收端应把其他 scheme 当作文字，打开动作必须由用户主动触发。

## 限额与内存实测

| 项目 | 限额/实际结果 |
| --- | --- |
| 文字、URL | 非空、合法 UTF-8、无 NUL，最多 64 KiB |
| 编码图片输入与 PNG 输出 | 分别最多 32 MiB；输出使用流式限额，不能先无界编码再检查 |
| 解码图像 | 最多 40,000,000 像素，单边最多 32,768 |
| 单边额外限制原因 | 避免极长单行图片把 PNG 行缓冲放大；40 MP 限制单独不足以约束该开销 |
| 最坏 PNG 像素深度 | 标准库 16 位 RGBA 可能使用每像素 8 字节，不声称固定 160 MB 上界 |
| Windows 40 MP / 16 位 PNG 实测 | 编码 559,320 bytes；解码总分配 321,550,296 bytes；heap 增量 320,095,152 bytes，低于此次 400 MiB 分配测试预算 |
| 实际输出限额测试 | 9 MP 随机 RGBA PNG 达到 32 MiB 时拒绝，临时正文已清理 |

内存数字来自本机单次独立测试的 Go `runtime.MemStats`，不是全进程 RSS，也不包含 macOS ImageIO 的原生分配。平台内存仍需真实环境分别测量。

## 已执行验证

工作目录为独立 worktree，Windows PowerShell 7、Go 1.27.1。

```powershell
$env:GOWORK = 'off'
go test -race ./internal/content -count=1
go vet ./internal/content
# apps/desktop 中：
go test -race ./nativeclipboard -count=1
go vet ./nativeclipboard
# 根目录中，独立执行较大内存用例：
$env:LINKSEND_CONTENT_MEMORY_TEST = '1'
go test ./internal/content -run 'TestSnapshotMaximumImageMemory|TestSnapshotImageEncoderReal32MiBLimit' -count=1 -v
```

上述针对性普通/race/vet 和两项真实内存/编码限额测试 PASS。覆盖有效中文/多行、空与非法 UTF-8、危险 URL、编码和像素超限、剪贴板来源随后变化不改变快照、重启引用、并发 Retain、持久化失败保留引用、所有权替换、symlink、未知用户文件、坏临时记录保留及原生 Windows GlobalLock/RtlMoveMemory 复制。

初轮 desktop vet 发现 `uintptr` 转 `unsafe.Pointer`；已改用原生 RtlMoveMemory 直接复制至有界 Go 缓冲，再通过 vet 和真实原生内存复制测试。两次 gofmt 命令因相对工作目录错误未执行，随后从根目录重新格式化并验证；未把命令错误当作通过。

Windows 原生剪贴板只读探测实际结果为 **SKIP：当前剪贴板无受支持图片**。没有为了测试覆写用户剪贴板，该结果不代表真实截图捕获通过。

macOS 的真实 PNG/TIFF 验收已 PASS。`mac-test-102342413`（macOS 26.5 / arm64 / Go 1.27.1 / `GOWORK=off` / minimum macOS 13）执行根 `go test -race -count=1 ./internal/content`，以及 desktop `go test -count=1 ./nativeclipboard`、`go vet ./nativeclipboard`，全部退出 0。

`scripts/test-content-clipboard-macos.swift` 创建命名板 `com.linksend.native-test.1B50AAF7-E1AF-4E80-8D5F-16CF6E1F7125`；PNG 158 bytes 与 TIFF 226 bytes 分别由 `TestDarwinNamedPasteboardSnapshot` 调用相同 NSPasteboard/ImageIO 读取/存储路径。两次均验证 2×2、RGB 180/60/30、alpha 255、owned snapshot、Release 和 Cleanup，最后成功 clear 命名板。全过程未触碰用户 general clipboard；这证明原生格式读取路径，不代替完整 Wails UI 的截图按钮验收。

源码归档 SHA256 `5f50b99f301dacdf408d6eb9e1054113168e7b9b0c7d6398e8c61374edf409be`；远程 job `/tmp/codex-ssh/desktop-m5-mac-content-native-20260912T062416Z` 退出 0。完整命令、stdout/stderr 与结果已下载至 `.artifacts/desktop-six-features/m5-mac-native/m5-native-content-evidence.tar.gz`，SHA256 `09762e5bdcef39760536111da1170d62d644ac95fc93094c394edf6d8d706750`，展开证据位于同目录 `evidence/`。

## 尚待集成与验收

- App 层的草稿/任务引用事务、历史正文保留开关、错误状态和清理入口。
- M4 合并后的原生内容能力协商、类型与摘要绑定、旧端明确的普通文件降级选择。
- 接收文字复制、HTTP(S) 手动打开、图片保存，以及真实 Windows 图片剪贴板和完整双平台 UI 流程。
- 此独立包没有改动 `main`、App、既有 bindings 或协议，也没有自动推送或发布不完整的 S04；里程碑发布由主任务完成集成、验证和文档后统一执行。
