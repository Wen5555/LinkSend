# E3 Windows Share Target 源码候选

日期：2026-09-14。范围 E3-01；当前为未签名源码候选，真实系统安装/激活为 **NOT RUN**。

`apps/desktop/native-share/windows` 是正式 `windows.shareTarget` 适配器，不复用 E0 fixture-copy 原型。它从 `ShareTargetActivatedEventArgs.ShareOperation` 读取 `StorageItems`，展示 Go 发布的真实已配对设备别名；选择后逐项用 WinRT 打开授权、拒绝文件夹及无持久绝对路径的临时/云端项目，再把路径元数据写入当前用户 LinkSend profile。它不复制文件正文，不打开身份、SQLite 或网络连接。

本机交接记录为 schema 2，固定字段 `request_id/peer_id/paths/wait_for_peer/source`。请求文件名绑定 32 位小写十六进制 request ID；同内容重试幂等，不同内容冲突。记录刷盘并原子改名后才 `ReportDataRetrieved/ReportCompleted`，随后用 `--native-share-background` 唤起 Wails owner；Go 仍重新验证设备授权并写入现有队列。

实际运行：

- `dotnet build -c Release`：PASS，0 warning/0 error。
- `dotnet run -c Release -- --self-test`：`PASS share_target_journal`，覆盖真实文件写入、设备快照读取、同请求重放与冲突。
- Taskfile 等价的 self-contained `dotnet publish -r win-x64 -p:PublishSingleFile=true`：PASS；未签名适配器 SHA256 `152396e89ba63c9b4ab441cf740fb21d6ea0dc88736a53ba1640fb6b73cbe241`，仅作源码构建证据。
- desktop `go test ./...`、`go vet ./...`、`go build ./...`：PASS；包含 schema 2 journal、损坏保留、容量、并发和设备快照测试。

`AppxManifest.xml` 使用产品身份 `LinkSend.Desktop`、宿主 `LinkSend.exe` 与独立 `LinkSend.ShareTarget.exe`，支持任意文件类型的 `StorageItems`。用户明确没有 Windows 签名证书并暂缓签名，因此 identity package 构建签名、安装、Explorer/系统共享激活、升级和卸载均为 NOT RUN；E0 自签失败不复用为本轮结果。

## E3-C1 审查修复

设备快照现在由 Go 后台每 15 秒发布，Share Target 按 60 秒 TTL 将陈旧可达状态降级为离线。列表加载和目标选择不会入队，离线等待必须另行勾选，并提供“添加设备”入口。普通持久本地文件保持零复制；没有稳定本地路径的临时/虚拟 StorageFile 才复制到本请求独占的 share-owned-v1 目录。

schema 2 journal 入队后继续保留，进程重启会重新验证来源并幂等重放；只有 queue 为 completed/cancelled/expired 时才删除 journal 和 owned 临时源。单个坏来源或撤销目标保留为可见错误，不再阻塞后续请求。后台 share 唤起不显示主窗。NSIS 将 Share Target 放入安装器独占子目录，卸载只递归删除该目录。

## E3-C2 审查修复

Windows 安装器恢复同一 package root，manifest、宿主和 Share Target 路径一致；卸载显式删除安装器拥有的 Share Target 文件和 Logo。在线设备按钮直接提交，离线目标仍需勾选确认；错误不会调用 ReportError 锁死面板。临时目录/Temporary 属性或无稳定路径的 StorageFile 强制复制，累计上限 16 GiB，复制后校验长度并 Flush(true)，入队前失败会清理 owned 目录。

Go 对清理失败保留 journal，主界面列出未入队请求并提供明确放弃入口。Mac 使用 NSWorkspace.openApplication(arguments: --native-share-background, activates: false) 覆盖冷启动，解析带小数秒的 RFC3339Nano；provider 调用并发上限 4，按实际复制 chunk 扣 16 GiB 总预算。定向 Mac 作业 /tmp/codex-ssh/linksend-e3-c2-mac-swift-20260913T171611Z 通过。
