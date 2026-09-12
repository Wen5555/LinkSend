# M4/M6 Windows 原生界面与真实数据验收

2026-09-12；来源 `a26b8ba` 加未提交 M4/M6 UI 接线，明确是测试快照。
Windows EXE SHA256：`50fe39117fa729db5727d5aead4056fcc06a60e78b1ac52256017ceb393ced23`。
独立目录：`.artifacts/desktop-six-features/m1-native-tests/run-20260912T073452754Z/`。
仅操作此目录下的两套新身份/profile和本次窗口 PID229592；未使用用户默认 profile。

## 已运行的真实路径

UIAutomation 操作本次 Wails WebView2 原生窗口；第二个 Go 进程通过实际 Pion ICE、双向 pin、QUIC TLS1.3（772）、ALPN linksend/1 提交四项请求。连接证据为 loopback，`relay=false`，不声称物理 LAN 或双 NAT。

- 原生目录选择器改到带中文/空格的自选目录；取消 B 文件勾选；点击预览后读到持久计划 revision 对应的新名称，再确认接收。
- 原始 97 B、3文件+空目录。选中2文件+1目录，实际接收/验证/提交67 B，跳过1文件30 B；双方完成确认一致。A原有文件SHA未变，新 A (1).txt与发送源SHA一致，B未出现，空目录存在。
- 第二次显式全部跳过：原始100 B，正文0 B、4项跳过；发送端NoContent、接收端no_content，双方已确认，无伪造全量完成。
- 收件箱输入中文完整文件名的一部分 `C 正常`，后端FTS返回2条记录；点击已完成记录的定位文件，由真正Explorer打开自选目录且选中 `A 中文 冲突 (1).txt`，通过Shell.Application读取实际选中项确认。仅关闭此次验收目录窗口。
- 选中两条历史并确认“只删除记录”，界面查询归零；原始文件和接收文件均保留。再实际执行孤立暂存清理，接收文件仍存在。

原始证据：`m4-peer-{subset,allskip}-result.json`、`m4-*-preview-uia.txt`、`m6-search-uia.json`、`m6-explorer-reveal.json`、`m6-final-uia.json`、`m4-m6-native.json`。测试前端39项、typecheck/lint及Wails production构建均exit0。

## 验收中修正

初次脚本错误地期待任务页标签“全部跳过”，产品当时使用“未接收内容”，导致脚本在已完成双方NoContent后超时。保留原结果，不重发请求；修正脚本匹配后从已有结果继续检查磁盘和收件箱，exit0。新源码同时将原始no_content阶段映射为中文，避免内部枚举泄露。

复核Go DTO发现 reserved_bytes 是本次16MiB元数据预留，并非其他任务的预算；界面据此改为“其中元数据预留”。required_bytes 包含正文和该预留，空间结果只是预估，不是配额保证。同步修正低速率的小数显示和收件箱顶部标签。上述文案修改在本次EXE之后，后续重建验证。

最后通过本次窗口的“退出应用”退出，fixture亦已结束，无强制杀GUI。此轮未捕获GUI exit code，不能写成exit0；`shutdown.json`的gui_exit_code为null。

## 后续原生边界

以上是真正Windows窗口、Go/SQLite/QUIC及Explorer流程，不是浏览器mock。仍不替代macOS Finder完整菜单、通知点击、物理双机新功能、睡眠/网络切换、故障卷与精确CI发行包验收。已有后端故障和兼容矩阵见M4-APP与M4-M5-ACCEPTANCE-INTEGRATION；M5完整内容动作及最终M6包级验证继续执行。
