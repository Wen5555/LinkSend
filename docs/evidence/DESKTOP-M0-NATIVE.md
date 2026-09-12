# M0 桌面原生能力与发布环境审计

审计时间：2026-09-12；基线 `main` / `fbfc250159ea65dbb5840d6dadb03fe4423cf868`。主体记录实现前的只读能力核实，不是 S01–S06 原生验收结果。用户本轮明确允许后续自动推送、部署和发布；随后另行给出 Mac 新地址并授权安装工具、测试，后续环境准备记录见末节。本记录没有执行推送、产品部署、发布或主机网络变更。

## 固定源码与实际 API

本地源码基准为 `github.com/wailsapp/wails/v3@v3.0.0-beta.18`，位于 Go module cache。以下路径均相对于该 module；与项目独立模块 `apps/desktop/go.mod` 固定版本一致。不得把这些 API 套用到 Wails 2。

| 能力 | 已核对的固定版本源码 | 可用 API 与实现约束 |
| --- | --- | --- |
| 同 profile 单实例 | `pkg/application/single_instance.go:22`，`application.go:207`，`single_instance_windows.go`，`single_instance_darwin.go` | `application.Options.SingleInstance` 接受 `*SingleInstanceOptions`。字段为 `UniqueID`、`OnSecondInstanceLaunch func(SecondInstanceData)`、`AdditionalData`、`ExitCode`、`EncryptionKey [32]byte`。`SecondInstanceData` 有 `Args []string`、`WorkingDir string`、`AdditionalData map[string]string`。Args 来自完整 `os.Args`，包括 argv[0]；WorkingDir 来自第二进程 `os.Getwd()`。 |
| 窗口拖放 | `pkg/application/webview_window_options.go:186`、`webview_window.go:1625`、`context_window_event.go:15` | `WebviewWindowOptions.EnableFileDrop=true`，前端真实接收区使用 `data-file-drop-target`；Go 用 `window.OnWindowEvent(events.Common.WindowFilesDropped, callback)`，从 `event.Context().DroppedFiles()` 取路径。只传路径及受限元数据，不能读 WebView File API 的正文。 |
| 托盘/菜单栏 | `pkg/application/system_tray_manager.go`、`systemtray.go:231` | `host.SystemTray.New()`、`SetIcon`、`SetTemplateIcon`、`SetMenu`、`SetTooltip`、`OnClick`、`Show/Hide/Destroy`。现有方法没有承诺托盘图标拖入文件能力。 |
| 窗口关闭与唤回 | `pkg/application/webview_window.go:942`、`:968`、`pkg/events/defaults.go` | `RegisterHook(events.Common.WindowClosing, callback)` 是同步取消入口；调用 `event.Cancel()` 并隐藏窗口。唤回可用 `Show()`、`Focus()`。`OnWindowEvent` 与 `RegisterHook` 均返回取消订阅函数，需要在退出时调用。真正退出仍通过 `Options.ShouldQuit`、`host.Quit()` 和 `ServiceShutdown`。 |
| 原生通知 | `pkg/services/notifications/notifications.go`、`notifications_windows.go`、`notifications_darwin.go` | `notifications.New()`，服务生命周期 `ServiceStartup/ServiceShutdown`，`RequestNotificationAuthorization()`、`CheckNotificationAuthorization()`、`SendNotification(NotificationOptions)`、`OnNotificationResponse(callback)`。NotificationOptions 至少有 `ID/Title/Body/Data`；通知响应 `Response.ID/UserInfo` 可映射不透明 task ID。Windows 基于 `git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3`，新增 import 后须补齐 desktop module graph，不能误以为无新增传递依赖。 |
| 文本剪贴板 | `pkg/application/clipboard_manager.go`、`clipboard.go` | `host.Clipboard.Text() (string,bool)` 与 `SetText(string) bool`。没有图片或文件列表统一 API。图片必须由薄 Windows/macOS adapter 在原生/Go 层读取、限额解码和快照。 |
| 睡眠事件 | `pkg/events/events.go:13`、`application_darwin.go:63`、`application_darwin_delegate.m:40` | `events.Common.SystemWillSleep/SystemDidWake` 为真实已定义事件，Mac 使用 NSWorkspace 通知。应用仍需在 wake 后恢复自己的必要连接；不能以事件存在推定真实网络切换已通过。 |
| 防自动睡眠 | 对固定 module 的 Go/ObjC 源码定向查询 | 未发现 `SetThreadExecutionState`、`PowerCreateRequest`、`IOPMAssertion` 或统一 PreventSleep API。需桌面平台 adapter；Windows 可评估线程固定的 SetThreadExecutionState 或句柄式 Power Request，Mac 可评估 IOPMAssertion。后两者是待实现选择，尚未完成原生验证。 |
| Finder Services | 对固定 module 定向查询 `NSServices/setServicesProvider/NSUpdateDynamicServices` 无命中 | Wails 未封装该功能；需桌面模块 ObjC/CGo service provider 与 Info.plist 的 NSServices 声明。不能把普通文件打开事件、Share Extension 或菜单按钮当成已完成 Finder Services。 |

### 单实例的边界与最低接入方式

Wails 的 single-instance manager 在 `application.New` 内取得锁，ServiceStartup 在应用运行时执行。项目现有 `NewApp()` 仅创建 `&App{}`，不会打开数据库；保持此构造函数无副作用即可先设置 SingleInstance，再初始化真实 core。

UniqueID 应由规范化绝对 profile 路径的稳定摘要派生，而不是固定全局 `com.linksend.desktop`；不同 profile 应可并行，同一 profile 应共用写入者。原生激活回调可能早于 UI/core ready，需要有界积累并在真实草稿服务启动后消费；入口幂等与重复路径去重在 Go 完成。Windows 使用 named mutex 和消息窗口，macOS 使用 flock 与 distributed notification；这不是 core/CLI 的通用写锁，必须补上 core profile OS 锁。

单实例 Args 是元数据入口：明确跳过 argv[0]，解析已支持参数，用传入 WorkingDir 解析相对路径，保留 Unicode/空格，错误逐项可见。不同入口都只创建可见草稿。使用文件数/序列化字节限制，不能静默截断。Windows/macOS IPC 对二次激活没有应用级持久接收确认，应在原生验收中覆盖同 profile 连续激活及启动竞态；不得单凭类型存在称“不丢内容”。

### 生命周期与通知注意事项

当前 `apps/desktop/app.go:164` 的退出提示要求调用 `CancelTask` 取消全部未终态任务；这与六项能力要求的“退出保留恢复状态”不一致。应将隐藏、暂停派发、暂停当前传输和真正退出分开，并让真正退出停止接受新任务/派发、落盘和有界释放资源。

通知包 Windows `CheckNotificationAuthorization/RequestNotificationAuthorization` 都是返回 true 的 stub，无法证明系统通知开关或勿扰模式允许展示。Windows Startup 会写本用户 CLSID/AppUserModelId 相关注册表；必须限定到应用作用域，并在安装/卸载设计中记录。`UpdateNotification` 在 Windows 是再次投递，不是真正原位替换；task/revision 去重应由应用拥有。macOS 需要有效 .app bundle，授权拒绝不应使 core 启动失败。通知内容默认只有“新接收请求/任务已完成/任务失败”等摘要。

### Finder Services 一手资料

实际读取 Apple 官方归档 [Providing a Service](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/SysServices/Articles/providing.html)，HTTP 成功，保存原文到忽略目录 `.artifacts/desktop-m0-apple-services.html`（26,491 bytes）。它明确规定：

- provider 方法签名为 `messageName:(NSPasteboard *)pboard userData:(NSString *)userData error:(NSString **)error`。
- 在应用完成启动、能够立刻处理请求后，调用 NSApplication `setServicesProvider:`；每个应用只有一个 provider，不能抢占 Wails 的 application delegate。
- Info.plist 的 NSServices 中声明 `NSMessage`、`NSPortName`、`NSMenuItem`、`NSSendTypes` 等；文件输入需按目标 SDK 核实 file URL/pasteboard 类型。
- app 安装在 Applications 域，独立 `.service` 安装在 Library/Services；`NSUpdateDynamicServices()` 可请求刷新。
- `pbs` 只用于人工/调试检查，Apple 明确说明其接口不保证稳定，不能让正式产品依赖 `pbs`。

最低实施是 `.app` 内注册薄 service provider，在 ObjC 层读用户选中的文件 URL，交给既有 Go 草稿；读取系统临时授权 URL 时必须在队列接管前保留可靠访问权或快照。当前 Windows 主机无法编译或运行此 ObjC 路径，需 macOS CI 构建和物理 Finder 实测分别记录。

## 发布与安装现状

| 项目 | 审计发现 | 本轮实现/发布时应处理 |
| --- | --- | --- |
| Git | origin 为 `https://github.com/Wen5555/LinkSend.git`；基线为 main；工作树已有规划、`.playwright-cli/`、desktop build/linux 与 build/ios 未跟踪文件 | 保留现存文件，按实际提交记录，不将旧产物改标为新 SHA。 |
| core CI | `.github/workflows/core.yml`：独立 GOWORK=off mod verify/vet/test/build/race | 按核实后的版本更新 setup-go。 |
| desktop CI | `.github/workflows/desktop.yml`：Windows，bindings→前端检查→独立 desktop Go/build | 当前 PowerShell 多原生命令块没有逐条 `$LASTEXITCODE` 检查，较早失败可能被最后成功掩盖，需修正。 |
| 三平台包 | `.github/workflows/wails3-packages.yml`：Windows amd64、macOS arm64、macOS amd64，push/workflow_dispatch 触发 | 仅上传 Actions artifact，权限 contents:read；没有自动 Release、tag 或服务器部署任务。 |
| 版本工具 | 工作流目前 Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails beta.18，产品 0.4.0 | 本轮候选工具链须经过兼容探针再统一锁定；本审计不把规划中的目标当成已验证组合。 |
| Windows 安装 | 活动路径为 `build/windows/nsis/project.nsi`，不是历史 `build/windows/installer/`；CI `INSTALL_SCOPE=user`，默认 Taskfile scope 仍为 machine | SendTo 应进入当前用户 SendTo 目录，不改默认关联；注销只删除 LinkSend 创建的项。已有卸载 `RMDir /r $INSTDIR` 应限制应用拥有内容，避免删除用户放入安装目录的文件。 |
| macOS 包 | `scripts/build-macos.sh` 真实执行 bindings、DMG、挂载、bundle ID、lipo 与 strict ad-hoc codesign；Info.plist/CGO target 均为 macOS 12 | Vite target 当前未显式固定，升级必须明确最低 WKWebView/Windows WebView2。现有 codesign 为 ad-hoc，未公证，不能称 Developer ID 正式签名包。 |
| 构建来源 | Windows/macOS production Taskfile 使用 `-buildvcs=false`；工作流 BUILD-INFO 记录 source SHA/clean | 包来源以验证的工作流 SHA + BUILD-INFO + 包哈希说明，不能宣称二进制自带 go VCS 元数据。服务器部署仍需独立干净提交构建并验证 `vcs.modified=false`。 |
| 香港测试主站 | `docs/DEPLOY-HK.md`：`hk-main`，`/opt/linksend-lan-test`，systemd `linksend-rendezvous.service`；文档记录当前 0.4.0/schema 2 | 按 manager resolve/probe/audit + inspect→backup→change→verify→rollback-ready。纯客户端/文档提交无需为 SHA 外观重启服务；产品或服务端依赖改变再同步。此次没有联网复核香港健康，以上是仓库记录。 |

## 真实 Mac 环境探针

通过 `codex-ssh-manager` 读取和探测唯一目标 `mac-test-102342413`，没有 raw ssh、没有扫描其他目标，没有上传脚本或变更远端。

| 操作 | 时间 UTC | 实际结果 |
| --- | --- | --- |
| resolve | 2026-09-12T04:22:57Z | reusable key alias，用户 `wen`，当前 inventory 地址 `10.234.171.192:22` |
| probe 1 | 2026-09-12T04:23:23Z | `status=failed`，SSH `exitCode=255`，`connect-timeout` |
| probe 2 | 2026-09-12T04:24:01Z | 相同 `status=failed/exitCode=255/connect-timeout` |

注意 manager 命令本身返回 0 表示结构化调用完成；不能把它误读为 SSH 成功，实际连接结果在 `data.status/data.exitCode`。上述旧地址当时无法进入物理 Mac，没有生成远端 job/backup 路径。随后用户更新地址后的恢复情况见末节，不能将旧地址超时泛化为一直没有 Mac 环境。

## 实际审计命令与限制

PowerShell 7 使用：`git status --short`、`git branch --show-current`、`git rev-parse HEAD`、`git remote -v`、`go env GOMODCACHE`、定向 `rg`/`Get-Content` 读取以上固定源码与配置；CodeGraph 根目录不存在，没有自行建索引。`web-access/scripts/check-deps.mjs` 返回 0（Node v22.15.0，Chrome/CDP ready），之后只读取 Apple 公开一手文档，没有操作用户已有浏览器页面。

两次 manager 的完整入口均为：`pwsh -NoLogo -NoProfile -File C:/Users/Wen/.codex/skills/codex-ssh-manager/scripts/codex-ssh-manager.ps1 -Action resolve|probe -Alias mac-test-102342413 -OutputFormat json`。Apple 获取命令为 `curl.exe --fail --location --max-time 25 <上述官方 URL> -o .artifacts/desktop-m0-apple-services.html`，退出 0。

本审计没有运行新能力原生测试，不能替代后续 M2/M3/M5/M6 的功能和包级验收。历史 LAN PASS、NAT FAIL 与其他 NOT_RUN 保持原记录。

补充：在 `apps/desktop` 执行 `$env:GOWORK='off'; go mod verify`，退出 0，输出 `all modules verified`，确认本轮所读 beta.18 module cache 内容通过 Go module 完整性验证。

## M0 profile 写入锁基础

新增 `internal/app/profile_lock.go`、`profile_lock_windows.go`、`profile_lock_unix.go` 与定向测试。Windows 使用 `x/sys/windows.LockFileEx` 的 `LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`，macOS/Linux 使用 `x/sys/unix.Flock(LOCK_EX|LOCK_NB)`，通过稳定的 `.profile.lock` 文件持有内核锁。同一 profile 被占用时返回可 `errors.Is` 匹配的 `ErrProfileInUse`，不同 profile 可并存。

锁文件不写身份或 PID，不截断原内容，不在释放后删除，避免 unlink 后两个进程锁住不同 inode。Close 幂等，进程正常退出或强杀由 OS 自动释放。Service 的接入必须位于 identity/trust/SQLite 初始化前，构造失败释放，完整 Shutdown 最后释放；光有新增锁文件不代表已经完成 Service 接入。

实际运行均退出 0：

- `GOWORK=off go test ./internal/app -run '^TestProfileLock' -count=1`，Windows：同进程互斥、不同 profile、重开和保留文件内容；真实子进程持锁期间父进程拒绝，子进程 os.Exit/Process.Kill 后父进程成功获得。
- `GOWORK=off go test -race ./internal/app -run '^TestProfileLock' -count=1`，Windows：相同测试通过。
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c ./internal/app -o .artifacts/desktop-m0-app-darwin.test`：跨编译通过；这是编译检查，尚不能替代 Mac 执行测试。

## 用户更新 Mac 地址后的环境恢复

用户随后明确给出新动态地址 `10.234.184.6` 并允许 SSH 安装软件和测试。仅更新 `mac-test-102342413` 的 HostName，保留原用户、key、alias、notes 和其他字段；本地 inventory/config 备份位于 `C:/Users/Wen/.codex/ssh-manager/backups/mac-dynamic-20260912T123106`。

当前 manager `apply` 忽略 Alias 参数，会重生成全部 Host 并修正全部 key ACL。因此使用单目标临时 inventory，由 manager 生成目标配置，再只替换真实 config 的 managed block 内唯一目标 Host stanza；未运行全量 apply，也未触碰其他 key ACL。实际差异仅 `HostName 10.234.171.192` → `HostName 10.234.184.6`。

新地址 manager resolve（04:31:22Z）、probe（04:31:32Z）、audit-host（04:32:20Z）均为 `status=ok/exitCode=0`。Mac 是 `wen@wenair.local`，Darwin 25.5.0 / macOS 26.5 / arm64；根文件系统显示约 544 GB 空余。

只读检查 job `/tmp/codex-ssh/desktop-m0-mac-inspect-20260912T043317Z` 退出 0，实际发现 Homebrew Go 1.26.4、Node 26.0.0、pnpm 11.3.0；wails3 不在检查 PATH；CLT/Apple clang 21.0.0/macOS SDK 26.5 可用，没有完整 Xcode。现有 `/Applications/LinkSend.app` 产品版本 0.3.0，运行进程 1 个、控制台用户 wen；没有关闭或替换该应用，没有改用户 LinkSend profile。

工具链准备使用新的 `/Users/wen/linksend-desktop-six/tools`，不替换 Homebrew、不改 shell 配置。后台安装 job 为 `/tmp/codex-ssh/desktop-m0-mac-tools-20260912T043432Z`，最终 `exit-code=0`；Go 1.27.1、Node 24.21.0、pnpm 12.4.1、Wails CLI beta.18 已安装并输出实际版本。固定官方来源：

| 工具包 | 一手校验来源 | 已取期望值 |
| --- | --- | --- |
| go1.27.1.darwin-arm64.tar.gz | `https://go.dev/dl/?mode=json&include=all` | SHA256 `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12`，68,100,347 bytes |
| node-v24.21.0-darwin-arm64.tar.gz | `https://nodejs.org/dist/v24.21.0/SHASUMS256.txt` | SHA256 `bed7eea5325e1108f32ce5228ddd6a5f0f08a499ee42aa7442aea583702f6057` |
| @pnpm/exe.darwin-arm64 12.4.1 | `https://registry.npmjs.org/@pnpm%2fexe.darwin-arm64/12.4.1` | SRI `sha512-6rkZkT3iGfaxknUdGHraqSWFvTa6N0ajAHluv9Ax0GRWs0sIcGNiFhDopv6xSZCsJZmG483aNS/b6UEDy3blfw==` |

远端安装脚本在解压前验证上述 SHA256/SHA512，全部 PASS，再将 Wails CLI beta.18 安装进隔离 bin。每次构建显式追加 PATH：`/Users/wen/linksend-desktop-six/tools/bin:/Users/wen/linksend-desktop-six/tools/go/bin:/Users/wen/linksend-desktop-six/tools/node-v24.21.0-darwin-arm64/bin`。

manager 的 tail-job 在 Mac 默认 zsh 中出现 `read-only variable: status`，这是状态查询包装脚本的既有 shell 兼容问题。没有重新启动安装；通过 manager run-script 执行 POSIX sh，只读原 job 的 `exit-code/stdout.log/stderr.log`，复核 job `/tmp/codex-ssh/desktop-m0-tools-status-20260912T043556Z` 退出 0 并确认安装成功。后续在修复 manager 前采用相同只读查询方式。

物理 Mac 的 profile 锁测试也已运行：上传 Go 1.27.1 编译的 darwin/arm64 测试二进制（SHA256 `38c0b914a21704a7dd9ce387e896787dc0b49e4a2d133c7de5711b5ef3169b8b`），执行 `-test.run '^TestProfileLock' -test.v -test.count=1`。job `/tmp/codex-ssh/desktop-m0-profile-lock-test-20260912T043634Z` 退出 0，同进程互斥、独立 profile、原文件保留、真实子进程正常退出/强杀释放全部 PASS；测试后用户原 LinkSend 进程仍为 1 个。这份二进制仅覆盖新增 OS 锁测试，后追加 Service 构造接入用例应随最终测试包重新执行。

安装运行状态和功能原生验收仍需分开记录；连接恢复、工具安装和 profile OS 锁测试不是 Finder/通知/睡眠等产品交互 PASS。
