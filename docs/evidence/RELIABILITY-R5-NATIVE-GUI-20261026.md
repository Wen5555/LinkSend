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

## 第三轮夹具就绪屏障与精确时间

新 gated observer 在真实签名 pending Claim 核验后等待 root 的新普通文件标记，再复核同一 Claim/pin/generation 并触发原 sender 的真实批准。七项本地测试/vet/build exit0，新工具 SHA256 `9ad28a53c07f5507503e971958d09ffa91dfd47cc341c49d8779f04a83427b4c`；旧工具/driver/产品期限未改，生产与系统剪贴板未触碰。

仍未取得预览：sender记录23:37:53.233602534→23:38:28.237797711，35,004.195177ms；receiver记录23:38:28.6515725→23:38:58.6773551，30,025.7826ms。记录UTC窗口不重叠，receiver开始比sender终态晚413.774789ms；没有测跨机时钟偏差上界，不能将亚秒先后当严格物理证明，也没有证据证明scope/Origin故障。双方真实verified/committed及receiver received均为0，未取得接收预览/计划或本机Completion，见 `scene03/{TASK-TIME-COMPARISON,receiver-live-state}.json`。

GUI正常退出0、forced=false、422155ms；observer/forward/coordinator自然退场，18445两端空闲。只退役临时runtime，按诊断需要保留双方测试identity/profile、源、接收目录与日志。协调库schema5/quick_check=ok、approved1；协调stdout/stderr实际0B，没有per-session trace，不能杜撰会话细节。后续独立修复等待语义；本轮未启动scene04。

同轮定向回归已证明 typed signaling 原因丢失并修复，见[诊断增量](RELIABILITY-R3-SHARE-SIGNALING-DIAGNOSTICS.md)；它不改上述原生失败或人机等待期限。

## 准确36候选与第四轮边界

`36cfce3d2e8f3d5eed8ab6588c5d172bc05f4d73` 的 core `40251000156`、desktop `40251000220`、packages `40251000157` 均 attempt1/completed/success，三平台 package job 全通过；原8082 Intel Mac失败保留。原始查询在 `.artifacts/share-owner-draft/peer-wait-ci-36cfce3.json`。实际执行 `fetch-milestone-artifacts.ps1` 与 `verify-milestone-packages.ps1` 均 exit0，来源/内外BUILD-INFO/Actions摘要/PE版本已核对，回执 `.artifacts/share-ui-draft/candidate-36cfce3/{artifact-verification,PACKAGE-VERIFICATION}.json`。

| 准确36资产 | SHA256 |
|---|---|
| Windows ZIP | `815c5e099795202fcbba6d195ceacaee5a92133b698271e18f418e270aa23aac` |
| Windows installer | `50500aecdb772958d232b8147f77d736f8c9348937d3541dea706895fb65d766` |
| macOS arm64 DMG | `77bf3c4cfeb2a09428eeb1693f1c0fc68e62308b3acaa82b7591852dd3461b7c` |
| macOS amd64 DMG | `acbf69324224902363a5774931101da02d4d6d3978753f23c683536ce88d6dbf` |

实际Wails载荷 `verified-windows-payload/LinkSend.exe` SHA256 `4d90408843347d637be7e2eb54b41cbcd8450cc7f48fa358ec4f585883cbb68d`，PE0.6.2。该exe无嵌入vcs字段，来源由Actions/BUILD-INFO/摘要证明，不声明其vcs.modified=false。随包native code-copy目标真实run1/pass1/skip0/fail0/exit0，只证明对应原生代码保护及后续文字同步子目标，不替代文件键盘粘贴或Wails传输。

scene04准确36 GUI真实领取，observer核验pending Claim后等待root-ready；本轮没有写该marker，因此没有发送方批准、preview、产品peer-wait或本机Completion。原10分钟夹具耗尽，sender停在fixture `wait_claim_confirmation`、observer停在 `wait_root_approval_ready`。GUI PID58844真实wrapper exit1、forced_by_timeout=true、603709ms，不记正常退出。退场回执 `.artifacts/share-native-ui-draft/scene04/RUN-RECOVERY.json` 及 `gui-process-exit.json` 保留。

scene04远端runtime已精确退役，8个受保护文件hash保持。本地两个自有凭据的删除在CreateProcess前被自动审批以 `blocked by policy` 拒绝，未给更具体原因；未换工具绕过，文件仍在用户专属私有目录，见 `scene04/LOCAL-CREDENTIAL-RETIREMENT-BLOCKED.json`。

新scene05只调整测试工具：WaitTimeout45秒→10分钟，与桌面人机等待默认值一致；Check35秒保持（桌面本身30秒），actor/forward/observer/GUI总窗口统一30分钟。产品码、claim、grant的绝对期限不变，不续期旧场景。来源和构建在 `scene05/LAUNCH-PLAN.md` 与 `scene05-preparation/BUILD-RECEIPT.json`，不以准备完成预记实机通过。用户允许Linux替代Mac后按Windows↔Linux接续，不再等待Mac；Finder/DMG专属项目保持未验证。
