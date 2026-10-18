# 2026-10-18 N4 托盘快速发送

契约见[ADR0012](../adr/0012-tray-quick-send.md)，当前状态由[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。源码增加托盘/菜单栏“快速发送文件”，唤起既有窗口中的轻量对话面板；没有新实例/快捷键权限/传输队列。

## 6ec3cc4准确Mac原生目标选择补齐（2026-10-18）

同一准确包在隔离preset-native-6ec3cc4中从真实菜单打开快速面板，原生下拉显示正常已配对B。菜单打开后AX仍只暴露窗口外壳，但本次使用原生上箭头/Return清空，再重新打开并用下箭头/Return选回B，Go持久quick revision4→5→6；目标与原路径/规则恢复相同，main/queue/tasks保持、未入队，随后原生关闭面板。此为实际目标选择成功，补上下面旧检查点的NOT RUN；不把旧检查点改成当时已通过，也不增加新文件/物理测试声明。

job `/tmp/codex-ssh/job-ls-ae-mac-preset-quick-target-6ec3cc4-20261018-01`实际exit0；完整选择收据包含在`mac-preset-6ec3cc4-final-receipt.json`。app/receiver/专属服务已正常停止，监听关闭、clipboard=false；准确包原生45B实际发送、取消/后台/重启和离线撤权分别保留原来源，物理与Windows托盘仍未验。

## 本机目标与身份加载边界修复（2026-10-18）

旧QuickSendDialog仅按trusted/blocked筛选，核心目录中的本机同样trusted。新增三个真实回归先RED：本机出现在候选中、保存本机目标的发送按钮启用、identity尚未读取时仍启用目标。后端授权并未被绕过，这里修复的是用户可见候选与发送入口。

App传配置identity（缺失时shell身份），面板排除自身；identity未加载时禁用目标选择/发送并显示等待；历史self草稿保留目标与路径、明确不可发送。入队处理也核对当前界面条件，最终仍由Go授权。第四个回归验证目录先到、identity后到时正常peer恢复可选/发送且未改草稿；原有离线等待、撤权、取消、picker取消、异步选择与重复点击均保持。

实际命令：固定Node24.21.0/pnpm12.4.1，`pnpm test src/components/QuickSendDialog.test.tsx`先3FAIL/7PASS，修复及第四例后完整`pnpm test`为12files/119PASS；`pnpm typecheck`、`pnpm lint`、`pnpm build`均exit0。核心workspace `go test ./...`通过（app83.514s），`GOWORK=off go test ./...`和`go build ./...`通过；desktop workspace/off `go test ./...`、off vet和新dist后的off build通过。无Go/wire/schema/宿主网络修改。6ec3cc4新准确包结果见文末；旧准确包结果保持各自来源。

## 实际行为

### 6ec3cc4准确Mac目标边界与文件复验（2026-10-18）

源码`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`已推送且远端一致；core push/PR `39095472375`/`39095475109`、desktop push/PR `39095472386`/`39095475115`、[packages39095472398](https://github.com/Wen5555/LinkSend/actions/runs/39095472398)与三平台jobs全部success。四资产外层GitHub digest、内层SHA/BUILD-INFO/COMMITTED/clean/0.5.0及目标平台/架构核验通过：`.artifacts/ae-20261018/pkg-6ec3cc4/{artifact-verification,PACKAGE-VERIFICATION}.json`。准确arm64 DMG SHA256 `3942bc8b3cb787afe6c5d70bc99d8ad868bafe3d4748509512bf81c00f90b77e`，实际EXE `e13f0391f5074996c74fdd7e6ebedd241f0966612a827f9ce676cef39cee3e87`，strict/deep签名和arm64检查通过。

新Mac-only夹具经正常Initialize/Invitation/Join建立A/B/C，生产SaveDraft预置main与self quick目标/首份测试文件，然后释放profile owner。fixture源码`.artifacts/ae-20261018/quick-target-fixture/main.go`；临时工具EXE SHA256 `9a99d1980dd9813851cd28f093a7c46ac5fd13288ea4e230afc34349ef6d6ee6`。私有原profile备份在Mac；精确app由DMG安装到`/Users/wen/.codex/supervision/linksend-ae-20261018/quick-target-native-6ec3cc4/LinkSend.app`。

| 原生步骤 | 独立/持久核验 | 结果 |
|---|---|---|
| 菜单quick反复两次、保存self目标 | 真实AX显示N4 Native A/当前不可发送；不勾/勾离线等待都禁发；只有一个面板、两草稿整行保持、queue/tasks=0 | PASS |
| 激活目标下拉 | AX仅返回准确窗口外壳；没有可核实的选项或选择收据，已Escape并从状态菜单显示主窗 | 原生目标选择NOT RUN，不推断产品空白 |
| 停owned GUI后用正常SaveDraft设置已配对B并清quick路径，重启同app | quick revision2/paths空、main原行、零队列/任务；AX确认B在线/直连待检查 | 明确夹具设置，不冒充原生选目标 |
| 真实“选择文件”→Open/Go to Folder/精确路径/Open | 生产quick revision3含唯一quick-source.txt；main原行保持；取消/菜单重开整行保持 | PASS |
| 真实点击“加入发送队列” | 唯一queue `6d47f2e1138daf2d52d2ea688e5d743c`、request `quick-send-v1-3`、sender `20261018T040706.126103000Z-00000001`；仅quick路径清空为revision4，main revision1保持 | PASS |
| 同机正常配对CLI接收 | Pion host↔host/固定身份QUIC，transfer `2280689c0a7e9691ebd644c75e35b7f9`、45B verified/committed、重传0、bilateral_confirmed=true、relay=false/direct_unknown；CLI exit0，实际用户文件仅一份且源/目标SHA256 `623e59d8e0d8a81606d07d8db376538c83d2e19ac4ad95353a7a8106950592c0` | PASS；loopback，不是物理LAN/双NAT/性能 |
| 原生关闭主窗→菜单quick唤回→真实重启→原生菜单退出 | 关闭后窗口数0且同PID存活；重启同一completed队列未重发，main保持/quick空；最终身份/偏好字节/源hash、原self备份保持 | PASS |

首轮self检查在勾选等待后遇AppleScript直接遍历表达式的specifier错误；只读确认仍禁发，再改成先取列表的正确脚本完成两轮，无重复入队或产品失败声明。CLI完成退出后有Pion关闭UDP socket日志，保留原始日志；不影响实际提交/双方确认。快速双击第二次送达、原生目标选择、Windows托盘及完整物理/同提交矩阵仍缺；0ae7cdf离线撤权、6f29814两种恢复与其他旧包结果不迁移。

正式入口另外新鲜检查：Windows与Mac两次严格HTTPS `/healthz` 均HTTP200、TLS校验0，Mac收据`mac-public-health-6ec3cc4.json`；只证明对应进程HTTPS，不能代替新包WSS/完整跨网。这里GUI/receiver使用专属loopback信令及STUN覆盖，不操作日常profile或宿主网络。

最终app经原生菜单正常退出，receiver与fixture均finished/exit0，监听关闭、SQLite quick_check=ok、clipboard=false/enabled grants=0；仅一completed队列/一任务、main revision1/quick revision4空路径。非敏感最终收据`.artifacts/ae-20261018/mac-quick-target-6ec3cc4-final-receipt.json`已下载；文件/profile/备份/原始失败日志保留。关键jobs统一在`/tmp/codex-ssh/job-ls-ae-mac-`：`quick-target-fixture-6ec3cc4-20261018-01`、`install-quick-target-6ec3cc4-20261018-01`、`quick-target-self-check-6ec3cc4-20261018-02`、`quick-target-prepare-normal-6ec3cc4-20261018-01`、`quick-target-picker-{goto,path,confirm}-6ec3cc4-20261018-01`、`quick-target-cancel-reopen-6ec3cc4-20261018-01`、`quick-target-enqueue-6ec3cc4-20261018-01`、`quick-target-send-receipt-6ec3cc4-20261018-01`、`quick-target-background-wake-6ec3cc4-20261018-01`、`quick-target-restart-check-6ec3cc4-20261018-01`、`quick-target-final-cleanup-6ec3cc4-20261018-01`均exit0；`quick-target-receiver-6ec3cc4-20261018-01`同样exit0。

- quick草稿和main为不同持久行；打开/重复打开不改草稿，取消保留quick与main，选择器取消保留来源，提交成功后只清quick路径。SaveQuickSendDraft拒绝main、多目标和筛选元数据。
- 最近/常用排序沿用Go设备pin/last-used目录，搜索名称/identity、最多10个候选；文件10条分页。目标撤权禁发送，离线需明确等待。新发现不获得授权。
- Go EnqueueQuickSend只接受revision/等待选择，读取精确quick草稿，通过原Enqueue预检/事务与顺序队列发送。8并发只有一项；同revision已提交后改变等待策略拒绝；清草稿和重启后返回原收据，重启不自动派发。
- `TestQuickSendSavedDraftActuallyTransfersThroughExistingQueue`经真实Pion、固定身份QUIC发送中文文件，实际目标内容一致，main revision/来源不变。不是物理Win↔Mac或系统托盘点击。

## 命令与终态

| 范围 | 实际命令/日志 | 结果 |
|---|---|---|
| 定向核心 | `go test -race -count=1 -timeout 90s ./internal/app -run '^TestQuickSend'`；n4-core-second.log | PASS，6.168s |
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`及vet/build；n4-verified-core.log | PASS，app84.022s |
| 完整应用并发 | `go test -race -count=1 -timeout 6m ./internal/app`；n4-verified-race.log | PASS，192.535s |
| 桌面两模式 | workspace test、off-race/vet/build；n4-verified-desktop.log | PASS，7.522s/6.736s；最后dist后off build通过 |
| 前端 | 固定Node24.21.0/pnpm12.4.1 test/typecheck/lint/build；n4-select-capture-tests.log与n4-select-verified-*.log | PASS，12 files/108 tests |
| 真实绑定 | Wails beta.18/Go1.27；n4-bindings.log | PASS，99methods/66models/2events |

测试覆盖main/quick并存、重开/重启、陈旧CAS、未配对不增信任、离线/撤权、请求幂等、清空后收据、重复点击、取消/焦点、picker取消及晚到旧快照。一次新增重复点击断言没有等待mutation微任务，初读0 calls失败，补正确等待后通过；初次真实useCommand导入未mock Wails runtime造成测试内localhost3000拒绝噪声，补mock后最终日志无该错误。上述是夹具问题，不冒充产品网络故障。

## 浏览器与资源

Playwright独立session ae-quick、生产QuickSendDialog/CSS，临时Vite resolver明确替换为内存元数据backend。960×640浅色/1100×720深色无水平溢出，13文件10条分页与独立滚动、离线等待、完整identity搜索、取消/重开保留选择、Tab/Shift+Tab闭环及Escape通过。窗口内滚动可达footer，长列表小窗不宣称所有header/footer同时可见；截图实际查看。

浏览器曾发现目标选择后仍回到旧identity：React受控select恢复值早于异步mutation读取event.target.value。改为事件内立即捕获identity，再交给mutation；新增108th测试及实际浏览器select复验目标正确、发送按钮可用。旧失败截图保留，最终dark-1100-fixed.png。0 errors/1 warning，warning为Wails浏览器模式提示。

截图在`output/playwright/ae-quick-20261018/`，原始日志/profile/DB在忽略artifact。owned浏览器、Vite及4个临时入口/配置/metadata文件已收尾，原`.playwright-cli`、ios/linux目录保持。原生Windows托盘/Mac菜单栏真实点击、后台唤起和准确包物理旅程仍待后续同提交验收；不能用浏览器按钮模拟代替。

N4提交 `f23383d` 已推送，五项CI/三平台包success，准确四资产与Windows/Mac实际启动来源见[原生检查点](DESKTOP-AE-NATIVE-CHECKPOINT.md)。Windows原生N1目录/QUIC接收、N3设置通过，取消picker错误已定位并修复源码；这些不代表N4托盘真实点击已通过。

## Mac原生续验与菜单可发现性（2026-10-18）

用户已开启现有控制方式的辅助功能。6edddad准确arm64包、原隔离profile/PID38062：只读status-state第02轮观察到真实菜单及“快速发送文件”，随后通过System Events点击该精确菜单项，实际Wails面板含“托盘快速发送”/独立草稿说明/设备与文件选择控件。用户随后确认“托盘里面有啊”。菜单出现的具体触发时序未知，不能归因为右键注入成功。

真实点击面板“选择文件”打开macOS `Open`窗口，读取其按钮后点击`Cancel`，再点击快速面板“取消”。任务库`send_drafts`操作前后整行一致（均为空）；无配对设备、未入队或发送文件。这只证明原生菜单→quick面板→文件picker→取消与空草稿保持，不证明非空主草稿或完整发送。job前缀`/tmp/codex-ssh/job-linksend-ae-mac-`：`status-state-6edddad-20261018-02`、`click-observed-quick-6edddad-20261018-02`、`quick-picker-open-6edddad-20261018-04`、`quick-picker-cancel-6edddad-20261018-01`均exit0。之前几次脚本因AppleScript语法/对象列表和只读DB定位失败，保留失败，不作产品失败。

为响应入口难以辨认，候选只在Mac添加`LinkSend`菜单栏文字，并允许主点击使用锁定Wails原生菜单默认行为；Windows点击唤主窗保持。核心off全test、桌面workspace与off-race/vet/build通过，标签追加后desktop off-race/vet/build再次通过（11.021s）。前端未改，沿用108测试边界；新准确包/主点击待验。

本次Mac实例环境`LINKSEND_SERVER_URL=https://127.0.0.1:1`，主页信令不可达是故意隔离条件。正式`linksend.oooai.de`的HTTPS/WSS与Mac真实客户端必须另验；旧公网TLS超时没有因本节原生通过而消除。

## 98469ce准确包主点击续验（2026-10-18）

五项workflow全部success：core push/PR39087260830/39087262943、desktop push/PR39087260258/39087262949、packages39087260261。三平台四资产外层digest、内层SHA/BUILD-INFO/COMMITTED/clean/版本核验通过，完整表在忽略artifact `pkg-98469ce/PACKAGE-VERIFICATION.json`。Mac arm64 DMG SHA256 `6044b02725d209c50fd1ebb917d7a39def87f674e69d7ccd2bac12633ea7568a`；实际EXE SHA256 `b722bc257ef300462638097f8d069c651971e178f4c341c7e1a178834aced87a`，codesign deep/strict/arm64/bundle通过。

新专用Mac-only loopback fixture经既有正常Bootstrap/邀请/Join完成配对并释放profile owner；通过生产Service.SaveDraft预置main revision1/held-main.txt，不写trust/SQL。新准确app PID39645、profile-a，clipboard=false，旧准确包与公共实例先按PID/path停掉。原生status AX名称现在为LinkSend；真实AXPress使PRIMARY_MENU_COUNT=1，再点击“快速发送文件”打开面板，job `.../job-linksend-ae-mac-primary-quick-98469ce-20261018-01` exit0。这验证本次标签/主点击修复，不继承旧包其他原生结果。

随后点击已观察到的AXPopUpButton“快速发送目标”，旧AX引用失效，独立读取只返回窗口外壳；只读确认PID39645前台/可见、AXMain=true/AXFocused=false/未最小化。不能仅凭AX外壳断言产品空白或选择成功。截图技能只读权限检查requested=false/screenCapture=false，未请求或改变安全权限，没有截图/视觉QA。已异步请用户观察并选择M1原生对端；回答前不盲输或用RPC代替原生选择。

checkpoint job `.../job-linksend-ae-mac-quick-checkpoint-98469ce-20261018-01` exit0：main整行/revision1精确保持、quick行仍空、SQLite quick_check=ok、clipboard=false。`quick-native-checkpoint-receipt.json`已下载；完整quick文件/后台重开/重启旅程仍NOT RUN。app和30分钟fixture为续验保留，精确所有权及收尾入口见唯一TODO。

## 98469ce准确Mac原生文件闭环（2026-10-18 10:05）

从原生菜单“显示 LinkSend”恢复了可读的quick控件。自动下拉框/键盘尝试未形成选择收据，不能计入原生选择通过；随后以生产Service.SaveDraft准备仅含测试B的quick目标，目标预置明确是夹具设置。用户回复已选择M1原生对端，后续AX实读“快速发送目标=M1 原生对端 · 在线 · 直连待检查”。此前CLI及桌面后台发布文件实际均有两设备、UI一度为0；交互后设备与选项恢复，具体原因未证实，没有据此修改设备逻辑。偏好版本缺失假设被实际format_version=1排除，原偏好未改；该脚本在发现版本合法前已停owned app，随后正常按既有profile恢复。

真实点击quick“选择文件”，macOS Open→Go to Folder→精确专用路径→Open，持久quick revision2只含`quick-source.txt`与精确B，main revision1/整行不变。再点击面板取消、原生菜单重开，quick整行/revision2与main完整保持。没有通过RPC添加文件或入队。

真实点击“加入发送队列”前再次只读核对精确来源/目标/revision和空队列。产生唯一queue `1eb9be4e9e36f24ee67b59255caa3991`、request `quick-send-v1-2`、sender task `20261018T020514.364662000Z-00000001`、transfer `a5dd9f522307377f2d5028b16fad0903`。准确桌面与同机正常配对CLI经Pion/固定身份QUIC/TLS1.3/ALPN linksend/1完成31B，sender verified/committed31、retransmitted0、bilateral_confirmed=true、relay=false、direct_unknown。CLI exit0、实际接收31B且只有一个提交目标，源/目标SHA256均为`d37347cbc04900dab1462c0759a836c73b39401ac4fa77397db82bfce1c15a83`。CLI终态后有关闭UDP socket的Pion stderr，保留日志，不把它当成未完成或隐藏。

入队成功后仅quick路径清空，revision3，main仍原整行。原生主窗关闭后AX窗口数0、同owner仍在；原生菜单quick唤醒成功、仍仅一个completed队列。再真实停止/重启准确包，main完整保持、quick空、无重复入队/派发。收据`quick-native-send-receipt.json`已下载到忽略artifact，含实际sender快照/文件hash/背景和重启断言。相关job（前缀`/tmp/codex-ssh/job-linksend-ae-mac-`）`quick-picker-goto/path/confirm-98469ce-20261018-01`、`quick-cancel-reopen-98469ce-20261018-01`、`quick-native-enqueue-98469ce-20261018-01`、`quick-final-receipt-98469ce-20261018-01`、`quick-background-wake-98469ce-20261018-01`、`quick-restart-check-98469ce-20261018-01`均exit0；receiver job同样exit0。

边界：Mac-only loopback、31B单文件、夹具预置目标且用户协助选择，不是物理双机/吞吐/三设备或自动目标选择全通过。Windows真实托盘、离线/撤权/反复提交等完整原生矩阵仍缺，源码既有测试保持原范围。截图权限未改变，没有视觉QA。N4现场转入独立的公共信令恢复旅程前已保留整个post-quick profile备份，loopback fixture已正常终态exit0，CLI receiver退出，clipboard=false。
## 6f29814离线原生续验暴露设备读取阻塞（2026-10-18）

准确6f29814 app（EXE SHA a003120b4866463d029ad5aad1b39be93e653d56eb5858473f3fc6d7fa1eee71）在新的Mac-only loopback三设备夹具中复现：独立C正常Devices确认A在线、B离线/未撤权，本地trust含B/C，profile设备快照已写3条，但原生主设备页/quick未收到设备列表，保存的B显示当前不可发送。未勾选等待、未入队、tasks=0，两草稿原行保持；这不构成离线门槛PASS。

fixture源码`.artifacts/ae-20261018/quick-offline-fixture/main.go` SHA71582b1b3c01189c121ff547d709d4fb6bbef15e2e826f07caf8e989bdfd9e55；基于不变6f29814核心构建临时Mac工具，EXE SHAe5a573b3b0894193d0614f2f34b0b6b5f71048d66f084983263aa8a4187c9a4b。A/B/C正常Initialize/Invitation/Join，生产SaveDraft预置目标与初始文件，不冒充原生选择通过；profile owner释放后准确GUI独占A。原生菜单可打开quick；手动取消→设备→刷新仍没有B/C，端点健康/成员在线与UI分别记录。

保留现场并以同包、同profile、同URL做一次带stdout/stderr的受控重启，问题再次出现。2秒sample未提供足够Go调用关系；在确认queue/tasks=0且两草稿整行保持后，对**仅任务owned app**主动SIGQUIT取Go栈并退出。这是诊断终止，不是应用自发崩溃。栈明确有App.Devices和native-entry worker两个goroutine卡在publishNativeShareDevices→writeNativeShareDevices→os.OpenRoot；App.Devices调用前已拿到3条设备，RPC被后续系统共享快照I/O拖住。实际包包含App Group entitlement，属于ad-hoc签名；没有打开/修改系统权限，也未进一步读取或清理共享容器。

候选修复移除普通Devices RPC的同步共享发布，沿用owned native-entry worker原有15秒发布；显式/非默认profile不调用系统App Group resolver；默认profile还须通过Security.framework的非空TeamIdentifier与准确group entitlement条件。隔离profile仍保存/读取自己的系统入口目录。三个隔离条件及两个真实配对目录/入口副作用回归先RED，再GREEN；默认合法profile的resolver/空容器/去重路径也通过。无前端、wire、schema或权限修改。

本地`GOWORK=off go test ./...`核心缓存PASS；desktop workspace test7.231s、off `go test -race -count=1 ./...`9.211s与nativeclipboard1.636s、off vet/build全部PASS。Mac实际SDK核对Security函数/签名；修改后的Objective-C `clang -fsyntax-only -Werror`及真实ad-hoc小探针编译/执行exit0，确认不返回共享容器。新准确包/原生设备恢复与剩余离线、撤权矩阵待继续；不把源码GREEN升级为包级修复成功。

远端fixture job `/tmp/codex-ssh/job-ls-ae-mac-quick-offline-fixture-20261018-01`为45分钟owned loopback服务；当前旧app已诊断退出，profile/文件/私有备份保留，clipboard=false。关键失败jobs为quick-offline-open-check第01/02轮；栈job `job-ls-ae-mac-quick-offline-capture-stacks-20261018-01` exit0，C验证 `job-ls-ae-mac-share-signature-check-20261018-01` exit0。所有原始日志保留在任务根`quick-offline-native-6f29814`与对应jobPath，后续按准确新包继续同现场。

## 0ae7cdf准确Mac修复复验与N4离线/撤权矩阵（2026-10-18）

修复源码`0ae7cdfecb9dc929a219dae352ae09c3ad24ec47`已推送、远端一致。core push/PR `39094061712` / `39094064097`、desktop push/PR `39094062029` / `39094064059`、[packages39094061744](https://github.com/Wen5555/LinkSend/actions/runs/39094061744)及三平台jobs全部success；四资产外层digest、载荷SHA、BUILD-INFO/COMMITTED/clean/版本完整核验通过，报告`.artifacts/ae-20261018/pkg-0ae7cdf/PACKAGE-VERIFICATION.json`。前端源码/dist未变，115项前端边界与CI结果保持。

准确arm64 DMG SHA256 `9e0cb6e1317ccbf307599b35ec7021fc3bb1f0f049244b1599bb13634025528b`；实际EXE `a258ea9f52e89de111664a371c1cb1d47fe93c5df2c07eb627a2aa3d7d65f7a1`。同一旧现场的profile/URL/初始两草稿/三设备保持，私有备份后安装为`quick-offline-native-6f29814/LinkSend-fixed-0ae7cdf.app`，旧6f29814 app和诊断日志保留。目录名称不是版本证明，所有结果绑定上述新EXE。新包真实菜单打开quick立即读到离线B，两次打开都只有一面板；该现场明确完成FAIL→PASS，未改系统权限或网络。

| 原生操作 | 持久/独立核验 | 结果 |
|---|---|---|
| 连续两次菜单quick、未勾离线等待 | 独立C确认A在线/B离线；唯一面板、入队禁用、初始两草稿整行保持、queue/tasks=0 | PASS |
| 勾选离线等待→真实点击入队 | 唯一queue `6160c8cfa0c393a89cd48c4a5f1134cb`、request `quick-send-v1-1`、draft=quick/revision1、wait=true、state=waiting_peer、task_id空；仅quick路径清空为revision2、main整行保持 | PASS |
| 再次反复菜单重开 | quick空/入队禁用，同一queue未复制、tasks=0 | PASS；不冒充第二个快速双击已送达 |
| 原生picker重新选择测试文件 | Open→Go to Folder→精确quick-source.txt→Open，quick revision3、main整行保持；取消/重开保留两行 | PASS |
| 原生关闭主窗→菜单quick唤回 | 关闭后窗口数0，同一PID仍存活，菜单唤回quick；同一queue未派发 | PASS |
| 真实停止/重启准确新app | 两草稿整行保持；同一queue变needs_attention/RESTART_CONFIRMATION_REQUIRED、task_id空、tasks=0，菜单恢复B目标 | PASS |
| C正常签名撤销B，保持quick打开 | C独立目录两条有效组关系+B removed历史；A trust移除B，quick显示当前不可发送；实际勾等待仍不能入队、取消/菜单重开两次仍禁用，两草稿保持且仍只有原queue/tasks=0 | PASS |
| 原生菜单退出应用 | owned进程正常退出，原profile本地设备发布worker此前仍更新且只含A/C；fixture正常exit0、端口关闭、SQLite quick_check=ok | PASS |

初始quick目标/首份文件是夹具通过生产SaveDraft预置，不冒充自动原生选择设备；后续picker补回文件是真实系统选择。这里没有接收方receiver或文件传输，received为空，不能升级成0ae7cdf文件/物理LAN/性能通过；98469ce的31B真实QUIC证据继续保留原版本。Windows托盘原生、快速双击送达、物理与完整N1–N4同提交矩阵仍缺。

picker首次点击后的0.3秒窗口检查过早停止，后续前置检查也停止；观察到真实Open窗口后只操作现有picker，经有界就绪等待完成Goto/精确路径/Open。原失败保留，没有归因产品失败。撤销首次脚本把包含removed历史的目录总数误当活跃组人数；Revoke已exit0后停止，先只读核对两条active+B blocked/removed/revoked，再继续原生禁发检查，没有重复撤销。共享签名探针还加上与旧包相同group entitlement再ad-hoc签名，仍返回无系统容器、exit0；正式Team签名/App Group激活继续NOT RUN。

最终身份字节、偏好字节、main revision1、picker后的quick revision3和两份源hash保持；quick最终保留未发送选择。仅原queue needs_attention、task_id空、tasks=0，clipboard master=false、enabled grants=0，旧/新准确app、源码/日志和两份私有备份保留。原生“退出应用”后应用终态、fixture正常exit0，独立socket确认loopback监听关闭；未清理用户文件或共享容器。

非敏感收据已下载：`.artifacts/ae-20261018/mac-quick-fixed-0ae7cdf-{install-receipt,native-receipt,final-receipt}.json`；受控旧栈为`mac-quick-offline-6f29814-controlled-stack.log`。关键job均在`/tmp/codex-ssh/`且exit0：`job-ls-ae-mac-install-quick-fixed-0ae7cdf-20261018-01`、`job-ls-ae-mac-quick-fixed-open-check-20261018-01`、`job-ls-ae-mac-quick-fixed-offline-enqueue-20261018-01`、`job-ls-ae-mac-quick-fixed-reopen-after-enqueue-20261018-01`、`job-ls-ae-mac-quick-fixed-picker-confirm-20261018-02`、`job-ls-ae-mac-quick-fixed-cancel-reopen-20261018-01`、`job-ls-ae-mac-quick-fixed-background-wake-20261018-01`、`job-ls-ae-mac-quick-fixed-restart-20261018-01`、`job-ls-ae-mac-quick-fixed-revoked-native-check-20261018-01`、`job-ls-ae-mac-quick-fixed-native-quit-cleanup-20261018-01`、`job-ls-ae-mac-quick-fixed-final-closed-20261018-01`。
