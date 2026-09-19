# Desktop E5 联合验收

日期：2026-09-14。候选提交：`387b57c76596975d0da61f01e060e0cc2940b0b5`。分支：`codex/desktop-experience-upgrade`。

本页只记录准确候选包和真实验收事实。源码、loopback QUIC、命名 pasteboard、浏览器布局与物理准确包验收分开；未执行的项目保持 `NOT RUN`。

## 2026-09-19 E5-A：0c59 发布包的物理双向文件

本节只适用于已发布预览 `v0.5.0-desktop-preview.1` 的源提交 `0c59ae255d631b496a3e483e9c558f2998ee314c`、workflow `34803743360`。Windows 使用已核验 ZIP payload `LinkSend.exe` SHA256 `97b3a63a122df148edbf5b3038b225bda84ad9e8d45b3bd0a5c8f598b16e0e3e`，macOS 使用 arm64 DMG SHA256 `ce89b37fea7658c57c58de8e1bb648d8e15d9a580233444e0fdd2c4ffe460ee5`。二者均在新的隔离 profile 和当前 schema 4 / membership-v2 测试组运行；Windows 当前以太网 `10.234.16.254/16`，Mac `en0=10.234.35.5/16`。Mac 连接、审计及所有远程 job 经 `mac-test-102342413` 管理，不修改宿主防火墙、路由、代理或 HK 服务。

| 流向 | 包内实际 queue owner | 结果 | 文件与完整性 | 网络证据 |
|---|---|---|---|---|
| Windows → Mac | Windows 准确包消费 V2 `windows_share` journal；Mac 准确包自动接收 | PASS | `e5a-win-to-mac.bin`，1,048,576 bytes；两端 SHA256 `2c7c48b84ae0b706befdd874f058bde9b86d2cf78404c3aee7ddd92f244dd443` | task `20260919T142911.207564800Z-00000001` completed；session `2f7d72cd6d1de8a515ab7f638479fd69`；`lan_direct` / QUIC / TLS 1.3 / `linksend/1` / `relay=false`；host `10.234.16.254:58074` ↔ host `10.234.35.5:62194` |
| Mac → Windows（先完成） | Mac 准确包消费 V2 `macos_share` journal；Windows 准确包自动接收 | PASS | `e5a-mac-to-win-b.bin`，1,048,576 bytes；SHA256 `d5ca225803f96c5a6b1bb51a54ba0815b5f7e060ed6509fef2e59e043fdd0f29` 一致 | task `20260919T143318.472039700Z-00000002` completed；session `0041445e4fe2bde11d8a0cd2b52db019`；`lan_direct` / QUIC / TLS 1.3 / `linksend/1` / `relay=false` |
| Mac → Windows（连续下一份） | 同一 Mac 准确包 queue worker | PASS / session reuse | `e5a-mac-to-win-a.bin`，1,048,576 bytes；SHA256 `44abae6db34d86fd81de269778d335d2e52ece9a2320cea9c1e104cd6b5c6316` 一致 | task `20260919T143319.008859500Z-00000003` completed；与先完成文件同一 session、Windows base `10.234.16.254:51996`、remote host `10.234.35.5:56125`、ICE generation 1；两 task 相隔约 0.54 秒 |

Windows task-history 的三个 task 均记录零重传、1,048,576 verified/committed bytes；反向 task 的本地候选被如实记录为 srflx `183.247.26.53:11139`，base socket 仍为 Windows 物理地址，不能据此扩大为跨 NAT 或任意网络保证。两条反向 task 相同 session 证明连续队列文件的认证 QUIC 复用。正向 task 在先前观测与反向注入之间超过产品 3 秒 idle 边界，使用不同 session，因此本节不把它描述为跨方向 session reuse。

V2 journal 仅是受控的 native-entry fixture：它保存路径与目标元数据，由运行中的准确包自行验证、入队、建立 ICE/QUIC 和落盘；正文没有经 journal、信令或 JavaScript IPC。此结果不等同于 Windows `ShareOperation`、macOS Share Extension/App Group 或系统签名安装激活；这些仍按用户已暂缓的边界记录。一次早期辅助 CLI 发送曾停在 `AwaitingAcceptance`、0 bytes（其新 identity 未获 Mac package 自动接收授权）；该日志保留为辅助失败，不用于本节结论。

### E5-B：0c59 双端临时系统剪贴板最小尝试（FAIL，已收尾）

用户已授权一次仅限 15–20 分钟的双端临时覆盖窗口。窗口从 `2026-09-19T14:50:18.3854890Z` 开始，因首条传输失败于 `2026-09-19T15:04:25.5588969Z` 关闭。两端均为本节 E5-A 所列的准确包、隔离 profile 与同一受信任 peer；没有读取、导出、备份、恢复或检查写入前的日常剪贴板内容。

开始时，纯配置 helper 将两端 `clipboard_enabled` 和对该 peer 的 send/receive × text/link/image 六项 grant 设为开启；helper 不加载原生 adapter，也不读写剪贴板。启动准确 package 前，Windows 仅用 `System.Windows.Forms.Clipboard` 写入并回读已知预置文本，Mac 仅用 `NSPasteboard.general` 写入并回读已知预置文本。两者都是标准系统剪贴板消费者，预置值仅用于建立 watcher 基线。

随后 Windows 通过 `System.Windows.Forms.Clipboard` 写入并回读已知文本 `E5B-WIN-TEXT-20260919T1501Z`（`2026-09-19T15:00:57.1855645Z`）。Mac 对同一预期值进行 `NSPasteboard.general` 标准消费者断言，于 remote job `linksend-e5b-assert-mac-text-0c59-20260919T150130Z` 以 exit 1 / `SYSTEM_CONSUMER_TEXT_ASSERTION_FAILED` 失败。此事实只证明该次 Windows→Mac 文本没有在断言前落到 Mac 系统板；不推断会话、watcher 或协议的具体根因，也不把系统消费者的预置回读当作跨端成功。

失败后未继续发送链接、3×2 PNG、Mac→Windows、快速 A/B/C 或文件与剪贴板并存。主开关先写为 false，macOS package 由受管 stop job 结束并以独立 job 确认 watcher 进程终止；Windows exact package PID `93952` 已结束。停止 package 后，两端再以 helper 关闭六项 grant，均输出 `grant_count=6`、`master_enabled=false`。没有恢复任何剪贴板内容，也未在窗口内进行进一步调试。

原始入口：本地 `C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5a-0c59\e5b-window.json`、`windows\logs\e5b-clipboard-enable.json`、`windows\logs\e5b-win-to-mac-text-write.json`；Mac 受管 jobs 为 `/tmp/codex-ssh/linksend-e5b-configure-mac-0c59-20260919T145620Z`、`/tmp/codex-ssh/linksend-e5b-start-mac-package-0c59-20260919T150003Z`、`/tmp/codex-ssh/linksend-e5b-assert-mac-text-0c59-20260919T150130Z`、`/tmp/codex-ssh/linksend-e5b-stop-mac-after-text-failure-0c59-20260919T150243Z`、`/tmp/codex-ssh/linksend-e5b-disable-mac-grants-0c59-20260919T150353Z` 与 `/tmp/codex-ssh/linksend-e5b-verify-mac-stopped-0c59-20260919T150425Z`。

离线定向回归 `TestClipboardConcurrentEnsureSessionsRemainReady` 使用内存 clipboard adapter，双端同时启动 inbox 和 `EnsureClipboardSessions`，确认双向 lease 就绪并完成文本 commit。它通过，故没有复现“同时建会话”这一假设；loopback/memory 结果不替代本节物理准确包的 FAIL，也没有读取系统剪贴板。

重开系统剪贴板测试前，先离线定位此失败；需要新的用户明确临时覆盖授权。下一次复制前后必须复用准确 package 已有 `ClipboardWatcher` 设置页，记录其可见的 master、active/paused/reason/error 与汇总 peer 状态；进程存活或固定等待不能代替 ready 证据。不得重试 private WinSta、CDP 或创建新的隔离桌面路线。

#### 下一次有界复测准备（未执行）

本轮用户已授权一次临时覆盖，但启动聚合命令在执行层 `CreateProcess` 前收到唯一可见原因 `blocked by policy`，没有启动 watchdog、Mac deadline job、package、master/grant 或任何系统剪贴板操作；审查文本为 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5h-4d36132/e5h-rejected-clipboard-window-launch-command.txt`。已准备的下一次窗口仍以 hidden window 启动 `C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5a-0c59\e5b-retest-watchdog.ps1`，传入不超过 18 分钟的 UTC deadline；它在启动和停止前校验 4d Windows portable EXE SHA256 `c22c3d2b...abbc4edd`，Mac 使用已核验 DMG `8145ede7...0b4b80c9` 提取的 4d App，同时继续使用 E5-A 的既有 profile/identity。它不处理剪贴板正文，到时只关闭两端 master/grant 并停止已核验 package。需要提前结束时只创建同目录 `e5b-retest-close-now.flag`，watchdog 随即执行相同收尾。候选、profile、master/grant 和独立截止配置正确即可运行标准消费者测试；可读的设置页状态仅补充诊断，Mac AX 不可读不阻止该受授权窗口。

窗口启动后写入一个已知文本并立即用对端标准系统消费者断言；可读的设置页状态在前后作为诊断证据。`ClipboardWatcher` 后端对象包含 `last.sequence` 与 send/receive ready，但准确包页面只显示汇总 peer 状态、且没有可用 CDP 外部读取入口，因此这些 raw 字段本轮标为不可观测，不新增调试功能。首项失败不原样重试：在同一 18 分钟窗口剩余时间仅进行有针对性的诊断或独立反向项，所有路径仍由 watchdog 自动收尾。

### E5-C：0c59 Windows 准确包键盘复测（UNRESOLVED，未发送按键）

`2026-09-19T15:24:36.1159385Z` 的只读 `OpenInputDesktop` 结果为 `Default`，已不再是此前的 `Screen-saver` 条件。随后以 SHA256 `97b3a63a...16e0e3e` 的准确 Windows package 启动 PID `80752`，主窗口可被 UIA 找到；测试在验证前台所有权时收到 `FOREGROUND_REJECTED`。安全门槛因此未发送任何 Tab/键盘事件，PID 于 `2026-09-19T15:26:35.2786935Z` 停止，clipboard master 和六项 grant 全程保持关闭。

该结果不能证明键盘顺序，也没有扩大为产品键盘缺陷。下一次仅需用户在 package 启动后点击其 Windows 窗口使其成为前台，再运行一次既有 Tab 顺序检查；不强制前台、不注入备用自动化路线。

### E5-C 补充：0c59 Windows 准确包后台 UIA 可达性（PARTIAL）

`2026-09-19T17:14:06.8479737Z` 使用同一准确 Windows package、`0c59ae255d631b496a3e483e9c558f2998ee314c`、SHA256 `97b3a63a122df148edbf5b3038b225bda84ad9e8d45b3bd0a5c8f598b16e0e3e`，在新建隔离 profile 中运行既有 `UIAutomationClient`/`UIAutomationTypes` 探针。脚本只调用 UIA navigation 的 `InvokePattern` 和读取的 `ValuePattern`/`TextPattern`，没有调用 `SetForegroundWindow`、`ShowWindow`，没有发送键盘事件、使用 CDP/新自动化框架或读写剪贴板。退出后核验 profile 路径在该运行目录和工作树内，再删除；测试进程已退出。回执位于 ignored `.artifacts/e5-windows-uia-reachability-20260920T011406Z/result.json`，脚本 exit 0。

设置导航的 Invoke 通过并显示“本地配置”；“自动剪贴板 同步与运行状态”和“保存通用设置”均导出 `InvokePattern`。设备导航 Invoke 通过并显示“跨网络配对”；“配对码” Edit 导出 `ValuePattern` 与 `TextPattern`，但没有输入任何代码或提交配对，且无服务/空代码下“生成配对码”“完成配对”保持禁用。传输导航 Invoke 也返回成功，不过探针只看到“传输视图/标签页栏”等 `SelectionPattern` 容器，没有导出名为“接收”的 `SelectionItem`，预期“接收状态”未出现；因此没有选择接收页、更没有开始接收。此结果证明后台 UIA 足以执行后续 U5 设置保存和已授权服务配对的界面导航，不能证明 Tab 顺序、接收流程或准确包网络验收；后两项仍分别需要前台点击或相应真实环境。

### E5-C 补充：0c59 Windows 准确包 U5 通用设置保存（PASS，局部）

`2026-09-19T18:01:54.8018470Z` 用同一 package 和 SHA256 新建隔离 profile 后，后台 UIA Invoke“设置”，找到“本机名称” Edit 的 `ValuePattern`/`TextPattern`，以 `ValuePattern.SetValue` 写入本轮临时 marker，再 Invoke“保存通用设置”。轮询并直接读取隔离 profile 的 `desktop-preferences.json`，marker 与 `device_name` 一致、`revision=1`；脚本 exit 0。回执为 ignored `.artifacts/e5-windows-uia-settings-save-20260920T020154Z/result.json`。

本轮没有调用 `SetForegroundWindow`、`ShowWindow`，没有发送键盘事件、使用 CDP/新框架或读写剪贴板。测试进程退出后，profile 删除前先验证其规范路径属于此运行目录和工作树，删除结果为 true。此项只验收通用分类的一次本地持久化；没有变更网络、接收目录、设备覆盖或自动剪贴板，不替代物理双机设置矩阵。

### E5-C 修复：剪贴板完整 lease 策略刷新（源码，非物理剪贴板结果）

`da53ed6028f6eb15ada925fbbb119b31bb74e3d6` 修复了 `562bf6b` packages Windows job 中唯一的 `image lease did not arrive` 失败窗口。受控回归先使两端各持有两张未过期 text lease，再仅由 receive:image 设置更新触发完整 text+image 策略；旧 text snapshot 在写出前被门闩阻塞，设置在旧写完成前不能提交，随后新 image lease 到达并继续既有大图路径。相同策略续租仍受两张 TTL 租约上限约束；取消的 lease 写入在 deadline/parent cancel 下释放权限锁。根 `GOWORK=off go test ./...`、root `go vet ./...`、受影响 race，以及 desktop `GOWORK=off go test/vet/build ./...` 均 exit 0。此为源码/loopback 证据：`562bf6b` 的 Windows packages 失败记录仍保留，尚未有修复提交的替代准确包或物理系统剪贴板结果。

### E5-D：U3 IPv6 与 mDNS 有界原型（PASS，非准确桌面验收）

荷兰 nl-highdefense 只有 loopback/link-local IPv6，故以下均在两个 disposable network namespace 的同一 ULA 链路完成；没有改宿主防火墙、路由、代理或启动常驻 DNS-SD 服务。两个 endpoint 通过 veth 位于 fd42:5d:1::/64，实验结束均删除 namespace、身份、profile、私钥、payload 和上传二进制，只保留脱敏拓扑、摘要和哈希。

IPv6 信令/ICE/QUIC 使用干净 detached 77ce6bf221740cba7c4588c0d55d66fc27835131 构建的 Linux amd64 CLI（SHA256 fe976c4fc0b3175d93e45046fc2798e02cee55e2825b88af74487c4bf84a6cd3）与 rendezvous（ffb3077de050188e5352f0a62926c4d5079dbe3c4a1765d4fe1c2ffbd9cd43ac）。manager job /tmp/codex-ssh/linksend-e5d-v6-quic-r3-20260920-20260919T161737Z exit 0：IPv6 literal SAN fd42:5d:1::2 的 HTTPS/WSS 经 openssl verify_ip 和 CLI 均校验成功；两个方向各传 1,048,576 bytes，a→b source/received SHA256 均为 8fcb03cc49c5723f401ae93cfc3a0420ebf3538f43e16259ca2084f750384fa7，b→a 均为 0ea1a3fe657735fbceaaea180b1367de58aeb80f1832a3fc7bbe12f96b3944f9。四个 DirectEvidence 均为 IPv6 host↔host、lan_direct、QUIC、TLS 1.3（772）、ALPN linksend/1、relay=false；每端信令计数约 1.7 KiB，未承载文件正文。脱敏回执位于本机 supervision e5d-v6-20260920T0012Z。

签名发现/LAN TLS 使用提交 4edb7422efbe7d723160d45e13a8e41676bf283f 的 test-only TestE5DIPv6DiscoveryPrototype，test binary SHA256 为 4f188a9d077fa38ad8635385971be6ebc62da148526fdbea058ea9bc763d763d。job /tmp/codex-ssh/linksend-e5d-ipv6-discovery-20260920-20260919T164804Z exit 0：A 端同一 Ed25519 DeviceID 从 fd42:5d:1::2 与 fd42:5d:1::4 发出真实 IPv6 multicast signed packet；B 接受两条签名 packet 并将其合并为一个 DeviceID、两条 route。B 随后通过其中的已发现 route 建立一次到 A 的 LAN TLS 控制连接并发送 heartbeat；A 作为 TLS server 验证 B 的发现公钥，B pin A 的发现公钥。双方验证 TLS 1.3（772）与 linksend/1。这是一条 B→A 已认证控制连接，不是双向独立连接，也没有由该发现触发 QUIC 或文件传输。

mDNS/DNS-SD 使用 Pion github.com/pion/mdns/v2 v2.2.0、项目固定的 golang.org/x/net v0.56.0。首次 job /tmp/codex-ssh/linksend-e5d-mdns-20260920-20260919T162655Z 在 browse_dns_sd 超时，保留为原型输入错误；修正 SRV target 的完整主机名后，job /tmp/codex-ssh/linksend-e5d-mdns-r2-20260920-20260919T163100Z exit 0。A 发布 _linksend._udp，B 成功 browse 到 AAAA/SRV/TXT 候选 fd42:5d:1::2:41001；veth A TX 由 1 增至 13、B RX 由 1 增至 13。TXT 仅含 candidate、proto，结论是 IPv6 同链路 DNS-SD 可提供候选 hint。

复现入口为 scripts/e5d-ipv6-quic-netns.sh、scripts/e5d-ipv6-discovery-netns.sh 与 scripts/e5d-mdns-netns.sh。前者运行双向文件和 IPv6 HTTPS/WSS/ICE/QUIC 分项；第二项运行 test-only 签名/route/TLS 原型；第三项运行 cmd/e5d-mdns-prototype。三者都要求 LINKSEND_ISOLATED_LAB=1、root network namespace 能力和本轮交叉编译 Linux binary。

当前不启用生产 IPv6 discovery 或 mDNS provider。internal/discovery.Manager 仍是 IPv4 listener/route/probe 实现；mDNS 的正确边界是未信任候选 hint，未来接入必须把候选送回有界的 LinkSend 签名 discovery 和 TLS identity pin 验证。以上不证明公网 IPv6、准确 Windows/macOS package IPv6、物理多网卡/DHCP/VPN/sleep 矩阵，且不改变 relay=false。
### E5-G：0c59 准确包 U1/U2/U5 合并验收（PASS，范围受限）

本批固定 Windows 准确包为 `0c59ae255d631b496a3e483e9c558f2998ee314c`，SHA256 `97b3a63a...16e0e3e`；Mac 发送端复用本页 E5-A 已核验的 arm64 准确 package。新的 Windows 隔离 profile 以唯一名称 `E5 U1U2U5 Windows 20260920T0236Z` 运行，其公开 DeviceID、加入时间、incarnation、文件摘要与撤销关联保留在受管回执 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-u1-u2-u5-20260920T0236Z/E5-G-EVIDENCE.json`；profile 在服务端撤销已核验后仍保留，未删除或覆盖用户文件。

- **U5 通用设置与配对**：后台 UIA 先以 `ValuePattern.SetValue` 写入该唯一“本机名称”，再 Invoke“保存通用设置”；隔离 profile 直接复核名称和 `revision=1`。重启准确 package 后，设备页的配对码 Edit 输入一次性码并 Invoke“完成配对”，页面出现“配对成功”。随后 CLI 只读身份/目录确认新设备为 `group_paired / membership_synced`。全程未设前台、未发键盘、未使用 CDP/新框架或读写剪贴板。
- **U1 一次默认接收**：Mac 准确 package 先只读刷新成员目录并确认该新 ID，再经 `macos_share` journal 向其发送一个 1 MiB 文件。Windows 准确 package 出现独立“接收确认”弹窗，文件名可见；“查看详情”保持折叠，仅 Invoke 一次“接收文件”，没有尝试旧“接收”Tab。目标接收目录预置 54-byte 同名 fixture，默认 `keep_both` 产出 `e5g-u1-keep-both (1).bin`，新文件 SHA256 与 Mac 源一致，原 fixture 摘要未变。Windows receive task `20260919T184030.415785300Z-00000001` 和 Mac send task `20260919T184029.353509000Z-00000001` 都为 `completed`，各自 verified/committed 1,048,576 bytes、0 retransmit；实际为 IPv4 host↔host `lan_direct`、QUIC、TLS 1.3、`relay=false`。这是准确 package 的一份默认来件、同名保留与哈希证据，不扩大为跨 NAT、应用重启续传或系统 Share Extension 通过。
- **U2 准确 UI 删除与不复活**：邀请方使用相同准确 Windows package 的设备页，以唯一显示名定位新设备行。该行“设置”导出 `ExpandCollapsePattern`，展开后“删除设备”和“确认删除”各通过 `InvokePattern` 一次。HK `hk-main` 的只读 SQLite 查询确认该精确 ID 的 `revoked=1`，并记录同一 incarnation 的 membership request 于 `2026-09-19 18:48:53 UTC` 推进 group revision 至 10。邀请方刷新后将其显示为 `removed / revoked / blocked=true`，不是活动组成员；被删除 profile 在准确 package 中重启超过 5 秒后保持同一 identity，未调用加入或修复路径，服务端记录仍为 revoked。两端 package 均已停止。
- 早先 `2026-09-19T18:15Z` 的另一台 U2 准确 package 配对后，初始 CLI 撤销只保留 exit 1；其 stderr、临时 identity/join/owner 快照和 profile 已按旧脚本删除，无法恢复稳定错误码，文档不将其推断为 `UNPAIRED`。后续以 HK 邀请 `used_at=18:15:06 UTC`、issuer、DeviceID、名称和 incarnation 精确关联目标，正常 `Service.Revoke` 返回 0，HK 只读查询为 `revoked=1`；非目标候选保留。该次脚本将本地 denied 目录项误当服务成员，已通过 `removed/revoked/blocked` 语义和服务端状态纠正，不将该脚本错误记为产品删除失败。

本批不读取、写入或恢复系统剪贴板；Mac manager 的一次 `tail-job` 返回 zsh `status` 只读变量包装错误，立即 `run-script` 回执、独立 Mac task-history 只读查询和 Windows task-history/文件摘要均成功，二者分列。当前 `4d36132` 的 core（2）、desktop（2）与 wails3-packages（1）五个 CI run 都为 success。

### E5-H：4d36132 修复候选重配对与设备设置（PASS，范围受限）

候选固定为已成功的 `4d361320bce1c0bb354a590fe0cc3c73be0242aa` packages run `35459981462`，不称为后续 `e065fe2` 或 `5038003` 的包。Windows artifact `10589072915` 的外层 ZIP 为 84,240,520 bytes / SHA256 `d0344809...c08f37cb`，portable `LinkSend.exe` 为 23,936,000 bytes / SHA256 `c22c3d2b...abbc4edd`；BUILD-INFO 的 source/workflow head 均为 4d、`source_state=COMMITTED`、`source_checkout_clean=true`。Mac arm64 artifact `10589072767` 的 DMG SHA256 为 `8145ede7...0b4b80c9`，已在实际 Mac 上核验 bundle ID、0.5.0、arm64 与 `codesign --verify --deep --strict`。

- 用该 Windows 准确包复用 E5-G 的已撤销 identity `f090...1969` 重新配对，页面反馈“配对成功”。HK 只读记录邀请码于 `2026-09-19T19:34:57Z` 使用，identity 更新为 incarnation `2f8e...1008`、`revoked=0`、group revision 11。未创建新身份或保留邀请码正文。
- 同一准确包的设备页以 `ExpandCollapsePattern` 展开邀请方设备设置。设备接收目录为空并继承全局；全局冲突策略为 `keep_both`；`device_profiles` 和 `clipboard_grants` 均无该设备记录；六个文字/链接/图片的 send/receive checkbox 全为 `Off`。这是默认权限与继承的准确包 UI/持久化读证据，未开启剪贴板主开关、grant 或系统剪贴板。
- E5-A Windows profile 的已配对 Mac peer 另行覆盖验证：以 `SelectionItemPattern` 选择“遇到冲突时停止”并保存后，该 peer 的 `conflict_policy=error`、revision 6；再选“继承全局设置”并保存后恢复为空、revision 7。全局目录和冲突策略均保持为空，因此设备仍继承全局；第一次 `ValuePattern.SetValue("skip")` 虽返回但未更新 React select，随即以 `SelectionItemPattern` 恢复，未留下覆盖。
- 设备目录的修正受管 picker 核验使用唯一测试名 `E5A Mac 0c59` 同行的 `ghost` “设置” `ExpandCollapsePattern` 进入编辑器。标题“选择接收目录”的对话框由本次 4d package PID 直接拥有，UIA 树在 7 次采样、97 个 descendants 后稳定；其标准控件为“文件夹:” Edit（AutomationId `1152`，`ValuePattern`）、“选择文件夹” Button（`1`，`InvokePattern`）及“取消” Button（`2`，`InvokePattern`）。随后同一入口以 `ValuePattern` 设置受管空目录并 Invoke 确认；返回应用后重取同一 PID 的 UIA 根仍未找到可 Invoke 的“保存设备偏好”，因此没有保存，SQLite 仍为目录/策略空、revision 7，package 已停止。目录覆盖验收为 PARTIAL；此前未加 owner/就绪约束的“控件不可达”记录不作为结论。
- 同一准确包再次精确 Invoke“删除设备”和“确认删除”各一次。HK 只读查询确认同一新 incarnation 为 `revoked=1`、group revision 12；专用 profile 仅在该确认后按规范路径删除。所有候选 package 进程已停止，证据保留在 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-u1-u2-u5-20260920T0236Z/E5-H-EVIDENCE.json`。
- Mac 的独立 AX 探针在自启动的准确 package PID、隔离 profile 和 loopback 服务下得到 `AX_TRUSTED=false` / `AX_WINDOWS_ERROR=-25211`。没有 AXPress、改值、系统权限修改、新自动化框架或剪贴板读写；这仅是未获用户辅助功能授权的外部条件，不是剪贴板失败根因，也不把 Mac 原生 U5/U7 控件列为已验收。
- 18 分钟剪贴板复测仅完成准备：现有 watchdog 的 Windows 所有权检查固定至 4d portable EXE `c22c3d2b...abbc4edd`，Mac 固定至已提取的 4d App 与 DMG `8145ede7...0b4b80c9`，两端继续复用 E5-A profile/identity。Windows PowerShell 解析和 EXE hash、Mac 三份清理脚本的 `sh -n` 与候选根/DMG hash/E5-A profile 锚点均通过；没有启动 watchdog、package、master/grant 或读写系统剪贴板。
- `e065fe2` 的 packages run `35462833862` 在 Windows `Verify core module` 中失败于 `TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists: fill issued text lease 1: CLIPBOARD_CONTENT_LIMIT`。调查确认是测试 fixture 与 7 秒续租竞争及满窗后等待 before-write hook，不改写为构建网络问题。`5038003` 只将 fixture 的满窗刷新和旧快照门闩分成两个受锁场景，未改产品/wire/TTL/MaxLeases；定向和受影响 race 已本地通过，随后 core（2）、desktop（2）和 packages（1）五个 CI run 全部 success。

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

后续兼容诊断源码候选为 `1fa21b4073e12d40e87dc6e23f3ec4d67e5e1819`。其 core push/PR、desktop push/PR 与 packages run `34802741572` 全部成功，Draft PR #8 head 与远端分支一致。新 artifact 元数据为：Windows `10332650029` / `sha256:9d87a778...90b4f`，macOS arm64 `10331629840` / `sha256:5e66dd3d...eb68f`，macOS amd64 `10332155493` / `sha256:b12d4882...3fe5d`。用户要求暂停时下载已主动停止，监督目标目录为空；因此这些只证明 GitHub artifact 来源和外层摘要，尚未完成 `BUILD-INFO`、包内 `SHA256SUMS`、Windows ZIP/EXE 或 DMG 载荷复核。下述准确包实机结果继续只属于 `387b57c`。

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

## 暂停时外部条件与恢复矩阵

| 未完成项 | 当前事实 | 恢复所需条件 | 恢复入口/首个动作 |
|---|---|---|---|
| `1fa21b4` 三平台包内核验 | CI 与外层 artifact 摘要 PASS；下载目录为空 | 下一次启动命令 | 从 run `34802741572` 下载到 `e5-ci-34802741572`，运行 `verify-milestone-packages.ps1`，不得继承 `387b57c` 载荷摘要 |
| Win→Mac 文件、Mac→Win 反向复用流 | 新 Windows 隔离 profile 已加入当前控制面；Mac 尚无配套 profile | Mac 唤醒或提供新地址，SSH input desktop 可用 | 用 `codex-ssh-manager` 解析 alias；确认新地址后再创建 10 分钟邀请并建立干净 Mac profile |
| 系统文本/链接/图片剪贴板及文件并存 | Windows general clipboard 非空且未读取/覆盖；私有 WinSta 已判不可用 | 两端可丢弃隔离剪贴板，或用户明确临时允许覆盖 | 只运行一次准确包双向矩阵；不恢复 private WinSta/CDP 路线 |
| Windows 原生 Tab 顺序 | 进程/console 在 session 8，但 input desktop 为 `Screen-saver` | 用户退出屏保，input desktop 回到 `Default` | 先只读确认 desktop 名，再运行一次真实 Tab 顺序；不以 DOM/CDP 替代 |
| 150% DPI、深色 | 当前物理 125%/light；准确 EXE 无运行期 CDP 入口 | 可使用的真实 150% 显示环境与系统深色主题 | 运行准确包原生布局检查并记录实际 DPI/主题；不修改当前用户全局设置 |
| 网络切换、睡眠/唤醒 | 源码/loopback 门闩通过，准确双机时间线未运行 | 双机同时在线且允许自然网络/睡眠事件 | 保留接口、session、任务 revision 与 hash 时间线，健康 QUIC 和重连结果分列 |
| 签名安装/系统共享激活 | 用户已暂缓；现有 HKCU/HKLM 安装受保护 | 新的明确启动命令改变该决定并提供相应签名条件 | 不覆盖现有安装、快捷方式或卸载记录 |

暂停时没有活跃本地下载、测试进程或远程实验 job。香港生产服务继续运行已验证的 `387b57c`/schema 4，荷兰 namespace 与敏感材料已清理；不得因恢复新包下载而重复部署或重做双 NAT。
