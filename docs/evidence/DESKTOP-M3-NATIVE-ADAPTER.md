# M3 托盘、通知、防睡眠与可选自启动 adapter

日期：2026-09-12。代码在独立分支 `codex/desktop-native-features` 实现，不修改主 App/main/用户偏好或核心网络状态机。Go 1.27.1，Wails 固定 beta.18，最低 macOS 13。主程序仍需串行接入生命周期、真实 task 事件和设置；本记录不把 adapter 验证当作完整 S03 产品验收。

## 稳定接入 API

| 功能 | 调用方式 | 责任边界 |
| --- | --- | --- |
| 托盘 | `newNativeTray(host,iconPNG,nativeTrayCallbacks{Show,Inbox,PauseQueue,Quit}) (*nativeTray,error)` | 在 host.Run 前创建；只用应用自己的 PNG 图标，Wails 在 native loop 就绪后创建真正托盘。 |
| 队列菜单 | `tray.SetQueuePaused(bool)` | Go 核心成功提交暂停/继续派发后更新。菜单是普通项，不在持久提交前先乐观勾选。 |
| 托盘释放 | `tray.Close()` | 在 native loop 结束前调用；自动注册 App.OnShutdown 作为幂等兜底，移除原生图标和菜单 handler。 |
| 通知构造 | `newNativeNotifier(onTaskClick func(string))` | 回调仅返回不透明 task ID；主 App 自己查询任务并唤醒/定位，不执行路径字符串。 |
| 通知生命周期 | `notifier.ServiceStartup(ctx,application.ServiceOptions{})` / `ServiceShutdown()` | 推荐由现有 App 生命周期手动调用，保持仅 App 一个 Wails service。底层初始化只依赖 application.Get 的配置及 AppKit bundle，无需自己的 JS 绑定注册。 |
| 通知提交 | `notifier.notify(nativeNotification{TaskID,Revision,Title,Body})` | 返回 nativeNotificationResult 与参数错误；仅喂真实接收请求和既定终态，Title/Body 必须是通用摘要。 |
| 许可与状态 | `requestNotificationPermission()` / `notificationStatus()` | 都是未导出 Go 方法，不能被 Wails 自动绑定。授权请求只由用户显式启用通知操作触发；启动和传输事件绝不自动请求。 |
| 防睡眠 | `newNativeSleepInhibitor()`，`Acquire/Release/Active/Close` | 布尔 lease，重复 Acquire 幂等，不累计引用；仅活跃传输持有，暂停/终态/退出释放。 |
| 可选登录启动 | `installNativeAutostart(exe)` / `uninstallNativeAutostart(exe)` / `nativeAutostartEnabled(exe)` | 默认不调用安装函数；主 App 在用户明确更改设置后提交，设置存盘失败需自行回滚系统入口。当前作用于默认应用 profile。 |

底层 notification wrapper 只有 ServiceName/ServiceStartup/ServiceShutdown 导出。没有导出的任意 Notify、RequestPermission 或 OS 路径命令，前端不能直接驱动这些方法；主 App 仅公开有 task ID 校验的业务命令。

## 真实系统实现

托盘使用已核实 beta.18 的 SystemTray.New/SetIcon/SetMenu/OnClick/Destroy 和 MenuItem API。显示应用、打开收件箱、暂停队列派发、退出均为回调，未复制任何传输状态机；窗口关闭时隐藏还是退出由主 App 决定。

通知使用固定 Wails `pkg/services/notifications`，desktop 独立 module 只增加该已固定版本要求的 `git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3` 及对应 sums。任意底层 Startup/权限检查/投递失败转换为非致命状态，不使核心传输改变结果。状态区分 `submitted`、`duplicate`、`not_authorized`、`denied`、`unavailable`、`closed`；submitted 只表示 OS API 接受请求，不保证系统设置/勿扰模式让其显示。

Windows beta.18 的 Check/RequestAuthorization 都是 true stub，因此始终如实报告 `permission=unknown`，不把 stub 当成用户已允许。macOS false 的检查结果称 not_authorized（不能冒充用户明确拒绝），只有显式请求返回 false 才称 denied。OS 正式授权 prompt 使用上游真实 API；没有修改权限数据库或自动点击系统提示。

通知 ID 为 `linksend.<opaque task ID>.<revision>`，TaskID 只允许有界 ASCII 字母/数字/短横线/下划线，数据仅含 task_id 和 revision 字符串。点击校验 ID、revision 和可用的 UserInfo 后交给任务 lookup callback。关闭后 handler 注销，迟到点击被忽略。近期去重缓存最多 4096 个任务并限制单任务 pending 数；它是进程内缓存，主应用必须过滤/seed 启动历史快照，不能重放整个历史为新通知。

Windows 防睡眠使用 Kernel32 PowerCreateRequest/PowerSetRequest(PowerRequestSystemRequired=1)，由专用 handle 持有，不依赖会在线程间移动的 SetThreadExecutionState。REASON_CONTEXT 的布局、Version=0、SimpleString=1 和枚举值已对照本地 Windows SDK 10.0.26100.0 的 minwinbase.h/winnt.h 及 Microsoft PowerCreateRequest 文档核实。释放调用 PowerClearRequest 并关闭本应用独占 handle；Close 失败保留 owner 供重试。没有要求屏幕常亮，也不试图阻止用户明确发起的睡眠。

macOS 使用 IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleSystemSleep) / IOPMAssertionRelease，IOKit/CoreFoundation、CGo minimum=13。所有 Acquire/Release/Close 在 Go 互斥锁下维护，关闭后不能再取得 lease。OS sleep/wake 的核心连接重建由主 App/core 处理。

可选自启动：Windows 仅当前用户 Run 的 LinkSend 值，并在 Software/LinkSend/AutoStart 保存所有权与精确 command；命令由 Windows.ComposeCommandLine 组成，路径不会经 shell 展开。macOS 写当前用户 Library/LaunchAgents 下单独标记的 plist，ProgramArguments 数组与 XML 正确转义、RunAtLoad=true、Aqua session，不设置 KeepAlive。注册/移除仅匹配本应用 owner 和当前 exe，第三方/另一安装/修改后内容都保留并报错；Mac 文件以临时写入+原子 no-replace 安装。它只影响下次登录，不立即 bootstrap、终止或重启用户当前应用。

M2 parser 同时识别可选 `--background` 和 Windows COM 的 `-Embedding`（go-toast 固定源码明确要求），不会将其当成文件草稿。主 App 应将无文件激活与显示窗口策略分开。

## 自动化和真实原生验证

Windows：desktop `GOWORK=off go test -race -run '^TestNative(Sleep|Notification|Autostart|Background)' -count=1 -v ./...`、`go vet ./...` 退出 0。9 项测试通过：真实 PowerRequest 生命周期/并发、隔离 registry 的命令 quoting/所有权、激活参数、通知并发 task/revision 去重/失败可重试/默认不请求权限/Windows stub/关闭后点击丢弃。通知状态单元测试使用故障注入 backend，不能作为 OS 通知展示证据。

随后 opt-in TestMain harness 实际运行 Windows Wails loop：应用自有 PNG 托盘创建、菜单状态更新和 cleanup，PowerRequest Active=true → Released=true，真实通知 API 返回 `submitted / permission unknown`，进程退出 0。执行方式为 `go test -c -o .../native-background-tests.exe` 后显式设置 `LINKSEND_NATIVE_BACKGROUND_TEST=1` 与图标路径。普通 go test 不运行此 GUI harness。注册名为含 PID/时间戳的 `LinkSend Native Test ...`；完成后仅清理本轮拥有的 AppUserModelId/CLSID/通知类别测试注册。裸测试 exe 缺产品 icon resource 3，通知图标提取给出 warning，但通知提交成功；这不是产品安装包图标验收。

Mac：初始 M3 adapter archive SHA256 `d40e516c6ad5aec483cbc87e326c92c4b55612bb2f08307a7403dac96456b1d7`，隔离目录 `/Users/wen/linksend-desktop-six/m3-native-adapter`，job `/tmp/codex-ssh/desktop-m3-native-adapter-20260912T055605Z` 退出 0。mod verify、8 项定向 race 测试、vet 通过，包含真实 IOPMAssertion 与临时目录 LaunchAgent 读写/转义/第三方保护。没有往真实 Library/LaunchAgents 注册登录启动。

该 job 还构建并 ad-hoc 签名独立 NativeBackgroundTest.app（独立 bundle ID），实际运行 Wails loop、菜单栏托盘、IOPMAssertion 获取/释放和正常退出。真实 macOS 通知检查返回 `not_authorized`，其余原生流程仍完成且退出 0；没有弹授权窗、没有改系统权限，也没有宣称通知已显示。

最终新增 Finder 持久回调 error 与启动错误提示 helper 后，全部 adapter archive SHA256 `b6de3890a74650d3f3075c3cede358cf6c0409fb8fd7cffefd0c609272e864ac`。Mac job `/tmp/codex-ssh/desktop-m3-native-final-check-20260912T060427Z` 退出 0，14 项 Native race 与 vet 通过，包含 Finder ENOSPC/未就绪拒绝；两平台再次编译了 MessageBoxW/NSAlert helper，没有人为弹错误模态框。Mac link 仅有上游重复 -lobjc warning。

## 仍须主程序集成验收

真实 tray 图标点击/菜单点击、通知点击→真实任务定位、用户批准后的 macOS 通知展示、关闭窗口继续接收、活跃传输睡眠/唤醒、登录重启后的自启动、安装/卸载仍须与主 App/核心/prefs 接入后验证。本阶段只证明 adapter 的真实 API、故障行为与资源生命周期，不声称上述完整交互已通过。用户原 LinkSend 应用、general clipboard、默认自启动及网络设置保持原状态。
