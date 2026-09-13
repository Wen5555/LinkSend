# E4-03 自动剪贴板行为证据

日期：2026-09-14

候选分支：`codex/desktop-experience-upgrade`

实现提交：`fb599c9bc1dde8f4f6b86b38eddf00e10401e653`；Windows clipboard generation 收尾修复：`4725c40`。

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

- Windows 普通本机测试不会修改用户剪贴板。workflow 仅在隔离 runner 设置 `LINKSEND_TEST_CLIPBOARD_WRITE=1`，用真实测试 HWND 写入并读回文字，同时验证过期写入不改变剪贴板。`fb599c9` 的 push/PR desktop 首轮都真实复现：写入函数在 defer `CloseClipboard` 前过早返回 sequence，关闭后读回 CAS 报 `CLIPBOARD_CHANGED_DURING_CAPTURE`；`4725c40` 改为显式关闭后读取 settled generation。随后 push run `34785043704` 与 PR run `34785046227` 的完整 desktop workflow 均 PASS，证明隔离原生写读与过期拒写通过，没有以重跑掩盖首轮失败。
- macOS arm64 最终源码补丁作业 `/tmp/codex-ssh/linksend-e4-03-final-mac-r3-20260913T213854Z` 在 alias `mac-test-102342413`（user `wen`、host `10.234.212.116`）退出 0：`internal/clipboardsync` race、无文件 QUIC/文件取消/后台收件 race、desktop/nativeclipboard race、desktop vet/build 全部通过。保留既有 SDK 26 object 与 deployment target 13/11 linker warning。首次后台作业 `/tmp/codex-ssh/linksend-e4-03-final-mac-20260913T213620Z` 因 manager 的 zsh `status` 只读变量失败，第二次因非登录 PATH 找不到 `git` 退出 127；两次都未记为产品失败或通过，修正 PATH 后的 r3 才是最终结果。
- 这些隔离测试没有操作两台物理设备的 general clipboard，不能记为物理 Win↔Mac 用户剪贴板通过。

## E4-03 集中修复候选

总控复审后，本候选补齐以下边界：真实系统变化在读取前推进唯一owner，即使无lease、无send权限或格式不支持也不回灌；发送改为异步两slot、64 KiB复核与节流，lease deadline同时约束QUIC写，新复制、暂停和撤权淘汰旧worker；会话协商增加独立`clipboard_sync`，两端本地authorization generation按方向使用而不比较数值相等；lease/event绑定permission revision，origin必须等于认证peer；PNG验证移出最终state锁，mutation前在grant→state锁序中复核授权、暂停、deadline和OS generation；macOS普通文字在URL解析失败后回退text，并识别明确禁止同步标记；设置新增持久且默认关闭的总开关、说明、暂停原因和ready/connecting/unsupported/error状态。

Windows本机实际通过根模块完整`go test ./... -count=1`、`go vet ./...`、根`GOWORK=off go test ./... -count=1`，以及定向`go test -race ./internal/clipboardsync ./internal/app -run 'Clipboard|SessionReuseRequiresBilateralCapability' -count=1 -timeout 240s`。桌面独立模块`GOWORK=off go test -race ./... -count=1`、vet、build通过；前端typecheck、lint、66项测试和production build通过。Node 24.19.0/pnpm 11.19.0仍低于仓库声明的Node 24.21.0，仅产生既有engine warning。

真实loopback Pion ICE + quic-go用例把A端本地generation保持1、B端对A的本地generation改为29，验证lease和event按方向通过且双向文字仍能写入；旧`session_reuse`而无`clipboard_sync`的envelope被判为unsupported。持续20次新复制期间文件任务完成；接收方占满两路receive slot后，32 MiB剪贴板正文在真实QUIC流上阻塞，发送方Shutdown在3秒上下文内关闭连接并等待worker退出。定向负例还覆盖origin冒充、permission revision篡改、关闭/重开后的旧lease和正文、解码验证期间并发撤权、无lease/unsupported watermark、latest-only取消和发送分块/节流预算。

macOS alias `mac-test-102342413` 的最终源码作业`/tmp/codex-ssh/linksend-e4-fix-mac-final-20260913T232901Z`使用Go 1.27.1/darwin arm64，根clipboardsync、定向app、desktop nativeclipboard race、完整desktop test和vet全部退出0。命名pasteboard测试实际覆盖普通文字不误当link、有效URL、PNG像素读写和`org.nspasteboard.ConcealedType`拒读，未触碰general clipboard。保留既有SDK 26 object与deployment target 13/11 linker warning。前一作业`/tmp/codex-ssh/linksend-e4-fix-mac-20260913T231151Z`在测试前因Windows补丁换行不匹配于`git apply --check`退出1；改用临时Git index打包tracked工作树后才得到上述通过结果，未把首次工具输入失败记为产品失败。

## 尚未完成

- `4725c40` 的 core push `34785043702`、core PR `34785046240`、desktop push `34785043704`、desktop PR `34785046227` 全部 PASS；packages run `34785043707` 的 Windows amd64、macOS arm64、macOS amd64 三个 job 全部 PASS。workflow 仅有 GitHub Actions Node 20 强制运行于 Node 24 的弃用提示，不是产品测试失败。
- 准确安装包中的锁屏、睡眠、连续快速复制、32 MiB 图片峰值内存、物理 Win↔Mac 双向和网络切换矩阵，统一留 E5。
- Apple Developer ID、App Group 正式签名、Windows package identity、证书信任和签名安装按用户要求暂缓。
