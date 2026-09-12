# Windows ↔ macOS 联调报告与复测步骤（2026-09-11）

本轮为真实 Windows/Mac 联调；没有把模拟、浏览器预览或私网地址替代原生双机验收。实现状态与验证状态分别记录。所有相对证据路径均相对于 `D:\apps\Osend\.artifacts\lan-live-20260911`；该目录含本机测试身份，**不得整体提交或对外导出**。对外分享仅选择脱敏日志及 `deliverables`，不包含 profile、邀请、私钥、测试磁盘镜像或文件正文。

> 后续实现补充：本报告主体保留物理双机联调当时 `0.1.0` dirty snapshot 的现场事实。其后产品版本提升为 `0.2.0`，实现 schema 2 任务持久化、桌面暂停/显式恢复、重启恢复和缺块字节续传，并通过本地真实 Pion ICE + quic-go 回归；协议仍为 V1。这些新能力尚未重新完成物理 Windows↔Mac 恢复矩阵。香港测试主站已经同步 `0.2.0`，部署后完成/拒绝的立即重试 PASS，因此正文中“旧服务 FAIL”仅是当时历史结果。当前 [v0.2.0 测试预发布](https://github.com/Wen5555/LinkSend/releases/tag/v0.2.0) 的 tag/源码为 `426d58b6ab62ab7213475007305c0a403955c00f`，PR [#6](https://github.com/Wen5555/LinkSend/pull/6) 已合并，三平台 Release 构建 run 为 [`34600609161`](https://github.com/Wen5555/LinkSend/actions/runs/34600609161)。不得用旧 `deliverables`/r2/发布前候选包覆盖 Release 资产或本报告的验证边界。

## 一、环境与拓扑

| 项目 | 实际环境 | 状态 |
|---|---|---|
| 仓库 | `D:\apps\Osend`；main；基线 HEAD `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`；远程 `https://github.com/Wen5555/LinkSend.git` | PASS |
| 工作树 | 开始时已有大量无关 untracked 文件；本轮修复尚未提交，保留旧文件 | PASS |
| Windows | amd64，PowerShell 7；物理以太网 `10.234.232.205/16`，原 TUN 仍启用 | PASS |
| Mac | `wen@10.234.241.3`，SSH alias `mac-test-102342413`；macOS 26.5 / Darwin 25.5.0，arm64，10 核、16 GiB RAM | PASS |
| Mac 网络 | 物理 Wi-Fi en0 `10.234.241.3/16`；默认网关 `10.234.0.1`；TUN `198.18.0.1`；DNS `114.114.114.114`；仅见 link-local IPv6 | PASS |
| Windows 工具 | Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails v3.0.0-beta.18 | PASS |
| Mac 工具 | 物理联调阶段 Go 1.26.5；r2 桌面构建 Go 1.26.4；Wails v3.0.0-beta.18；实际 Node 26.0.0、pnpm 11.19.0；Xcode Command Line Tools、SDK 26.5 | PASS |
| 产品身份 | LinkSend 0.1.0；Bundle ID `com.linksend.desktop`；最低 macOS 12.0 | PASS |
| 正式数据目录 | Mac `~/Library/Application Support/LinkSend`；Windows 通常 `%APPDATA%\LinkSend`（`os.UserConfigDir()`） | PASS |
| 隔离目录 | Windows `win-profile`；Mac `/Users/wen/LinkSend-tests/lan-20260911/profile`；GUI 单独 `build-20260911-v2/gui-profile` | PASS |
| 信令 / STUN | `https://linksend.oooai.de:443`；`stun:stun.oooai.de:3478`；中继未启用 | PASS |
| 荷兰 VPS | 此前 SSH 超时，本轮未取得其 STUN 或双 NAT 实验新证据 | BLOCKED_BY_EXTERNAL_ENV |

SSH 使用已安装公钥及 codex-ssh-manager。Mac 原非交互 PATH 未列出 Homebrew 工具，随后在 `/opt/homebrew/bin` 找到并使用。未修改生产防火墙、路由、DNS、代理、香港服务配置或原 `/Applications/LinkSend.app`。磁盘故障只作用于独立 64 MiB 镜像，目录权限只作用于新建测试目录。

这是 Windows 物理以太网 ↔ Mac Wi-Fi、混有现有虚拟网卡的一个真实场景。没有执行完整网卡开关/metric/DHCP、Windows↔Windows、Windows↔Linux、独立双 NAT 或可用全局 IPv6 矩阵。**模拟环境不能替代真实双机验收。**

## 二、测试矩阵

负向场景的 CLI 退出码 1 是预期结果；只有同时满足稳定错误码和文件/回滚断言，测试才为 PASS。

| 场景 | 环境 / 命令 | 退出码 | 证据路径 | 状态 | 备注 |
|---|---|---:|---|---|---|
| Mac SSH 审计 | manager resolve → probe → audit-host，随后 run-script | 0 | `mac-app-audit.json`，`mac-native-verify.json` | PASS | 公钥登录；未输出秘密 |
| Windows→Mac 基线文件组 | CLI send / receive，显式物理地址，STUN，`--evidence` | 两端 0 | `win-to-mac-result.json`，`mac-receiver-result.json`，`mac-received-sha256.txt` | PASS | 12,582,949 bytes；四文件 SHA256 一致 |
| Windows→Mac 第二轮 | 同上，新接收目录 | 0 | `success-after-stale-expiry.json` | PASS | 旧服务协商过期后的单次成功，不是立即重试修复证据 |
| Mac→Windows 基线 | 同上，方向交换 | 两端 0 | `mac-send-job.json`，`win-receiver-result.json` | PASS | 该轮暴露两端路径标签不一致 |
| Mac→Windows 修复后 | `mac-send-v3.sh` + Windows v3 receive | 两端 0 | `mac-send-v3-job.json`，`win-receiver-v3-result.json`，`hash-verification-v3.json` | PASS | 四文件及中文空目录；双方 direct_unknown |
| 拒绝接收 | `mac-reject-v3.sh`；Windows v3 send | CLI 1，断言脚本 0 | `reject-v3-send.log`，`mac-final-evidence.json` | PASS | RECEIVE_REJECTED；接收目录为空；stdin 脚本拒绝，非原生 UI 点击 |
| 目标已存在 | `mac-conflict-v3.sh`；Windows v3 send | CLI 1，断言脚本 0 | `conflict-v3-send.log`，`mac-final-evidence.json` | PASS | FILE_CONFLICT；原文件 SHA256 未变 |
| Mac 目录无写权限 | `mac-permission-v3.sh`，仅测试目录 chmod 500 | CLI 1，断言脚本 0 | `permission-v3-send.log`，`mac-final-evidence.json` | PASS | PERMISSION_DENIED；没有写入，权限恢复为 755 |
| Mac 磁盘不足 | `mac-disk-full-v4.sh`；64 MiB HFS+ 独立卷，有限 dd，12 MiB 文件发送 | CLI 1，断言脚本 0 | `disk-full-v4-send.log`，`mac-disk-final.json` | PASS | 真实 ENOSPC，双方 DISK_FULL；卷已卸载。首次错误 hdiutil 参数保留在 `mac-disk-before.json` |
| 历史旧服务短时间重试 | 首次错误后，同一对设备连续 send | 1 | `fault-exits.txt`，`fault-*-send.log`，`mac-fault-inspect.json` | FAIL | 当时线上旧版本仍保留约两分钟协商、源码修复尚未部署；此历史 FAIL 不能标外部环境问题，当前主站复测见下文 |
| ICE 诊断泄露最小复现 | `go test -count=1 ./internal/transport -run '^TestICEQUICAuthenticatedBidirectional$'` | 修复前 1，修复后 0 | `credential-regression-before.log`，`affected-after.log` | PASS | 原失败已保留；新候选诊断不包含 ufrag/密码/扩展 |
| 错误类别最小复现 | app `TestClassifyTaskErrorKeepsStableCodesAndHidesRawDetails` | 前 1，后 0 | `error-regression-before.log`，`affected-after.log` | PASS | 本地和远端 typed error 映射一致 |
| QUIC 拒绝帧丢失 | `TestTransferFailureSurvivesQUICStreamClose` | 前 1，后 0 | `quic-terminal-before.log`，`quic-terminal-after.log` | PASS | 使用真实 Pion+QUIC；不以 net.Pipe 单独证明修复 |
| 立即重新协商源码回归 | `TestReconnectAllowsFreshNegotiationImmediately` | 0 | `lifecycle-after.log`，Mac `core-test.log` | PASS | 新旧连接清理；没有延长超时等待旧协商过期 |
| 根模块 | `go test -count=1 ./...`；`go test -race -count=1 ./...`；`go vet ./...`；`GOWORK=off go test ./...` | 均 0 | `root-*-final.log`，`regression-final-exits.txt` | PASS | Windows；包含文件安全、源文件变化、检查点、取消、历史进程测试 |
| 独立桌面模块 | `GOWORK=off go env GOMOD GOWORK`；test/vet/build | 均 0 | `desktop-env.log`，`desktop-*-final.log` | PASS | 没有用根模块测试代替桌面模块 |
| 前端 | frozen install；typecheck；lint；`test -- --run`；build | 均 0 | `frontend-*.log` | PASS | 当前 8 tests；保留原断言 |
| Mac 原生 Go | `GOWORK=off go test -count=1 ./...` 和 `-race`；桌面 test/vet | 均 0 | Mac `build-20260911-v2/evidence`，本机导出的证据包 | PASS | Apple arm64 原生运行；非交叉编译冒充测试 |
| 格式与 bindings | gofmt；diff --check；Wails generate bindings；两端 SHA256 比较 | 0 | `gofmt-final.log`，`diff-check-final.log`，`bindings.log`，`mac-final-evidence.json` | PASS | 先保留并修复 CRLF 引起的 diff 失败 |
| 历史重启、损坏记录、revision | `internal/app/history_test.go`，含正常子进程退出与强杀 | 0 | 两平台根模块 test/race 日志 | PASS | 自动化测试范围；不等于 GUI 全阶段退出验收 |
| r2 原生 idle 退出 | Windows `WM_CLOSE`；Mac quit Apple Event | 0 | `.artifacts/final-20260911/mac-native-r2-result.txt`；r2 资产 manifest | PASS | 修复 Wails 3 `ShouldQuit` 返回值；Mac 窗口约 1008×684；仅证明 idle 退出 |
| 原生 UI 其余流程 | 原生文件/目录选择、打开目录、红点实际点击、Cmd+Q 按键、活跃任务退出保护、键盘/高 DPI 等 | — | `mac-app-audit.json`（Accessibility=false） | NOT_RUN | Accessibility 阻止自动点击；未伪造对话框、按键或点击证据 |
| 独立 Linux 双 NAT：固定 UDP 映射 | 两个独立 NAT namespace、重叠私网、独立 WAN、STUN-only/HTTPS | 0 | `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz` | PASS | 8,388,608 bytes；摘要一致；relay=false；direct_unknown |
| 独立 Linux 双 NAT：MASQUERADE-only | 同一隔离拓扑，不增加固定映射 | 1 | 同一脱敏证据包与 manager job | FAIL | `CHECK_TIMEOUT`；是产品能力边界，不归类为外部环境 |
| 多 NIC 切换 / 全局 IPv6 | 原要求完整矩阵 | — | 此报告范围说明 | NOT_RUN | 源码/单元测试不替代物理切换、睡眠或公网 IPv6 |
| 每个阶段强杀并恢复正文、断网后缺块请求 | 原要求恢复矩阵 | — | 后续本地真实 QUIC 恢复测试见 PROGRESS/TESTING | NOT_RUN | 当前源码已实现；物理双机恢复矩阵仍未运行 |

### 当前 Release 补充矩阵

| 场景 | 源码 / run | 状态 | 准确边界 |
|---|---|---|---|
| Linux core CI | `426d58b`；main run `34600609146` | PASS | 包含真实 QUIC code-0 正向与非零关闭负向回归；`9cce3ee` push core FAIL 仍保留 |
| desktop CI | `426d58b`；main run `34600609047` | PASS | Wails 3；桌面 Go、bindings、前端和构建检查 |
| 三平台打包 | `426d58b`；main run `34600609161` | PASS | Windows ZIP/NSIS、macOS 双架构 DMG 包级验证，不等于全部原生交互 |
| Release Windows 原生窗口 | Release ZIP 内 EXE；隔离 profile | PASS | 非零窗口句柄；idle `WM_CLOSE` 被接受，10 秒内 exit 0 |
| Release macOS arm64 启动 | 物理 Mac | NOT_RUN | 发布前候选收尾时 probe/audit-host TCP/22 timeout；Release 包未启动 |
| 安装/卸载与完整原生交互 | 真实系统窗口/对话框 | NOT_RUN | 不由 CI runner、浏览器截图或仅窗口创建替代 |

修复后双向候选记录保留了本地 srflx 与对端 host 的实际组合，但两端都不再仅凭类型标为互联网。最新反向会话 `24349d4780c01d9a206a132fc6e79a84`，generation 1，Windows base `10.234.232.205:53665`，Mac base `10.234.241.3:58164`，TLS 1.3 / ALPN `linksend/1`，relay=false。四文件 SHA256 验证结果见 `hash-verification-v3.json`。

物理 Windows↔Mac 当时的证据仍为 PARTIAL：CLI 输出有 transfer/session ID、generation、选中候选、base socket、TLS/ALPN、peer ID、STUN **字节**计数、manifest/result digest；当时缺独立 task/attempt ID、完整 ICE 状态时间线、接口名字段、逐包 STUN 请求响应计数、信令字节全量核对、单独提交摘要及异常阶段的统一证据对象。当前源码已增加 task/attempt/revision、接口/地址族、ICE 时间线、STUN 请求/响应和信令 JSON 字节等结构化字段，但尚未重新完成物理 Windows↔Mac 全矩阵，不能把源码字段倒填成旧现场证据。peer ID 是身份公钥摘要，不把线上 presence 当信任或路径证明。

## 三、缺陷与修复

| 缺陷 | 实现状态 | 修复验证 |
|---|---|---|
| Pion Marshal 把 ufrag 写入候选诊断 | IMPLEMENTED | PASS：诊断字段采用允许列表；签名候选交换不变 |
| 文件冲突误标 DIRECT_FAILED；权限误标 DISK_FULL；对端错误类型丢失 | IMPLEMENTED | PASS：FILE_CONFLICT / PERMISSION_DENIED / DISK_FULL；未知远端文本不获得类型语义 |
| QUIC 异步缓冲中的拒绝帧被立即 reset 丢弃 | IMPLEMENTED | PASS：有界终态发送收尾，真实 QUIC 与 Mac 复现回归 |
| 接收端读完 `confirmed` 后正常 code-0 关闭先于 stream FIN，使发送端误报失败 | IMPLEMENTED | PASS：`a885531` 仅在终态帧已写出后接受远端 code 0；真实 QUIC 正向、非零关闭负例及 Linux CI PASS |
| 断开的信令连接留下旧协商，阻止立即重试 | IMPLEMENTED | 源码测试 PASS；香港测试主站已部署 `0.2.0`，完成/拒绝后立即重试 PASS |
| 暂停请求被迟到 ACK 进度覆盖，任务误回到 transferring/取消 | IMPLEMENTED | Windows 真实 QUIC 高重复与 GitHub macOS arm64 同架构回归 PASS；计数保留但控制状态不回退 |
| srflx 就被标为 internet_p2p，双方标签不一致 | PARTIAL | PASS：移除无证据推断；真正 LAN/Internet 路由分类尚缺实现和证据 |
| 界面混淆历史与恢复能力、常驻固定测试码、Mac 提示双击 exe | IMPLEMENTED | PASS：修正文案及能力展示，生成绑定与前端回归 |
| Wails 3 空闲退出被 `ShouldQuit=false` 阻止 | IMPLEMENTED | PASS：空闲返回 true；Windows `WM_CLOSE`、Mac quit Apple Event 均退出；活跃任务对话框仍待原生交互验收 |
| Windows checkout 的 shell/HTML CRLF 影响 Mac 构建与 diff 检查 | IMPLEMENTED | PASS：LF 规则和重新构建 |

所有网络故障原始/脱敏日志均保留。没有修改生产网络配置，没有删除断言、跳过 race 或延长原有连接超时。测试脚本的首次 hdiutil 参数错误与管理器 zsh `status` 只读变量错误属于测试/工具缺陷，不归入产品传输成功证据。

## 四、内核、协议和安全影响

仍由 Pion ICE v4.4.2 执行检查/提名，quic-go v0.62.0 独占 UDP socket 并传输文件；TLS 指纹验证保持启用。文件正文没有经过 HTTP、WSS、JavaScript IPC 或第三方中继。

V1 帧结构不变，error 字符串只发送稳定允许码，不再泄漏本地文件路径；旧客户端可解析同一 error 帧，但可能只给通用失败提示。诊断候选字符串不再是可回灌的 ICE wire serialization。两项兼容与负向测试已加入；具体边界见 PROTOCOL.md 与 ADR 0001。

现场测试时 `history_persisted=true` 仅表示历史记录可写，彼时重启恢复和字节续传两个能力标志都为 false。后续当前源码已将三项能力拆分并实现后两项：本地真实 QUIC 证据为 12 MiB 无损坏恢复实际发送/接收 12,582,912 bytes、重传 0；首个 4 MiB staging 块损坏时实际发送/接收 16,777,216、重传 4,194,304、唯一验证/提交仍为 12,582,912；完整 Service 重启恢复 8,388,608 bytes、重传 0。该补充不是新的物理双机验收。

## 五、桌面界面和原生窗口状态

保留原导航、颜色和主要交互。退出缺陷的根因是将 Wails 3 `ShouldQuit` 返回值理解反了：无活跃任务返回 false 会取消原生退出。历史 r2 修复后，Mac arm64 构建在独立数据目录启动，CoreGraphics 返回 1 个该 PID 的原生窗口，边界约 1008×684；发送 idle quit Apple Event 后应用退出。Release `426d58b` Windows EXE 也已重新证明原生窗口句柄非零、idle `WM_CLOSE` 被接受并在 10 秒内 exit 0。该 PASS 仅证明窗口创建和空闲退出，不证明具体按钮、键盘快捷键或活跃任务保护。原 `/Applications/LinkSend.app`、用户既有 `/Volumes/LinkSend` 挂载和正式身份数据保持不变。

修复前物理联调阶段两端生成的 service binding SHA256 为 `1ba146cda3ecda1c791ae601f7d9d02cb8f03b273964da23708bc8001b807836`，任务模型为 `83c9cc0a0571b5c9a1913c2d71caf8c1679692a4e1314f0e49744f1e1ce9ab4f`，主要前端 JS 为 `d05be5bb5f12105e6b4a61c370c080a77ef0ae43e16a533f8b21f96277421e40`。Release main run 已重新生成并核对 1 service / 27 methods / 14 models，且构建使用 tag 内 dist；这些一致性证据仍不替代原生控件交互。

辅助功能权限当前为 false；文件选择器、打开接收目录、红点/Cmd+Q、活动任务退出保护、Tab/Enter/Esc、高对比度、深色/高 DPI、reduced-motion 均未完成本轮原生验收。应用最小宽度 720；不能声称原生窗口已完成 720 以下检查。

## 六、构建与发布资产

Release 资产来自 GitHub Actions main run `34600609161` 的真实 workflow/head/tag `426d58b6ab62ab7213475007305c0a403955c00f`，本地核验副本位于 ignored `.artifacts/v020-release/`。包内 `BUILD-INFO.txt` 明确 `source_state=COMMITTED`、`source_checkout_clean=true`；本地、workflow 清单、合并校验文件与 GitHub asset digest 一致。

| 资产 | 大小（bytes） | SHA256 | 工具 / UTC | 签名 / 公证 | 原生验证 |
|---|---:|---|---|---|---|
| Windows amd64 portable ZIP | 8,403,529 | `dcdd7127c527b1464f0ea025046ad871a35ed6eaba4a2c725884779071d81577` | Go 1.26.5；Node 22.15.0；pnpm 11.19.0；Wails beta.18；`2026-09-11T12:49:31Z` | unsigned / N/A | Release EXE 窗口创建、idle `WM_CLOSE` PASS；其余 NOT_RUN |
| Windows amd64 installer EXE | 9,996,124 | `99a4d5d25dbc9761e4839ff435f362ec65aa75fcdafdf46f285f05defdde7370` | 同上；NSIS 3.10；`2026-09-11T12:49:31Z` | unsigned / N/A | 安装/卸载 NOT_RUN |
| macOS arm64 DMG | 8,170,928 | `c637897290677c7deb8c50395b95568f55a93a14bdf51e2babbf7eaf974fdf4c` | Go 1.26.5；Node 22.15.0；pnpm 11.19.0；Wails beta.18；Apple clang 17；`2026-09-11T12:48:16Z` | ad-hoc / TeamIdentifier none / notarization NOT_RUN | runner 包级 PASS；Release 包物理启动 NOT_RUN |
| macOS amd64 DMG | 8,808,743 | `f5f066634429e61312066c056b836482ca228315ea858cab3de28653e67120fe` | 同上；`2026-09-11T12:51:36Z` | ad-hoc / TeamIdentifier none / notarization NOT_RUN | runner 包级 PASS；Intel 真机 NOT_RUN |

### 历史 dirty r2 资产

现场阶段 `deliverables` 和 `.artifacts/final-20260911/` 的 `-r2` 资产仅用于复核当时结果。其来源为基线 HEAD 加未提交修复，`commit=null`、`source_state=UNCOMMITTED_TEST_SNAPSHOT`；r2 源包 `linksend-source-fef3e3f-dirty-r2.tar.gz` 为 2,772,920 bytes、SHA256 `8612b015bdbdacae26d9d3915b95f9d9744d3576fbd280bfbef751faa940e3f8`。历史 Windows ZIP/installer 为 8,403,482 / 9,996,336 bytes，SHA256 分别为 `13d6e97ecbadf56f4f501af529eb7cde22d068f1dcdd23091bdaf7cd76454562` / `7b5c810a6a6a6609e130d870ba5754a203e27f65cdeb4d41020f71e8d679382b`；历史 macOS arm64/amd64 DMG 为 8,077,571 / 8,806,122 bytes，SHA256 分别为 `e0057c0293dc1cbb40f200f5fba82cb4985c37de8114eeae6a9e2fa7007e4d94` / `2671d8e4f3c057112eb9f6bfb66e562c660aba6c9f692219c8654e5ccf6ac068`。这些文件没有 workflow run ID，不能改名、复用或改写元数据冒充 committed `0.2.0` 资产。

`v0.2.0` 已按用户后续明确授权合并并作为 GitHub Pre-release 发布，且不是 Latest。`9cce3ee` 虽生成过 committed 包，但其 push core run `34592549314` 真实 FAIL，没有进入 Release；发布只使用含终态修复并从实际 tag SHA 重建的 main-run 资产。完整原生交互、安装/卸载、签名、公证和 MASQUERADE-only NAT 仍未通过，因此本版不得描述为稳定或生产版本。

## 七、未完成能力和外部阻塞

- IMPLEMENTED / LOCAL PASS：桌面暂停/显式恢复、应用重启后恢复正文、字节级缺块恢复；物理双机恢复矩阵仍为 NOT_RUN。
- NOT_IMPLEMENTED：中继。`relay=false` 保持不变。
- PARTIAL：LAN/Internet 路由分类以及完整的文件/重启/网络切换现场验收；当前仅在证据不足时报告 `direct_unknown`。
- PASS：香港测试主站已同步 `0.2.0`，完成/拒绝后的立即新连接通过；旧服务 FAIL 作为历史前后对照保留。
- BLOCKED_BY_EXTERNAL_ENV：当前没有可操作的原生辅助功能权限、独立 Intel Mac/Windows 第二台及可用全局 IPv6 现场；本轮收尾时 `mac-test-102342413` 的 manager probe 连续两次 TCP/22 timeout，因此未在该物理 Mac 启动 committed DMG。Linux 独立双 NAT 已运行，不再列为环境阻塞，但 MASQUERADE-only 结果仍为产品 FAIL。未操作生产路由来补造拓扑。
- NOT_RUN：全局 IPv6、VPN/metric/DHCP/睡眠切换、信令中断后实际正文连续性、接收各提交阶段正常退出/强杀、完整源变化/部分提交/恶意路径双机矩阵。已有自动化测试范围单独记录，不冒充现场验收。

## 八、已知限制、数据兼容范围和回滚方法

身份格式、Bundle ID、正式数据目录和文件协议 V1 不变。当前任务数据库升级为 SQLite `user_version=2`：v1→v2 前通过 `VACUUM INTO` 创建权限 0600 的 v1 备份；旧程序不能直接打开 schema 2。回滚必须关闭所有 LinkSend 进程，再恢复 `task-history.sqlite.schema-v1-<UTC>.bak`。旧客户端可能把新错误码显示为通用失败；新客户端不会从未知文本猜测错误类别。

正式数据备份必须在所有 LinkSend 实例退出后复制整个数据目录，包含身份、trust、偏好与 SQLite 数据及必要 sidecar；不导出给他人。测试仅用独立 profile，清理时先核对绝对路径属于 `LinkSend-tests` 或本轮 `.artifacts`，不可删除正式数据目录。

本轮未安装替换原应用，所以应用回滚是结束本轮测试副本后继续打开原应用。新测试目录保留供检查。权限回滚为 `chmod 755 /Users/wen/LinkSend-tests/lan-20260911/receive/permission-v3`，已执行；实验卷已卸载，最终证据应同时核对 mount 列表。源码修复已形成实际分支提交；若经审阅决定撤销，应针对具体提交执行新的 `git revert`，不能 reset/clean 整个现有工作树。香港测试主站二进制与数据库的准确回滚步骤和备份路径见 [DEPLOY-HK](DEPLOY-HK.md)。

### 下一轮完整复测步骤

1. 阅读 AGENTS.md、SPEC、PROGRESS 和本报告；保存 `git status --short`、分支、HEAD。关闭需要备份的正式应用，备份数据目录；为测试选择全新 profile 和接收目录。
2. Windows 运行 codex-ssh-manager 的 `resolve`、`probe`、`audit-host`，alias 为 `mac-test-102342413`。远端命令写入 LF `.sh`，使用 manager `run-script`；不要手写业务 SSH 或将密码写入脚本。
3. 优先使用 [v0.2.0 发布记录](RELEASE-v0.2.0-TEST-CANDIDATE.md) 列出的 Release 资产，并同时核对 tag、run/head SHA、`BUILD-MANIFEST-v0.2.0.json` 和 `SHA256SUMS-v0.2.0.txt`。`.artifacts/final-20260911/` 的 `-r2` 资产只保留为历史 dirty snapshot。Windows 解压后按 README 设置隔离 `LINKSEND_DATA_DIR`；Mac 挂载后复制到隔离测试位置，不替换原 `/Applications/LinkSend.app`。
4. 两端使用现有组成员生成的独立一次性邀请加入，分别核对完整设备指纹并手动信任。不要使用测试固定码或在日志记录邀请。
5. 设置真实绑定地址：Windows 当前 `10.234.232.205:0`，Mac 当前 `10.234.241.3:0`；每次重新检查 IP 是否变化。保留现场 VPN/TUN，记录接口/地址/路由/metric；不要为通过测试修改生产网络。
6. 接收端选择全新空目录并点击等待接收。发送端依次测试空文件、中文文件、长文件名、多个文件、含空目录的目录和大于两个 chunk 的文件。先不接受，验证未写正文；再明确接受。完成后比较两端 SHA256 与协议结果，然后交换方向。
7. 新建测试目标文件重复发送，验证 FILE_CONFLICT 且原摘要不变；拒绝、取消、超时应分别验证。Mac 权限和磁盘不足可复用本轮 `mac-permission-v3.sh`、`mac-disk-full-v4.sh`，先查看脚本中的范围、命令与 rollback，改用新的测试路径后执行，避免覆盖现有证据。
8. 核对候选/base socket/TLS/ALPN/peer/session/generation；候选类型和在线状态都不能代替路径证据。核对界面稳定码、任务终态、逻辑完成量和能力声明与 CLI/后端一致。
9. 香港测试主站当前应报告产品 `0.4.0`、协议 V1；重复完成→立即完成、拒绝→立即完成。若回归失败直接记录产品 FAIL，并按 [DEPLOY-HK](DEPLOY-HK.md) 的备份回滚，不能靠等待两分钟、延长超时或修改网络配置掩盖。
10. 有原生交互条件后逐项检查文件选择器、打开目录、焦点键盘、深色/高 DPI、红点/Cmd+Q/Windows 关闭和活动任务保护；保存真实原生窗口证据。历史持久化、重启恢复和字节级缺块续传分别验收，不以窗口创建或历史列表替代正文恢复。
11. 执行根模块 test/race/vet/独立模块、前端 frozen install/typecheck/lint/test/build，再执行桌面独立 test/vet/build。前端构建与桌面编译不要并行，因为 Vite 会重建 dist。运行 Wails bindings 生成并核对差异，然后构建 Windows 与两种 Mac DMG。
12. 将全部可运行 FAIL 修复并复测后，再按发布流程从真实提交构建、等待 required checks、核对合并后 SHA 和 Release 实际 digest。未具备的拓扑/签名/公证/原生证据继续明示，不复用本轮快照的提交来源声明。
