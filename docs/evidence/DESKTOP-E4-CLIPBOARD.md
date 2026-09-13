# E4 自动剪贴板权限与原生监听证据

日期：2026-09-14

## 已实现边界

- task-history schema 9 按 peer、authorization generation、send/receive、text/link/image 保存 revision CAS grant；无记录或代际不匹配即关闭。
- 启用 grant 前重验 peer 授权；本机阻止、成员移除或 authorization generation 变化删除该 peer 的 grant。
- 桌面只在至少一个 grant enabled 时启动 watcher。Windows 使用 `AddClipboardFormatListener` 与 `SetWindowSubclass`；macOS 使用 `NSPasteboard.changeCount`。回调只报告 sequence 和格式类别。
- 睡眠、锁屏、撤销、全部 grant 关闭和 Shutdown 停止 watcher 并清空变化基线；恢复后从当前系统 sequence 开始，不补发停用期间变化。
- 启用与撤销共享 grant 提交门闩；watcher start/stop 由单一 owner 串行。睡眠、锁屏、用户暂停分别记账，wake 不会解除仍存在的锁屏或用户暂停。Windows 容量1队列淘汰旧变化并保留最新 sequence，注册基线与迟到旧通知不产生事件。

## 实际验证

- Windows amd64：桌面独立模块 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 通过；隐藏原生窗口实际安装 subclass/listener，并用 `WM_CLIPBOARDUPDATE` 验证非阻塞通知和停止。
- macOS arm64（alias `mac-test-102342413`，macOS 26.5，Go 1.26.4）：独立命名 pasteboard 实际改变 `changeCount`，watcher 收到 text 类型且不修改 general pasteboard；desktop/nativeclipboard test、desktop test/vet/build 通过。保留作业：`/tmp/codex-ssh/linksend-e4-clipboard-watch-mac-r4-20260913T192325Z`。
- macOS 链接器继续报告 SDK 26 object 与 deployment target 13/11 的既有 warning；命令退出码为 0。

## 未完成

本候选不读取或发送自动剪贴板正文。双端 grant 交集、短期 lease、事件 freshness、origin/application generation、接收端写入和文件/剪贴板公平调度仍属于 E4-03，不能把 watcher 触发当作跨设备同步通过。
