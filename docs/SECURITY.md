2026-10-26 R5显式能力门：只有既有experimental_share_control明确为true且初始化成功的服务声明share_grants_v1与ready=true，默认不变。客户端将规范能力bit、ready、版本/epoch一起作为分享入口前置条件；旧端/控制原型在传码或开scoped WS前拒绝，不借能力取得任何永久成员/剪贴板权限。Create/Claim在源准备、码生成或intent持久化前检查，实际操作仍重新核验新epoch与权威签名响应，取消/幂等/双确认保持。Control4→5需一致性备份及schema5兼容回退；不能切旧程序后用旧库覆盖新写入。范围见[能力与入口证据](evidence/RELIABILITY-R5-CAPABILITY-READY.md)及[未部署草案](evidence/RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)。

2026-10-19 文件剪贴板候选：文件权限独立默认关闭，必须协商新能力且绑定原有身份/会话/双向permission revision/代次/顺序/OS generation。10秒内获准的预约最长24h，不能授权普通旧事件；取消或新复制使预约失效。文件仍由V1引擎检查路径和完整性，接收只写profile内任务专属缓存，并在CAS复查后发布本机路径。未知预约/能力或畸形流被拒绝；失败清理未发布缓存，成功缓存暂保留供粘贴。原生路径元数据有128根/64KiB上限，不传播剪切删除；新包实机验证另列。

# Security

2026-10-26 R2 publisher owner政策：控制写之前在已回读的真实durable授权digest上不可逆登记export_possible；写失败、ack丢失、main取消或退出不再强杀可能活跃的私有helper。Shell政策的deadline只约束准入，export后没有硬TTL；该政策尚未接实际系统publisher，当前受控helper仍保留自身测试期限。它不认证原生refs、不签静止证明、不赋删除权。路径出口、实际OLE与完整大目录/长路径仍须先闭合，见[ADR0016](adr/0016-automatic-clipboard-file-cache.md)和[owner证据](evidence/RELIABILITY-R2-PUBLISHER-OWNER.md)。

2026-10-26 R2构建信任锚：可信来源是同次构建后编入桌面的版本/平台/实际helper SHA/源提交/输入摘要/真实clean状态；BUILD-INFO与sidecar只作审计，不作为运行信任根。私有resolver严格拒绝空/未知/重复/非规范字段及错误PE/路径reparse，只从自身EXE固定摘要目录取镜像；未来启动仍需core稳定image/child句柄复核。ZIP先验精确条目和摘要再解压，NSIS仅本次文件及空目录，不增加系统发布或删除授权。dirty本地构建和symlink权限SKIP边界见[证据](evidence/RELIABILITY-R2-HELPER-BUILD-ANCHOR.md)。

2026-10-26 R2真实进程寿命后续：私有launch v2对完整prepare/原绝对期限签child challenge；真实native consumer/stream已持有后才签Held，成功detach不强杀helper。helper读取门只信原稳定launcher HANDLE实际exit0，非零/到期/未知保持Native保护且不签。driver接收直接复制的helper内核句柄，独立新main重载原profile与原run单次消费，已有一次正例与非零退出保护证据。30秒硬期限及commit已发/Held未确认失败窗可终止helper只属当前受控合同，不能用于未来系统Shell消费者；不宣称取消失败窗保活，也没有系统clipboard/删除权限。见[进程证据](evidence/RELIABILITY-R2-NATIVE-PROCESS-LIFECYCLE.md)。

2026-10-26 R2 Windows私有broker只接受预先固定的可信镜像摘要，持稳定image/child句柄并在挂起启动后核对实际image；本机继承管道绑定随机challenge、child临时公钥、BootIdentifier及原sealed inventory。helper只持最小只读源句柄和预建spool写句柄，不持profile私钥/SQLite；原owner回读实际签名授权后才commit，原生引用结束且真实源句柄关闭后才签固定回执。这个receipt-only实现没有系统clipboard发布、默认调用或发行包信任锚，任何Native对象仍受collector保护；原Service重开的一次消费不证明主进程退出时活跃粘贴可继续。见[实现及最小验证](evidence/RELIABILITY-R2-WINDOWS-NATIVE-BROKER.md)。

2026-10-26 R5本地owner固定profile身份、server origin、原双方key/grant/claim/task/generation/manifest/plan、源选择与保存目录；记录在现有SQLite metadata内用设备Ed25519及固定用途域签名，raw CAS写入后回读。明文分享码不入ledger、task、错误或诊断，创建当次丢码需撤销新建。重启不联网或自动恢复，源变化持久撤销意图；普通Retry不能借永久授权继续。同步准备/hash及HTTP操作同时受调用方与profile退出取消，退出join真实工作。只有持久本机Completion可标本机完成，服务端Completed且无该凭据时保留needs_attention，不能补造verified/committed或继续传输。自动回收只删服务已确认终态且首次观察满七天、无活动/控制操作/待处理意图的本profile签名授权metadata，raw CAS防替换；未知、旧记录无时间、回拨及篡改均保护，用户文件/历史/checkpoint不删。预览完整清单仅经固定公钥QUIC，新transfer再核验原scope；无永久成员、剪贴板或普通会话复用权限。实现与回归边界见[本轮证据](evidence/RELIABILITY-R5-OWNER-DATA-PATH.md)。

2026-10-26 R5专属协调使用原双方固定公钥、完整grant/task/phase scope和双层原签名；独立registry及有界消息/连接/重放/期限owner。所有新准入与出队重验记录，持久终态先退役，旧回调不能复活；普通组与健康连接不受分享终态清理。清单与正文无信令字段，默认完整能力关闭，实际文件授权仍须本地owner及原V1引擎，见[证据](evidence/RELIABILITY-R5-SCOPED-SIGNALING.md)。

2026-10-26 临时分享HTTP客户端沿用HTTPS与禁止重定向，每次签新epoch/nonce证明，不重放旧证明或自动重试副作用请求。服务响应经有限解析及原actor/任务/清单/计划绑定，错误只留稳定白名单码及固定摘要；不把远端detail、分享码或私有record带入日志/错误。此控制客户端不单独授予永久成员/信任/剪贴板或文件权限；该历史增量当时未接数据面，当前实现见上段及[客户端证据](evidence/RELIABILITY-R5-SIGNED-CLIENT.md)。

2026-10-26 候选准入在拒绝zone后先规范化IPv4映射IPv6，防止绕过0/8、240/4和有限广播限制；移除无网段依据的末字节255单播误拒绝。签名、同组、原session/generation和明确危险地址拒绝均通过真实forward回归。候选准入成功不表示ICE或身份验证成功，生产服务未因此自动替换，见[证据](evidence/CANDIDATE-ADMISSION-20261026.md)。

R5实验HTTP控制要求现有Ed25519签名、DeviceID/公钥一致和本次随机epoch；重启换epoch，不以丢失内存nonce记录重新接受旧证明。领取对真实IP与已验证身份双限频，忽略任意X-Forwarded-For；严格大小/字段与白名单响应，无凭据/组/文件正文泄露。experimental_share_control默认关闭；该历史控制增量当时不提供临时文件通道或广告完整分享能力，当前显式就绪与客户端门见能力增量，详见[签名控制证据](evidence/RELIABILITY-R5-SIGNED-CONTROL.md)。

R5授权核心私有record只保存分享码用途域SHA256验证器及固定公钥/摘要/任务/代次，不写成员表，不授予组枚举、邀请或剪贴板。真实SQLite事务/CAS处理竞争，终态不重新授予文件操作，原任务恢复不延长期限；未知记录/索引不一致保护。显式Control4→5迁移先一致性备份并核验，旧程序不兼容升级库时必须拒绝，不能用旧备份覆盖新业务写入。最初核心增量当时不激活迁移或对外分享；当前显式实验激活、签名协调与数据面范围见上段，不能把Go actor参数当网络身份认证。设计与历史边界见[ADR0017](adr/0017-one-time-share-grant.md)及[证据](evidence/RELIABILITY-R5-GRANT-CORE.md)。

R3诊断采用字段白名单重建安全DTO/报告，不对原始TaskSnapshot、恢复或缓存owner做正则脱敏后整体导出。未知字符串不原样输出；临时任务编号不携带稳定身份、路径、URL或原始异常。安全JSON构造完成后才写入用户选择的本地位置，O_EXCL临时文件/Sync/不覆盖Link保护已有文件，取消不写stale选择；不自动上传。诊断导航不授予重试或网络权限，旧原生错误owner保持独立。schema12/wire/身份校验不变，实际验证和未验原生边界见[证据](evidence/RELIABILITY-R3-DIAGNOSTICS.md)。

2026-10-19 可靠性R2增量：文件剪贴板接收/checkpoint支持借用profile owner固定的`os.Root`，目录字符串不能替换其文件系统授权；发布前复核可见路径的身份。原生写入被尝试后，错误也不能证明未发布，因此保留已验证缓存并显示不确定状态。boot变化与TTL不能单独证明粘贴结束；完整自动回收仍待可靠原生交接与读取结束证据。见[ADR0016](adr/0016-automatic-clipboard-file-cache.md)和[增量验证](evidence/RELIABILITY-R2-CACHE-PATHS.md)。无文件wire/schema改变。

R2持久记录借用既有SQLite owner，使用设备Ed25519身份、固定用途及严格版本/大小/字段校验，CAS写入后回读；不保存私钥，不授予网络信任或剪贴板权限，不从行内字符串重建删除路径。stage/part必须匹配引擎实际打开的目录/文件句柄；publishing先于native，运行代次及RAM-only native-start证明不能跨重启推断。未知/损坏/结果不确定记录继续保护；这些记录包含私有相对清单和文件身份，诊断不得直接导出原行。验证见[持久owner证据](evidence/RELIABILITY-R2-CACHE-LEDGER.md)。

未发布残留的回收只接受当前worker在原生调用前、接收器实际关闭后封存的签名条目清单；活动引用直到所有句柄关闭才释放。启动对账不重放复制，逐项核验固定Root/文件身份和改动，不再递归删除整个缓存目录。未知条目/链接、未封存或可能已发布的对象仍保护；扫描、尝试删除、读取大小及退避有界。无手动清理或用户容量/期限选项，见[生命周期增量](evidence/RELIABILITY-R2-CACHE-LIFECYCLE.md)。原生读结束许可和完整缓存管理仍未完成。

桌面GitHub更新由用户明确触发，固定仓库/正式标签/平台与资产地址，保留HTTPS校验；下载在Go中以有界内存写入应用缓存，并核对GitHub提供的SHA256与大小。只有完整校验后的包可由原生入口打开，打开前再次核验，前端不能指定可执行路径。摘要是GitHub发布资产的一致性检查，不是独立发布者签名；现有Windows未签名及Mac ad-hoc/未公证边界保持。安装使用系统向导/DMG并由用户退出旧程序后完成，不自动改写运行中的程序或用户profile，见[ADR0014](adr/0014-desktop-github-updates.md)。

## 2026-10-18 快速发送

托盘只发唤起请求，不代表同意发送。quick草稿独立CAS，Go从已保存来源/目标入队，仍查配对/离线选择/源hash/原代次。稳定revision收据不能扩大权限；quick保存入口拒绝main、多目标和筛选字段。路径仅作本机元数据，文件正文始终走Go认证P2P。详情见[ADR0012](adr/0012-tray-quick-send.md)。

## 2026-10-18 文件正文预算

速率输入由Go验证为0或1024–1073741824整数，非法持久值阻止正常启动发送，不静默改成不限速。写盘成功后才更新运行时；等待不持任务/队列锁，暂停/取消/撤权仍通过原context及授权屏障。单次grant最多64KiB，改速率不能收回已获准的一块。预算不是传输授权，也不替代安全hash/落盘或恢复校验，见[ADR0011](adr/0011-process-file-body-budget.md)。

## 2026-10-18 多设备批次

批次ID不是权限。Go从精确持久草稿读取全部目标，拒绝重复/未配对目标与未明确等待的离线目标；parent和所有子项单事务提交。每子项保存原authorization generation、规则和来源身份，撤权/源变化仍在派发与建连复核。UI不能通过循环单目标命令消费多目标草稿。

批次取消意图先持久化，再取消活动任务，调度/确认/直接恢复/重试/启动均有屏障；晚到的durable完成回执保持已接收文件并更新摘要，不删除源或接收文件。任务库11旧端拒绝，迁移备份不能覆盖后续写入；正文只经认证Go P2P、relay=false。见[ADR0010](adr/0010-atomic-device-batches.md)。

## 2026-10-18 目录筛选边界

排除规则为有界相对路径模式，不是脚本或完整gitignore；无取反、绝对路径或`..`。Go统一准备/预览，`os.Root`与现有路径验证保留，包含的链接/特殊文件拒绝。排除子树不读正文，预览统计链接只读元数据、不跟随；规则不授予权限。规则版本、实际内容摘要与精确草稿来源在队列事务内绑定，现有授权代次仍在入队、派发、建连与开流复核。

任务库10和筛选恢复v2防止旧端把保存规则误读为完整发送；迁移前可读备份保留，未知规则或损坏元数据拒绝。后续写入后的当前库不能用旧备份覆盖；需要旧端检查时使用隔离副本。暂停/重启/重试/历史重发沿用规则，源变化不能自动扩大范围。正文与文件完整性边界未改，见[ADR0009](adr/0009-versioned-source-selection.md)。

## 2026-10-18 显式成员恢复

恢复不由401、网络错误或缓存推断触发；用户确认后先持久旧任务授权屏障，正常API核对新组并原子退休旧context。独立LAN和LocalBlock保留，旧queue/task不会重绑新权限，邀请码只在本次调用内存。schema3的退休/intent状态由旧客户端明确拒绝；升级前原字节备份和失败关闭标记保留，不能自动回退旧策略。

首次DELETE也必须证明live actor和固定原目标；旧actor/legacy后台请求不借新成员权限发送。新actor仅在用户新删除动作下CAS换新request ID，原actor最多一次revision重试。完整服务快照与成员变更串行，LAN补组不会穿越恢复屏障。状态、遗留策略范围和边界见[ADR0008](adr/0008-membership-recovery-and-retired-contexts.md)；源码/夹具结果不代替原生或双NAT。
## 2026-09-14 E3 原生共享交接

系统共享适配器只能读取 Go 最近发布的已授权设备 ID、用户别名和可达性，不得到公钥、token、信令凭据或数据库。选择结果不直接授予权限：v2 本机交接记录持久化 `request_id/peer_id/paths/bookmarks/source`，Go 消费后仍调用现有 `Enqueue` 重验 peer grant generation，并以同一 request ID 做数据库幂等。记录提交失败、损坏、未来或同 ID 不同内容时保留证据且不发送；正文不进入 JSON、JavaScript、WSS 或第三方服务。

Windows Share Target 只接受 `StorageItems` 中能由 broker 打开且具有绝对本地路径的普通文件；当前版本明确拒绝只有临时/云端表示的项目，不用默认复制大型文件伪造持久授权。macOS 原位表示生成只读 security-scoped bookmark，由 Go 进程解析并在进程期有界持有；临时表示必须在 `NSItemProvider` 回调返回前复制到本请求独占目录。取消和全部失败分支终结系统 request，不能留下 4097 式悬挂。

正式 macOS App Group 要求签名 Team ID，Windows package identity 也要求可信签名。用户当前没有两平台证书并明确暂缓签名，因此源码保留真实 entitlement/manifest，不开发不受支持的假 App Group，也不把 ad-hoc/未安装包记为原生验收通过。

## 2026-09-13 E2-C1 设置与授权补充

同名策略只影响新建接收计划，优先级为设备覆盖、全局设置、`keep_both` 安全默认；恢复计划不可被新设置改写。设备覆盖随 schema 7 revision CAS 保存。删除后的设备仍保持拒绝，只有用户点击“允许重新添加”才清除本机 block，且不会自动恢复旧 pin 或免确认。

## 2026-09-13 E2接收与设置边界

普通接收按钮携带当前 task ID、attempt ID 和 revision；Go 在创建默认 `keep_both` 计划前后重验这些门闩、授权 generation、目录和空间，并在计划持久化成功后才发送同意。重复点击和陈旧窗口不能同意新的 attempt。免确认偏好在本次 acceptance 之后独立保存，失败会明确反馈且不谎称本次接收失败。

桌面偏好增加单调 revision。分类保存仅合并该分类拥有的允许字段，并以 expected revision 拒绝陈旧表单；背景生命周期继续由独立命令拥有。该机制避免多个页面或异步刷新用旧整份快照覆盖较新的目录、网络或后台设置。

## 2026-09-13 E1成员代际与LAN同意实现

本机 trust schema 2 将完整组快照、组/LAN grant、双方incarnation、服务端revision、本机 `grant_generation` 和provisional LAN transcript统一持久化。组删除先原子移除全部组/LAN pin与免确认并写入带request ID的拒绝屏障及待同步outbox，再取消目标任务，最后提交服务端幂等事务；服务不可达时本机拒绝与outbox保留。完整新快照会撤销已消失成员，旧revision或同一incarnation不能清除屏障；只有可验证的新incarnation和更高revision能建立新的组grant。显式本机block（含schema1迁移记录）永不被成员同步解除。

任务、恢复记录、持久队列和已认证PeerSession保存创建时的授权generation；派发、建连完成、实际开流、恢复和迟到接收确认均与当前generation精确比较，从而拒绝删除前状态在重新配对后复活。授权epoch独立持久，解除本机屏蔽也不能让generation回退。发现、WSS和LAN提交继续重验屏障/generation；授权文件损坏或未来schema仍按拒绝处理。桌面任务库因此由schema5迁移到schema6，迁移前保留可读备份。

Windows 上 `x/net/ipv4.ControlMessage` 不提供有效源地址选择，因此发现发送使用最多 32 个临时 source-bound UDP socket；每个 socket绑定具体本地 IPv4、发送后在同一 socket有界接收回复。Darwin/Linux 保留 pktinfo/cmsg。组播、广播、受限单播与 TLS 控制分别降级，单个 provider 失败不关闭其余路径；peer、route、响应表和握手 goroutine均有固定上限。

2026-09-14 E4-03修复候选：自动剪贴板正文只通过固定身份的TLS 1.3 QUIC传输；独立clipboard_sync能力阻止只支持文件会话复用的旧端进入该owner。两端本地授权代次可不同，lease/event按方向绑定接收方实际generation；origin必须等于认证peer。receive lease和event digest绑定双方permission revision，关闭/重开同一权限不能复用旧lease或已读正文。图片验证在最终状态锁外，实际原生mutation前仍重验暂停、当前peer generation、receive revision、OS generation与原始deadline。密码管理器等明确的Windows/macOS禁止同步标记会拒绝读取；总开关和细分权限均持久默认关闭。物理双机用户剪贴板仍未验收。

E0-ADR-review-v1要求未来删除先使目标peer的全部既有组/LAN grant generation失效，禁止授权回退；新配对不恢复旧LAN/免确认/剪贴板。剪贴板提交门闩内重验授权、lease期限与OS/application generation。以上是待实现契约，不是当前M5安全能力声明。

2026-09-12 M0 安全更新（源码 0.5.0）：本机撤销先原子删除 pin/auto_accept 并保存 denied_peers，
随后尝试服务器撤销。服务器失败不会取消本机拒绝；成员同步、LAN、新发送、确认和恢复都不能复活授权。
解除拒绝不自动恢复 pin/免确认。trust 迁移后丢失、损坏或未来 schema 拒绝授权，禁止从拒绝前的
`.previous` 自动复原。旧二进制不理解拒绝/所有权，须按 [ADR 0003](adr/0003-desktop-ownership-and-local-denial.md)
在隔离备份中回滚，不能直接访问新版 profile。

M1任务库schema3不存第二份授权。草稿路径是本机元数据，网络不发送接收绝对路径；
队列在实际派发前重新验证身份和源摘要，提交失败不派发，源变化需重新确认。
晚到的已完成事件只能推进最近真实使用时间；presence和远端改名不覆盖本地别名或授予免确认。

M2 系统入口仅保存有界本机路径元数据；从 Explorer/Finder 收到路径不授予信任、不自动发送。
入口持久化失败会显示原生错误；未来/损坏日志不执行，SendTo 的安装/移除核对本安装所有权。
Finder 临时/受限 URL 不能作为永久排队路径；具体支持范围见 [M2 记录](evidence/DESKTOP-M2-EXECUTION.md)。

M3 通知使用通用摘要和不透明task ID，点击后重新查询现存任务；不携带绝对路径或正文，不自动打开收到的内容。
默认不开启登录自启动或通知权限；所有系统偏好可撤销。退出时取消信号与控制状态在同一task锁内发布，
数据库写入变慢不能留下“已请求退出但仍未取消”的调度窗口。

当前安全边界适用于 `0.5.0` 源码、协议 V1。设备身份是长期 Ed25519 公钥；动态配对码包含 40 位随机量，显示为 `ABCD-EFGH`，10 分钟有效且单次使用，服务端只保存摘要并按来源限流。输入有效配对码后，客户端自动固定认证成员列表中的设备公钥，不再要求人工核对完整指纹。已固定公钥发生变化仍会拒绝，但首次配对明确依赖信令服务返回正确成员；这比独立 OOB 指纹核验更弱。撤销会关闭该设备的 WSS 和活动协商。

配对码消费现在记录消费设备 ID 并提供同身份幂等重试，不能被另一身份复用。无效、过期、已使用和身份冲突使用稳定错误码；服务端仍受来源限流，数据库仍无明文邀请码。`ttl_seconds` 只修正用户倒计时和时钟偏差，不延长服务端 10 分钟有效期。

LAN 公告是可伪造网络上的不可信输入：消息限 2048 bytes、限频、限时、nonce 防重放并验证 Ed25519 签名、设备 ID、公钥和直连来源。发现只证明“持有该私钥的设备最近从这个 on-link 地址可达”，不证明用户授权。临时 TCP 控制端口强制 TLS 1.3 和双向证书校验；陌生设备必须经过发送方选择、接收方确认、QUIC 身份/文件完整性及双方完成闭环后才写入本地 pin。手工 IP 只接受本机直连前缀内的单个非广播 IPv4 地址，不允许 CIDR、范围或扫描。

QUIC 使用 TLS 1.3、自签 Ed25519 证书和已经固定的对端公钥，验证证书用途、有效期、URI 身份与自签签名；0-RTT 关闭。配对流程虽取消人工核对，数据面仍不存在无条件信任证书的路径。文件正文只在认证 QUIC 数据面传输。

接收正文前必须经过 manifest 摘要决策。默认弹出发送者、内容、数量、总量和目标目录；用户可对单个已配对设备启用自动接收，该偏好保存在本地信任文件中并可随时恢复为每次确认。自动接收不会跳过设备密钥、TLS、路径、块哈希、完整文件哈希或安全落盘校验。

控制消息绑定 sender、recipient、session、generation、freshness 并使用明确的签名编码。断开连接只清理其拥有的协商；旧连接、旧 attempt 和旧 generation 不能修改新状态。拒绝、错误和 confirmed 在 QUIC 发送方向 FIN 后执行有界终态收尾，避免缓冲帧被 reset/close 丢弃，同时受调用者 deadline 限制。

正常连接关闭的容错只适用于已验证 `completed`、已写出匹配 `confirmed` 之后的 terminal flush，并且只接受远端 QUIC application code 0。它不会吞掉非零 close、stream reset、deadline 或正文阶段连接错误；真实 QUIC 正向/负向用例分别锁定这两个边界。

暂停/取消请求也是本地完整性边界。请求后的迟到 ACK 只能补记已发生的传输与验证计数，不能回滚控制状态或恢复正文调度；否则 UI 可能把用户已暂停的任务重新显示为传输中，甚至误记为取消。本边界由真实 QUIC 暂停回归覆盖。

恢复私有元数据保存在本地 schema 2 数据库，不通过 WSS 或 Wails DTO 暴露。它绑定 TransferID、manifest、块策略、路径和 pinned peer；恢复前重新验证源文件、staging、已提交文件及对端指纹。接收提交使用受根目录约束的路径和默认不可覆盖策略；损坏历史记录会隔离，不阻塞启动。

diagnostics 采用允许列表，仅记录实际 base socket、接口、地址族、脱敏候选类型、ICE 时间线、TLS/ALPN、连接方法、relay 实际值、STUN 请求/响应计数、信令字节和稳定失败阶段。禁止记录邀请、固定码、令牌、私钥、ICE ufrag/password、候选扩展、文件正文、用户主目录或未清洗的远端错误细节。

信令和 STUN 仍可能观察在线时间与网络地址；项目不宣称服务器完全不知道元数据。`relay=false`，直连失败不会改走文件中继。

固定配对码 `orion123` 仅用于明确配置的测试实例和内部隔离范围，通用配置默认关闭。香港入口属于测试主站，不是生产安全边界；固定码仍具共享管理员入口风险，应保留限流、隔离、审计与一键关闭说明。正式配对应使用动态单次短码。

## M4 接收计划与提交归属

[ADR 0005](adr/0005-receive-plans.md) 将子集与原 manifest 摘要绑定，并在 acceptance 和终态核对 selection digest。未经对端明确声明能力不能减少正文请求后假报全量成功；跳过不计入验证/提交字节。接收目录仅留在本机，文件正文与截图仍不得走信令、HTTP上传/下载、JavaScript IPC 或中继。

所有目标相对路径继续使用便携 NFC/大小写、Windows 保留名/ADS、路径长度/深度检查以及 os.Root 约束。保留两份最多尝试100个后缀名称；每次最终提交前重查父目录中的 Unicode/大小写等价名称，真正 file commit 使用原子 no-replace hard link，没有覆盖式 fallback。目录枚举可取消且有40,000项扫描上限；空间预估不是落盘成功保证。

原先“目标同 hash 即可视作恢复提交”的宽松路径已收紧。文件必须有匹配的持久 commit record，或在已持久 commit intent 后证明目标与 staging 为同一真实 inode/file ID，才可恢复提交；外国同名同内容文件不计入本任务。目录单独保存真实目录身份（Windows volume/file index，Unix dev/ino），重启不以 CommitStarted 推断 ownership。mkdir意图与目录identity记录之间的窄崩溃窗口若无法证明归属，明确报冲突并保留用户文件与原计划，不合并、不另起名称重复创建。

计划 checkpoint 与 App PlanChanged 持久化回调都必须成功后才能接收正文或按新名称提交。恢复保持已接受集合与已持久名称；已提交文件/目录不允许重新命名，目录失效或身份变化需用户处理。实际 App/UI 和平台验收仍以证据表为准。

M5 内容原生动作按 task ID 重新验证已确认接收、原 manifest、内容绑定和实际文件摘要，不能由前端路径或历史标签授权读取。未知 URL scheme 仅显示/复制为文字，打开动作每次限定 http/https；图片另存不覆盖。快照清理保护活动/恢复/队列/草稿及普通文件路径重叠引用，只释放本应用的引用命名空间，受控 Store 再检查持久 OS 文件身份与摘要，不删除接收用户文件。详情见 [ADR 0006](adr/0006-owned-content-lifecycle.md)。


## E4 认证会话复用

复用键包含固定 peer identity 与本机 authorization generation；每次打开文件流前重新读取当前授权，成员代际变化或本机阻止会关闭该 peer 的池中连接。0-RTT 仍关闭。取消只 reset 对应 QUIC stream；未知连接级错误、路径失效、空闲超时和退出关闭整个连接。复用不扩大文件路径或剪贴板权限。

## E4 自动剪贴板权限

任务库 schema 9 按 peer、authorization generation、方向（send/receive）和内容类型（text/link/image）分别保存 revision CAS 授权。没有记录或代际不匹配等同关闭；新配对、解除阻止或重新加入不会继承旧授权。启用前重新验证当前可信 peer grant，启用提交与撤销使用同一门闩；本机阻止、成员移除或授权 generation 变化使旧授权立即失效，即使清理行失败也不能恢复。该表只保存权限和时间戳，不保存剪贴板正文、图片或系统格式数据。

原生监听器默认不注册。Windows `WM_CLIPBOARDUPDATE` 回调和 macOS `changeCount` 轮询只生成本机 sequence 与格式类别，不在系统回调中打开剪贴板、解码图片、访问文件 URL 或进入 JavaScript。睡眠、锁屏、撤销及退出会注销监听并清空最近变化基线；唤醒或解锁只观察后续变化。

正文事件必须同时满足双方按方向和类型启用的授权交集，并绑定固定 peer、当前 authorization generation、当前 QUIC session 和接收方预签的 10 秒 lease。首帧、完整正文和最终原生写入都不能越过签发时的 deadline；续租不能给旧事件续命。撤销与最终写入采用相同的 grant→trust 锁顺序，因此撤销提交后不能跨过最终 gate 落板。暂停、锁屏、睡眠、重连或 generation 变化会清空 lease 和 readiness。

event digest 覆盖 lease ID、origin device/boot/sequence、Lamport、类型、发送端 OS generation、正文长度与正文。接收方在读取大正文前验证 header，并在读取后再次验证 digest、状态 revision、OS generation 和稳定顺序。Windows 使用本应用有效 HWND 打开剪贴板，在分配和验证数据后执行 generation CAS、deadline 复核、`EmptyClipboard` 与 `SetClipboardData`；macOS 在解码后紧邻 mutation 复核 `changeCount` 与 deadline，但 NSPasteboard 不提供跨进程原子 CAS，仍存在检查与写入之间的系统级窄竞态。该限制必须保留在验收记录中。

2026-09-22 起，lease 的可选 `lamport` 只从已认证且当前授权的 QUIC peer 接收，成功安装租约后才推进逻辑时钟。安装失败、权限/代次不符、暂停或溢出不得推进；时钟更新不改变已固定事件或给旧正文新的寿命。该兼容扩展解决复制次数不同造成的新事件误判，不降低稳定排序、撤权、解码后 revision 或本地复制优先的检查。它不证明旧 Mac 实机失败已修复，仍需双方准确候选的原生复验。

远端 lease 时钟最多允许领先本地 `2^32`，拒绝单次极端跳跃，且在 `uint64` 自然计数即将耗尽时先拒绝而不回绕。按相对差值限制，已同步的高时钟不会在下一次续租被绝对上限误拒绝。新版传输入口拒绝现存 nonce 重放，不重复推进时钟或更新租约期限/采集基线；全部拒绝均先于窗口替换。该边界不承诺抵御已授权对端对自身连接的所有拒绝服务行为。

文字/链接 64 KiB、PNG 32 MiB，并复用 40MP/尺寸和 URL/UTF-8 校验。header 最多 8 KiB，每方向最多两个 lease，全局最多两个接收正文/解码槽；正文只驻留受限内存，不写入临时文件。最坏情况下两个 32 MiB 图片正文及解码对象可同时存在，因此物理平台内存矩阵仍需 E5 测量。原生回写通知按 generation 与 payload digest 仅抑制一次，防止回环同时允许用户真实重复复制相同内容。

## 2026-10-18 成员切换与独立LAN授权屏障

旧组的group+lan关系降为LAN或更换组/本机incarnation时，保留有效独立LAN pin及地址记录，但提升聚合授权代次并更新授权时间、关闭免确认。原任务/队列不能重绑新代次；已有service owner清理剪贴板grant/会话并取消旧任务，重启不放宽。普通同上下文刷新保留原授权，代次耗尽原子拒绝。该本地修复保持wire/schema，不替代仍待实现的显式灾后恢复与旧快照/旧撤销隔离。
