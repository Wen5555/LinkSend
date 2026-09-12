# M5：原生内容、收发动作与本轮收尾

2026-09-12。用户最新要求：完成M5后合并、测试、修正文档、上传GitHub和Release，然后停止。本轮保留已合入的M6收件箱功能与局部证据，不再推进M6完整验收。最低macOS13、Go1.27.1保持；Mac最新地址为10.234.14.15。

## 实现

- 文字/URL采用显式受限表单，Go建立不可变快照；图片只通过原生Clipboard adapter读取，返回类型、大小和像素元数据。原图和文件正文不进JavaScript IPC。
- 用户确认目标、离线等待和可选的普通文件降级后，通过Go幂等队列发送。未知提交结果保留原请求ID；明确入队前拒绝允许重新选择。图片不确定读取显示“核对上次图片读取”，不会误称刚读取了新图片。
- 文字收到后由用户手动预览或复制；URL再次验证http/https后才交给系统浏览器；PNG通过原生另存对话框和O_EXCL保存。每次动作按task ID重新验证完成状态、双方确认、manifest/content绑定、实际文件identity及摘要。
- schema5记录内容草稿、队列和实际传输解释。默认不额外保留已结束发送的正文快照；草稿、等待、暂停和可恢复引用受保护，清理历史/发送快照不删除用户接收文件。迁移备份与恢复见ADR0006。
- 73个真实Wails绑定方法、2个枚举、51个模型。ContentTask仅在用户展开时按task/revision读取；不会为所有历史图片持续重算摘要。
- 本次保留M6既有收件箱作为M5内容动作入口；可恢复的非队列任务按实时Go能力继续原任务，队列任务仍须在队列确认，不重复派发。

## Windows 真窗口与真实Go链路

原生测试EXE SHA256：`13a3d637c62cffb4204e035ec30df1d872d699a0ca8d91e9c1b01a41b6a6e647`。
来源为`ed4b330`加M5界面/队列持久化修改的未提交测试快照，不能冒充最终CI资产。
证据目录：`scripts/desktop-native/m1-native-tests/run-20260912T091113650Z/`（忽略提交，含独立测试profile）。

真实UIAutomation操作Wails窗口及系统对话框；两个独立Go Service通过真实Pion ICE、双向身份pin、TLS1.3/QUIC收发，网络范围为loopback。

- GUI显式创建文字、链接与原生剪贴板PNG草稿，经真实队列发送；三类型都实际完成双方确认。
- GUI逐一预览接收计划、确认文字/URL/PNG；收到后原生文字预览显示实际正文，复制结果由本机原生clipboard读取核对。
- URL预览通过；点击确认后真实原生Browser.OpenURL返回submitted。仅报告系统打开请求成功，不把它扩称为网页可达性验收。
- PNG原生另存完成，保存文件与原接收文件SHA256一致；再从原生剪贴板捕获3×2图片。修改剪贴板后选取持久图片草稿发送，接收端PNG与捕获来源SHA256一致，证明已排队内容不依赖当前剪贴板。
- 退出应用和fixture正常结束，GUI exit0，没有强制终止。

原始证据：`m5-*-incoming-plan.txt`、`m5-three-completed.txt`、`m5-native-content-actions.json`、`m5-native-exchange.json`、`m5-outgoing-{text,url,captured-png}*.txt`、`m5-peer-tasks.json`与`shutdown.json`。

验收脚本曾把原生SaveFile的编辑框ID误当PickFiles的ID，随后又在子控件尚未就绪时查询，以及把文字Info对话框确认按钮ID2当作ID1。保留这些脚本失败记录；核对实际Win32控件后继续既有成功步骤，没有重发已完成任务。Toggle按钮补用UIA TogglePattern，组合框用真实ListItem/SelectionItem，而非把未选中操作算成功。

## 剪贴板验收工具事件

前几次测试在内存保存7种剪贴板格式，恢复函数返回true；最后一次PNG快照后的恢复检查返回false，已明确告知用户原剪贴板不能保证恢复。该测试进程未保存原内容到日志/磁盘，原始报告保持`clipboard_restored=false`。不将此项写成PASS。

检查发现Windows在关闭clipboard后可能合成TEXT/OEM/LOCALE格式；仅用打开期间的序号判断会把它当作外部更改。验收工具现要求用户已清空的专用测试剪贴板，非空即在任何EmptyClipboard前拒绝；清理还核对owner和已知测试payload。没有再次改写用户剪贴板验证这个工具修补，标为未重跑。产品CaptureImage始终只读，不包含测试工具的Empty/Set操作。

## 物理Windows→Mac内容协议

物理Windows10.234.232.205→Mac10.234.14.15:48935，文本88B、URL71B、PNG240B全部Completed；两端TLS1.3、ALPN linksend/1、content_v1、选择/内容/文件摘要一致，relay=false。
Mac接收job `/tmp/codex-ssh/m5-physical-win-to-mac-20260912T091016Z`，退出0；独立复核job `/tmp/codex-ssh/inspect-m5-physical-results-20260912T091305Z`。
此工具使用指定地址的生产pin/QUIC/内容协议，**没有运行ICE发现、桌面UI或NAT穿透**。可复用源码在`scripts/contentprobe`，编译后用`--help`检查显式参数；不读系统剪贴板、不打开收到的URL。

反向Mac→Windows job `/tmp/codex-ssh/m5-physical-mac-to-win-20260912T091104Z` 退出1，真实错误`sendmsg: no route to host`；未改防火墙、路由或代理，不宣称物理双向成功。Mac当前系统26.5，不宣称已实测最低macOS13。

## 持久化与兼容修复

M5首次Mac Intel打包CI34681703733中，队列已completed但立即历史重发返回INBOX_NOT_RESENDABLE。原因是任务内存终态早于SQLite保存，调度器先持久化了队列completed。现要求匹配任务revision/state已经落盘后才能发布队列终态；落盘失败进入needs_attention/TASK_PERSISTENCE_FAILED。确定性的延迟/失败注入与真实三类型QUIC回归race10轮通过，未用延长sleep掩盖竞态。

完整content能力、旧端明确fallback、accepted持久屏障、失败零正文、URL约束、像素/编码上限、恢复原快照及引用清理证据见DESKTOP-M5-CONTENT-BACKEND.md、DESKTOP-M5-CONTENT-WIRE.md与DESKTOP-M4-M5-ACCEPTANCE-INTEGRATION.md。

## 发行边界

本轮已发布 [v0.5.0-m5](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-m5)，[最终包、CI及部署回执](DESKTOP-M5-RELEASE.md)单列精确来源。Windows未代码签名，Mac仅ad-hoc签名，未Developer ID公证。Mac Finder完整菜单/AX红点点击/通知点击受现场权限限制；MASQUERADE-only双NAT原CHECK_TIMEOUT仍保留，公共IPv6、真实睡眠/网切、更多安装环境及完整M6验收不在本次已通过声明内。最终包来源和哈希以对应GitHub Release的RELEASE-MANIFEST与SHA256SUMS为准。

最终本机回归（2026-09-12）：根独立mod verify/test/vet/build、全量race（app57.073s）和workspace test全部退出0；桌面独立verify/race/vet/build、73方法绑定生成及Wails production构建退出0。后续加入的独立physical probe源码再做定向检查；最终CI资产仍须由同一提交构建。
