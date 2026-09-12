# M3 Mac 后台冷启动修复与实机证据

2026-09-12，实体 Mac `wen@10.234.184.6`，SSH alias `mac-test-102342413`；macOS 26.5 / Apple arm64，Go1.27.1、Wails3 beta.18。所有连接/上传/作业均经 codex-ssh-manager。没有修改 TCC，也没有替换或退出用户原有 `/Volumes/LinkSend 3/LinkSend.app`（PID 47950）。

## 可复现产品问题

精确源码归档 `408287c7f494a24d45cbf6bc33a7fa279a21c3b3`，SHA256 `6a72f7628604049c1afda4477b223434072dc08db2d122164987aa8e41a3efb9`，完成真实 ARM64 App/DMG 构建。新 profile 预置 `background.close_mode=background` 后用真实 EXE 启动 `--background`：日志到达 runtime ready，进程在约 1 秒内自行 exit 0，后台模式 FAIL。

初始探针曾在 NSRunningApplication 注册前查询，脚本失败不能算产品失败；该日志仍保留。后续探针用 `proc_pidpath` 独立核对 PID/路径、Popen.poll 确认进程退出，最终负例为 PID 63326：`m3-408287c/m3-final-evidence/native-r3/native-results.json`。

## 最小改动

只改变 `apps/desktop/main.go`：

- 关闭 AppKit 的 `ApplicationShouldTerminateAfterLastWindowClosed`；既有 `onWindowClosing` 和 `shouldQuit` 继续决定后台、保存和退出。
- 在创建 WebviewWindow 时根据明确 `--background`、无文件参数和已保存后台策略设置 `Hidden`，避免 runtime-ready 的 Hide 与后续原生启动显示竞争。
- 若启动后没有托盘或后台策略不再有效，显式显示窗口；带文件参数的冷启动仍显示草稿。

仅设自动退出为 false 的第一版保留了进程，但初始显示又让窗口可见，故该版仍记 FAIL。最终增加创建期 Hidden 后才通过完整后台启动/唤回/退出。未修改 Wails、系统策略、关闭 hook 或后台默认偏好。

## 修复版原生结果

验证快照为 `a6aa1f6`（已含通知修复）加本次 main.go 改动；main.go SHA256：`077c764ffc1016f7650ba48ebe931868393839540aa48c7cbcde7ba4cc005636`。它是明确记录的补丁验证快照，不冒充 CI tag 构建。

| 检查 | 实际结果 |
|---|---|
| 新 profile `--background` | PASS；PID 64717 存活，真实屏幕窗口列表为空 |
| 同 profile 第二次启动 | PASS；第二进程 exit 0，原 PID 64717 唤回真实 1008×684 窗口（window ID 8007） |
| 唤回后明确退出 | PASS；NSRunningApplication terminate 请求被接受，主进程 exit 0 |
| 用户旧 App | PASS；PID 47950 保持运行 |
| 红点关闭、Finder Services 菜单点击 | BLOCKED_AX_PERMISSION；AXIsProcessTrusted=false，未请求授权 |
| 原生窗口截图 | BLOCKED_SCREEN_RECORDING_PERMISSION；CGPreflightScreenCaptureAccess=false，未请求授权 |

原生作业：`/tmp/codex-ssh/desktop-m3-native-bgfix-v2-20260912T074819Z`，exit 0。证据目录 `/Users/wen/linksend-desktop-six/m3-bg-fix-a6-v2/m3-final-evidence/`；原生结果 `native-r3/native-results.json`。

修复快照 EXE SHA256：`c626f9b22b49900dfcbfd961749ddf8711c1065b5c1be292e876fc08a19f74a4`。
DMG SHA256：`4253954e70185ccadef5978282b510be1b649581947096127f07d4fc5516ce9f`。
真实 `scripts/build-macos.sh --arch arm64 --format dmg` 包含挂载、bundle ID、架构和 ad-hoc 签名核验，已通过；没有 Developer ID 签名或公证声明。

## 测试范围与保留失败

Mac 修复快照：桌面独立 `go test -race -count=1 ./...`、`go vet ./...`、前端 offline frozen install/build 和 ARM64 DMG 构建均通过。Windows 本工作树桌面 `GOWORK=off go test/go vet/go build ./...` 通过。

精确 408287c 的根 workspace/独立普通 test、vet/build、前端所有检查和 DMG 均通过；首次根完整 race 出现 `TestQueueSourceChangeRequiresNewPreparationAndExplicitContinue` 15 秒超时，无 DATA RACE 报告。原样定向 race 20 次通过（5.517 s），原样完整 race `-count=2 ./...` 随后通过（App 26.690 s）；首轮 FAIL 保留，未声称定位或修复其根因。

本地 a6aa1f6+本次 main.go 的根普通测试另出现既有 `TestPairingCodePersistentInboxAndAlwaysAccept` 第二次 always-accept 超时。核心文件未改动，该次仍记 FAIL；不能用 desktop PASS 覆盖根网络回归失败。当前主分支后续 M4/M6 核心修复的结果应单列。

408287c 真实主应用的冷启动中文文件参数、第二实例相对路径转交至同一 Go/SQLite 草稿，以及明确空闲退出已通过（`m3-408287c/m3-final-evidence/native-r2/`）；这些证据与修复版后台行为分别标注来源。最终发布仍须在合并后的精确候选上进行 CI 与包来源核验。本子任务未部署、推送或发布。
