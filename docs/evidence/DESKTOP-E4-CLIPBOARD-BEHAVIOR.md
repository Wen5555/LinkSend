# E4-03 自动剪贴板行为证据

日期：2026-09-14

候选分支：`codex/desktop-experience-upgrade`

基线提交：`422d96d` 加本文件所述未提交 E4-03 候选；最终提交 SHA 与 GitHub CI 在提交后补记。

## 实现边界

- `internal/clipboardsync` 实现接收方预签 10 秒 lease、`LSCB01` 有界 wire、origin sequence/Lamport、稳定顺序、OS generation/revision gate、一次回环抑制和 text/link 64 KiB、image 32 MiB 上限。
- 自动正文只走已固定身份的 TLS 1.3 QUIC 单向流；信令、JavaScript IPC、HTTP 和第三方 relay 不承载正文。文件继续使用独立双向流，单个剪贴板流 reset/过期不结束文件连接。
- 无文件任务时，授权 peer 可主动建立认证复用会话。接收方 lease 与双方本机 send/receive、text/link/image grant 取交集；授权默认全关。桌面设备设置保留六个独立开关，托盘暂停与文件队列暂停分离。
- Windows 原生写使用有效本应用 HWND、generation CAS 和 deadline 后执行 `EmptyClipboard`/`SetClipboardData`；macOS 使用 `NSPasteboard.changeCount` 并在 mutation 前复核 deadline。macOS API 不提供跨进程原子 CAS，检查与写入之间仍有窄竞态。
- 接收正文/解码全局最多两路；图片正文驻留内存，不落地 staging。最坏内存还包含两个 PNG 的解码对象，需在 E5 准确包矩阵测量。

## 本机自动验证

以下命令在 Windows PowerShell 7、Windows amd64、Go 1.27.1 下实际退出 0：

```powershell
go test ./internal/clipboardsync -count=3
go test -race ./internal/clipboardsync -count=3
go test -race ./internal/app -run '^TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists$' -count=2
go test ./internal/app -run 'TestPairingCodePersistentInboxAndAlwaysAccept|TestCancelledStreamKeepsAuthenticatedSessionForNextFile|TestRejectedStreamKeepsAuthenticatedSessionForNextFile|TestRemoteCancelledStreamKeepsAuthenticatedSessionForNextFile|TestLegacyCapabilityUsesFreshSessionForImmediateNextFile' -count=3

cd apps/desktop
$env:GOWORK='off'
go test -race ./...
go vet ./...
go build ./...

cd frontend
pnpm test
pnpm run typecheck
pnpm run lint
pnpm run build
```

真实 Pion ICE + quic-go loopback 集成测试没有创建文件 Task，主动建立认证会话后验证 A→B 单向文字；随后增加反向 grant，在同一 session 验证 B→A。2 个默认 chunk 以上的文件正文被 hook 阻塞时，剪贴板仍在 3 秒内提交；释放 hook 后文件完成。暂停后 readiness 清零，3 秒空闲池最终回收。该结果是同机 loopback，不是 LAN、双 NAT 或物理 Win↔Mac。

前端共 65 项测试通过；新增设备授权测试验证首次无记录时六项全关，以及仅用 `{peer,direction,kind,expected_revision}` 开启一项。Node 实际为 24.19.0、pnpm 11.19.0，低于仓库声明的 Node 24.21.0，但上述 typecheck/lint/test/build 均退出 0；CI 使用仓库锁定环境再次判断。

## 平台原生证据

- Windows 普通本机测试不会修改用户剪贴板。workflow 仅在隔离 runner 设置 `LINKSEND_TEST_CLIPBOARD_WRITE=1`，用真实测试 HWND 写入并读回文字，同时验证过期写入不改变剪贴板；本候选提交后的 CI 结果待补。
- macOS arm64 最终源码补丁作业 `/tmp/codex-ssh/linksend-e4-03-final-mac-r3-20260913T213854Z` 在 alias `mac-test-102342413`（user `wen`、host `10.234.212.116`）退出 0：`internal/clipboardsync` race、无文件 QUIC/文件取消/后台收件 race、desktop/nativeclipboard race、desktop vet/build 全部通过。保留既有 SDK 26 object 与 deployment target 13/11 linker warning。首次后台作业 `/tmp/codex-ssh/linksend-e4-03-final-mac-20260913T213620Z` 因 manager 的 zsh `status` 只读变量失败，第二次因非登录 PATH 找不到 `git` 退出 127；两次都未记为产品失败或通过，修正 PATH 后的 r3 才是最终结果。
- 这些隔离测试没有操作两台物理设备的 general clipboard，不能记为物理 Win↔Mac 用户剪贴板通过。

## 尚未完成

- 最终提交 SHA、GitHub core/desktop/packages CI 和 Windows 隔离原生写回执。
- 准确安装包中的锁屏、睡眠、连续快速复制、32 MiB 图片峰值内存、物理 Win↔Mac 双向和网络切换矩阵，统一留 E5。
- Apple Developer ID、App Group 正式签名、Windows package identity、证书信任和签名安装按用户要求暂缓。
