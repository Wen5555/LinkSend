# M1 Windows 原生窗口验收

2026-09-12 已在本机真实 Wails/WebView2 窗口完成 M1 的三页导航、系统文件选择/取消、设备别名与专属目录保存，并核实退出重启后的持久数据。交互通过 Windows UI Automation 和经过进程归属校验的 Win32 Common Item Dialog 控件完成；没有使用普通浏览器、CDP、DOM 脚本或伪造后端事件代替原生验收。

本记录只对应下面这一份 **UNCOMMITTED_TEST_SNAPSHOT**，不是最终 CI/Release 资产验收，也不表示 S01–S06 或 M2–M6 已完成。

## 被测资产与隔离条件

| 项目 | 实际值 |
| --- | --- |
| Windows EXE | `D:/apps/Osend/apps/desktop/bin/LinkSend.exe` |
| 产品版本 | `0.5.0` |
| 文件大小 | 21,583,360 bytes |
| SHA256 | `7936a3b91868981ac7859af3a25c2243df43cda325f5fcc06e45c74fe618fe73` |
| 来源状态 | `UNCOMMITTED_TEST_SNAPSHOT`，主线程当次 M1 构建；未把 dirty 构建映射为干净 commit |
| 初次 GUI | PID `231448`，HWND `11407154`，class `WailsWebviewWindow` |
| 重启 GUI | PID `252880`，HWND `7278040` |
| 初始化证据 | 实际 HWND、WebView UIA Document、`desktop runtime ready` 日志 |
| UIA 环境 | 当前用户的 PowerShell 7，系统 `UIAutomationClient` / `UIAutomationTypes` |
| 信令 fixture | 独立 `127.0.0.1` 动态端口上的真实 rendezvous；没有使用香港服务或用户真实身份 |
| 身份准备 | 两份临时 profile，经真实 Bootstrap → Invitation → Join 配对；GUI 接手 profile A 前两个 Go Service 均已 Shutdown 释放 profile 锁 |
| 网络约束 | `127.0.0.1:0` 绑定、明确允许测试回环；所有非回环接口写入 ExcludedInterfaces；STUN 仅指向回环测试地址 |
| 自有源文件 | `中文 验收 文件.txt`，内容为本次创建的测试文本，未向对端发送 |
| 源文件 SHA256 | `d0ed1de2de6a0102f68f17a4cf4367733601edd47c8e47f4763bf53808e6a937` |

回环配对与本机 UI 检查不算物理 LAN、跨 NAT、网络切换或跨操作系统传输证据。设备 B 保持离线，仍可对其真实已配对身份编辑本地偏好。

## 实际结果

| 检查 | 结果与证据 |
| --- | --- |
| 切换“设备”页 | PASS；UIA Invoke 导航按钮，实际出现“我的设备与附近设备” |
| 切换“设置与诊断”页 | PASS；UIA Invoke 导航按钮，实际出现“服务与网卡” |
| 返回“传输”页 | PASS；UIA Invoke 导航按钮，实际出现“准备发送” |
| 原生 PickFiles 选择文件 | PASS；打开标题为“选择要发送的文件”的系统 `#32770` 对话框，选择本次自有中文空格路径；真实 Wails 返回后，Go 草稿库保存唯一绝对路径 |
| 再次打开 PickFiles 并取消 | PASS；系统 Cancel 控件关闭对话框，草稿仍只有原路径，最终整套运行中 revision `3 → 3`，没有清空或另加文件 |
| 设备本机别名 | PASS；针对 fixture 的确定身份打开设备设置，用原生 UIA ValuePattern 输入“原生验收 中文别名”，点击“保存设备偏好”后 Go SQLite 中出现相同值 |
| 设备专属目录 | PASS；通过标题为“选择接收目录”的系统目录选择器选中本次创建的“对端 专属目录”，保存后 Go SQLite 的对应 peer 行路径精确一致，最终 profile revision 为 `2` |
| 真实退出与重启 | PASS；同一 profile 的新原生进程启动后，UIA 再次观察到原草稿、别名，并通过实际只读目录控件 ValuePattern 核对已保存专属目录 |
| 最终有序退出 | PASS；向本次 GUI HWND 发 `WM_CLOSE`，Win32 `GetExitCodeProcess` 返回 `0`，未强杀 GUI |
| fixture 清理 | PASS；发送本次 fixture 停止标记，等待进程退出；没有结束用户旧 LinkSend、其他应用或其他服务 |

完整套件最后一次运行时间为 UTC `05:42:44–05:42:49`（北京时间 `13:42:44–13:42:49`），脚本退出码 `0`。最终退出时间为 `2026-09-12T05:43:39.2828042Z`。

另用全新独立 profile 于 `05:46:09Z` 补查“只打开并取消文件选择器”：取消前后通用 `Error` 文本数量均为 `0`；该补查进程也通过 `WM_CLOSE` 退出码 `0` 正常关闭。过程中较早见到的通用错误横幅没有在这一受控取消检查中复现，不能将它推定为文件选择取消失败。

## 工具适配与证据边界

- HTML 原生可访问性按钮使用 UIA InvokePattern；带 `aria-expanded` 的设备“设置”按钮实际暴露 ExpandCollapsePattern，按该模式展开。别名输入框实际暴露 ValuePattern，直接使用该原生接口。
- Windows Common Item Dialog 的文件名 Edit（ID `1148`）、目录 Edit（ID `1152`）及确认/取消按钮（ID `1/2`），在本机 UIA 中被报告为 Pane，未暴露 Value/InvokePattern。PowerShell 5 复查结果相同；注册客户端 provider 的尝试返回异常，没有更改系统权限、注册表或安装配置。
- 因此对这些**实际系统控件 HWND**先校验所属 PID 和对话框 class，再使用 Win32 `WM_SETTEXT` / `WM_GETTEXT` / `BM_CLICK`。所有提交路径只来自本次 fixture，未使用全局 SendKeys，也没有注入网页或 Go 事件。
- Common Item Dialog 在 HWND 创建后还会异步恢复上次目录。初版脚本过早填写时，文本被初始化覆盖；最终脚本等待初始化并复读实际 Edit 内容，再点击系统确认按钮，完整重跑已通过。
- 初次重新取得 Process 对象后，PowerShell 没有提供退出码，故该次只记录“进程已退出”，没有填成 0。最终退出验证提前打开自己的进程状态句柄，并用 Win32 读取真实退出码，结果为 0。
- 曾观察到测试窗口被最小化，脚本恢复的始终是自己的 HWND。首张屏幕区域截图被其他窗口遮挡，已作废删除；后续只在确定自己的窗口位于前台时允许截图。**没有可用于验收的最终截图，本文依据真实 UIA 控件、系统对话框、数据库及进程证据，不冒用其他应用画面。**
- 本次没有操作“加入发送队列”执行正文传输，没有验收托盘、单实例、系统 SendTo、macOS Finder Services、通知、选择性接收或剪贴板内容功能。

## 可复用脚本与实际证据

脚本限定保存在 `.artifacts/desktop-six-features/m1-native-tests/`：

| 文件 | 用途 |
| --- | --- |
| `fixture/main.go` / `fixture.exe` | 独立回环服务器、真实配对和临时 profile；不输出邀请或密钥 |
| `start-native-session.ps1` | 校验 EXE SHA、启动隔离 fixture 和 Hidden GUI，再按需要恢复自己的原生窗口 |
| `uia-tools.ps1` | 有界查找、UIA 控件操作、只针对归属验证 HWND 的 Win32 对话框与退出状态读取 |
| `run-native-suite.ps1` | 三页导航、原生选择/取消、别名/目录编辑与 Go 数据库确认 |
| `read-workspace.py` | 只读检查临时草稿和设备偏好，明确 UTF-8 输出 |
| `stop-native-session.ps1` | 校验 PID、可执行文件和创建时间后，有序关闭自己启动的 GUI/fixture |

复跑入口：

```powershell
. .tools/use-desktop-toolchain.ps1
go build -o .artifacts/desktop-six-features/m1-native-tests/fixture.exe `
  ./.artifacts/desktop-six-features/m1-native-tests/fixture

& .artifacts/desktop-six-features/m1-native-tests/start-native-session.ps1 `
  -BinaryPath D:/apps/Osend/apps/desktop/bin/LinkSend.exe `
  -ExpectedSha256 7936a3b91868981ac7859af3a25c2243df43cda325f5fcc06e45c74fe618fe73 `
  -SourceState UNCOMMITTED_TEST_SNAPSHOT

# 使用上一步返回的 SESSION_FILE；更换 EXE 时必须更新其实测 SHA。
& .artifacts/desktop-six-features/m1-native-tests/run-native-suite.ps1 -SessionPath '<SESSION_FILE>'
& .artifacts/desktop-six-features/m1-native-tests/stop-native-session.ps1 -SessionPath '<SESSION_FILE>'
```

主要实际证据目录：

```text
.artifacts/desktop-six-features/m1-native-tests/run-20260912T052137970Z/
  session.json
  navigation.json
  native-suite.json
  pickfiles-cancel-preserves-draft.json
  device-profile-native-save.json
  native-restart-persistence.json
  final-workspace.json
  shutdown.json
  native-stderr.log
  native-restart-stderr.log

.artifacts/desktop-six-features/m1-native-tests/run-20260912T054448895Z/
  cancel-error-probe.json
  shutdown.json
```

这些临时 profile 含测试身份文件，保留在 Git 忽略的 `.artifacts` 中用于现场复核，不应加入源码、诊断导出或发布包。证据摘要没有包含邀请、私钥或文件正文。

## 精确 CI 发布资产的追加验证

`2026-09-12T06:12Z`，对 Actions `34676198796` 的精确提交 `2f2656d940c96e142d5639b2bc3012ab0ce5fb6e`
Windows ZIP 下载、官方 artifact SHA、包 SHA、嵌入 BUILD-INFO 和 EXE 产品版本逐层校验后，使用包内原始 EXE 重跑真实 UIA 流程。
该 EXE SHA256 为 `8a47285a1c0ba27e868a5bf32ee66f9905d24940c46b95bea91c45793d7a304d`。

三页切换、原生文件选择、取消选择保持 revision `1→1`、设备别名/专属目录保存，共 6 项 PASS。
最后 WM_CLOSE 正常退出 0，没有强制终止；独立 fixture 已停止。
具体日志 `.artifacts/desktop-six-features/m1-native-tests/run-20260912T061148040Z/`。
该追加验证证明最终 CI payload 的原生界面，不复用前述未提交快照的哈希，也不扩写为全部六项验收。
