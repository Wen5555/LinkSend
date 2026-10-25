# R5 准确候选原生入口与保留失败（2026-10-26）

本记录不是完整临时分享通过。准确 Windows 包已经实际打开、领取并刷新到发送方批准；本轮没有取得加密清单或本机文件完成凭据。另完成该包的原生诊断保存及两次正常退出。Windows 双端公网、双 NAT、Windows↔Mac、键盘文件粘贴及 R0–R6 整体仍未完成。

## 来源与准确 CI

候选来自 `3d20ccb6d84a6c1edbeb9879e2fb721191f53f47` 的 [packages40237184625](https://github.com/Wen5555/LinkSend/actions/runs/40237184625)，workflow_dispatch，attempt1、completed/success，未重跑。实际执行 `scripts/fetch-milestone-artifacts.ps1` 与 `scripts/verify-milestone-packages.ps1`，回执在 `.artifacts/share-ui-draft/candidate-3d20ccb/{artifact-verification,PACKAGE-VERIFICATION}.json`：外层 GitHub artifact digest、内外 BUILD-INFO 的准确 head、平台、版本和资产 SHA256 均核对，Windows PE/installer 为 0.6.2，随包 README 不再写旧 0.6.0。

| 资产 | SHA256 |
|---|---|
| Windows portable ZIP | `a9cfd24428d4f8546b455641212e01dfcdda24fdc9af569b9928d8a5dd786336` |
| Windows installer | `d07114615226ac08dd64ed7094e9f23d8496aad922c3adfa16ba3b07cea93a05` |
| macOS arm64 DMG | `328904a68f297b066b98dea5c7a493fb06beb06b3655ac36e8a075f1f8fda0b9` |
| macOS amd64 DMG | `eaf5575927747b8721f44c0de92bb646dea94c1e88f8af7b5f06283378262771` |

实际启动的是解包后 `verified-windows-payload/LinkSend.exe`，SHA256 `96574683f7deaee4c4792bb8741d83f2ec61a673cc117f287381fa4debaecbfc`。二进制 Go build info 含 windows/amd64、CGO0、固定 Wails beta.18，但没有嵌入 vcs 字段；来源证据是准确 Actions head、BUILD-INFO 与摘要，不能声称 binary vcs.modified=false。两个 Mac 包只核对构建产物，不代表 Mac 原生运行。

2026-10-26 07:28 后只读再次查询：`52f9bf20c3fb2ac71bffa0ee71521e9af40048dc` 的 [core40240526707](https://github.com/Wen5555/LinkSend/actions/runs/40240526707)、[desktop40240526725](https://github.com/Wen5555/LinkSend/actions/runs/40240526725)、[packages40240526706](https://github.com/Wen5555/LinkSend/actions/runs/40240526706)均准确 head、attempt1、completed/success。原始安全回执 `.artifacts/share-native-ui-draft/current-exact-ci-status.json`。52 的 receipt-only 缓存增量不倒记进本次3d包；托管 native code-copy 子目标也不替代文件粘贴或 helper 回收证明。

## 隔离及第一轮未领取

Windows 用户专属新测试根在仓库外，使用独立 profile、目标目录及私有凭据；系统剪贴板没有被本轮 UI 读写，没有点击分享码复制按钮。远端仅为 JP 自有临时协调服务和 Go sender，未替换生产、未改宿主网络。控制通路是 SSH-L 的 loopback HTTP/WS；数据设计仍为独立 Pion/QUIC，未将 HTTP 或 SSH 当文件通道。

第一次 GUI 只设置 `WEBVIEW2_USER_DATA_FOLDER`，实查发现固定 Wails 封装仍将 WebView 放到通常的 `%APPDATA%/LinkSend.exe/EBWebView`。在输入任何分享码前即拒绝该隔离状态，核对自有 PID/路径/创建时间后强制关闭，wrapper 退出 `4294967295`。没有旧全局 WebView 缓存备份，因此不能证明其逐字节未变；未猜测恢复或删除全局缓存。回执 `gui-attempt-01-isolation-rejected.json` 是恢复后补记，不能当作原时刻成功写出的收据。

随后仅覆盖 GUI 子进程的 `APPDATA=<本轮receiver>/appdata`。实际子 WebView 的 `--user-data-dir` 已逐次核对确实位于该私有根，再允许后续操作；不是仅相信环境变量。第二次启动复用第一轮未领取的自有 Go profile，没有重建身份。该轮 offer 已过期，observer 退出1、`wait_gui_claim/FIXTURE_TIMEOUT`，sender 退出1、`wait_claim_confirmation/DRIVER_TIMEOUT`，coordinator 退出0，均不计传输通过。临时 SSH 按原稳定句柄和实际期限退出。GUI 在空任务状态经原生关闭对话框选择“退出应用”，退出0、forced=false、486403ms，随后自有进程、WebView 与 UDP endpoint 均不在；只证明空任务正常关闭。

原始隔离/退出回执及 `scene01-retirement/` 保留在 `.artifacts/share-native-ui-draft/`。没有用 shell 删除被应用工具判定为 primary checkout 的旧验证 worktree。

## 第二轮：真实领取、批准后连接期限已过

全新 scene02 的根、identity、runtime、requestId 均未复用旧场景。native observer SHA256 `3cf3e272275b6d92b30a812b2f124d2a553ef0a10240507254aeec7440176e0e`，仅在 GUI 前创建身份，运行期间 mode=ro/query_only 读取实际 SQLite；完整核验本机签名 owner/claim/key/generation 后才通知 sender。它没有调用 GUI、Approve、Bind 或注入完成记录。该辅助工具构建时 vcs.modified=true；它不是产品来源证明。

sender 是准确 `701e23b` 干净源码 driver（来源及 hash 见[已有实网记录](RELIABILITY-R5-CROSS-NETWORK-20261026.md)），配置 CheckTimeout35秒、WaitTimeout45秒，未启用 loopback 候选。GUI 用原生临时分享入口填写自有码，输入后清空；observer 从真实签名 ledger 验证领取，sender 自己刷新并精确批准。批准后 GUI 刷新实际显示“身份已确认，等待继续”，没有永久配对或剪贴板授权。

真实 sender task 从 UTC23:19:58.301283137 启动，到23:20:33.305255117失败，约35.004秒，停在 `signaling_connect/DIRECT_FAILED`，verified/committed=0，local share 为 needs_attention、grant 为 approved，engine_completion 不存在。GUI 到约23:20:49才触发“读取加密文件清单”，已晚于对端实际连接期限；稍后本机同样 failed/signaling_connect/DIRECT_FAILED，share needs_attention/approved。没有 preview-verified、没有选择或确认接收目录，没有本机完成凭据，也没有文件完成声明。

这说明本轮测试两端参与时序不足，不能将它当成用户 Windows 双端现场的已证根因。早期按 driver 的45秒 WaitTimeout 描述失败不够精确，以上以实际35秒 CheckTimeout及两端任务时间纠正。原 sender job 未重启或改超时；下一轮只在合法Claim核验后增加夹具“GUI就绪再通知批准”的屏障，不改变产品连接期限或身份校验。

退场前快照 `scene02/windows-before-exit.json` 与 `sender-before-exit.json` 固定了真实失败状态；正常退出可能按原默认设置取消未完成任务，不能覆盖退场前原证。GUI 经原生退出应用正常 exit0、forced=false、574904ms。旧失败保留，不将新的成功追记到该场景。

## 同一准确包的 R3 原生保存与发现的缺口

在 scene02 失败后，从“设置→诊断与关于→导出本地诊断”实际打开 Windows SaveFile，指定测试根内全新 `.json` 并保存，GUI 显示“诊断已保存到所选位置，不会自动上传”。输出 SHA256 `2c4d3cf952dcdfa7180f6a375ca802276bc0485fd657b861b07213c14df8eced`，report_version1、product_version0.6.2、windows/amd64、relay_available=false，包含真实 failed task-1、unknown method、UTC23:20:51 的实际尝试时间。报告仅为安全投影，没有分享码、稳定身份/公钥、服务URL、候选地址、文件名、完整路径或正文。

同一报告也保留了尚待修复的可操作诊断缺口：task 的已知 `signaling_connect` 在 guidance 仍为 unknown，`DIRECT_FAILED` 只显示“任务未完成，具体原因需要进一步检查”，临时分享行也只显示通用失败。不能将导出成功写成所有错误说明均已完善。此轮未做原生 SaveFile 取消，不用旧包取消结果替代；真实文件传输、实际恢复与两个平台旅程继续。

Mac保存别名在本日07:14再次 SSH255/connect-timeout；这是测试入口不可达，不能冒充产品连接失败。保留 `.artifacts/share-native-ui-draft/mac-probe-20261026-071409.json`。其余 R1–R6 必做项与 Goal active 状态不变。
