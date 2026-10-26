# LinkSend 桌面体验全轮 TODO

## 2026-10-26 优先：Windows跨网连接

最新版Windows双端公网在线但连接未验证优先修复。正式版main最小分支的edf7cac已交付Windows候选和[PR #12](https://github.com/Wen5555/LinkSend/pull/12)，默认备用STUN、内置主机安全解析及ICE失败统计已验证；准确干净源码验证器Windows↔公共JP Linux双向16MiB/中文/空目录、6/6落盘条目/hash、双方Completed通过，约3.3秒建链。准确Windows包来源/摘要/版本核验通过，但用户暂时无法试用，两台Windows现场和受限双NAT不因此打勾；吞吐仍偏低，原R0–R6未完成。见[跨网证据](evidence/CROSS-NETWORK-20261026.md)。

另行修复服务端把所有IPv4末尾255当广播的准入缺陷，并先Unmap防止映射地址绕过危险scope限制；真实签名WSS/forward回归通过，详见[准入证据](evidence/CANDIDATE-ADMISSION-20261026.md)。这是服务端代码，未替换生产或改宿主网络，不能通过只更新Windows候选生效。代码同步进入本可靠性分支；临时测试进程已结束、隔离peer已撤销。Mac SSH本次仍超时，台湾NAT别名不可用；不能把SSH失败当产品NAT失败。

## 2026-10-19 可靠性 自动缓存与临时分享 Goal（active，继续实施）

用户已明确启动[实施计划](RELIABILITY-AND-SHARING-PLAN.md)与[Goal提示词](prompts/RELIABILITY-AND-SHARING-GOAL.md)。缓存不增加手动清理或容量/期限设置；保存常用发送方案不做。已授权相关SSH及阶段commit/push，具体边界见计划；旧Goal和历史缺项不因本轮启动改变状态。

- [x] R0：核对 `f8358ce`、干净 tracked 工作区及三个既有未跟踪目录；复用本地 Go1.27.1/Node24.21.0/pnpm12.4.1；Mac arm64/macOS26.5 和日本自托管信令/STUN 主机 SSH 可用；修正 README 过期版本。见[基线证据](evidence/RELIABILITY-R0-BASELINE.md)。
- [ ] R1（实施中）：已复现并修复网络通知破坏文件空闲但剪贴板活跃的健康QUIC会话；新增真实LAN TLS/Pion/QUIC回归，通知后双向剪贴板及同会话文件复用/关闭清理通过。见[源码增量证据](evidence/RELIABILITY-R1-SESSION-REFRESH.md)；准确候选双向 LAN、中断恢复、睡眠/网络变化与真实不同网络传输仍待。
- [ ] R2（实施中）：[缓存ADR0016](adr/0016-automatic-clipboard-file-cache.md)已先冻结；落实真实manifest自动空间预留、并发容量保护、拒绝后残留清理及稳定空间原因/新复制恢复。自动所有权/回收、跨启动保护及 Finder/Explorer 真实键盘粘贴仍待；现有文件条目读回不抵扣。见[容量增量证据](evidence/RELIABILITY-R2-CAPACITY.md)。

  R2后续增量：已复现并修复原生结果不确定时删除可能已发布文件；接收/checkpoint借用固定Root，新增路径身份/空目录/Windows junction保护和准确不确定状态。见[路径与发布证据](evidence/RELIABILITY-R2-CACHE-PATHS.md)。ADR修正boot变化并不足以单独授权删除，原生读取结束证明仍必做。

  2026-10-26补充：真实CI发现磁盘压力后的新复制被同权限lease续期吞掉；屏障稳定复现旧实现FAIL，现只为当前本机观察选择仍有效重叠lease，普通10轮/race3轮通过，缓存回收/容量/期限/用户文件保护不变。准确新CI继续核对，旧eccb903/a22f691失败保留；见[证据](evidence/RELIABILITY-R2-OBSERVED-LEASE.md)。

  R2持久owner首批：同一SQLite连接保存带用途/身份/运行代次的签名记录，实际stage/file句柄来源在body前登记，publishing在原生写入前登记；损坏/冲突/旧运行不能重获发送或未发布删除权限。见[持久记录证据](evidence/RELIABILITY-R2-CACHE-LEDGER.md)。活动引用、启动对账/自动回收与原生完成证明继续。

  R2生命周期增量：活动引用持续到接收器及目的目录句柄关闭；关闭前观察实际存储，当前worker在native前封存确定未发布清单，启动/退出/剪贴板/周期/压力接入有界分页回收、删除意图与退避。未知/用户改动/可能已发布对象仍保护，超出完整封存预算不拒收正常文件。见[生命周期证据](evidence/RELIABILITY-R2-CACHE-LIFECYCLE.md)。大对象增量封存、未封存崩溃残留、原生读取结束许可与真实键盘粘贴仍必做。

  R2原生引用隔离原型：同进程、两个真实COM消费者与launcher退出后helper寿命三阶段通过，最后stream/clone释放前仍保护，源文件保留、系统剪贴板未改。helper本身仍是存活的COM producer；未接生产发布/回收，缺签名交接与安全inventory/持久receipt，不能把原始trace当删除许可或抵扣键盘粘贴。见[原型证据](evidence/RELIABILITY-R2-NATIVE-REFERENCE-PROBE.md)。

  R2后续私有基础：52f9bf2已实现receipt-only用途签名/实际inventory/跨main单次CAS，并修复collector信任剥离Native副本的误删入口；任何Native仍保护，未接生产helper或已发布回收。准确core/desktop/三平台包CI均attempt1/success；见[签名基础证据](evidence/RELIABILITY-R2-NATIVE-AUTHORIZATION.md)。不能将签名或CI当真实native读取结束证明。

  推送来源`1bc92e0`：core/desktop CI成功；包CI39172841244的Mac arm64旧直连时序断言失败，Windows/Intel Mac成功。因果回归改为扣住两端候选结束通知，Mac普通100轮/race50轮、独立负例及双模块完整矩阵通过，见[证据](evidence/RELIABILITY-R1-TRICKLE-CAUSAL-CHECK.md)。新准确提交CI继续核对，旧失败不抹去。
- [ ] R3（候选实施）：Go白名单原因/阶段/下一步、安全任务与记录DTO、实际尝试时间及本地JSON导出已实现。01b5c16原生保存/取消及任务导航已验，准确3d包再次实际导出失败任务安全报告；已用真实TLS/WSS因果门修复typed原因降级，legacy仅精确signaling phase获得安全指导，补固定P2P/限频提示。普通/race各3轮、前端171项与完整Go12矩阵通过；准确新CI/候选旅程与Mac仍待。见[诊断增量](evidence/RELIABILITY-R3-SHARE-SIGNALING-DIAGNOSTICS.md)及[原生观察](evidence/RELIABILITY-R5-NATIVE-GUI-20261026.md)。
- [ ] R4：准确Windows候选实际覆盖安装、profile逐字节保持、身份/草稿/历史/20MiB恢复状态、单实例唤回及原系统入口恢复已验；基线活动任务取消退出/保存退出通过。候选活动完整续传、SendTo/拖放和Mac替换/活动退出仍待，见[原生证据](evidence/RELIABILITY-R4-WINDOWS-UPGRADE.md)。
- [ ] R5（owner/数据面/桌面候选，整体待验）：[ADR0017](adr/0017-one-time-share-grant.md)先冻结的授权核心、显式备份迁移、签名HTTP及独立scope WSS已实现并验证；普通配置关闭，实验health的file_transfer_ready=false，不广告正式share_grants_v1。历史增量及旧失败保留在[核心](evidence/RELIABILITY-R5-GRANT-CORE.md)、[控制](evidence/RELIABILITY-R5-SIGNED-CONTROL.md)、[客户端](evidence/RELIABILITY-R5-SIGNED-CLIENT.md)与[协调](evidence/RELIABILITY-R5-SCOPED-SIGNALING.md)证据中，不将旧包或CI移记本候选。

  当前同profile签名/CAS owner、明确重启恢复、七天终态metadata回收、固定公钥Pion/QUIC两阶段预览及原V1引擎已接入。8MiB任务在4MiB暂停、双端重开Service后显式恢复原任务通过；同步Create/hash/HTTP退出取消及服务Completed不能替代本机Completion的受控失败均已修复，最终20项TemporaryShare测试race3轮通过。双模块workspace/off的test/vet/build共12命令全部exit0，前端19files/155tests及typecheck/lint/build通过。

  桌面临时分享源码与真实页面浏览器夹具已验证明确确认、长目录/指纹/分页及无自动派发；夹具不代表Wails RPC、原生picker/剪贴板或P2P实机。当前原生复制目标因未证实可备份格式SKIP；独立WindowStation尝试遭CreateWindowStationW错误5，native未运行且用户剪贴板未改。仅GitHub托管Windows可创建空夹具的严格CI入口已准备，准确新提交CI与包结果另验。Windows↔Mac、原生保存与最终旅程仍必做；不同网络Go核心双向证据见下段，不抵扣Windows双端现场；见[本轮证据](evidence/RELIABILITY-R5-OWNER-DATA-PATH.md)，不标R5完成。

  R5准确源码实网：`701e23b`干净验证器Windows↔公共JP Linux两个方向各16,777,247B、中文/零字节/空目录、树hash及双方本机/授权终态通过，永久trust/剪贴板权限0。控制metadata经专用SSH HTTPS/WSS通道，文件仍独立UDP/Pion/QUIC；测试服务/通道已关闭、临时凭据已移除。Windows正向后台进程退出码不可观察，明确保留；不是Win↔Win/双NAT、WinMac或Wails包原生证据，见[实网记录](evidence/RELIABILITY-R5-CROSS-NETWORK-20261026.md)。

  R5准确CI历史失败：701 core/desktop成功、Windows native因测试scanner读独占空锁失败；仅测试修复103后core/desktop成功，native又在未分阶段的会话就绪检查失败，当时未记PASS。103 Mac arm64另因sum.golang.org IPv6无路由在安装工具链时失败，Intel包成功。后续因果修复及cd真实成功见下段，旧失败不抹去，未强改用户剪贴板。

  R5托管原生子目标：cd95479的core/desktop/三平台包CI均success，无重试；Windows真实active/paused保护码与随后正常native文字QUIC目标7.54s、run1/pass1/skip0/fail0。初始注册/租约10ms、恢复租约6934ms；只覆盖该Go桥接/原生generation子目标，不抵扣Wails UI、文件键盘粘贴或WinMac。旧701/103失败保留，见[完整记录](evidence/RELIABILITY-R5-OWNER-DATA-PATH.md)。

- [ ] R6：触及职责拆分、版本一致性、同候选完整 CI/产物与资源收尾待完成。

当前接续点（2026-10-26）：Goal实际active。3d准确包/来源及52完整CI已核对；3d GUI真实领取/批准/导出与scene02/03失败均保留，未取得预览/Completion，不证明scope或用户现场根因。typed信令原因及安全提示已修复并完成同版12矩阵/171前端，准确新CI与包另核对；接续分离已批准对端人机等待与网络短期限，并继续真实WinWin跨网验证，详见[诊断证据](evidence/RELIABILITY-R3-SHARE-SIGNALING-DIAGNOSTICS.md)。R1用户现场/受限双NAT，R2完整回收/键盘粘贴、R4双平台完整旅程及R6仍必做。Mac07:14 SSH仍connect-timeout/255，未改宿主网络或把SSH不可达当产品NAT失败；旧审批拒绝保留原边界。

2026-10-19历史接续记录（保留当时状态，非当前下一步）：当时要求核对 `f2712dc` 包CI run `39165099505`（core `39165099512`/desktop `39165099516` 已success；Windows/Mac arm64包success，Intel Mac DMG仍运行），只读watch日志 `ci-39165099505.log` 后记录结果，不能把运行中记PASS。后续结果见PROGRESS对应R2路径记录。Mac补查job `/tmp/codex-ssh/job-ls-r2-mac-preflight-20261019-001` 已退出0，无远程运行中作业；系统boot标识API只测可读/短时稳定，未测重启。

后续核对：`f2712dc` 的 run `39165099505` 已completed/success，三个包job均success；前述“仍运行”为当时记录，不是当前阻塞。当前继续缓存持久owner和原生交接完成证明，不以boot/TTL清理未知发布对象。

## 2026-10-19 本轮剪贴板与0.6.2发布

本地候选已完成Windows↔Mac同LAN系统文字、独立像素图片和文件条目双向验证；64MiB复制、两端原生居中进度、取消及未完成缓存删除通过。最终两端备份恢复，测试进程停止。新文件同步须独立开启，≥32MiB才弹进度窗。最终文件完成状态绑定FileID的自动回归通过；物理结果仍绑定修复前状态展示候选包。用户已明确要求GitHub版本更新推送，已通过PR #11合并并正式发布[0.6.2](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.2)，准确源码63c86c2c5ce183e1400d27ca0571610d50a42bdc、三平台run 39157767929与六资产digest均通过；下方为历史状态，不代表仍在等待本轮实机。Explorer/Finder键盘粘贴、完整双NAT及其余A–E边界保持，见[本轮记录](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)。

## 2026-10-19 同一局域网自动剪贴板修复（本地候选）

LAN认证后立即启动剪贴板/文件流owner，移除剪贴板两层10秒额外截断，并清除恢复后残留的连接错误。新增真实LAN TLS控制+Pion/QUIC的同机隔离回归；双向文本、并发启动、文件复用及分阶段超时通过。现有GitHub更新改动保留，未提交推送或发布；本次物理双机OS剪贴板仍待实机试用，原A–E缺项保持。准确命令和产物见[实施记录](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)。

## 2026-10-18 新增桌面GitHub更新（本地已实现，待发布）

当前新增工作分支`codex/github-desktop-updates`基于main `3aa5148`，实现手动检查正式版、匹配本机安装包、下载进度/取消、SHA256校验和安装包入口；Windows安装向导/Mac DMG需要先退出旧程序后完成。源码/绑定/前端、本地生产EXE、真实GitHub下载与定向安全回归已验证，尚未提交推送或发布新版本。旧0.6.1包不会原位出现此入口，首次需安装含此功能的新构建；实际原生安装覆盖/重启未验。详见[实施记录](evidence/DESKTOP-GITHUB-UPDATES.md)。下方A–E与0.6.1历史状态保留各自范围。

## 2026-10-18 0.6.0后用户反馈修复

0.6.1发布准备沿用修复分支`codex/fix-desktop-workflows`，基线main `9d8334c`。四项源码已实现：文件打开/定位、自动剪贴板启动与恢复、删除与黑名单、统一收发队列/退出清空设置/红点。旧发布源与外部未验项保持；详见[本轮证据](evidence/DESKTOP-060-WORKFLOW-FIXES.md)。用户随后明确授权发布0.6.1；PR #10已合并main，准确发布源`72161e3f806d742d1d401564031ab4844fe88b2d`，三平台包run 39113644308和六项发布资产核验通过。不恢复被拒绝的原生旅程。

## 2026-10-18 A–E Goal（已启动，当前 blocked）

本聊天 Goal 已通过平台创建，2026-10-18 经 `update_goal` 返回状态 blocked；用户明确执行[目标文档](prompts/DESKTOP-AE-IMPLEMENTATION-GOAL.md)，整体未完成。起点 `638b2aa`，原工作分支 `codex/lan-rebuild-ae`，PR #9已合并main，当前交付分支main。不恢复旧代理/heartbeat；保留 `.playwright-cli/`、`apps/desktop/build/ios/`、`apps/desktop/build/linux/` 未跟踪内容。

| 工作包 | 实现 | 自动测试 | 原生/网络 | GitHub | 下一具体动作 |
|---|---|---|---|---|---|
| A LAN 独立主流程 | 端口/reader/记忆、刷新/provider/早期host完成 | Pion/并发回归通过 | 后测发现/刷新双方10/10；Mac反控0/10；新准确Win LAN启动被工具策略阻断 | 6edddad五项CI及三平台包success | [原生证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)：可用原生环境下继续独立旅程，不绕过策略 |
| B 新信令/客户端恢复/备份 | 服务/备份上线；显式恢复与schema3退休/UI完成 | 两模块/兼容/race；输入回归6FAIL→115前端GREEN | 6f29814准确Mac正式HTTPS不同URL恢复PASS；同URL空库原生prepare/cancel/重启/加入/旧队列代次拒绝PASS（Mac loopback） | 6f29814五项CI/三平台包与四资产核验success | [恢复证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)：Win同URL原生、物理与legacy原生输入继续；外部STUN/双NAT条件保持 |
| B2 LAN 性能/剪贴板/撤销/系统入口 | 既有实现待物理闭环；发现前后测完成 | 本轮未改源码，沿用6ec3cc4模块/119前端边界 | 6ec3cc4准确Mac Finder真实菜单两轮后台唤回/去重/重启/110B三文件及中文空目录loopback通过；物理/OS/性能仍待 | 6ec3cc4准确包已核验，本轮仅文档证据 | [原生证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)：继续拖放/Windows入口与物理、OS剪贴板、两方向各3次分段性能 |
| C / N1 目录预设 | Go筛选/预览、schema10草稿/队列、恢复v2与UI完成 | 既有完整模块/race/兼容/QUIC恢复及6ec3cc4模块/119前端通过 | 6ec3cc4准确Mac原生picker/源码预设/自定义排除与80B三文件/空目录集合hash PASS；旧Win保留6edddad来源 | 6ec3cc4五项CI/三平台四资产核验；本轮仅文档 | [N1证据](evidence/DESKTOP-AE-SOURCE-SELECTION.md)：可用收尾已完成，物理与新包恢复未追加 |
| C / N2 多设备分发 | Go原子批次、schema11/独立子项/取消/重启与UI完成 | 完整模块/race/100前端/兼容/真实QUIC三目标通过 | 6ec3cc4准确Mac双目标在线/离线、源变化只确认C、独立子项及活动批次取消、两次重启/直接恢复屏障PASS；同机范围 | 6ec3cc4五项CI/三平台包已核验，本轮仅文档；旧Win6edddad保持 | [N2证据](evidence/DESKTOP-AE-DEVICE-BATCHES.md)：物理双机仍待，不扩展重复矩阵 |
| D / N3 文件正文限速 | 共享可取消预算、动态偏好CAS与KiB/MiB界面完成 | 既有预算/race/QUIC/内存剪贴板夹具及6ec3cc4模块/119前端边界 | 6ec3cc4准确Mac原生256→1024KiB/s活动限速/32MiB hash、1KiB暂停/恢复/取消、活动改0完成4MiB、重启保持PASS；旧Win结果保留来源 | 6ec3cc4五项CI/三平台四资产核验；本轮仅文档 | [N3证据](evidence/DESKTOP-AE-FILE-BUDGET.md)：OS剪贴板并存/物理速率仍待 |
| D / N4 托盘快速发送 | quick/Go入队、菜单主点击/共享存储隔离与本机目标边界完成 | 两模块/off测试构建、119前端；本机/身份加载回归RED→GREEN | 6ec3cc4准确Mac self禁发/原生目标选择保存/picker/取消重开/45B QUIC及hash/后台重启退出PASS；0ae7cdf离线撤权保持来源 | 6ec3cc4五项CI/三平台包/四资产完整核验success | [N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)：Mac可用收尾完成，Win托盘与物理仍未验 |
| E 同提交候选/最终回归/文档 | v0.6.0发布源8bad932；旧分项保留原来源 | main core/desktop/三平台CI全部通过；版本同步检查覆盖遗漏Windows清单 | 发布前6ec3cc4 Mac N1–N4可用收尾保持原来源；v0.6.0新包仅来源/版本/架构/签名核验，物理未验 | PR #9已合并main，v0.6.0/8bad932四程序包及六发布资产核验success | [发布说明](RELEASE-v0.6.0.md)：保留完整A/B/B2/系统入口/双NAT缺项，不扩展重复测试 |

用户2026-10-18最新明确授权合并main并发布v0.6.0，覆盖此前本任务不合并/不正式发布的边界；已正常合并PR #9并公开发布[v0.6.0](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.0)，tag/新包源码为`8bad9323ece46b2e32054ddb321340dfdd1e3ec8`、main run 39103713740，版本和六项发布资产摘要核验通过。保持不force-push。只有目标全部必交付证据满足后才标 complete。下面规划与旧里程碑保留原日期及证据边界，以上为本轮唯一当前工作包状态。

2026-10-18 用户最新要求：不做过多测试，优先功能可用；随后明确选择把上述四项现有功能收尾到可用。后续验证收敛到构建、关键操作与必要的一次实际传输；复用已有通过证据，不扩展重复矩阵，未验证的网络/原生范围仍如实保留，不新增F09等范围。

阻塞收束：四项功能交付、随后阻塞复核及本轮只读复核中，相同Windows原生执行条件/JP UDP补充授权均未变化，当前无可继续的独立实现事项；平台Goal已标blocked而非complete或paused。本轮Windows LinkSend进程数为0，preset专属服务job仍finished/exit0，工作树无跟踪文件改动；未重试被自动审批在CreateProcess前拒绝的隔离LAN启动，也未改宿主防火墙/路由/代理。下一步需Windows原生执行条件解除后继续一次最小真实双机闭环，跨NAT还需可用拓扑及JP UDP3478精确补充授权。用户随后已明确答复Windows限制“尚未解除，保留未验项”；不再将该问题列为待答，不重复询问或启动测试。恢复时先核对本页、Git和实际外部状态，不重跑已完成四项矩阵。以下旧active记录保留历史含义。

最近准确发布包：**v0.6.0**，源自主分支`8bad9323ece46b2e32054ddb321340dfdd1e3ec8`，run 39103713740；全部源码CI/三平台构建通过，六资产GitHub digest与本地SHA256一致。Windows/Mac包均为新构建0.6.0，不重标旧包；本次无新物理包验收，旧原生证据继续绑定下述来源，完整Goal保持blocked。详情见[发布说明](RELEASE-v0.6.0.md)。

发布前原生候选源码：`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`，五项CI/三平台包/四资产来源、架构与hash核验success。四项功能可用收尾已完成：准确Mac N1原生目录/源码预设/自定义排除及80B三文件/中文空目录实际集合hash；N2双目标在线离线/源变化单项确认/独立与批次取消/重启屏障；N3活动256→1024KiB/s、1KiB等待暂停恢复取消、改0完成；N4真实菜单/目标原生选择保存/picker取消重开/45B QUIC/后台重启退出。Finder真实菜单/110B闭环也已记录。均为同机范围，不报物理或双NAT。0ae7cdf离线撤权、6f29814恢复、98469ce旧31B及6edddad旧Win仍绑定来源。全部新app/receiver/fixture终态、监听关闭、clipboard=false；Windows策略/托盘、物理/OS/双NAT和JP UDP3478授权条件保持。

本地终态：N2/N3/N4浏览器夹具已停止，19346无owned Vite监听，临时入口/元数据配置移除；最新Go/前端本地作业已终态。原.playwright-cli与ios/linux保留，未更改日常profile、OS剪贴板或宿主网络。阶段记录已通过Orion writer备份/回读/收据和增量索引，项目/index SHA一致；其余无关pending未处理。

运行现场：最新Mac根`/Users/wen/.codex/supervision/linksend-ae-20261018/preset-native-6ec3cc4`；app真实菜单退出、CLI receive与专属rendezvous均exit0、监听关闭。clone最终2queues/2tasks均completed（含原各1）、0batch、main revision5空且source/*.tmp规则保持；quick原生清空/选回B为revision6，内容除revision/timestamp恢复相同；rate=0、clipboard=false/grants=0，身份/偏好/全部源与目标hash/原profile/私有备份保持，两库quick_check=ok。收据`.artifacts/ae-20261018/mac-preset-6ec3cc4-final-receipt.json`。前述batch/budget/Finder/quick及更旧现场均终态并保留，batch的预期SOURCE_CHANGED failed历史未覆盖。用户手动短码已核对且未提交，不再等待输入；Windows启动策略与Mac录屏未授权保持。

早期原生候选（历史检查点）来源：`d5412642e49460413fdda249f77d03647075f41c`，五项workflow success，包run `39060667017`。测量器与基线同步提交 `d70d923`。修复前发现harness十轮已收束；Mac反控失败不能随批次exit0标PASS。Windows/Mac准确包隔离profile启动、LAN监听/Windows状态读取已核验；Mac辅助访问拒绝、Windows UI未形成可验证导航，未做配对/文件旅程。owned Windows PID135152、Mac PID31256均已停止，剪贴板master=false、接收目录为空。

服务现场：`linksend.oooai.de` 已指85.149.217.165并仅该hostname启用Cloudflare strict；后端127.0.0.1:8787，源站/公共WSS及重启通过。b8ad809的standalone备份修复已事务安装；v2完整恢复及最新撤销快照恢复均exit0，主库未替换，8788测试监听已停止。主服务最终PID283864、备份timer active。首轮root sidecar失败保留，不覆盖历史。

公共信令最新复验：本轮Windows/Mac各一次严格`https://linksend.oooai.de/healthz`均200/TLS verify0；首次Windows误用`/health`返回426，更正后200，未当作服务异常或WSS通过。此前两平台正常Initialize/Join/邀请码重放拒绝/两路认证WSS/同身份verify与6edddad准确Mac正式主页“可以接收文件”的结果保持各自来源；旧TLS超时未确认根因。此前JP只读信令/STUN-only/备份timer active；没有新外部STUN/双NAT声明或宿主修改，见[信令证据](evidence/DESKTOP-AE-SIGNALING-REBUILD.md)。

B/STUN现场：18a88f2/eb421f9已推送。新独立STUN-only服务本机Pion srflx/Binding/TURN负例通过，User=turnserver、PID285596、67/96 tasks、约7MiB；标准coturn.service保持masked/inactive。首轮TasksMax64导致auth线程创建失败，已安全停用后准确unit事务修复，证据/回退见信令重建文档。外部Windows/Mac UDP无响应，JP UFW只开放TCP22/80/443；已异步询问仅UDP3478授权，答复前不改规则/不切STUN DNS。旧默认STUN在Windows可取得srflx，Mac假IP/TUN公网路径仍失败；不替代双NAT文件。固定Node24.21.0/pnpm12.4.1及嵌套命令已核验，86前端测试和其余检查通过。

B客户端：显式intent/API/UI、已退休context永久拒绝、首次DELETE前live-actor检查和完整快照/成员操作所有权已实现；独立LAN、LocalBlock、identity/文件/历史/偏好保留。schema3精确备份及旧库拒绝通过；正常新码重配保持，旧队列/撤销不自动重绑。核心off/desktop/前端90tests、并发race五轮等已通过，最终identity/app完整race终态exit0（app122.250s），e43043d已推送且五项workflow/三平台包success；[证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。客户端原生与新服务实际恢复旅程仍待准确包，不能标B整体完成。edb67ef、STUN资产修复eb421f9的五项workflow均success，18a88f2 package被后续并发策略cancelled。

B来源记录：Git内部导出的两个server二进制仍被Go标记父仓库dirty，不用于部署。已在Git仓库外重建 `rendezvous-d541264-export-linux-amd64`，源归档SHA256 `13d69b85062a2a2dcba506081bfa7648b1df7b70fca93d09a8f415610868af6b`，二进制SHA256 `688702887cf7b87351d81384e941a352e13c3cd4701e437757f0922241e83d45`；Go metadata为(devel)，提交/归档另有明确来源。首次部署/恢复及DNS已完成，原preflight仍保留独立历史。

> **2026-10-18 A–E Goal提示词：** 用户要求撰写覆盖A、B、B2、C、D、E所有任务的执行提示词，并明确授权较大改动完成后阶段commit/push。见[完整执行提示词](prompts/DESKTOP-AE-IMPLEMENTATION-GOAL.md)。N1–N4均列必做；本次撰写/同步提示词不等于已启动功能实施，明确执行后再更新本Goal状态。

> **2026-10-18 前提与P0修正：** 用户确认原信令数据已丢失，改为日本猫猫机重建新控制库和显式重新配对；此前“迁移原组/旧数据库”方案已被替代。局域网独立发现、同意/配对、双向传输及性能优化列为P0，测试必须覆盖信令/STUN不可达，不以新服务完成为前置。完整开发/测试范围见[重建方案](DESKTOP-RESUMPTION-PLAN.md#4-信令服务重建与客户端恢复)和[LAN主线](DESKTOP-RESUMPTION-PLAN.md#5-局域网独立开发与优化主线p0)。本次仍为规划修订。

> **2026-10-18 功能规划补正：** 本轮规划不只验收。具体开发行为、建议新增N1目录预设/N2多设备分发/N3限速/N4托盘快速发送，以及逐项测试见[功能建设清单](DESKTOP-RESUMPTION-PLAN.md#3-功能建设清单与实施顺序)。现有托盘/草稿/队列/收件箱继续复用；一次性临时分享列独立后续批次。N编号仅用于本规划功能映射，不表示已实施或用户已接受新增范围。

> **2026-10-18 规划修订（尚未恢复功能实施）：** IPv6 暂缓；信令目标改为日本猫猫机 `85.149.217.165`，Mac 当前 LAN 地址更正为 `192.168.1.243`，两机均已只读 SSH 连通核验。按[修订计划](DESKTOP-RESUMPTION-PLAN.md)先固定环境/准备迁移与收敛 U3，再完成新入口的同版包联合验收；签名/公证仍暂缓。以下旧地址、Goal、任务归属与历史待办保留原日期边界，不据此自动执行。

> **2026-09-23 用户终止实施并要求主分支推送、整理日志。** 当前 Goal 已 `paused`，子代理已中断，旧 heartbeat 仍 PAUSED；不再自动恢复或执行待授权的宿主规则实验。主分支收录已验证源码 `8fcc76d5059ab49b5d7cb81415e4a37885bc7abf` 及本次收尾文档，U1–U7 未全完成。最后 U3 候选没有进入主分支：保留在 `208a/Osend` 原实施树，并按 SHA256 备份至 `D:/apps/Osend/.artifacts/astra-stop-20260923/`。恢复须另有明确指令；见 E5-X / DELIVERY。

> **2026-09-22 恢复实施：** 用户明确要求当前 Astra ultra 接管；旧停止指令已由本次恢复覆盖。实际接管 HEAD 为 `6eb23bab94e2a3d9fe615c0adceb1509e28494e0`，与远端/开放 Draft PR #8 一致，既有七个 check 均成功。preview.2 产物仍绑定 `0229cbf3e8373c544431102ed904c4a19ed02dc4`；旧包验证不迁移到新源码。Mac 本轮暂不可用，香港/荷兰可用；U1–U7 未全部验收，Goal 保持 active。

创建日期：2026-09-13；当前负责人：`01a0c8e2-a09d-79f0-9947-2c346172bfc9`（local，Astra ultra 当前任务）。旧执行者/旧总控仅作历史来源，不再派发；任务内子代理按文件与机器资源分工，由主代理统一集成和 Git 同步。
执行树：`C:/Users/Wen/.codex/worktrees/208a/Osend`。工作分支：`codex/desktop-experience-upgrade`；原总控 `01a096c4-2e59-7db3-a7a8-ec2a125409d6` 和旧执行任务均 idle，旧 `linksend` heartbeat 已 PAUSED。
基线：`3bcb73019d1ac6de1d9f341bb7d22e2813875081`。E4 自动剪贴板源码范围已由总控在 `3363fd5+387b57c` 验收，当前执行 E5 准确包、物理双机、恢复、部署与网络联合验收。

依据：[用户执行提示词](prompts/DESKTOP-EXPERIENCE-GOAL.md)、[完整方案](DESKTOP-EXPERIENCE-IMPROVEMENT-PLAN.md)。
七项均为必交付：U1 一次接收；U2 删除与配对；U3 发现可靠性；U4 移除手动内容并自动剪贴板；U5 设置；U6 真正系统共享；U7 有界四视图。

## 生命周期与证据规则

### 当前最短实施路径（2026-09-22，沿用原稳定 ID）

1. **E4-03 / U4：源码已验证。** `7d3bfb8` 修复双方准备就绪但复制次数不同时的新事件被误判；loopback RED→GREEN，lease 因果时钟、兼容/重放/跳跃上限/高时钟续租及 race 通过。物理 Mac 首条失败仍未复验。
2. **E1-02 / U2：源码已验证。** `1849c31` 修复 `group+lan` outbox 和离线关系；真实服务断线→重启→恢复同步/新 incarnation/保留文件历史及 race 通过。准确 preview.2 的隔离 Windows 启动被工具自动审批在 CreateProcess 前拒绝（`blocked by policy`），GUI 未启动；fixture 已正常停止，UI 包级结果仍 NOT RUN。
3. **E3-01/02/03 / U6：三平台源码自测已验证。** `535a77c` 修复满 64 条记录时重试误报容量；两个适配器先按 ID/内容去重，再对新请求限额。Windows 自测 RED→GREEN，`096d79a` 的 Windows/Mac ARM/Mac Intel packages 自测全 PASS；签名和正式系统激活仍未验收。
4. **E5-02：阶段已同步，准确候选已核验。** `096d79a` 与 `36302a9` 各自七个 CI jobs success，三平台四包均已完整下载并分别核对 hash/COMMITTED来源/平台与版本，见最新 DELIVERY。双 Go 模块（含 GOWORK=off）、受影响 race 与 Windows 构建均通过；最新 `36302a9` 包不含后续 E5-V 的“继续撤销”UI。历史包验证不迁移到后续源码。
5. **E5-01 / U5 支撑：跨主机验收 FAIL，诊断已同步。** `8c6e1e8` 七个 CI jobs 全 PASS；新同版单轮在 HTTP 525 / signaling_connect 退出，未进入 QUIC，旧 `096d79a` 握手失败仍未定位。源站同时段存在 Cloudflare 网段的 TLS read timeout，但无请求关联证据。独立短期 HTTPS 入口的 HK 本地验证通过、NL 唯一访问超时，未启动配对/文件；全部实验资源已收尾，生产和宿主网络保持，各轮来源与限制见 E5-S/T/U。不可将新诊断、旧双 NAT 或本机测试替代仍失败的实际路径。
6. **E1-02 / U2：组版本冲突源码及 CLI 现场收尾已验证。** `36302a9` 的 helper、actor 持久绑定、legacy 明确新意图/CAS 已完成 RED→GREEN、完整双模块/race/Windows 构建及独立安全审查。准确 CLI 通过正常 API 撤销 NL legacy pending，新的明确请求/原 incarnation/actor 绑定持久为 revision 22；HK 独立复核两目标 revoked、原 23 条设备和 owner 不变。没有手改 trust/DB；准确桌面 GUI 的离线删除仍 NOT RUN。
7. **E5-01 / 网络后续：宿主规则变更待明确授权。** 只读核对证实 HK 外部 NEW TCP80 会落现有 nftables INPUT 默认 DROP。一次仅 NL 单一 IP→HK TCP80、600 秒自动撤销的 runtime 规则提案已写 E5-U，并按提示词第 6 节的授权排除向用户单独询问；没有执行规则或新探针。此条件只涉及隔离 HTTPS 入口，不将它归作旧 QUIC 故障的根因。
8. **E1-02 / U2 桌面入口：阶段已同步。** `6cb0705` 的 pending 显示、确认“继续撤销”、成员变化与键盘焦点已完成 82 tests/typecheck/lint/build、desktop 双模式/race/vet/Windows build、Playwright 页面夹具与七个 CI jobs。新三平台 artifact 来源与日志摘要已核对，未下载新载荷；准确桌面包仍待独立验收。最终纯文档提交复用该源码检查，见 E5-V / DELIVERY。
9. **E1-05 / U2/U3 事实状态与恢复：阶段已同步。** `8fcc76d` 已修复发现即“可达”、未知即“离线”、握手前/暂停 SessionID 误作 connected，以及 LAN-only 已配对/已有认证连接被入队误拒；Windows 补齐原生地址/路由通知。定向 RED/GREEN、根 workspace/off 完整 tests/vet/build 与完整 app race（166.576s）、desktop 双模式/race/vet、前端 86 tests/typecheck/lint/build 和 Windows Wails build 全 PASS；七个 CI jobs 全 success，三平台新 artifact 来源/日志摘要已核对，未下载新载荷。当前准确包/物理项仍未验收，详见 E5-W/DELIVERY，不借用旧候选验包。
10. **E1-04 / U3：按用户要求暂停，未合入主分支。** 本地候选包含固定 UDP 临时端口降级/恢复、最多 32 路探测和当前 Manager 的记忆重探。定向测试及中间集成通过，但审查最后补的旧 reader 状态写入原子性修复尚未完成最终 GREEN/RACE，整批不作为主分支已验证成果。原文件、可逆补丁、逐文件备份和 SHA256 均保留；若以后明确恢复，应先验证该准确候选，再处理刷新 burst/抖动、设备页触发及桌面 provider 诊断。当前不继续派发。

香港/荷兰均完成只读接管核对，无未收尾旧作业。香港仍是已核实 `387b57c`、schema 4、health PASS；本批没有更改服务端源码或部署。Mac 暂不可用、用户暂缓签名及本轮 Windows 启动策略拒绝分别记录，不能把它们合并成“所有工作阻塞”。旧下面账本保留各自历史来源，以本节及最新追加 evidence 为当前状态。

生命周期为：待办 → 执行中 → 待审查 → 验收通过 → 阶段已同步；外部阻塞单列原因与可继续工作。
只有当前主代理根据证据判定验收通过。提交、测试通过、原生通过、网络通过、GitHub 同步互不替代。
各验证列使用 NOT RUN / PASS / FAIL / PARTIAL，非适用项须解释 N/A；未运行一律 NOT RUN。
每份 evidence 记录精确提交/dirty 状态、包 SHA256、OS/架构、真实拓扑、步骤、实际命令/退出码、日志、失败/限制和下一步。
包级结果不能继承源码测试快照；loopback、指定地址协议探针、真实桌面与双 NAT 分开记录。

## 稳定验收项

| ID | U 要求 | 阶段 | 依赖 | 改动范围 | 验收条件 | Evidence 路径（相对 docs） |
|---|---|---|---|---|---|---|
| E0-01 | U1–U7 | E0 | 无 | 准确包、源码、工具链与复现矩阵 | 固定 M5 包与新源码提交及 SHA256；七项逐条复现，严格区分源码审阅与准确安装包结果 | evidence/DESKTOP-E0-BASELINE.md |
| E0-02 | U2/U3 | E0 | E0-01 来源核对 | Windows/Mac 只读网络证据、香港与荷兰审计 | 当前接口/地址/路由/监听；发现→配对→控制→双向发送→hash；香港 health/认证/在线列表分列；荷兰真实拓扑及隔离双 NAT 可行性，指定地址探针不等于桌面通过 | evidence/DESKTOP-E0-NETWORK.md |
| E0-03 | U6 | E0 | E0-01 来源核对 | Win11 Share Target 包级最小原型 | 真实 ShareOperation/StorageItems、身份/签名、冷启动/交接、源权限及安装卸载回执；开发自签仅证明原型 | evidence/DESKTOP-E0-WINDOWS-SHARE-PROTOTYPE.md |
| E0-04 | U6 | E0 | E0-01 来源核对 | Mac Share Extension .appex 包级最小原型 | 真实 App Group/entitlements、文件表示期限、后台冷启动及扩展退出后可读；明确实际凭据/系统授权缺口 | evidence/DESKTOP-E0-MAC-SHARE-PROTOTYPE.md |
| E0-05 | U2/U3/U4/支撑 | E0 | E0-01；结合 E0-02/03/04 证据 | ADR、PROTOCOL、SECURITY、SPEC、产品范围 | 冻结配对/撤销/同步作用范围、成员版本及旧端拒绝、LAN 同意事务、剪贴板权限/冲突/TTL 与会话调度；总控审查 | evidence/DESKTOP-E0-DECISIONS.md |
| E1-01 | U2 | E1 | E0-05 | 配对服务、控制库、身份与桌面流程 | 有效码直接配对、首台初始化、幂等与明确跨组切换；普通成员仅组内权限；重放/跨组负测 | evidence/DESKTOP-E1-PAIRING.md |
| E1-02 | U2 | E1 | E1-01 | 撤销/设备删除、连接、重试、同步 | 先持久撤销全部既有组/LAN grant，再关闭权限/连接/重试；幂等组删除、离线待同步；重启/旧快照/WSS/LAN/旧控制会话不复活；新配对不恢复旧LAN/免确认/剪贴板；保留文件历史 | evidence/DESKTOP-E1-REVOCATION.md |
| E1-03 | U2 | E1 | E0-05 | LAN 独立添加/同意事务 | 被连接端确认；拒绝/超时无残留，nonce/身份绑定、双向幂等；服务失败状态真实 | evidence/DESKTOP-E1-LAN-PAIRING.md |
| E1-04 | U3 | E1 | E0-02 | 发现 providers、地址/接口/回程与缓存 | 多接口多地址、独立降级、明确源/回程、记忆地址退避、容量/过期/socket 重建；核实 LocalSend 一手来源并记录双栈/mDNS 选择 | evidence/DESKTOP-E1-DISCOVERY.md |
| E1-05 | U2/U3 | E1 | E1-01/02/03/04 | 统一目录、状态、网络/睡眠恢复 | 事实状态；健康 QUIC 不随 WSS/发现重启取消；准确包真实双向矩阵 | evidence/DESKTOP-E1-DIRECTORY-RECOVERY.md |
| E2-01 | U1 | E2 | E1-01/02/03/04/05 | Go 默认接收命令、接收计划与 UI | 原子默认接收、默认保留两份、一次点击；代际/撤销/空间/安全落盘；恢复原计划；免确认保存失败独立反馈 | evidence/DESKTOP-E2-DEFAULT-RECEIVE.md |
| E2-02 | U5 | E2 | E0-05；E1 授权模型 | 设置 API、设备覆盖与分类 UI | 分类设置、每设备继承、revision 局部更新、统一保存反馈；并发不丢字段、不打断健康任务 | evidence/DESKTOP-E2-SETTINGS.md |
| E2-03 | U4 | E2 | E0-05 | 旧内容入口、迁移、历史管理 | 移除手动文字/链接/图片创作；旧草稿/队列显式处理且不自动续发；历史/收到文件/快照可管理 | evidence/DESKTOP-E2-LEGACY-CONTENT.md |
| E2-04 | U7 | E2 | E2-01/02/03 | 传输/记录/设备/设置四视图、有界列表 | 首屏选文件/目标/发送；1100×720、960×640、125%/150%、中文长名、深色、键盘验证 | evidence/DESKTOP-E2-LAYOUT.md |
| E3-01 | U6 | E3 | E0-03；E1-01/02/03/04/05 | Windows 正式共享适配器与设备面板 | 真实可用设备；当前用户 IPC、持久去重、文件授权、独立共享请求 | evidence/DESKTOP-E3-WINDOWS-SHARE.md |
| E3-02 | U6 | E3 | E0-04；E1-01/02/03/04/05 | Mac 正式扩展与设备面板 | App Group/进程身份、临时表示接管/bookmark、后台唯一 owner、真实设备 | evidence/DESKTOP-E3-MAC-SHARE.md |
| E3-03 | U6 | E3 | E3-01/02 | 双平台准确包与安装生命周期 | 准确身份与签名/权限；冷启动/后台/主窗打开/连续共享/扩展退出继续发送；安装升级卸载 | evidence/DESKTOP-E3-NATIVE-PACKAGES.md |
| E4-01 | U4/支撑 | E4 | E0-05 | profile 存储所有权、会话与调度 | 先测量再收敛；提交屏障、认证会话复用、流级取消、有界文件和剪贴板调度 | evidence/DESKTOP-E4-STORAGE-SESSIONS.md |
| E4-02 | U4 | E4 | E4-01 | 权限、原生监听/写入、能力协商 | 默认关闭；按设备/方向/类型双端授权；Windows AddClipboardFormatListener、Mac changeCount；正文不经 JS/WSS | evidence/DESKTOP-E4-CLIPBOARD.md |
| E4-03 | U4 | E4 | E4-02 | 自动同步状态机、资源与托盘 | 文本/链接/图片、防回环；origin sequence与Lamport分离、串行落板revision门闩；预签发lease拒绝首帧/整图/重传晚到，续期不续旧事件、重连旧lease无效；快复制/离线不补发/锁屏/撤销/限额/暂存回收/托盘暂停 | evidence/DESKTOP-E4-CLIPBOARD-BEHAVIOR.md |
| E5-01 | U1–U7 | E5 | E1–E4 全部待总控验收完成 | 同提交候选联合验收 | 七项联合、文件/恢复/异常、旧版升级/混合版本、网络与原生实机矩阵；准确包 hash/来源/OS | evidence/DESKTOP-E5-ACCEPTANCE.md |
| E5-02 | U1–U7 | E5 | E5-01 | 工程文档、PR/CI、产物与部署回执 | 最终工作分支 PR/CI、包来源、部署/备份/回滚证据齐全；只有总控判定全部达标后结束 Goal | evidence/DESKTOP-E5-DELIVERY.md |

## 独立状态账本

E0 准备记录：[DESKTOP-E0-PREPARATION](evidence/DESKTOP-E0-PREPARATION.md)。正式 E0 已派发；各验证结果按实际完成更新。

| ID | 生命周期 | 代码 | 自动测试 | 原生 | 网络 | GitHub | 总控判定/阻塞 |
|---|---|---|---|---|---|---|---|
| E0-01 | 阶段已同步 | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PASS | draft PR #8 / 1518e31，core/desktop/三平台包CI全PASS；见分项记录 |
| E0-02 | 执行中 | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PASS | 已有网络证据复用，真实双向与双 NAT 缺项不改写 |
| E0-03 | 外部阻塞 | PARTIAL | PARTIAL | PARTIAL | NOT RUN | NOT RUN | 构建/签名PASS、系统安装FAIL、激活NOT RUN；不降格U6 |
| E0-04 | 待审查 | PARTIAL | PARTIAL | PARTIAL | NOT RUN | 执行中 | 总控接受2342009原型IO修补；服务4097与文件交接仍未通过，见MAC-SAFEIO证据 |
| E0-05 | 验收通过 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 仅ADR语义：总控E0-safeio-close-v1接受11c44df；具体API/schema/实现及兼容测试仍待E1/E4 |

| E1-01 | 源码验收通过 | PASS | PASS | NOT RUN | NOT RUN | PASS | 总控已接受 `666cbaa` E1 后端范围；准确包配对 UI/物理矩阵留 E5 |
| E1-02 | 源码验收通过 | PASS | PASS | NOT RUN | NOT RUN | PASS | 总控已接受 `666cbaa`；准确包离线 outbox/重启同步仍未实机 |
| E1-03 | 源码验收通过 | PASS | PASS | NOT RUN | PARTIAL | PASS | LAN 同意/恢复源码已接受；旧物理 LAN 仅 PARTIAL，准确 package 系统确认留 E5 |
| E1-04 | 部分验收通过 | PARTIAL | PASS | NOT RUN | PARTIAL | PASS | IPv4 主路径源码已接受；E5-D test-only 双栈/mDNS 原型已通过但生产 provider 未启用，物理多网卡/准确包IPv6/睡眠矩阵未闭合 |
| E1-05 | 源码验收通过 | PASS | PASS | 源码构建PASS/准确包NOT RUN | NOT RUN | PASS | 事实目录、原生事件与5秒快照已接线；准确包事件延迟、网络/睡眠矩阵未闭合 |
| E2-01 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | 复用E1 | PASS | 默认计划读取设备覆盖/全局同名策略；普通/免确认一致，恢复计划不变；准确包联合验收留E5 |
| E2-02 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | schema7设备策略继承、全局策略、字段级dirty合并、deferred-save和CAS重试已验收 |
| E2-03 | 源码验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | 旧草稿可见/取消、队列直达且不自动续发；E4 自动剪贴板源码已接受，物理同步留 E5 |
| E2-04 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | 完整外壳960×640主按钮可见可点；准确包原生DPI/键盘与双机UI留E5 |
| E3-01 | 源码验收通过 | PASS | PASS | 源码构建PASS/系统激活NOT RUN | NOT RUN | PASS | `c205b12` 源码范围已接受；签名/包身份系统激活按用户暂缓 |
| E3-02 | 源码验收通过 | PASS | PASS | arm64源码构建PASS/签名激活NOT RUN | NOT RUN | PASS | `c205b12` 源码范围已接受；签名/App Group 系统激活按用户暂缓 |
| E3-03 | 源码验收通过 | PARTIAL | PASS | 构建材料PASS/准确包系统生命周期NOT RUN | NOT RUN | PASS | 安装布局/manifest 源码接受；签名、系统安装与激活按用户暂缓 |
| E4-01 | 源码验收通过 | PASS | 定向/race PASS | N/A | loopback QUIC PASS | PASS | 总控已接受单一 SQLite owner、QUIC 复用、流取消与生命周期门闩；物理网络留 E5 |
| E4-02 | 源码验收通过 | PASS | 定向/race PASS | Win监听PASS、Mac命名pasteboard PASS | N/A | 387b57c五组checks/三平台包PASS | 总控已接受schema9权限、原生watcher、生命周期FIFO与单owner；准确包用户剪贴板留E5 |
| E4-03 | 源码验收通过 | PASS | 定向/race/前端PASS | Win真实HWND隔离写读PASS、Mac命名pasteboard PASS | loopback QUIC PASS | 387b57c五组checks/三平台包PASS | 总控已接受固定worker/latest pending、deadline/cancel/reset、singleflight、非对称generation与写入错误状态；物理Win↔Mac留E5 |
| E5-01 | 执行中/待复审 | PASS | PASS | PARTIAL | PARTIAL | PASS | `0c59ae2` 的 E5-G 已通过；4d36132 的 E5-H 已完成准确包重新配对、策略覆盖后恢复继承、六项默认关闭、精确删除与 HK revoked 收尾；设备目录系统 picker 已按 package PID/控件树核验，受管目录覆盖已保存（revision 8）并经“继承全局目录”恢复（revision 9）。E5-B 首条 Windows→Mac 系统文本仍 FAIL；链接、图片、反向、防回环和文件并存未运行，窗口已关闭且两端 grant/master 均已关闭。网切/睡眠、DPI/键盘仍待 |
| E5-02 | 阶段已同步/待最终验收 | PASS | PASS | PARTIAL | PARTIAL | PASS | 已核对 `560f6f2` 五项 CI success；E5-A/B/C 证据已推送。仅最终物理/系统矩阵未闭合；未合并 main、未移动 tag |


## 当前交付对账（2026-09-19）

| U | 已接受范围 | 尚缺的完成证据与依赖 |
|---|---|---|
| U1 一次接收 | E2-01 源码、计划/策略/恢复约束；E5-G 的准确 package 默认“接收文件”弹窗、`keep_both`、双端 1 MiB hash PASS | 断线/应用重启中的接收恢复、更多目录/异常矩阵仍待；本次不依赖剪贴板授权。 |
| U2 配对/删除 | E1-01/02/03 主路径源码；E5-G 准确 package UI 配对、精确删除、HK `revoked=1` 与 profile 重启不复活 PASS；E5-H 用同一已撤销 identity 显式重新配对后由 4d 准确 UI 再次删除/服务端清理 PASS | LAN 反向现场路径、离线 outbox 和更多系统确认矩阵仍 PARTIAL。 |
| U3 发现/恢复 | IPv4 主路径、原生事件与快照源码；E5-D test-only IPv6 signed discovery/LAN TLS 与 mDNS 原型 | 生产 IPv6 discovery/mDNS provider 未启用；物理多网卡、准确包IPv6、DHCP、VPN/TUN、睡眠矩阵也未执行，均须在相应真实环境和操作下完成。荷兰隔离 namespace 仅验证原型，不能替代物理双机矩阵。 |
| U4 自动剪贴板 | E2 旧入口迁移、E4 源码 | 准确包物理同步首条 Windows→Mac 文本 FAIL；用户已允许本轮临时覆盖，但启动聚合在执行层被 `blocked by policy` 拒绝、窗口未启动；其余双向文本/链接/图片、防回环和文件并存待可实际执行的同范围有界窗口。 |
| U5 设置 | E2-02 源码/浏览器范围；0c59 准确 Windows package 的两次“本机名称”后台 UIA 保存/持久化 PASS；E5-H 4d 准确包确认设备目录继承、策略覆盖后恢复继承、全局 `keep_both`、六项 clipboard 权限默认关闭及受管系统目录 picker 的标准控件可达 | 文件接收、更多设备覆盖及物理双机设置矩阵仍待；Mac AX 未获用户授权，不扩展自动化。 |
| U6 系统共享 | E3 源码和非签名构建 | Windows package identity/系统激活、Mac App Group/Share Extension 注册与安装生命周期；用户已暂缓签名/信任条件。 |
| U7 四视图 | E2-04 浏览器范围、部分准确包 125%/小窗证据 | Windows Tab 顺序需用户点击准确 package 置前台；150%/深色需要用户提供相应系统显示/主题条件，Mac 原生控件矩阵未运行。 |

U3 独立原型已在荷兰隔离 namespace 完成，范围为 IPv6 签名发现、TLS 控制、QUIC 文件分项与 DNS-SD 候选 hint；没有重做双 NAT、改宿主网络或扩产品 UI。后续只在明确产品收益与签名 hint 验证入口成立时评估生产 provider。

## 执行边界与恢复入口

- 当前批次为 E5 联合验收；E4 自动剪贴板源码范围已由总控验收通过。签名相关由用户暂缓，准确包 DPI、双向网络、双机 UI 与物理用户剪贴板按实际证据推进。
- 授权含阶段工作分支/PR、两平台项目依赖及隔离安装、香港 LinkSend 支持组件事务部署、荷兰构建与专属 namespace/container NAT。
- 不合并 main、不正式 Release、不购买签名服务、不改宿主防火墙/路由/代理、不影响无关业务。
- 远程全部使用 codex-ssh-manager resolve/probe/audit；2026-09-19 当前 Mac 地址为 `10.234.35.5`，保留登记 alias、密钥与 host key 校验。复杂任务使用 durable job，断线先 resume/tail；历史证据中的旧地址不改写。
- 生产先验证可读备份，再最小修改、独立验证和明确回滚；只使用已核实提交，旧数据库不能覆盖新业务写入。
- 保护用户 profile/文件/剪贴板；保留无关未跟踪目录并逐路径暂存，不使用 git add .。
- 根/desktop 两模块分别验证（含 GOWORK=off），前端与原生/网络结果独立。协议/schema 变更先文档与兼容安全测试。
- 本批 E5-A 已在 `10.234.35.5` 完成 resolve/probe/audit 和受管 remote jobs；HK 健康生产服务继续保持 `387b57c`/schema 4，未重新部署。系统剪贴板保持不读不覆盖；用户已允许同范围双端临时使用，但本次执行层拒绝了窗口启动，未产生 package 或 profile 副作用。

### E5-J 4d 准确包中断/重启恢复（2026-09-20）

- [x] 固定 4d361320bce1c0bb354a590fe0cc3c73be0242aa 的 Windows/Mac 准确包与载荷摘要；1a46564 只记录为证据工作树。
- [x] Windows 到 Mac 正常 256 MiB 文件及 SHA256 验证，路径为 lan_direct / QUIC / TLS 1.3 / ALPN linksend/1。
- [x] Windows sender 在至少一块已验证且整体未完成时中断；同一 sender task/TransferID 经重启后的恢复传输 UI 进入新 attempt，最终 1 GiB SHA256 一致、双方确认完成、0 retransmit。
- [x] Mac 的中断 checkpoint、恢复后缺失 1,065,353,216 bytes、同一 TransferID/manifest/selection/receive plan/target directory 和最终 SHA256 已核验。Mac 连接恢复会创建新的本地 receive task/attempt，文档不把它表述为双端 task ID 不变。
- [!] 单次 Mac 到 Windows V2 activation 为 0-byte QUIC_HANDSHAKE_FAILED；保留负例，不重试，不影响 Windows sender 恢复结论。
- [!] 测试 peer 接收目录清理被执行层 blocked by policy：SaveDeviceProfile helper 在执行前未运行，受管目录仍为 revision 12；用户 profile 未触碰。不得标注为已恢复继承。
- [ ] 物理网络切换/睡眠恢复、Mac 到 Windows 反向握手根因和系统 Share Extension/App Group 激活仍需独立验收。

### E5-K（NOT PASS）
- [x] 4889cbe handoff/diagnostic 与五项 CI；[!] 新反向 V2 在 ice_checking/CHECK_TIMEOUT、0 bytes 失败，未重试。


### E5-L：4889cbe 反向 QUIC 写路径诊断（NOT PASS）
- [x] 复用固定 4889cbe Windows/Mac 包和现有隔离 profile，完成 baseline V2 request：ICE 两端 selected/Connected，但 Mac sender 在 quic_handshake/QUIC_HANDSHAKE_FAILED、0 bytes 终止；持久受限分类为 udp_socket/0x41/EHOSTUNREACH。
- [x] 仅 Mac 设置 QUIC_GO_DISABLE_ECN=true 进行第二轮；仍得到 QUIC_EVENT_OPERATION=write_EHOSTUNREACH 和 0 bytes。该对照说明禁用 ECN 后仍复现，不能归因于 ECN 单因素；它不排除 WriteMsgUDP/sendmsg 或内核/链路原因，也不是把 ECN 永久关闭作为产品修复的依据。
- [x] 两轮均保存无凭据 ICE/QUIC 白名单日志、任务安全快照、source hash 和 PID 收尾回执；Windows 没有 incoming task，因此未 Invoke 接收。两端 clipboard master=false，测试 peer 六项 grant 全 disabled。
- [!] 无 sudo 改动的权限检查显示：tcpdump 可执行但 sudo -n 要求密码；当前 Mac 用户对 /dev/bpf0..3 无读写权限。因此头部观察 BLOCKED_BY_EXTERNAL_ENV，没有抓包、pcap、全机过滤或权限变更。
- [ ] 供总控/用户审阅的最小头部观察草案，未执行：在 Mac 的现有管理员权限终端中，仅在一次新的受控双机 4889 attempt 期间运行下列 90 秒/80 包有界命令。它只把 UDP/ICMP 的时间、IP、端口、长度和 ICMP 摘要写到终端；没有 -X、-A、-w，不保存正文或 pcap：
~~~sh
sudo sh -c '/usr/sbin/tcpdump -i en0 -nn -q -l -s 96 -c 80 "(host 10.234.35.5 and host 10.234.16.254) and (udp or icmp)" & p=$!; (sleep 90; kill -INT "$p" 2>/dev/null) & g=$!; wait "$p"; r=$?; kill "$g" 2>/dev/null || true; exit "$r"'
~~~
  观察仅用于区分 QUIC datagram 是否离开 Mac、是否收到相关 ICMP 拒绝；不得把没有样本当作成功或失败原因。双 IP outer filter 不会显示第三方网关发出的 ICMP 引述错误；没有匹配样本不能排除网关拒绝，暂不扩展观察方案。执行前仍需总控确认该用户侧权限窗口。

### E5-M：4889cbe Windows→Mac 接收端应用重启恢复（PASS）
- [x] 固定 workflow 35475890292 的 4889cbe Windows/Mac 准确包，并复用受管 1 GiB 源 e5j-win-to-mac-restart-1g.bin（SHA256 5fd17c10b75c7052a339acb81ff9ee8884e9b08d4d7d8763bd52b5cd39f4a0ae）；未新建或复制 GiB 文件。
- [x] 正确 fixture 使用 e5a-0c59/windows/profile（sender identity 83c68b7afd4698cc8affca4f0aa67c2460521c7875f9ce326b7807b9d670561f），Mac 既有该 identity 的 auto_accept。此前错误 profile 达 awaiting_acceptance、0 bytes，已停止且不计入恢复证据。
- [x] request a6faacd12d62457b86acb09bb217acfd：Mac 仅在新 TransferID 的 verified 4 MiB、received 8 MiB、committed 0 checkpoint 后由受管监控 SIGTERM；sender 随后为 recovering/connection_interrupted。UIA 仅 Invoke 恢复传输，不 Invoke 重新发送。
- [x] sender task 和 TransferID 保持一致并进入新 attempt；最终双方 verified/committed 均为 1 GiB，sender retransmitted 4 MiB、receiver retransmitted 0、bilateral_confirmed=true，lan_direct/QUIC/TLS 1.3/ALPN linksend/1，目标 SHA256 与源一致。
- [x] Mac 恢复任务只接收缺失 1,069,547,520 bytes；初始/恢复 receive task 的 TransferID、manifest、selection、receive plan 和目标目录一致，binding_consistent=true，receive plan digest 为 191827a88f13e4d09a2ea34afa9b8eec484437d77a3d83eb3a20fd99c4dd0415，最终无意外第二副本。
- [!] 两次旧同名 TransferID 监控漏列/时序错误和首次 UIA 参数名冲突均保留为无效基线；前者没有中断，后者没有 UI action，均不计入通过。已终态的旧受管 activations 可逆归档，不删除源或用户文件。Mac→Windows write_EHOSTUNREACH 仍是独立反向故障。
- [x] 两端 package 按 owned PID/path 停止；clipboard master=false、enabled grants=0。最终回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5m-4889cbe\e5m-final-machine-receipt.json。

### E5-N：4889cbe 小文件与空闲性能基线（PASS，样本范围有限）
- [x] 新增受管 fixture 为 3 个连续 1 MiB 文件和 12 个 512 KiB 批量文件，共 15 个、9,437,184 bytes，低于 32 MiB 上限；未重传已有 1 GiB 样本。Windows→Mac 4 次 run 均为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1、双方确认、0 retransmit，Mac 只读核验 4 个 completed task、15 个目标文件和全部 SHA256，未见 E5N 重名副本。
- [x] 实际端到端 elapsed：连续 1 MiB 为 3,588.242、1,312.396、1,856.090 ms（n=3）；12×512 KiB 批量共 6 MiB 为 2,509.479 ms（n=1）。样本不足，不报告 p95。
- [x] 空闲主进程各采 12 点、2 秒间隔：Windows 22.221 秒窗口的 CPU 秒差为 0.328125，即单逻辑核平均 1.476627%，working set 51,552,256–52,989,952 bytes、private 65,372,160–66,588,672 bytes；Mac 22.195 秒窗口的 RSS 恒为 130,514,944 bytes，ps 平滑 pcpu 为 0.1–0.7%、均值 0.316667%。两端均不含 WebView/helper 子进程，两个 CPU 口径不可直接等同。
- [!] 准确 package 没有端到端 discovery timestamp，也不暴露单独 DB commit 定时边界，均记为 NOT_OBSERVABLE；当前 n=3/n=1 不称 p95。初始 Mac 采样成功后 SSH-manager 的 tail-job 读回 helper 触发 zsh status 变量问题，未为绕过它新增样本。

### E5-P：4889cbe 海量小文件基线（PASS，1000 文件边界）
- [x] 一次单批目录传输：新增 1000 个 1 KiB 专用文件，合计 1,024,000 bytes，低于 4 MiB 上限；不重跑 E5-N 的 1 MiB/12 文件样本。产品 file_count=1001，因为它把所选根目录与 1000 个普通文件均计为条目。
- [x] sender/receiver 均 completed、verified/committed=1,024,000、bilateral_confirmed=true、retransmitted=0，lan_direct/QUIC/TLS 1.3/ALPN linksend/1。Mac read-only 逐文件 SHA256 复核 1000 个目标文件，mismatch=0；完整 1000 项 source hash manifest 保留在 E5P 最终回执。
- [x] sender task persisted start 至 bilateral completed 为 35,000 ms。activation 入队至完成的墙钟未被初始 monitor 持久化，entry 被消费后不可回补，明确记为 NOT_OBSERVABLE，不能用 journal 文件时间猜造。
- [x] 空闲主进程观测均小于 180 秒：Windows 15 点/28.309 秒，CPU 秒差 1.671875、单逻辑核平均 5.905756%，working set 51,404,800–53,501,952 bytes、private 64,425,984–66,347,008 bytes；Mac 12 点/22.181 秒，RSS 130,482,176–130,514,944 bytes、ps 平滑 pcpu 0.1–0.9%、均值 0.316667%。仅主进程，不称长时稳定性或全应用资源。
- [!] 首个 sender monitor 把 product file_count 误按为 leaf file count，完成后才恢复回执；首个 Mac read-only 查询误按 manifest_summary 精确等于目录名。两者都是收集假设，未重传、未改变传输或补造时钟。两端 package 已按 owned PID/path 停止，clipboard=false、enabled grants=0。

### E5-N 收束后的剩余必交付项

仍可自主完成的具体下一步：
- [x] 4d19adce4bcc759afedeca87f02670e5e235b250 的 core×2、desktop-wails3-checks×2、wails3-packages 五项 CI 已全部 success；restart-recovery race 用例的本地有界重复验证通过。
- [ ] 性能后续观察（非本里程碑新增必交付）：长时空闲/GC、发现延迟和独立 DB commit。E5-P 只覆盖一个 1000 文件目录批，不外推百万文件；当前准确包没有 discovery/独立 commit 时钟，NOT_OBSERVABLE 只表示本轮无测点，不表示完成或外部权限阻塞；不为补测点单独修改包。存储并非完全无证据：`history_test.go:327` 的 `BenchmarkTaskHistoryConnectionLifecycle` 测量 `persistRecord → SQLite autocommit` 全路径，E4 的 100×3 结果 2.140/2.176/2.190 ms/op 含序列化、锁和 SQL，不能写成单独 `tx.Commit`。

### E5-Q：U2/U7 有界源码收敛（源码 PASS，准确包待后续候选）
- [x] paired / nearby / removed 三类目录按名称、别名与完整 identity 查找并独立分页，每页最多 10 行；详情稳定绑定 DeviceID，目标消失后收起、页码钳制并回焦搜索。
- [x] 抽屉 header/footer 固定、正文独立滚动；关闭、保存、删除和二次确认可达。Chromium source harness 已核对 960×640 与 1100×720 的长中文名、无水平溢出、Tab/Shift+Tab 闭环。该结果不替代准确 Windows DPI、macOS AX 或系统主题验收。
- [x] 草稿 paths、队列、活动任务每页最多 10 项；enqueue 继续发全量 paths，跨页 ReorderQueue 继续传完整 pending IDs，动作使用稳定 ID，页码在后台删除/完成后钳制。收件状态只在顶栏并导航至既有“文件接收”设置；`no_content` 不列入进行中。
- [x] 前端验证：`pnpm test` 8 files / 76 tests、typecheck、lint、build 全部 PASS；本地源浏览器检查不是准确 package 结果。

待恢复的原生验收及现场、权限或签名条件：
- [ ] U2 离线删除：旧 4889 包因 UIA 可达性限制未执行删除；preview.2 已包含目录搜索、分页和固定详情操作区，但新准确包尚未复验。下一次启动先检验新包可达性与 pending-sync→恢复同步，不把旧 UI 限制自动列为新包必须现场的阻塞。
- [ ] E5-L 反向 Mac→Windows 写路径根因：最小两 IP UDP/ICMP 头部观察仍需 Mac 现有管理员权限终端。双 IP outer filter 不显示第三方网关的 ICMP 引述错误；无匹配样本不能排除网关拒绝，不扩大观察方案。
- [ ] Windows 原生 Tab 顺序：只缺用户把准确包窗口置前且 input desktop 为 Default 后的一次真实 Tab 检查；不强制前台或改用 CDP。
- [ ] 150% DPI/深色：只缺真实 150% 显示与系统深色环境的准确包布局观察；当前用户全局显示/主题保持不改。
- [ ] 系统剪贴板双机矩阵及与文件并存：用户已有临时覆盖授权，但执行策略仍拒绝窗口；只缺可执行的隔离/可丢弃 clipboard 窗口，不需再次授权或读取日常 clipboard。
- [ ] 物理网络切换、睡眠/唤醒：只缺双机同时在线且允许自然事件时的接口、session、revision 与 hash 时间线；不修改路由、代理或防火墙。
- [ ] 受信任系统安装、Share Extension/App Group、Developer ID/公证：用户已暂缓，需要相应签名身份、安装决定和凭据；现有受保护安装与快捷方式不覆盖。
