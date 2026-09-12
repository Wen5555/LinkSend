# M3 后台生命周期接线与 Windows 原生验收

2026-09-12。托盘、通知、防睡眠 adapter 已接入唯一 App 生命周期；界面新增可撤销的关闭方式、通知、
防自动睡眠和当前用户登录启动设置。首次关闭使用原生选择，默认不隐藏窗口、不打开自启动或通知权限。
窗口隐藏后 core 继续工作，明确退出仍经过保存/取消保护、停止入口、释放托盘/防睡眠和等待所有 owner。

通知只对新入站请求或真实终态产生通用摘要；任务 ID 由后端重新查询，点击不执行路径。
启动时已有历史不会重新通知，同一等待请求的进度 revision 不会重复通知；完成消息等待双方确认。
关闭通知时不启动 Windows toast 注册服务；用户明确启用后才请求权限，权限失败不改变传输结果。
防睡眠只在实际 transferring 状态持有系统 lease，暂停/完成/退出和系统睡眠事件释放。

## 原生测试发现并修正的问题

1. Wails beta.18 的 Windows `Question()` 实际用 MessageBox Yes/No，并按英文返回标签找 callback；
   自定义中文按钮并未得到调用。已改为 Windows SDK `TaskDialogIndirect`，核对 CommCtrl.h 的 packed ABI，
   真正显示“继续任务 / 保存并退出 / 取消任务并退出”或“留在后台 / 退出应用 / 取消”。
   macOS 保留其已支持自定义按钮的 AppKit/Wails adapter。
2. 原 SavePreferences 一律 StartInbox，使正在接收的 WSS 任务在保存后台选项时被取消。
   后台设置现在只保存自身配置；普通设置也只在接收参数变化时请求更新。
   M4 继续修正全局目录变化的下一次生效行为，保持正在接收的计划。
3. M2 CI `34677732541` 的真实 QUIC 保存退出回归发现：对外已显示 shutdown_requested，但磁盘保存尚未结束，
   context 仍未取消，可能多发下一块。现在在 task mutex 内同时设置意图并取消，再保存快照。
   真实 QUIC `TestSaveExitPreservesRealQUICCheckpointAndProfileOwnership` race 连续 30 轮 PASS。
4. M2 CI `34677732545` 的两项组件测试缺少预览 hook 所需 QueryClientProvider；已按生产入口补齐上下文，29/29 再次 PASS。

上述 Windows 按钮问题是在发布 M1 后的原生深测中发现，M1 Release 已追加已知问题，
没有把先前的核心保存退出单测或无活跃任务关闭冒充三按钮原生通过。

## 已通过的真实窗口流程

| 流程 | 来源与结果 |
|---|---|
| 首次关闭选择后台、持久化、隐藏不退出、二次启动唤回、明确退出 | 真实 Wails 窗口 + Windows TaskDialog + UIA/原生 Button 点击，exit 0 |
| 认证接收请求 → 原生确认接收 | 两个隔离 profile、真实 rendezvous/ICE/TLS1.3/QUIC，仅 loopback |
| 正文接收中启用防睡眠 | 已验证 4 MiB 后开启，真实 lease 状态显示，传输保持运行 |
| 活跃任务退出时选择继续 | 原生按钮正确回调，保持同一 task/attempt 和实际正文状态 |
| 选择保存并退出 | exit 0；SQLite 终态为 recovering，已接收/已验证均 4,194,304 B，CanResume=true，attempt/transfer 身份与恢复记录保留 |

后台关闭流程的测试快照 EXE SHA256：
`1ae07b1911fd4a96095523217cc57a33295599cf44debbe63864b63872c2ca7b`；
证据 `.artifacts/desktop-six-features/m1-native-tests/run-20260912T064020877Z/`。

包含后台偏好修复的活跃正文验收 EXE SHA256：
`782a26952193d62e74491a701bceed21ee7a96bfb9271b20ec16e1ff7d50cbb3`；
证据 `.artifacts/desktop-six-features/m1-native-tests/run-20260912T065407855Z/m3-native-save-exit.json`。
两个哈希均属于明确标注的未提交验证快照，不冒充后续 CI 资产。

脚本 `test-m3-native-lifecycle.ps1`、`test-m3-native-save-exit.ps1` 使用真实 native HWND/控件。
TaskDialog 的 UIA Pane 没有 InvokePattern 时，先核对标签和所属进程，再向真实子 Button 发 BM_CLICK。
曾尝试的鼠标定位未通过所有者检查，未发送点击；随后采用该原生按钮路径。
测试还修正了“等待 HWND 而未等待按钮 ready”和“用数据库热进度代替 live 快照”的脚本错误；
正文检查点、UI 进度与最终持久化分别核验，未修改生产逻辑以迁就错误断言。
所有本轮测试 peer、GUI、fixture 均已关闭；用户原有应用和 profile 保留。

## 剩余边界

Mac 精确 `62628d0` 源码归档实际完成普通测试、vet/build、前端与DMG构建，但完整race中
`TestAdapterDatagramsDeadlinesAndClose` 收到排队的 `context canceled`，桌面并发首次创建日志锁曾有一次 `openat .lock: ENOENT`。
前者现使已关闭适配器一致返回 `net.ErrClosed`；后者对同一 root 下创建锁的 ENOENT 做有界重试，其他错误仍直接拒绝。
实体Mac补丁验证：关闭读取race重复100次、激活日志并发race重复30轮均PASS；作业
`/tmp/codex-ssh/desktop-m3-platform-repeat-20260912T071151Z`。
该次为 `62628d0_PLUS_REVIEWED_PLATFORM_FIXES`，不把补丁结果归给原始归档。

Windows CI `34679582811` 的SendTo原生COM保存曾返回异常，本机原路径重复100轮未复现。
现改为在唯一临时目录内直接创建新`.lnk`，不向COM提供占位空`.lnk`，并在所有COM对象释放后才发布/读取。
Windows SendTo与激活日志race重复30轮PASS；最终CI需重新验证，未把未复现当作排除问题。

Mac 最终主应用的后台/关闭/菜单流程，Windows/Mac 通知实际点击、真实睡眠唤醒、最终安装卸载和跨设备六项联调
仍需对应独立记录。平台 lease/通知提交测试见 [adapter 记录](DESKTOP-M3-NATIVE-ADAPTER.md)，不能代替整机工作流。
