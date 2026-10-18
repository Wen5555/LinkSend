# 2026-10-18 准确包原生检查点与取消选择器修复

分项证据，状态仅在[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。物理双机、系统托盘、双NAT与原生loopback分别记录。

## 6ec3cc4准确Mac N2原生故障矩阵（2026-10-18）

同一准确arm64包原生扩展为B/C两个软件目标并明确等待，真实picker/一批原子入队后B完成74B、C离线等待。源变化触发C SOURCE_CHANGED失败且B保持；真实重启后只原生确认C，重新等待的接收器完成新98B/hash/双方确认，B未重复发送。另16MiB活动批次中原生独立取消等待C不停止B，再取消剩余批次使B取消且无提交文件；前一批完成文件保持。

取消后真实重启四张表整行不变/无自动派发，GUI释放owner后直接Resume被BATCH_CANCELLED拒绝。终态2批/5queues（3completed/2cancelled）/5tasks（3completed/1预期SOURCE_CHANGED failed/1cancelled）、main revision6空/quick原revision4保持。四receive（两个完成exit0、两个保护/取消中断exit1）与app/专属服务均终态，监听关闭、clipboard=false/enabled grants=0，原数据与源版本/备份保留。准确来源/角色边界、IDs、源hash、一次性接收器重开及失败观察器见[N2证据](DESKTOP-AE-DEVICE-BATCHES.md)；无新产品源码/构建或物理通过声明，收据`mac-batch-6ec3cc4-final-receipt.json`已下载。

## 6ec3cc4准确Mac N3活动预算（2026-10-18）

同一准确arm64包的原生256→1024KiB/s动态保存与32MiB实际文件通过；每阶段两个稳定4MiB确认区间约253/1005KiB/s，排除过渡与合并ACK。1KiB/s等待时原生暂停/恢复新attempt/取消、遗留待确认队列显式取消及独立4MiB等待中保存0及时完成/hash/双方确认通过。真实重启后0/quick/旧历史保持、无重复派发；完整口径、源码/EXE/文件hash、主动中断CLI exit1、失败观察器与收尾见[N3证据](DESKTOP-AE-FILE-BUDGET.md)。

本轮无产品源码改动，仍绑定6ec3cc4已核验五项CI/三平台四资产；没有新增物理或OS剪贴板证明。最新隔离根`budget-native-6ec3cc4`的app/四次CLI/只读采样器/专属rendezvous均终态、监听关闭、clipboard=false/enabled grants=0。最终4queues/4tasks（3completed/1cancelled，含原各1）、main revision7空、quick revision4保持，原profile/源/备份保留，完整收据`mac-budget-6ec3cc4-final-receipt.json`已下载。

## 6ec3cc4准确Mac Finder Services原生闭环（2026-10-18）

本轮无产品源码修改。准确包仍为`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`及其五项success CI/四资产核验，来源见[N4证据](DESKTOP-AE-QUICK-SEND.md)。实际arm64 EXE SHA256 `e13f0391f5074996c74fdd7e6ebedd241f0966612a827f9ce676cef39cee3e87`；新独立根`/Users/wen/.codex/supervision/linksend-ae-20261018/finder-native-6ec3cc4`复制准确app和已停止的前一测试profile，启动前无其他LinkSend进程。原profile/数据库/文件保留，仅在新clone进行系统入口旅程，私有备份仅在Mac。

实际打开专用Finder目录`LinkSend-6ec3cc4-入口验收`。System Events只操作该精确窗口，实读确认`plain file.txt`、`服务 中文.txt`、`目录 空格`三项AXSelected=true；后者内含`空目录`。先原生关闭owned app主窗，窗口数0/同PID仍在，再点击Finder“访达→服务→使用 LinkSend 发送”。服务项确实可见且enabled，实际点击后同PID唤回一个窗口；生产入口日志/Go owner把三路径合并进main，原held-main.txt、目标及筛选规则保持，quick整行、旧queue/task整行不变。这是实际Finder菜单动作，没有调用Go回调/CLI入口或NSPerformService代替。

第二次执行同一实际菜单旅程，main revision2→3，仍只有四条唯一来源；同PID从零窗口再次唤回，UI真实显示四来源。真实原生菜单退出并重启准确app后，main revision3、quick与旧queue/task整行、身份/偏好字节保持，SQLite quick_check=ok。最初外部信令使用已关闭loopback测试URL，入口合并/后台唤醒与重启不依赖信令可用。

为继续验证入口后的真实文件路径，使用原已停止夹具控制库的一致性副本和独立B profile副本，在专属loopback同URL启动既有rendezvous；未更改生产服务/原库/宿主网络。临时rendezvous由与6ec3cc4相同core/cmd源码编译（已核对git diff），EXE SHA256 `faa1eeb49589973d213ff317bbf0b5c48c1169729469aa75628e12af8314f292`；启动命令`rendezvous-6ec3cc4-darwin-arm64 --config finder-server.toml`。正常配对CLI接收端仅允许loopback，源码/EXE SHA同N4测试工具；桌面自然重新连接，实读目标B在线、发送可用，未在UI显示信令连接错误。

真实点击主窗口发送按钮，唯一新queue `75038f41b618ff7e305163216a73e935`、request `23e416c4e067be30d6e0022892df89d3`、sender `20261018T042243.587864000Z-00000001`、transfer `fe7fa7cb80ce761e6585e2b3efdc0bae`。经正常Pion/固定身份QUIC完成110B、verified/committed110、重传0、bilateral_confirmed=true、relay=false/direct_unknown；CLI completed/exit0，selected_files=3/selected_entries=5。实际接收恰有以下三文件，源/目标hash一致，且`目录 空格/空目录`结构存在并为空：

| 文件 | 字节 | SHA256 |
|---|---:|---|
| held-main.txt | 38 | eb2adf72e84bb5e0ce1613f1d76311504bbbc218a98c93787cf15cb869b8d6a5 |
| 服务 中文.txt | 36 | e64ec2eae302eb711d8d51786005fb7e814401533a394fd46cc386db0ac91c52 |
| plain file.txt | 36 | 714ddf46fcdab3fc02eb8abf780bbbb344e9c6a71a7bd720d189c4ccbe8d2bfd |

原main来源在系统合并/重启阶段完整保持，仅在上述明确发送成功后按正常行为清空为revision4；quick revision4原空行保持。旧completed queue/task整行仍在，最终共2条completed queue/2tasks。App原生菜单退出，专属rendezvous正常SIGTERM退出、receiver exit0、监听关闭；只关闭专用Finder窗口，测试文件/clone/原profile/私有备份保留，身份/偏好/全部源hash保持、clipboard=false/enabled grants=0、两库quick_check=ok。

保留失败：首轮准备因task BLOB不能直接JSON序列化，在app启动前停止；改为BLOB hash摘要后复核原现场续行，没有覆盖用户数据。第二次菜单检查原先只等路径存在，旧路径已存在导致读到revision2/窗口尚未唤回；没有重发菜单，只读核对新revision3和同PID窗口1后通过。复制已停止控制库的首轮SQLite backup出现unable to open database file；核对原库40960B/无WAL/quick_check=ok、目标仅0B空文件后，以原库immutable只读快照完成复制，原库未变。首次健康检查随后因启动未成功而停止，receiver尚未启动；所有失败都保留原job日志，不记为产品失败或PASS。

最终收据`.artifacts/ae-20261018/mac-finder-6ec3cc4-final-receipt.json`已下载。关键job前缀`/tmp/codex-ssh/job-ls-ae-mac-`：`finder-start-reconcile-6ec3cc4-20261018-01`、`finder-inspect-6ec3cc4-20261018-01`、`finder-select-services-6ec3cc4-20261018-01/02`、`finder-services-submenu-6ec3cc4-20261018-01/02`、`finder-service-click-6ec3cc4-20261018-01`、`finder-repeat-reconcile-6ec3cc4-20261018-01`、`finder-restart-6ec3cc4-20261018-01`、`finder-signal-start-6ec3cc4-20261018-02`、`finder-signal-health-6ec3cc4-20261018-02`、`finder-receiver-6ec3cc4-20261018-01`、`finder-presend-inspect-6ec3cc4-20261018-01`、`finder-send-6ec3cc4-20261018-01`、`finder-final-cleanup-6ec3cc4-20261018-01`均exit0。`finder-service-click`第二次script因过早窗口检查exit1，其已送达的第二次激活随后只读复核成功，不重复调用。

范围：准确Mac已有Finder Services实际菜单、两轮后台唤回/去重、草稿/重启、入口后普通原生发送与同机QUIC/实际目录文件。不是物理双机、Windows SendTo、拖放、冷启动到日常profile、正式Share Extension激活、性能或完整B2通过；这些必验/暂缓边界分别保持。

## f23383d 来源和 CI

提交 `f23383dc12945df42bc095cd9a2adc8dc011e7c0`，分支 `codex/lan-rebuild-ae`，[Draft PR #9](https://github.com/Wen5555/LinkSend/pull/9)。通过 `gh run list --commit <SHA>` 与 `gh pr view 9` 回读：core push/PR `39081829479/39081832198`、desktop push/PR `39081829495/39081832379`、[packages39081829568](https://github.com/Wen5555/LinkSend/actions/runs/39081829568) 全部success，PR head一致。

实际运行 `scripts/fetch-milestone-artifacts.ps1` 和 `scripts/verify-milestone-packages.ps1`，外层artifact digest、内层资产SHA256、BUILD-INFO/source SHA、COMMITTED/clean、平台和0.5.0版本通过。报告 `.artifacts/ae-20261018/pkg-f23383d/PACKAGE-VERIFICATION.json`。

| 资产 | SHA256 |
|---|---|
| Windows portable | `cb41a771fb97eb105ecc8e13ff6e689dbd846fa1fbaf8aed3e4d3e09189f5e04` |
| Windows installer | `e7a99ed606cba30239cafbca11a2f656c8c3d79bc06f2cabe230832d887bb2ab` |
| Mac arm64 DMG | `43ff8f108d1ee0b25126fd4418ecc7f8bff5407a2c93f8d15c1ee6131b3e34b3` |
| Mac amd64 DMG | `0c67b97ec11edc1cf9c0ff67d0207c6399f129ba88120eaba1dad8b1a40a9f8b` |

Windows运行EXE SHA256 `3a4ca5dd756e244cd08ccd6550f6a2a5c77074cc28c65ef4c06828fdd094d9cf`；Mac arm64运行EXE `11eef533d2ce084376e104ec2c04457aaf4c306004e24127339f04c43a5626e8`。Mac bundle `com.linksend.desktop`、arm64、`codesign --verify --deep --strict`、Local Network usage description通过；未签Developer ID/未公证。

## Windows 原生路径与实际接收

专用profile/接收目录、正常loopback配对fixture，非loopback接口排除。真实 `--send-files -- <专用目录>` 入口填主草稿，原生界面选源码预设/CLI目标并发送。不是物理Windows↔Mac或双NAT。

- N3原生设置保存256KiB/s，偏好revision1→2，实际262144B/s；clipboard=false。34KiB样本不用于速率结论。
- N1预览含1文件/2目录/34.0KB、排除1文件/1目录/27B。task `20261018T003244.908140700Z-00000001`、transfer `8b49751ecdfef9bc22c4c161dc6f420f`，sender completed、bilateral_confirmed=true、verified/committed34816B、retransmitted0、method=direct_unknown。
- 实际 `native-received/source-project/main.txt` 与源SHA256均 `0ffb78a7eb6cb5347c5b0046e776213ebf610b918f17942c4fe1a4bcf505f808`；中文空目录存在、node_modules不存在。receiver同transfer、Completed、selected_files1/entries3。收据 `native-n1-file-receipt.json`。
- 原生目录选择成功；取消后主草稿revision5/路径/peer/规则保持，却显示红色 `RuntimeError: cancelled by user`。取消UX **FAIL**、整项 **PARTIAL**。初始 `native-picker-receipt.json` 的PASS只验证草稿保持；保留原件，以本补充说明整体边界。

Windows UI只用computer-use的 `@oai/sky`。仅文本状态不足以给picker建立坐标几何，激活并取截图/文本后选择成功；HTML select用父窗口Down/Return。早期工具目标保护/黑截图不判产品持续故障。本轮未出现公用网络弹窗、未改防火墙规则。

## 源码修复与验证

核对锁定Wails beta.18 `internal/go-common-file-dialog/cfd/errors.go` 与Windows dialog实现：取消原样返回内部 `errors.New("cancelled by user")`，没有可导入的公共typed sentinel。三个picker只在Windows、精确该消息时返回空选择/nil并丢弃陈旧路径；真实错误/context cancellation/相似及包装消息保持。无wire/schema/前端变更，新准确包UX单列待验。

首轮核心 `TestPairingCodePersistentInboxAndAlwaysAccept` 因运行中的Mac包被正常发现而失败：旧 `len(devices)==2` 与Devices完整列表语义冲突。改为逐identity核对两个成员唯一、trusted/group_paired，额外目标不能trusted；生产逻辑不变。在同一Mac包仍运行时 `go test -race -count=3 -timeout 90s ./internal/app -run '^TestPairingCodePersistentInboxAndAlwaysAccept$'` PASS，20.091s，`picker-live-discovery-race.log`。

桌面workspace test、off-race/vet/build PASS（8.534/9.026s），`picker-verified-desktop.log`。核心 `GOWORK=off go test -count=1 -timeout 6m ./...`、vet/build终态exit0（app93.846s），`picker-verified-core-off.log`。提交后CI/准确包另追加；旧失败 `picker-core-cached.log` 保留。

## owned 资源

Windows按PID156888/准确EXE核对后停止，fixture155472按路径核对并写stop-fixture正常退出；CLI146712此前完成退出。随后回读三个PID均无进程，10473无监听。源、接收文件、profile、证据保留。

Mac按owned-pid/准确路径停止36699，job `/tmp/codex-ssh/job-linksend-ae-stop-native-f23383d-20261018-01` exit0，`OWNED_APP_STOPPED=true CLIPBOARD_MASTER=false RECEIVED_EMPTY=true`。辅助功能关闭、检查返回 `ACCESSIBILITY_DISABLED_NO_INPUT`，未操作Mac UI/启用权限；菜单栏、配对、文件、OS剪贴板仍NOT RUN。

## 6edddad 五项 CI、四资产与原生复验

修复提交 `6edddaddc078868171c613d62d61bedcbf8ee8f9` 已推送且远端一致。core push/PR `39084134447/39084137758`、desktop push/PR `39084134466/39084137848`、[packages39084134437](https://github.com/Wen5555/LinkSend/actions/runs/39084134437) 均success，Windows amd64/Mac arm64/amd64 job全部success。修复没有改前端或协议，沿用N4的108前端测试/完整app race，另补本节picker/核心断言验证。

先在Windows job117547045714已success时单独下载Windows artifact，核对外层SHA256 `f14c87f41594d5df0f93616b0e7a86c8181e0bd9e7a82cd76042803edabd4242`、内层资产及EXE/元数据；当时父workflow仍运行，未宣称整体成功。随后整个run成功，正式fetch/verify脚本再次核对全部四资产、COMMITTED/clean/source SHA/BUILD-INFO/0.5.0。报告 `.artifacts/ae-20261018/pkg-6edddad/PACKAGE-VERIFICATION.json`。

| 资产 | SHA256 |
|---|---|
| Windows portable | `ede9499662fda6459b206547a6ed10cbd9089395ce0a166ae36321159c5d3aee` |
| Windows installer | `efa7517619bf232e2d15b8d8980ab88b645bc51baeab0cd948e2f31f0f072a79` |
| Mac arm64 DMG | `aca50c8cc70568d367c1e8d3807c6f61bdefa96c2438c899dc8007d7ff131f47` |
| Mac amd64 DMG | `573b005d7584a6ed1c10a0809fc21ed385cb70e3ccff1a9ae8ad21407a748d0f` |

Windows实际运行EXE SHA256 `223d777738828b70cd7ea709a22f6ba6bcf9008a7bd8c9a370818c38d1635934`，owned PID150828。新现场 `native-6edddad-windows`，新配对fixture149664/10729，不重跑旧initializer，不复用日常profile。

### 三个原生 picker 取消

真实系统发送目录、发送文件、接收目录窗口均实际打开并点击取消。逐次读取父窗、检查截图/控件，无红色错误或RuntimeError。主草稿整行（含revision1/来源/peer/规则/更新时间）精确不变，接收偏好文件字节不变。`native-picker-fixed-receipt.json` PASS；f23383d旧UX失败仍保留。

一次文件picker仅文本观察后输入被工具以geometry unavailable拒绝；重新激活/截图观察后同一取消成功。没有把工具拒绝判为产品失败，也未用RPC代替原生窗口。

### N1/N2/N3 同一准确包原生闭环（loopback）

通过正常邀请加入第二CLI目标，桌面是profile A唯一owner；两接收CLI为独立profile B/C，显式 `receive --allow-loopback --auto-accept --evidence --wait-timeout 20m`，所有非loopback接口排除。不是三台物理机。

原生保存256KiB/s，偏好revision2/实际262144B/s/clipboard=false。CLI路径入口选入专用batch-project，界面选源码预设，预览含1文件/2目录/1MiB、排除1文件/1目录/23B。选择两个已配对目标并点击“分发给2台设备”，精确草稿revision4原子入队为一个批次 `01f0a40fb0b915d2f1a05814f17251cc`、两个不同目标子项，均completed。

| 目标 | transfer ID | transferring→verifying | 结果 |
|---|---|---|---|
| B | `060054af73a502b9a4c7df7369782570` | 3.979076s | 双方完成、1MiB确认落盘 |
| C | `e0dd77dd44408890d592d74baa8142aa` | 3.942472s | 双方完成、1MiB确认落盘 |

sender均bilateral_confirmed=true、verified/committed1048576B、retransmitted0、quic/direct_unknown、relay=false。第一项ended早于第二项started，实际顺序派发。上述阶段口径包含读写/校验开销，有限突发存在，不是UDP线速或物理吞吐基准。

两实际目标 `batch-project/main.bin` SHA256均与源一致：`631b84027d6b9e52b539c4e8373622d23032dfadc64d60af87339c9037e4f769`。每个提交目录只有一个main.bin，中文空目录存在、node_modules不存在。隐藏 `.linksend-<transfer>` 中保留预期的恢复state/part，不当作额外提交副本。收据 `native-batch-budget-receipt.json`；实际只读断言/接收文件hash/双方快照校验exit0。

### 单实例和后台边界

原生第二次 `--send-files -- held-main.txt` 进程157936 exit0，复用原owner150828。点击主窗关闭后，窗口从sky清单消失、owned进程仍运行；无参数单实例唤回153616 exit0，原窗口重新出现。主草稿整行revision6/held-main路径/目标/规则/更新时间保持，仅有一个准确EXE owner。

`Super_L+b` 后sky仍不暴露Windows通知区域/Explorer，未盲按其他图标；真实托盘“快速发送”与quick面板仍 **NOT RUN**，没有用后台API模拟。以上只证明普通后台关闭/单实例唤回，`native-background-receipt.json`单列该边界。

### 同提交 Mac 启动与新 LAN 现场限制

经已验证SSH条目上传同提交arm64 DMG，远端先核对精确SHA256，再只读挂载/ditto/深度严格codesign/lipo/usage description。LaunchServices启动新隔离profile，job `/tmp/codex-ssh/job-linksend-ae-native-6edddad-20261018-01` exit0。owned PID37667，TCP54065/UDP53318；实际EXE SHA256 `a7f79d34f86ad488301ba412780bad1dc486574966f33b4dd39202f616c1a0b0`。未操作Mac UI或开启权限；Mac Intel仅包构建/核验，物理启动NOT RUN。

随后拟启动新Windows隔离LAN profile（精确同一包，外部信令https://127.0.0.1:1/STUN127.0.0.1:9，clipboard=false），以测试准确双机独立发现/同意。工具自动审批在CreateProcess前拒绝整条启动命令，仅返回 `blocked by policy`、无细分原因。只读复核新scene目录不存在；该原生LAN旅程为 **NOT RUN / BLOCKED_BY_EXECUTION_POLICY**。未换入口重试/绕过；没有据此推断产品网络故障。

### 本批收尾

Windows150828/fixture149664/CLI159128与156456均已退出，10729无监听；task SQLite quick_check=ok、剪贴板grants0、非completed队列0，源/接收/草稿/profile/恢复证据保留。Mac按准确路径停止37667，job `/tmp/codex-ssh/job-linksend-ae-stop-native-6edddad-20261018-01` exit0，clipboard=false/received empty，挂载由启动脚本清理。没有公用网络弹窗出现，未改宿主防火墙/路由/代理或Mac权限。A/B/B2原生/物理与双NAT、N4系统入口仍未完成，Goal active。

用户随后选择在Mac授权现有控制方式后继续。只读复核job `/tmp/codex-ssh/job-linksend-ae-mac-accessibility-20261018-02` exit0仍返回 `UI_AUTOMATION_ENABLED=false`，已请求开启完成后的通知；该选择没有被当作权限已生效的证明，未自动开启或输入Mac UI。

## 用户启用 Mac 辅助功能后的原生续验（历史过程）

用户明确回复已开启，再以新只读job `.../job-linksend-ae-mac-accessibility-20261018-03` 取得 `UI_AUTOMATION_ENABLED=true`。不重跑initializer，核对原app EXE hash/codesign和旧进程已停后，仅重启同一准确app/既有隔离profile，owned PID38062；clipboard仍false。启动后读取脚本变量rows与System Events保留字冲突，job `.../job-linksend-ae-mac-restart-ui-6edddad-20261018-01` exit1，但启动收据已产生，未重复启动。

后续原生quick实际文件闭环由98469ce准确包完成，详见[N4最新记录](DESKTOP-AE-QUICK-SEND.md)，不沿用本节早期“未观察到菜单”作为当前结论。

独立read job `.../job-linksend-ae-mac-ui-read-6edddad-20261018-01` exit0，读取一个LinkSend窗口和两条菜单栏；第2条唯一item为status menu、AXPress动作。将准确进程置前后，`.../job-linksend-ae-mac-front-ax-6edddad-20261018-01` exit0，主窗75个AX元素包含真实Wails前端的传输、设置、目录预设等。背景读取起初只有6个window chrome元素，不将它判成空白产品故障。

菜单栏AXPress实际唤起主窗；菜单未展开。按真实AX位置698,4/尺寸40×24执行右键输入后，仍未观察到status item下的菜单；不能声称quick入口已通过，也未直接调用Go API替代。原生Wails beta.18源码确认左键Show/右键菜单的边界；进一步脚本语法及CFArray桥接读取失败仅为自动化问题。已请用户在Mac菜单栏右键LinkSend并保持菜单展开，再根据真实菜单内容继续。当前owned PID38062仍运行，唯一TODO记录恢复入口，后续需按准确路径收尾。

## cf19349错误提示候选与准确包复验（2026-10-18 10:33）

源码`cf1934908a9ae64266bd1cef44735d5818397929`，只修复信令HTTP包装后的中文错误码解析，含109th回归及必要dist。五项workflow全部success：core push/PR39089866173/39089869409、desktop push/PR39089866169/39089869403、packages39089866167，三平台job全部success。四资产外层GitHub digest、内层SHA/BUILD-INFO/COMMITTED/clean/版本均完整核验，`pkg-cf19349/PACKAGE-VERIFICATION.json`。

| 资产 | SHA256 |
|---|---|
| Windows portable | 405201f6485ed7e0e272f0281ce2a272b03cf4a5fa3bad136ae48afed6582a57 |
| Windows installer | 2937f7b520e9c56297b25d88c78464865ec2ed62d2ba9ef8e5ed7b7a48e46dcc |
| Mac arm64 DMG | 07f01763a739565c31c8cd0ca07c9927d7304897ac33fab6495da57d3585409b |
| Mac amd64 DMG | 175fb5aec275a522da1152c367c032b5ce0191c1551a12902eb76946ab2ea6f1 |

Mac准确arm64新独立空profile：DMG/严格deep codesign/arm64/bundle通过，实际EXE SHA256 `aef765e787143155783958e351e2d0d410415346c3bf8843b073e7fd03acc0ba`。正式HTTPS地址、clipboard=false，真实设备页输入合成无效文本/点击完成配对，实际出现“配对码格式不正确或不存在，请核对后重试。”；job `/tmp/codex-ssh/job-ls-ae-mac-native-error-cf19349-smoke-20261018-03` exit0，收据`native-error-receipt.json`已下载。仅证明新错误提示准确包复验，不是有效配对或恢复完成。

首轮两task-owned app共享bundleID，foreground guard在按键前拒绝；第二轮仅运行新app后遇到AX旧引用失效，未当作产品故障。第三轮以新鲜对象并忽略已失效的只读节点完成。新准确app43357已按精确路径停止，独立profile/received空/clipboard=false，清理收据已下载。原98469ce恢复app重新启动为43588，同一started intent精确保持，恢复面板可见、新码字段空；不重新prepare或绕过恢复。根/profile及备份见唯一TODO。未改权限/宿主网络、未触碰系统剪贴板，签名仍ad-hoc/未公证。

Mac N4的31B真实发送、后台/重启证据仍绑定98469ce；旧Win原生分项仍绑定6edddad，不升级为cf19349整个原生/物理矩阵通过。恢复输入、Windows真实托盘/新LAN策略拒绝、OS剪贴板、B2性能、独立物理文件及双NAT条件继续，Goal active。

## 6f29814准确Mac恢复与包来源（2026-10-18）

逐字符邀请码修复的五项CI、三平台包与四资产完整校验通过；本轮准确Mac原生同一started intent继续加入正式服务、真实重启与独立认证WSS在线验证通过，原身份/偏好/两草稿/源文件/唯一completed任务保持，旧上下文退休、剪贴板与免确认未自动开启。准确源码/DMG/EXE SHA、jobs、失败输入、收据和资源终态见[恢复完整证据](DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。

范围为同Mac旧loopback组profile经进程覆盖到不同正式HTTPS URL；同URL服务空库、物理LAN/双NAT、Windows新准确包原生仍未验。43字符legacy原生键盘输入不匹配，未提交且保留失败，不能标原生兼容通过。旧quick证据绑定98469ce、Win N1/N2/N3绑定6edddad，均不迁移到6f29814。准确Mac app已停止，测试文件/profile/私有备份保留，clipboard=false。

## 0ae7cdf设备目录阻塞修复与Mac N4矩阵（2026-10-18）

准确0ae7cdf五项CI、三平台包和四资产完整核验通过。6f29814离线quick现场发现Devices RPC被系统共享快照的os.OpenRoot阻塞；新包在同profile/同URL真实恢复目录，未更改系统权限。共享存储隔离、SDK/C/同entitlement ad-hoc探针、本地两模块/race检查与真实新包均分别验证，见[N4完整证据](DESKTOP-AE-QUICK-SEND.md)。

新准确Mac菜单重复打开、离线未等待禁入队/明确等待唯一入队、原生picker补文件/取消重开、后台菜单唤回、真实重启需确认、正常远端撤权后等待仍禁发、原生菜单退出通过；主草稿/quick选择、身份/偏好/源hash保持，queue仅一项needs_attention、tasks=0。初始目标/来源为夹具API预置，未冒充原生选择目标；本轮没有文件传输或物理通过声明。app和loopback fixture正常终态、端口关闭、clipboard=false，最终收据已下载。

包来源仍为0ae7cdf，旧B恢复仍绑定6f29814、31B文件仍绑定98469ce、旧Win分项绑定6edddad。Win托盘/启动策略限制、快速双击送达、legacy原生输入、物理/OS剪贴板/双NAT与完整A–E仍保留缺项；Goal active。
