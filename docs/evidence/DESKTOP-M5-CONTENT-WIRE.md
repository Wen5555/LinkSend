# M5 内容能力协议与兼容性验证

2026-09-12，在 `codex/desktop-content-wire` 独立 worktree、基线 `3feb03c316aac8ce71b52729f9ee940621541e47` 上实现。范围为 `internal/transfer` 的内容元数据、协商、完成与恢复绑定；不改 App/main、core tasks、队列、前端或平台适配，不将本记录称为完整 M5 用户流程验收。

## Go 接口

- `NewContentDescriptor(manifest, content.Snapshot)`：检查真实单文件 manifest 与快照 BLAKE3/大小一致。元数据含 Version/EntryID/Kind/MediaType/Size/Digest/Width/Height，无正文或本地路径。
- `SendWithOptions(ctx, stream, prepared, SendOptions{Hooks, Content, AllowFileFallback})`：现有 `Send`/`SendWithHooks` 保留普通文件语义；降级开关默认 false，由调用者持久化用户的明确决定。
- `ReceiveOptions.AcceptNativeContent=true` 加 `Plan(ctx, Offer)` 显式启用接收内容能力，Plan 接收新增 `Offer.Content` 和 `ContentDigest` 副本；可继续使用 M4 原计划、拒绝、全选/全跳过。该开关默认 false，旧式 Accept 或已有 M4 Plan 回调没有隐式内容授权；显式允许文件降级时才可按普通文件接受。
- 成功 `Result.Content/ContentDigest` 是双方实际确认的类型与摘要；`Result.FileFallback=true` 表示实际普通文件降级。`NoContent` 仍无正文，应用不得因存在 Content 元数据触发内容动作。

三个摘要含义明确分开：`Manifest.Digest()` 仍是旧原始 manifest；`ContentDescriptor.Digest` 与 `Snapshot.Digest`、`FileEntry.Hash` 同为正文 BLAKE3-256；`ContentDescriptor.BindingDigest(manifest)` 为域分隔的内容解释与原 manifest 绑定。没有把 SHA256 产物校验值冒充正文 BLAKE3。规范字段与限额见 [PROTOCOL.md](../PROTOCOL.md)。

## 实际验证

Windows PowerShell 7 显式使用 `D:/apps/Osend/.tools/go1.27.1/go/bin/go.exe`，所有 Go 命令设置 `GOWORK=off`、`GOTOOLCHAIN=local`。以下命令实际退出 0：

| 命令 | 结果 |
| --- | --- |
| `go test -count=1 -run TestContent ./internal/transfer` | 新内容测试全部通过 |
| `go test -race -count=1 ./internal/transfer ./internal/content` | 最终显式 opt-in 版本 PASS：transfer 8.169s，content 1.993s |
| 根 `go vet ./...` | PASS |
| 根 `go test -count=1 ./...`、`go mod verify`、`go build ./...` | 全部 PASS，包括原 transfer/真实 QUIC/集成套件 |
| desktop `go test -count=1 ./...`、`go vet ./...`、`go build ./...`、`go mod verify` | 全部 PASS；无新桌面依赖 |

测试覆盖：真实 snapshot → Prepare 的 BLAKE3 一致性；六组真实 UDP loopback / pinned TLS 1.3 / QUIC 的文字、URL、PNG 和各自全跳过；正文逐字节、子集与两种终态摘要一致；回调不能改写协商描述符；旧 frozen JSON receiver 默认 0 B 停止及显式完整文件降级；原 M4 frozen legacy sender/receiver 普通文件双向测试继续通过。

负例覆盖：未知 kind、错误内容摘要/大小、非法 MIME/尺寸/单文件限制、缺内容能力、拒绝/旧式回调无内容授权；逐个篡改 accept、finish、completed、confirmed、confirmed_ack 的内容摘要；用正确文件哈希发送非法 UTF-8/NUL、PNG 尺寸不符、PNG 截断，均在提交前失败；未知 URL scheme 正文保存为不透明文本且仍不通过 `ValidateURL`；源快照变化时连 offer 都不发出；内容恢复复用真实已验证块、0 B 重传；同字节换 kind/转普通文件/旧 checkpoint 增加类型均失败且原 checkpoint 不变。

只读并行审查还指出：已有 M4 Plan 回调可能忽略新增内容字段。因此最终接口增加默认关闭的 `AcceptNativeContent`，测试现有 M4 Plan 未 opt-in 时默认停止及显式文件降级；最终 Windows 普通/race、根完整 vet/build、desktop test/vet/build 均重新通过。此前第一轮检查没有被用来替代这次复核。

## 物理 Mac 上的独立协议检查

通过 SSH manager alias `mac-test-102342413`，在 macOS 26.5 / arm64、Go 1.27.1 独立目录运行；`GOWORK=off`、`GOTOOLCHAIN=local`、CGo minimum 13.0。初版源码 SHA256 `6514af403e9530f1ab9f3d665c313b69070d94f6a39f583e3cac8ee99ff23b36` 的 race/vet/verify 已通过，job `/tmp/codex-ssh/desktop-m5-mac-content-wire-20260912T064033Z`。

加入显式 opt-in 后重新冻结并上传源码，最终 archive SHA256 `63368a07c97ad5893a618182c9d13479c64fd2d2ade20207522461848c2176b4`，远端校验一致。job `/tmp/codex-ssh/desktop-m5-mac-wire-final-20260912T064524Z` 退出 0：`go test -race -count=1 ./internal/transfer ./internal/content` 分别 5.177s / 1.887s，`go vet ./internal/transfer ./internal/content` 与 `go mod verify` 均通过。这里也运行真实 loopback QUIC 内容测试，不表示图形界面原生动作验收。

日志、实际脚本与退出码归档 `m5-wire-final-evidence.tar.gz` SHA256 `1a31222ff03bf1e6586e76bfdc9134e372448739f0b665d4523929286d9a2ee5`；manager 下载后再次一致，位于 `D:/apps/Osend/.artifacts/desktop-six-features/m5-wire-worktree/.artifacts/`，已展开 evidence。测试只使用独立临时目录，不改用户应用、profile、剪贴板或网络配置。

## 范围限制

上述 QUIC 使用两个真实 loopback UDP socket 和生产固定公钥 TLS 验证，不能外推为实体双机 LAN 或跨 NAT。内容协议没有新建信令正文通道、JavaScript 图片通道或中继。复制文字、打开 http/https、保存图片等最终动作仍由 App/UI 明确绑定，原生菜单/剪贴板/后台交互与发布包由后续集成验收记录覆盖。
