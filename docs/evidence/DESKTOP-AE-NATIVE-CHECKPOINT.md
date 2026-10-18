# 2026-10-18 准确包原生检查点与取消选择器修复

分项证据，状态仅在[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。物理双机、系统托盘、双NAT与原生loopback分别记录。

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
