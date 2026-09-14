# Desktop E5 联合验收

日期：2026-09-14。候选提交：`387b57c76596975d0da61f01e060e0cc2940b0b5`。分支：`codex/desktop-experience-upgrade`。

本页只记录准确候选包和真实验收事实。源码、loopback QUIC、命名 pasteboard、浏览器布局与物理准确包验收分开；未执行的项目保持 `NOT RUN`。

## 候选来源与 CI

- Draft PR：[#8](https://github.com/Wen5555/LinkSend/pull/8)。
- core push `34792565801`、core PR `34792567181`、desktop push `34792565789`、desktop PR `34792567186` 全部 `success`。
- 三平台准确候选包 workflow `34792565786` 全部 `success`：Windows amd64、macOS arm64、macOS amd64。
- 三份 artifact 的 `source_commit` 与 `workflow_head_sha` 均为候选提交，`source_state=COMMITTED`、`source_checkout_clean=true`、产品版本 `0.5.0`、协议版本 `1`。

| 平台 | artifact ID | artifact 外层 SHA256 | 包内载荷 | 当前结果 |
|---|---:|---|---|---|
| Windows amd64 | `10327624502` | `8f816f2ebaf1852e5d57fccb60d6f4076c5e0dbe818260c73ca6f78ab173872f` | ZIP 9,826,428 bytes / `a9bb4ae0...e0e7`；installer 75,023,277 bytes / `c8b25dd1...cdd8` | PASS |
| macOS arm64 | `10328234657` | `cfd11ad3b10959da03b76fd4f923fc10291e33f48f93735c62734f0d06545afc` | DMG 9,443,902 bytes；`be69d1f15d0495cdb4e54a7ef85ad26f32df08d8f61fb4e0c738c7fd8f70f810` | PASS |
| macOS amd64 | `10328693977` | `a193c6e3d9da6386dde521018b936cc45ca5569cb6d2fbae05019a731be461ae` | DMG 10,176,906 bytes；`60fd64f7651f61bb5f286a0d54c22cc38d0e9711c5362e63d3b043044c43581d` | PASS |

本地核验目录为 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-ci-34792565786`，不纳入仓库。三份 artifact 的实际外层摘要、解包结果、`BUILD-INFO.txt` 与 `SHA256SUMS.txt` 已逐项一致；仓库 `scripts/verify-milestone-packages.ps1` 退出 0，完整回执为该目录 `PACKAGE-VERIFICATION.json`。Windows ZIP 内只有预期四项，嵌入 EXE 为 23,926,784 bytes / `d45bfa4c...72bf`，ProductVersion `0.5.0`。

## Windows amd64 准确包原生结果

portable ZIP 解压到仓库 ignored `.artifacts/e5-windows-package/portable`，使用独立 `.artifacts/e5-windows-package/profile`。信令指向测试 loopback 拒绝端口；没有使用或修改正式 LinkSend profile。

| 项目 | 结果 | 证据 |
|---|---|---|
| 首次准确包启动 | PASS | WebView2 environment 和 desktop runtime ready；进程 6 秒后存活 |
| 当前 DPI/窗口 | PASS | 120 DPI（125%）；窗口 1120×760，客户区 1105×721 |
| 同 profile 二次启动 | PASS | 强制清理第一轮隔离进程后，第二轮尺寸、DPI、运行状态一致 |
| 关闭行为 | PASS | 首次关闭原生确认中选择“退出应用”，持久化 `close_mode=exit` 并 exit 0；同 profile 二次启动后关闭不再询问并 exit 0 |

第一版验收脚本错误假设首次关闭主窗应直接退出，实际产品按设计等待“留在后台/退出应用/取消”原生选择并返回 `WINDOWS_NATIVE_CLOSE_TIMEOUT`。后续用 UI Automation 精确定位“退出应用”按钮；`InvokePattern` 和 SendKeys 两个脚本动作失败均清理测试进程，最终以该原生按钮 HWND 的 `BM_CLICK` 完成选择。成功回执在 `.artifacts/e5-windows-close/result.json`。两次正常退出各出现 Chromium `Failed to unregister class Chrome_WidgetWin_0 / Error=1412` stderr 警告，但进程 exit code 均为 0。

准确包 5 次顺序启动基线均 exit 0：窗口句柄就绪 200.7–264.1 ms，中位数 230.5 ms；3 秒时 working set 中位数 47,894,528 bytes，private bytes 中位数 62,156,800 bytes，24–25 threads。该数据来自当前 Windows、热 WebView2/runtime 缓存和 loopback 拒绝信令，只作本机基线，不外推冷启动或其他硬件。回执在 `.artifacts/e5-windows-performance/result.json`。150% DPI、WebView 键盘可访问性与深色矩阵仍待运行。

125% DPI 下的 native min-size 检查也使用同一准确 EXE：请求 960×640 物理像素时实际窗口为 960×700、客户区 942×652；继续请求 600×400 时钳制为 900×700、客户区 882×652，等于 720×560 逻辑最小值按 1.25 缩放后的外窗下限。进程随后 exit 0。该检查只证明原生 DPI/min-size 约束和稳定退出，无法读取 production WebView DOM，因而不替代 150% 内容、深色和键盘检查。

延迟 Raw UIA 可读取 production WebView 的 `RootWebArea`、TextPattern、按钮角色及 DOM。独立 39 字中文设备名在 960×700 物理外窗中 `IsOffscreen=false`，边界 x=670–808，位于外窗 x=580–1540 内；传输按钮声明 `IsKeyboardFocusable=true`，准确包 exit 0。因此长名可访问性/有界性为 PASS，但不含像素截图。

准确包键盘自动化仍为 UNRESOLVED：普通 UIA 初期只见 WebView 容器；Raw UIA 激活后虽能看到控件，对 renderer 建立焦点并分别用 PostMessage、真实键盘事件发送 4 次 Tab，FocusedElement 名称仍为 `|||`，无法证明 `传输→记录→设备→设置` 顺序。独立 desktop 不抢占用户前台，但 WebView2 在已有实例时先返回 `0x800700aa`，停止该隔离实例后虽启动成功，Raw UIA 仍不导出 DOM。所有失败尝试均清理测试进程；没有把浏览器 E2 键盘结果继承为准确包通过。随后一次只读输入环境核对显示：进程与 active console 均为 session 8，测试线程 desktop 为 `Default`，但 `OpenInputDesktop` 返回 `Screen-saver`，且无 LogonUI 锁定进程；真实键盘事件没有进入应用所在 input desktop。最小复测动作是用户退出屏保，使 input desktop 回到 `Default`，再只运行一次 Tab 顺序。准确 EXE 使用的 Wails beta.18 会把外部 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 覆写为编译期 `application.Options.Windows.AdditionalBrowserArgs`；该准确源码没有设置进程内调试参数，已有单次官方环境变量探针返回 `WEBVIEW2_CDP_NOT_AVAILABLE`，因此无法从该包取得 DOM 150% 内容缩放、暗色媒体查询或截图证据，也不再改用注入框架。当前 Windows 应用主题为 light；未修改用户主题或显示缩放，因此准确 150%/深色保持 NOT RUN。

## macOS arm64 准确包原生结果

主机 alias `mac-test-102342413`，用户 `wen`，探测地址 `10.234.212.116`，Darwin 25.5.0 arm64。通过 `codex-ssh-manager` 完成 resolve、probe、audit；DMG 上传到隔离 `/tmp/codex-ssh` 路径，没有替换 `/Applications` 中的应用，也没有使用用户真实 profile 或 general pasteboard。

| 项目 | 结果 | 证据 |
|---|---|---|
| DMG 摘要 | PASS | 远端重算为包内清单记录的 `be69d1...f810` |
| 布局/架构 | PASS | `LinkSend.app`、Info.plist 和主二进制存在；`file`/`lipo` 均确认 arm64 |
| 身份/系统版本 | PASS | bundle ID `com.linksend.desktop`，版本 `0.5.0`，最低 macOS 13.0 |
| 签名边界 | PASS | `codesign --verify --deep --strict` 通过；签名为 ad-hoc，未公证 |
| 首次启动 | PASS | 独立 profile 下进程 6 秒后存活，CoreGraphics 观察到 1 个真实 1097×745 窗口 |
| 同 profile 恢复启动 | PASS | 正常结束第一次进程后第二次启动，同样观察到 1 个 1097×745 窗口 |
| 收尾 | PASS | 两次进程均接受 SIGTERM；DMG 卸载；测试状态限于 `/tmp/codex-ssh` |

远程包检查 job：`/tmp/codex-ssh/linksend-e5-mac-package-387b57c-20260914T004708Z`。窗口恢复首轮脚本因 Swift 使用无效的 `.null` 常量退出 1，保留 job `/tmp/codex-ssh/linksend-e5-mac-window-restore-20260914T004906Z`；修正为 `CGWindowID(0)` 后，job `/tmp/codex-ssh/linksend-e5-mac-window-restore-r2-20260914T004932Z` 退出 0。该脚本失败不计产品失败，也未被改写为成功回执。

## 联合矩阵

| 场景 | 状态 | 当前证据/限制 |
|---|---|---|
| Windows 准确包布局、版本、DPI、恢复 | PARTIAL | 包/版本、125% DPI、小窗/长名、启动/恢复、首次关闭选择与持久退出 PASS；键盘自动化 UNRESOLVED，150%/深色 NOT RUN |
| Mac 准确包布局、启动、恢复 | PARTIAL | arm64 PASS；键盘操作、深色与缩放矩阵待运行 |
| Windows→Mac 文件 | NOT RUN | 准确 Win 包已通过 V2 native share journal 实时消费进入 `waiting_peer`，只证明入口/入队；旧 LAN-only profile 未互见，未传正文 |
| Mac→Windows 反向复用流 | NOT RUN | 不继承历史正向结论 |
| 文本、链接、图片自动剪贴板 | NOT RUN | Windows general clipboard 当前非空；不读取或覆盖用户内容，等待隔离桌面/空 disposable clipboard |
| 文件与剪贴板并存 | NOT RUN | 必须随物理双机剪贴板一起运行 |
| 旧版升级与混合版本 | PARTIAL | `387b57c` 与 M5 `d0c4a4b` 双向控制面 join 均拒绝且无幽灵成员；仅 loopback CLI/server，桌面升级与物理文件未运行 |
| 网络切换、睡眠/唤醒 | NOT RUN | 需保留实际接口和时间线 |
| 香港候选控制面部署 | PASS | `387b57c` Linux amd64 候选已按事务部署，独立复核通过；DB schema 2→4 |
| 荷兰隔离双 NAT | PASS | 两个独立 NAT、重叠私网和独立 WAN 完成 8 MiB QUIC；固定 UDP 映射，`relay=false` |

## 香港控制面事务部署

候选由独立干净 worktree 构建：Go 1.27.1、Linux amd64、`vcs.revision=387b57c76596975d0da61f01e060e0cc2940b0b5`、`vcs.modified=false`，SHA256 `2c9c352abb13b95cf2a0df48fc892f20488d7960ae48288724ed2004f9c16372`。

- 目标 alias `hk-main`，服务 `linksend-rendezvous.service`。部署前 PID 407573、二进制 `0923d9df...318c`、DB integrity `ok`、schema 2、TCP 443 与 STUN UDP 3478 正常。
- 事务 job `/tmp/codex-ssh/linksend-e5-deploy-hk-387b57c-20260914T014829Z` 依次完成 inspect、在线 SQLite 备份及 manifest 校验、原子替换、重启和 verify，退出 0，未触发回滚。
- 备份 `/opt/linksend-lan-test/backups/20260914T014842.005473007Z-desktop-e5-387b57c`；新 PID 553926，DB schema 4，公网 health 为 V1/QUIC/`relay=false`，最近 fatal/panic 为 0。
- 独立复核 job `/tmp/codex-ssh/linksend-e5-verify-hk-387b57c-20260914T014925Z` 再次核对运行文件、`/proc` 归属、唯一 443 listener、STUN、DB integrity/schema、备份 manifest 和公网 health，退出 0。
- 隔离回退兼容演练 job `/tmp/codex-ssh/linksend-e5-hk-rollback-compat-drill-20260914T022857Z` 使用当前 schema 4 数据库的在线副本启动旧 M5 二进制；旧端以 exit 2 明确拒绝 `unsupported control database schema version`。线上 PID、二进制和 DB 未变，临时 DB/config 在退出时删除。

部署脚本最初打印的“只替换旧二进制”回滚入口在 schema 迁移后已作废。安全恢复必须保留当前 live schema 4 数据：先在线备份并验完整性，只安装在 schema 4 副本上通过启动/行为检查的二进制，再原子替换和验证；当前健康候选可保持运行，后续缺陷采用 schema 4 兼容修复向前恢复。不得自动恢复部署前 schema 2 数据库，也不得用它覆盖部署后的成员写入。

## 荷兰隔离双 NAT

alias `nl-highdefense`，Ubuntu 26.04 / Linux 7.0 amd64。实际 job `/tmp/codex-ssh/linksend-e5-run-nl-dual-nat-387b57c-20260914T022524Z` 退出 0；独立复核 `/tmp/codex-ssh/linksend-e5-verify-nl-dual-nat-387b57c-20260914T022658Z` 退出 0。

- 拓扑含两个独立 NAT namespace、重叠 `10.77.0.0/24` 私网、两个点到点 WAN 和独立 rendezvous；不是 localhost 或单 Docker bridge。
- 产品 rendezvous/CLI 均来自准确提交，CLI build metadata 为 `vcs.modified=false`。主机没有 coturn，实验仅将 STUN 服务替换为锁定 `pion/stun/v4` 的有界 Binding fixture；Pion ICE 和 quic-go 产品实现未替换。
- 固定 UDP 映射正例完成 8,388,608 bytes；send/receive session 均为 `03bb1a7235ab5ab4bd1f3d797edb6852`，摘要一致，双方 STUN 计数大于 0，`relay=false`，NAT A/B 计数为 8/6。
- 该结果只证明明确的 endpoint-independent/固定 UDP 映射正例；不证明对称或 endpoint-dependent NAT 可穿透，也不改写历史 MASQUERADE-only `CHECK_TIMEOUT`。产品仍无 relay。
- cleanup 后无 `linksend-lab-*` namespace、veth 或实验进程；身份、邀请、私钥、正文、数据库和接收目录均删除。宿主地址、路由、规则的规范化前后快照一致。
- 脱敏包远端及下载后 SHA256 均为 `4be226d16a15e3a9b6bb854239fba9a3062cda0b32b5a594897dd98e7b2cb86b`，本地位于 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-nl-dual-nat-387b57c-sanitized.tar.gz`。

## M5 混合控制面兼容

旧端固定为干净 `d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06`，当前端固定为 `387b57c`；四个 Windows amd64 CLI/server 二进制均由独立 clean worktree 构建并记录 `vcs.modified=false`。两方向各使用独立 loopback rendezvous、独立数据库/profile 和单次邀请，不接触生产组。

- 当前 `387b57c` server + 旧 M5 client：旧端 join exit 1，稳定码 `VERSION_INCOMPATIBLE`；当前端 `devices` exit 0，组内仍只有当前管理员，没有旧成员。
- 旧 M5 server + 当前 `387b57c` client：当前端 join exit 1，稳定码 `AUTHENTICATION_FAILED`；旧端 `devices` exit 0，组内仍只有旧管理员，没有当前成员。
- 以上结果证明准确 `387b57c` 包在两方向均阻止半加入/幽灵成员，但当前端对旧服务的错误分类不准确。本次后续源码修复在 `Join`/跨组切换签名前读取 `/healthz`：使用真实 `d0c4a4b` rendezvous Windows 进程调用桌面 `PairDevice` 得到 `VERSION_INCOMPATIBLE: server capabilities incompatible`，前端映射为“当前信令服务版本过旧或能力不兼容，请升级服务端后重试”。进程级回执为 ignored `.artifacts/e5-current/legacy-m5-desktop-pair/result.json`；普通测试替身另证明不再调用旧 registration endpoint。修复后的重新打包、桌面升级和混合物理文件仍未运行。
- 回执位于 ignored `.artifacts/e5-mixed-compat/{result.json,reverse/result.json}`。邀请和测试身份只存在隔离目录，不进入文档或 Git。

## 当前限制与下一步

- Windows general clipboard fixture 返回 `CLIPBOARD_TEST_REQUIRES_EMPTY_DISPOSABLE_CLIPBOARD`。本轮未读取、清空或覆盖用户剪贴板内容。
- 私有 window station 原型与准确包分列：命名 station 在当前令牌下返回 Win32 5；匿名 station 可创建 station/desktop/child，但子进程初始化 user32.dll 失败，现有 `nativeclipboard` 写读测试无法创建 HWND。general clipboard sequence 在全部尝试中保持 `3882`，没有修改日常板。该路线已经停止，不写成 Wails 包通过。
- Windows Sandbox 可执行文件当前不存在；仅观察到 Hypervisor、`vmcompute` 与 HNS 已运行。没有启用系统功能或新建虚拟化环境。准确包系统剪贴板继续局部挂起，不阻塞文件、控制面和 NAT 验收。
- Windows 当前已有 HKCU 0.5.0 M5 安装（EXE `e08d8dce...b67ff`）、HKLM 0.3.0 安装（`d99f4fd4...a9c85`）及用户/系统/桌面三个快捷方式。为避免覆盖真实卸载记录和快捷方式，本轮没有运行 `387b57c` installer；签名及依赖签名的系统安装/激活继续按用户决定暂缓。
- 一次 CLI 只读意图的 `invite --help` 没有解析帮助标志，而在默认 profile 的服务端组创建了 10 分钟一次性邀请；没有本地 profile 写入，也没有设备使用该邀请加入。该邀请已于 `2026-09-14T02:00:23Z` 自动失效，未通过添加成员来“清理”这一错误。此项不计入桌面准确包通过范围。
- Apple App Group 激活、Developer ID、公证、Windows package identity 和依赖签名的系统安装信任按用户决定继续暂缓；这不替代其余功能验收。
- 香港升级后已新建 Windows 隔离 profile，并通过当前固定测试配对入口加入 schema 4 控制面；准确 Windows 包在该 profile 上有 1 个 listen、2 个 established 连接。Mac 动态地址随后连续 SSH timeout，尚未创建配套新 profile；恢复后再生成一次性邀请、加入当前组并执行准确包双向文件。剪贴板只在隔离条件或一次明确临时使用授权满足后开始。
