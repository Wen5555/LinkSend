## 2026-10-26 临时分享信令原因与可操作提示

- 真实SQLite/TLS/WSS孤立peer在认证后因果门复现裸deadline/cancel丢成DIRECT_FAILED；现在只在已知ConnectShare/WaitPeer阶段保留typed原因及errors.Is，已有grant/Auth/终态码不被取消竞争覆盖。legacy只有精确signaling_connect获得安全阶段指导，不从原始日志猜ICE/NAT原因。
- 临时分享补固定P2P及RATE_LIMITED提示，保留原intent/明确重试，无自动派发。旧Go/UI失败保留；普通/race各3轮、前端171项/typecheck/lint/build、同版双Go模块workspace/off完整12命令均exit0。准确新CI另核对，未将原生失败改为通过，详见[诊断证据](evidence/RELIABILITY-R3-SHARE-SIGNALING-DIAGNOSTICS.md)。
- 第三轮GUI加夹具批准就绪门后仍未传输：记录UTC接收端开始比发送端失败晚约0.414秒，但未测跨机时钟偏差，不能证明scope缺陷。两端0B、无预览/Completion，资源精确退场。原Check/Wait人机等候问题另作独立修复，不混入本增量。

## 2026-10-26 准确包原生入口与失败证据

- 3d20ccb准确三平台包已下载核对Actions digest、内外BUILD-INFO、四资产SHA及Windows PE/README；52f9bf2的core40240526707、desktop40240526725、packages40240526706再次查询均attempt1/completed/success，未重跑。52增量不倒记进3d包。
- 准确Windows GUI已实际领取且由sender精确批准，但本轮receiver参与晚于sender实际35秒连接期限，两端最终failed/signaling_connect/DIRECT_FAILED，sender verified/committed=0，Windows无preview或本机Completion；不记传输通过，不推断用户现场根因。两轮私有GUI正常退出exit0；首个WebView隔离拒绝/强制退出和未备份全局缓存边界保留。
- 同包实际SaveFile导出安全报告通过，也暴露DIRECT_FAILED及实际signaling_connect仍显示generic/unknown的诊断缺口；未将导出成功当错误说明完善。Mac07:14 SSH仍timeout255。详见[原生入口与保留失败](evidence/RELIABILITY-R5-NATIVE-GUI-20261026.md)，下一轮仅加夹具批准就绪屏障，产品期限保持，R0–R6整体未完成。

## 2026-10-26 R2 私有原生回执基础与删除状态认证

- ADR0016先冻结receipt-only范围；同schema12/cache metadata可选Native签名记录、实际inventory封存、原publication跨main重启单次CAS/回读已实现，原save的run限制保留。launch/boot接口无生产构造或调用，签名不证明native refs语义；任何Native在老化/压力/boot改变后仍保护，未接WriteFiles或发布回收。
- 新自有负例证明collector若信另传的副本，可将真实DB Native剥掉而误删；现重新认证实际row并只依该row判断删除态，源/hash保护。普通/race各三轮及两Go模块workspace/off完整12命令exit0，前端未改；详见[证据](evidence/RELIABILITY-R2-NATIVE-AUTHORIZATION.md)。准确新CI另核对，R2整体未完成。

## 2026-10-26 cd95479 完整CI与原生代码复制目标通过

- cd954790aebaf78b9673fce08351eece1de392e4的core40235670319、desktop40235670269、packages40235670231全部completed/success，三平台包job成功、attempt1无重试。Windows native目标真实run1/pass1/skip0/fail0/exit0，7.54s，恢复租约6934ms；active/paused码不读/缓存/转发，随后新原生text真实QUIC收取，三原始日志已留存。仅覆盖该native子目标，不代替准确Wails UI/文件键盘粘贴或Mac旅程。
- Windows随包说明仍写0.6.0，与BUILD-INFO/PE0.6.2矛盾；已移除重复硬编码，版本/来源引用BUILD-INFO，摘要引用同资产SHA256SUMS，链接通用releases。仅文档变更，将显式打包取得正确说明的候选，复用cd完整代码CI；准确新包结果另记。

## 2026-10-26 Windows原生复制夹具就绪门

- 纯Core/QUIC与copy/deliver桥接回归证明原三秒门在resume时可失败：A接收已就绪、发送等待对端既有七秒lease周期，连接保持健康。现夹具先等receiver实际注册，并按原15秒父context等待真实initial/resumed租约，不改生产、权限、周期或强迫对端续租；103旧CI仍保留调用点歧义。
- 最终普通三轮/race三轮与desktop双模式六检查全exit0，恢复实际约6.98秒、无额外WSS、保护码不读/不发而新文字实传。本机native optin未跑；新提交托管native及准确包继续核对，详见[证据](evidence/RELIABILITY-R5-OWNER-DATA-PATH.md)。

## 2026-10-26 R5 准确源码实网与R2原生引用原型

- `701e23b`干净源码验证器完成Windows↔公共JP Linux双向临时分享，每向3文件16,777,247B、中文/零字节/嵌套空目录1、树hash一致，双方task/local share/grant completed、TLS1.3/linksend/1、零永久授权。控制metadata经隔离SSH HTTPS/WSS，文件独立UDP/Pion/QUIC；Windows正向后台退出码不可观察，其余三actor exit0。服务与通道关闭，临时凭据精确移除，文件保留，见[准确实网证据](evidence/RELIABILITY-R5-CROSS-NETWORK-20261026.md)。不替代Win↔Win/双NAT、WinMac或准确Wails包。
- R2同进程166条trace、真实跨进程496条、launcher退出后helper499条原型已严格构建/运行通过，持有引用一直保护源，最后引用结束才quiescent，系统剪贴板958→958、源全部保留、无残留。未接生产/系统发布，helper仍是live COM producer，签名delegation、安全inventory与持久receipt未做；详见[原型边界](evidence/RELIABILITY-R2-NATIVE-REFERENCE-PROBE.md)。
- 103准确core40233307686/desktop40233307731成功，packages40233307711失败：Windows native真实run/no-skip但会话就绪检查失败，阶段尚不能确定；Mac arm64另为sum.golang.org IPv6无路由，Intel包成功。不重复跑以掩盖问题，正做无系统clipboard的因果回归；整体R0–R6未完成。

## 2026-10-26 R5 本地owner、两阶段数据面与桌面候选

- Core `e53a806` 与desktop `701e23b` 已提交推送并核对远端。701准确core40232117991/desktop40232117989成功，packages40232117997的两个Mac job成功、Windows native检查因测试误读独占零字节.profile.lock失败。仅测试scanner精确排除此根控制锁，纯文件真实持锁旧FAIL→普通10轮及desktop六矩阵PASS；其他读错/伪码仍拒绝，未改生产权限或强跑本机剪贴板，完整native仍待新准确CI。
- 同profile现有SQLite metadata的用途签名/raw CAS owner固定原身份、origin、授权、源与接收计划；稳定控制意图保留响应丢失状态，重启只展示，恢复须明确触发。仅回收服务确认终态且首次观察满七天、无活动/待处理意图的私有metadata，不删用户文件、历史或checkpoint，不新增缓存设置。
- 新独立driver复现目录无写入却SOURCE_CHANGED：Windows目录mtime延后可见。现完整重建源后仅同ID/路径/type目录沿用冻结时间，再比较完整摘要；真实两阶段6条落盘正例与目录增删/改名、同size/mtime换内容、文件mtime变化负例普通10轮及定向race3轮通过。普通文件检查和原wire保持；早期失败、wrapper收尾stderr拒绝与后续准确矩阵分别记录。
- 目录修复生产源码对应新12矩阵全部exit0（core test135385/134782ms，desktop12689/11544ms）。首个workspace test启动后仅新增精确路径/type测试断言，已由最终目录普通3轮及后续core off完整测试覆盖；准确提交CI仍另核对。
- approved经固定公钥的真实Pion ICE/QUIC交换LSSP01完整清单，明确保存/Bind后退役preview，使用新scope/ICE会话/QUIC及原V1引擎。8MiB在4MiB暂停、双端重开Service并显式恢复原grant/claim/task/generation/manifest/plan/目录闭环通过，无第二任务。同IP完整合法流程48–50请求，冗余证明导致的早期RATE_LIMITED失败保留，原全局burst60未提高。
- 受控旧实现分别暴露同步Create/hash/HTTP阻挡退出的五场景及服务Completed冒充本机完成的三场景。现在操作context同时响应profile退出并join；只有持久本机Completion才标本机完成，否则needs_attention且禁止继续。最终20项TemporaryShare测试race3轮app89.487s、相关vet及两模块workspace/off test/vet/build共12命令全部exit0（core test133863/125610ms，desktop16532/11166ms）。
- 桌面源码、19files/155tests、typecheck/lint/build及960×640/1100×720真实页面浏览器夹具通过；夹具Backend为假元数据，不证明RPC/native/P2P。原生复制目标因未证实可备份格式SKIP；独立WindowStation创建遭OS错误5，目标测试未运行、原用户剪贴板不变。严格限GitHub托管Windows的空夹具CI入口已准备，准确新提交CI/包结果待核对，不预记PASS。
- 实验file_transfer_ready仍false，Windows↔Mac/不同网络临时分享与原生完整旅程未完成，R5保持未勾选；Goal当前实际active。准确来源、命令、失败与边界见[本轮证据](evidence/RELIABILITY-R5-OWNER-DATA-PATH.md)。

## 2026-10-26 R5 专属授权协调原型（历史增量范围）

- 先冻结ADR0017两阶段/完整scope，再实现独立临时WSS和双层原AuthBytes签名；preview→Bind退役→新transfer scope，原成员认证/注册表保持。资源、消息结构与绝对期限有界，终态/撤销与普通断线分类分开，不发信令清单/文件正文，不授予剪贴板或普通会话复用。
- 真实SQLite/WSS、三处屏障、预算/重放/角色/期限/重启/关闭安全回归普通与race3轮通过；新客户端46项、race3轮及真实TLS完整协调链通过。root相关完整包/vet通过，完整候选矩阵与准确CI随后核对，见[证据](evidence/RELIABILITY-R5-SCOPED-SIGNALING.md)。
- 该协调增量当时尚未接本地owner、认证P2P/原文件引擎及UI；当前实现见本日owner节。仍experimental_share_control=true才启用、file_transfer_ready=false，准确实网未验，不标完整功能已可用。

## 2026-10-26 R2 文件新复制与续期交错修复

- 后续准确4f562b3969d0dc646e88f92142a566e1c9d07404的core40219427913、desktop40219427884与三平台包40219427841均completed/success；旧eccb903/a22f691真实core失败保留，不重标。

- 准确eccb903/a22f691 core CI的磁盘压力后新复制恢复失败被保留；真实QUIC+原生读取屏障证明同权限续期在Observe后安装，会让最新lease的baseline吞掉新复制，旧实现0.29s稳定FAIL，无第二job。不是空间预留泄漏或DATA RACE告警。
- 按仍当前的本机观察选择有效重叠lease，文件读取前/后均使用，最终BindScoped/权限/期限/新复制/撤权门保持；无有效旧lease不能借续期补发。新回归/原失败/负例普通10轮、相关race3轮和最终双模块workspace/off完整12命令均exit0（core106644/108627ms、desktop7759/7721ms），覆盖最终签名客户端语义去重；缓存删除/容量/用户文件规则不变，准确CI继续核对，见[证据](evidence/RELIABILITY-R2-OBSERVED-LEASE.md)。

## 2026-10-26 R5 签名HTTP客户端

- 新增类型化ShareControlInfo/ShareControl，每次取新epoch、签新nonce；有限响应按actor/ID/清单/task/gen/计划核对，合法后续幂等状态保留。HTTPS/禁止重定向/取消延续现有client，不自动重试副作用请求；远端错误只留白名单码及固定摘要，不回显分享码或detail。
- 真实TLS实验服务下未入组身份完成完整控制链，普通成员枚举/WSS仍拒绝；最终57项2.699s、race3轮5.658s及signaling双模式全包test/vet通过；独立审查发现并以旧实现13项FAIL证明大小写/Unicode别名可绕过去重，修后语义重复拒绝。两模块12矩阵早于最后解析修复，准确完整CI随后核对，app压力恢复竞态另行修复。见[客户端证据](evidence/RELIABILITY-R5-SIGNED-CLIENT.md)。
- 该签名客户端增量当时未接数据面或两端UI；当前实现见本日owner节。仍不广告完整分享，file_transfer_ready=false，R5及原Goal保持未完成。

## 2026-10-26 优先跨网修复同步

- 将已验证edf7cac的默认备用STUN、内置主机有界HTTPS解析和ICE失败真实证据同步到可靠性分支；原正式版最小分支Windows候选及[PR #12](https://github.com/Wen5555/LinkSend/pull/12)已交付，准确源码Windows↔公共Linux双向16MiB/中文/空目录/hash/双方Completed通过。Windows用户暂不能试用，Win↔Win准确候选、受限双NAT及其余Goal必做项仍未验。见[证据](evidence/CROSS-NETWORK-20261026.md)。
- 服务端准入独立修复末尾255单播误拒绝和映射地址危险scope绕过，普通/race及真实认证forward安全回归通过；最小分支99a6c0e已推送，未部署生产，旧CI无实际候选IP，不将其当用户现场已证根因。见[证据](evidence/CANDIDATE-ADMISSION-20261026.md)。
- 当前可靠性工作树两模块workspace/off完整test/vet/build共12命令全部exit0，core test118813/111465ms、desktop9088/8267ms，日志reliability-network-share-client-*.log含退出码和耗时。工作树同时包含接续签名HTTP客户端，分别暂存成可审阅增量，准确提交CI随后核对。无前端源码或文件wire改动。

## 2026-10-26 可靠性 Goal R5 签名控制增量复核

- 后续准确ff6a3274a756cb8c7d08fc87a0dbfd9684dc4745已推送核对：core40216320379、desktop40216320542、packages40216320548均completed/success。旧25f core失败仍保留，不倒记通过。

- 复核既有签名HTTP控制：显式实验配置才激活ShareGrant迁移，运行epoch绑定原AuthBytes/Ed25519，重启旧证明、篡改、重放、真实IP/身份双限频及普通成员隔离回归通过。实验health仍file_transfer_ready=false，不广告完整分享，客户端/P2P/两端UI及真实分享未完成，见[证据](evidence/RELIABILITY-R5-SIGNED-CONTROL.md)。
- 复用该增量源码最后修改之后的2026-10-19双模块workspace/off完整test/vet/build日志；2026-10-26补跑GOWORK=off的server/store/sharegrant定向race退出0（4.299s/3.058s/1.351s）。只读独立复核未发现阻止该增量提交的问题；没有前端或文件wire改动。
- 准确核对旧25f10a2：desktop39180492313及packages39180492292成功，但core39180492297因多个app真实Pion用例broadcast/reserved candidate address及检查超时失败。保留日志与失败，不能用本地通过覆盖；本次准确提交CI推送后另查，不预记成功或R5完成。

## 2026-10-19 可靠性 Goal R5 授权与持久核心

- 先冻结ADR0017，再实现160bit随机码/用途摘要、双重确认、原身份/清单/task/计划绑定、有限恢复、撤销/完成终态；Control同连接事务CAS、显式4→5一致性备份迁移及有界配额/对账。普通服务不激活迁移、不提供临时分享路由或能力，签名控制/P2P/UI及真实分享仍必做，详见[核心证据](evidence/RELIABILITY-R5-GRANT-CORE.md)。
- 实际SQLite32路领取1个winner，撤销与完成并发1个终态，跨组/未入组不新增永久成员，存储重开保持原授权与绝对期限。备份夹具journal_mode误改造成首轮FAIL，query_only核验修正后通过；最终定向普通/race各3轮、两模块workspace/off完整12命令退出0。CI推送后另核对，不标R5或Goal完成。

## 2026-10-19 可靠性 Goal R3 可操作诊断候选

- Go白名单投影提供任务/记录原因、阶段、下一步、版本/连接方法与真实尝试时间；仅用户主动SaveFile导出有界安全JSON，不复制私有路径/凭据/正文或原始异常，已有文件不覆盖，Windows取消不写旧路径。文件wire/schema12和原授权控制保持。
- 前端130项/typecheck/lint/build、核心组合race3轮、两模块workspace/off完整检查通过；浏览器布局发现并修复说明行负margin，双尺寸已看图。准确候选原生导出与最终旅程继续，见[证据](evidence/RELIABILITY-R3-DIAGNOSTICS.md)，R3暂不打整体完成勾。
- 上一565840e的core/desktop与三平台包CI全部success；旧1bc92e0 Mac arm64失败作为历史保留，不移记本批CI或原生通过。
- 后续01b5c16准确Windows包已通过原生保存/取消及任务诊断跳转；恢复阶段内部名在新源码映射为“已保存，等待恢复”，前端130项/构建与两模块workspace/off完整12命令通过。新代码CI与新包另核对，不倒记旧包已有修复。

## 2026-10-19 可靠性 Goal Mac候选交换因果检查

- `1bc92e0`已推送且远端一致；core/desktop CI成功，包CI39172841244的Mac arm64旧墙钟断言FAIL，Windows/Intel Mac包success。准确记录失败，没有用本机重跑覆盖。
- 透传真实Pion候选但故意不交付收集结束通知，验证选中host对后交换能独立结束；保留原6秒父期限，另用禁用结束分支的独立Mac负例证明6秒准确超时。Mac普通100/race50轮、两模块workspace/off完整12命令通过，Windows交换相关普通/race30轮通过。详见[证据](evidence/RELIABILITY-R1-TRICKLE-CAUSAL-CHECK.md)。新提交CI待核对，物理跨网/键盘粘贴未抵扣。

## 2026-10-19 可靠性 Goal R2 未发布对象生命周期

- 同profile owner活动引用持续至真实句柄关闭，Go-only关闭观察器提供实际存储来源；当前worker在native前封存完整未发布清单，再接入有界分页、压力、启动继续删除意图及退避。取消/UI终态不释放引用，未知条目和用户改动不递归删除；具体测试、早期失败及预算见[证据](evidence/RELIABILITY-R2-CACHE-LIFECYCLE.md)。
- 70个空目录的实际协议回归保持接收可用，超出清理封存预算不缩小文件协议。已发布、未封存/未知崩溃残留和大对象仍保护，原生读取结束许可与键盘粘贴继续，R2不标完成。
- 上一持久owner `c4910f9` 的core/desktop及三平台包CI全部success；本增量最终检查与新CI单独记录，不移记旧包或原生通过。
- 本增量最终两模块workspace/off的test/vet/build共12命令退出0；最终源码定向race3轮app43.659秒/transfer3.864秒，完整app文件/缓存/退出组合race3轮90.230秒。无前端改动，显式gofmt清单及diff检查通过；新提交CI推送后核对，不预记成功。

## 2026-10-19 可靠性 Goal Windows覆盖安装与原生诊断

- 准确01b5c16的core/desktop/三平台包全部success，包来源/digest/payload核验通过。隔离安装63c86c2后真实活动256MiB任务在20MiB验证处保存退出，覆盖01b5c16后身份、配对、偏好、中文草稿、历史和合法待恢复状态保持；单实例唤回通过。两安装与受管卸载退出0，原系统登记/三个快捷方式恢复回读通过，GUI/peer正常退出。
- 准确候选Windows SaveFile保存/取消及任务到诊断设置导航通过，1565B白名单报告hash/无身份路径文件名已回读。恢复阶段内部名可见待修，完整续传、SendTo/拖放和Mac旅程仍未验。协调服务续起被自动审批拒绝，仅保留原夹具自行到期，不绕过；详细来源/哈希/证据和边界见[Windows记录](evidence/RELIABILITY-R4-WINDOWS-UPGRADE.md)。R1–R6必做未齐，不标Goal完成。

## 2026-10-19 可靠性 Goal R2 持久owner首批

- 在现有SQLite metadata命名空间保存严格版本/用途/身份/运行代次及Ed25519签名的缓存归属，CAS+回读；实际引擎Root/Stage/File句柄来源在accept/body前登记，publishing先于原生写入，DB I/O在授权/状态锁外，实际mutation仍复核控制与CAS。无新数据库/schema/wire，损坏或未知记录保护。
- 首轮首次checkpoint漏接回调的安全失败保留，统一notifyPlanReady后修复；真实QUIC、before-body门、签名/外来inode/SQLite故障及真实profile key重载测试核对所有权与不重放，详见[证据](evidence/RELIABILITY-R2-CACHE-LEDGER.md)。完整活动引用/对账/自动回收、原生完成证明和键盘粘贴仍未完成。
- 本地最终普通3轮、app/transfer定向race各3轮及两模块workspace/off test/vet/build全部退出0；前端未改，复用同产品前端来源。推送后继续核对CI，源码记录通过不抵扣真实系统粘贴或自动回收验收。
- `bed8d2c` 的core/desktop及三平台包CI全部success，Linux实际目录替换分支已被完整core测试/race覆盖；不冒充系统粘贴或安装验收。

## 2026-10-19 可靠性 Goal R2 路径与不确定发布

- 原生adapter已接收路径但确认报错的注入回归先复现文件被删；修复后保留已验证缓存并显示准确不确定状态。接收/checkpoint使用同profile owner固定的Root，目录字符串不再可替换写入授权；无wire/schema变化。Root闭合、错误目录/伪checkpoint、Windows junction及已有用户文件保护均有回归，真实QUIC夹具与文件系统/内存原生adapter范围见[证据](evidence/RELIABILITY-R2-CACHE-PATHS.md)。
- 修正ADR0016的boot假设：系统恢复旧路径后仍可开始新粘贴，boot+TTL不构成安全回收证明；完整原生交接、持久所有权/自动回收和真实键盘粘贴继续，不记R2完成。
- 最终两模块workspace/off test/vet/build、app定向race3轮（23.836秒）、Root race3轮及前端127项/typecheck/lint/build均退出0；Windows真实junction边界通过，固定目录重命名被系统拒绝，Linux替换分支待CI。没有日常剪贴板或生产改动。
- `f2712dc` 的Intel Mac DMG随后success，core/desktop及三平台run39165099505全部通过。没有把CI包通过写成原生旅程或安装通过。

## 2026-10-19 可靠性 Goal R2 自动容量首批

- 先形成ADR0016，再落实真实manifest自动空间预留、32路竞争容量保护、发布返回后幂等释放、低空间安全拒绝和下一次复制恢复。原代码低空间仍发布的负例及第一轮本地错误被取消覆盖的失败均保留，现已修复；健康会话不因单流磁盘拒绝关闭。无wire/schema变更，不新增缓存用户设置。
- 定向普通3轮、组合race3轮/立即复制race10轮、前端126项/typecheck/lint/build及最终两模块workspace/off test/vet/build全部通过；一次组合race恢复失败和取消I/O确定性负例保留，取消意图修复后维持立即复制测试，见[容量证据](evidence/RELIABILITY-R2-CAPACITY.md)。已发布缓存仍保留，自动所有权/回收与真实键盘粘贴未完成，R2不打完成勾。
- 上一增量 `f5f05ef` 远端SHA一致，core/desktop及三平台包CI全部success，详见R1证据；不代表对应包已完成物理关键旅程。
- 本增量 `f2712dc` 已推送且远端SHA一致，core/desktop CI已success；Windows和Mac arm64包通过，Intel Mac DMG构建在本记录时仍运行，后续继续核对。Goal保持active，R2完整生命周期/自动回收及其余必做项继续，不将容量首批记作R2整体完成。

## 2026-10-19 可靠性 Goal R1 健康会话保护

- 新定向真实ICE/QUIC回归先复现 `NetworkChanged` 关闭剪贴板活跃/文件空闲连接的缺陷；仅清理已由QUIC报告关闭的空闲会话后，通过通知后双向剪贴板、同SessionID文件完整传输/确认及失效连接清理。保持原transport失效与恢复机制，无wire/schema改变。
- 五项定向组合普通3轮及race3轮（42.175秒）、两Go模块workspace/off的test/vet/build退出0；准确提交CI按[增量证据](evidence/RELIABILITY-R1-SESSION-REFRESH.md)另核对。R1物理双机/睡眠/真实跨网仍未完成，Goal保持active；不覆盖旧证据边界。

## 2026-10-19 可靠性 Goal R0 基线

- 用户明确启动 Goal，当前 active；基线为 `f8358ce`，工作分支与远端一致。保留三个未跟踪目录，复用现有便携合规工具链。Mac arm64/macOS26.5 与日本支持主机只读 SSH 核对通过，未影响日常进程或生产服务；台湾 NAT 别名探测255且无原因证据，不计可用环境。
- 修正 README 当前版本0.5.0→0.6.2及计划/路线入口的未启动描述；唯一 TODO 新增 R0–R6实际状态。详见[R0证据](evidence/RELIABILITY-R0-BASELINE.md)。本增量仅文档，检查差异与本地链接，不重跑应用测试；R1–R6功能和原生/跨网验收继续。

## 2026-10-19 可靠性 自动缓存与临时分享计划

- 形成[实施计划](RELIABILITY-AND-SHARING-PLAN.md)与[Goal启动提示词](prompts/RELIABILITY-AND-SHARING-GOAL.md)：连接/恢复/跨NAT、文件键盘粘贴、程序全自动缓存、可操作诊断、安装升级/退出、一次性临时分享与工程维护。保存常用发送方案排除；自动缓存不设手动清理和容量/期限选项。
- 记录用户本轮相关SSH及阶段commit/push授权，区分生产替换和宿主网络边界；明确各工作包验收、准确包来源和外部阻塞规则。本次为文档交付，未创建/恢复Goal、修改产品代码或执行远程测试。

## 2026-10-19 v0.6.2已发布

- PR #11正常合并main，准确程序包源码`63c86c2c5ce183e1400d27ca0571610d50a42bdc`；[v0.6.2](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.2)已公开并设Latest，tag一致。main core/desktop及[三平台包CI](https://github.com/Wen5555/LinkSend/actions/runs/39157767929)全部success，四个程序包加两份元数据共六项资产digest核验通过。
- 两Mac DMG的版本、架构、最低OS元数据及严格签名检查通过，未启动新app且挂载已卸载。Windows真实GitHub更新检查与安装器下载/摘要验证结果见更新证据；未执行安装。README和发布说明同步来源，物理候选证据与准确发行包检查保持各自边界。

## 2026-10-19 v0.6.2发布准备

- 用户明确要求GitHub版本更新推送。产品元数据统一0.6.2，合并本轮LAN连接修复、独立files授权/能力与schema12迁移、32MiB居中复制进度及既有GitHub更新入口；文件正文仍走认证QUIC/V1引擎，relay=false。
- Windows↔Mac同LAN系统剪贴板的文字、不同像素图片和文件条目双向通过；64MiB文件校验、两端原生进度与取消/未完成缓存删除通过。图片校验器颜色空间误判已纠正；最后两端备份恢复成功，测试进程停止，日常Mac恢复同步。完整命令/包来源及最终FileID状态回归见[实施记录](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)。
- 两模块workspace/off最终检查与三平台准确提交CI、资产版本/摘要、Mac包只读挂载和正式Release按顺序验收；本地候选实机不冒充后续发行包运行。Explorer/Finder键盘粘贴、Intel/macOS13原生、双NAT与历史A–E其他缺项保留，不部署生产或改宿主网络。
- 首轮CI发现同时LAN拨号的真实会话冲突，确定性屏障先复现失败，再以bootstrap范围的认证SessionID一致选择修复；文件预留/接收与已建立会话不替换。最终结果及准确来源另列，失败保留[证据](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)。

## 2026-10-19 局域网自动剪贴板连接修复（首轮本地未发布记录）

- 用户反馈同一局域网设置显示“连接失败”。补充经过LAN TLS控制入口的真实Pion/QUIC回归：旧接收路径始终等待文件双向流，未启动剪贴板单向流owner；单独恢复旧owner后，同一回归实际FAIL。现与WSS入口一致，把可复用认证会话立即交给pool，并释放该连接独占的LAN TLS控制通道。
- 剪贴板外层两处10秒期限会截断桌面既有30秒分阶段连接预算；10.1秒受控延迟在旧实现实际FAIL，移除额外截断后PASS，保留核心阶段超时、singleflight和关闭取消。重试/lease就绪清除旧连接错误及连接等待态，保留原生读写错误。
- 新回归覆盖无文件先行的LAN双向文本、同时连接、相同会话ID、文件空闲期限后保持及文件复用/实际落盘内容；两Go模块off test/vet/build、核心定向race、桌面完整race及前端123项/typecheck/lint通过。包与最后验收见[实施记录](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)。这些是同机隔离loopback控制路径证据，物理双机系统剪贴板仍未复测；无wire/schema/授权默认值变化，未提交、推送或发布。

## 2026-10-18 桌面GitHub更新功能（本地未发布）

- 基于main `3aa5148`在`codex/github-desktop-updates`加入“设置 → 软件更新”：手动查正式Release、按平台下载、进度/取消、摘要校验后打开安装包。Go负责网络与缓存，不依赖信令；Windows使用安装向导，Mac使用DMG，安装前需退出旧应用，用户配置和文件保持。关于页改为真实后端版本，协议V1与正文路径不变。
- 前端123 tests/typecheck/lint/build、两Go模块GOWORK=off test/vet/build与更新模块race通过；Mac两架构仅更新模块测试二进制交叉编译。真实HTTPS下载当前0.6.1的75,189,777B Windows安装包并核对SHA256通过，测试未执行安装器；本地Windows生产试用EXE已构建、未启动。前端重建与Go embed并行的一次资源读取失败已记录，完成前端后顺序重跑通过。
- 未提交、推送或发布新版本，原0.6.1 tag/资产与物理原生/NAT缺项保持。具体命令、下载摘要、试用来源及安装限制见[实施记录](evidence/DESKTOP-GITHUB-UPDATES.md)与[ADR0014](adr/0014-desktop-github-updates.md)。

## 2026-10-18 v0.6.1已发布

- PR #10已合并main，准确源码`72161e3f806d742d1d401564031ab4844fe88b2d`；[v0.6.1](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.1)已公开并设Latest，tag与源码一致。main core/desktop/三平台包CI全部success，四新程序包加校验/来源清单共六项资产digest核验通过。
- 两Mac DMG的版本、架构、最低OS元数据与严格签名通过；包未启动、所有挂载已卸载。README下载与使用说明、发布说明、ROADMAP、TODO和交付记录同步；物理系统剪贴板/文件关联/其余原生及完整双NAT未验范围保持，未部署生产信令。
- 实际CI失败的测试观察窗口修正后，`go test ./internal/app -run '^TestRemoteCancelledStreamKeepsAuthenticatedSessionForNextFile$' -count=15 -timeout 90s`及相同用例`-race -count=5 -timeout 90s`均通过；最终准确提交的完整core/desktop检查与三平台打包通过。

## 2026-10-18 v0.6.1发布准备

- 首个main候选e020d87的Windows打包在既有远端取消测试中读到过期快照而失败：先Task后hasActive会跨越worker结束。调整为确认worker结束后读取结果，保留ErrorCode/会话复用断言；15次旧实现定向运行未复现，但失败日志与锁内结束发布顺序确认了观察窗口。测试收尾提交后重新从准确main构建，不重标旧包。
- 用户明确授权推送0.6.1；在现有修复分支同步产品常量、Wails/Windows/Mac/Share Extension/前端与打包workflow。复用上一轮四项功能验证，继续记录准确提交和全新包的CI/版本/来源/hash；不部署生产信令或恢复受限物理矩阵。
- 修复源码与已验本地来源逐项匹配，备份未提交patch和新增文件于`.artifacts/release-061/`；原三个未跟踪目录保持。版本一致性与必要构建通过后提交/合并main，发布结果另记。

## 2026-10-18 0.6.0后四项桌面工作流修复

- 用户反馈后在`codex/fix-desktop-workflows`实现文件打开/定位、自动剪贴板原生监听与恢复即续期、删除与黑名单分离、统一收发队列/清理/退出设置/红点。修复源码与原0.6.0发布分开，未提交推送或覆盖Release。
- 剪贴板暂停恢复回归在旧实现实际失败，立即续期后真实认证QUIC新文本到达；核心定向race、桌面完整race及119项前端通过。布局夹具1120/720无横向溢出；物理Windows↔Mac OS剪贴板/文件关联/新Mac包仍NOT_RUN。原A–E外部阻塞保持。
- [ADR0013](adr/0013-desktop-transfer-workflows.md)与[实施记录](evidence/DESKTOP-060-WORKFLOW-FIXES.md)列出行为、命令与来源；当前修复留本地可审阅工作树。

## 2026-10-18 v0.6.0已合并main并发布

- PR #9正常合并，准确源码`8bad9323ece46b2e32054ddb321340dfdd1e3ec8`；[v0.6.0](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.0)已公开并设Latest，tag与源码一致，包内版本0.6.0/协议V1。新包来自主分支run 39103713740，全部源码CI和三平台包成功，六项发布资产digest与本地SHA256一致。
- README下载入口、版本使用说明、ROADMAP、唯一TODO及交付记录已同步；无新物理/原生矩阵或生产部署。旧功能证据保留原来源，完整Goal仍blocked。详见[发布说明](RELEASE-v0.6.0.md)与[交付记录](evidence/DESKTOP-E5-DELIVERY.md)。

## 2026-10-18 v0.6.0发布准备

- 用户明确要求合并main并发布0.6.0，覆盖此前不合并/不正式发布的限制；既有未验项继续保留，未恢复受限Windows原生测试或宿主网络变更。
- 产品常量、Wails/Windows/macOS/Share Extension/前端和打包workflow版本统一到0.6.0；协议保持V1。生成build assets到隔离artifact，保留项目定制模板及原未跟踪目录；版本一致性检查通过。legacy-server测试中的0.5.0保持为旧服务fixture。
- Go1.27.1重建的固定Wails beta.18已无警告生成绑定（无差异），desktop GOWORK=off测试通过；前端构建/typecheck及两模块GOWORK=off构建也已通过。待CI完成后，从合并后的准确主分支提交构建全新包，发布来源与资产收据在完成后补记；旧6ec3cc4功能原生结果不移记为0.6.0新包物理验收。说明见[0.6.0发布说明](RELEASE-v0.6.0.md)。

## 2026-10-18 Goal外部阻塞收束

- 四项功能可用收尾已交付，按用户要求不再扩展重复矩阵。相同Windows原生启动策略与JP UDP补充授权条件跨三轮保持，平台`update_goal`已返回blocked；整体A–E未标完成。
- 本轮只读核实Windows LinkSend进程数0、最后preset专属服务job finished/exit0、当前b73d132工作树无跟踪改动。没有新原生/网络测试、源码变更或运行资源；恢复入口及精确剩余条件见[唯一TODO](DESKTOP-EXPERIENCE-TODO.md)。

## 2026-10-18 四项功能可用收尾

- 按用户最新要求复用已有验证，缩减重复矩阵。6ec3cc4准确Mac只新增一个原生目录picker/源码预设/自定义排除保存与80B实际QUIC发送，三文件集合/hash/中文空目录及排除内容缺席均通过；原生快速目标下拉清空/选回B也已保存，不再仅依赖fixture预置目标。
- 两平台正式`/healthz`均HTTPS200/TLS verify0；首次误用`/health`的426保留为错误路径观察，不外推为WSS或跨NAT通过。所有新owned资源停止、监听关闭、clipboard=false，未更改产品源码、宿主网络/权限或日常profile。
- N1–N4实现与可用收尾证据已汇总，准确候选仍6ec3cc4，完整物理/网络缺项如实保留。见[当前检查点](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)与[唯一TODO](DESKTOP-EXPERIENCE-TODO.md)。

## 2026-10-18 6ec3cc4准确Mac N2故障、重试与取消

- 原生checkbox勾选第二目标、明确等待、picker与一次批次提交通过；B在线实际74B完成，C离线等待。专用源变化令C真实SOURCE_CHANGED failed/需确认，B保持；真实重启后明确只确认C并重开已终态的一次性接收器，同一子项完成新98B/hash/双方确认，B没有重试。
- 新16MiB批次原生独立取消等待C后B维持活动；再取消剩余批次，B停止、无提交目标，前一批文件保持。真实重启所有批次/队列/task/草稿整行保持；GUI退出后直接CLI恢复被BATCH_CANCELLED拒绝，无新增工作。
- 无产品源码改动，沿用6ec3cc4准确包/CI来源。同机两个软件目标，保留预期failed历史和两个接收中断exit1；最终app/CLI/专属服务终态、监听关闭、clipboard=false。详细源版本、IDs、失败观察器与收据见[N2证据](evidence/DESKTOP-AE-DEVICE-BATCHES.md)，物理与完整A–E仍待。

## 2026-10-18 6ec3cc4准确Mac N3活动限速与控制

- 原生保存256→1024KiB/s在32MiB活动发送中实际生效：每阶段两个稳定4MiB确认区间，正文中位数约253/1005KiB/s；实际32MiB加38B文件hash/双方确认通过。口径为同机确认进度，排除混合区间/合并ACK，不是物理吞吐或UDP线速。
- 1KiB/s等待时原生暂停约1.78s；恢复为同任务/Transfer的新attempt后原生取消约1.88s，源保持、无提交目标，随后明确取消遗留待确认队列。另独立4MiB样本在等待中原生保存0，约3.13s后完成/hash/双方确认；两次有意中断CLI exit1保持真实边界。
- 真实重启0上限/草稿隔离/旧记录保持、无重复派发；app/CLI/采样器/专属服务终态、监听关闭、clipboard=false。无产品源码修改或新构建声明，仍用6ec3cc4准确包；数据、失败观察器和jobs见[N3证据](evidence/DESKTOP-AE-FILE-BUDGET.md)。OS剪贴板并存与物理速率仍待。

## 2026-10-18 6ec3cc4准确Mac Finder Services闭环

- 实际Finder选择三项中文/空格文件和含空目录的文件夹，真实“访达→服务→使用LinkSend发送”两次从后台唤回同一PID，main合并四条唯一路径、原目标/规则/quick/旧任务队列保持；真实重启草稿保持。
- 随后原生主发送完成三文件110B/五条manifest、实际hash与空目录结构一致、双方确认、relay=false；仅显式发送后清main。专属同机loopback，正常配对fixture复用一致性控制库副本，不是物理双机/B2整体通过。无产品源码修改或新增构建测试声明；沿用6ec3cc4准确四资产与CI来源。
- App/receiver/专属服务均正常终态、监听关闭、原profile/源hash/身份偏好/备份保留、clipboard=false。准备/时序/备份脚本失败与续验收据均见[原生证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)。

## 2026-10-18 N4本机目标边界修复

- 三项新增回归在旧实现真实失败：候选包含本机、保存本机目标发送按钮可用、本机身份未加载仍可选/可发。App传入配置身份或shell身份，面板过滤本机，身份未知时等待，保留不可发送的旧目标/路径；新增身份晚到正常恢复回归。Go授权/wire/schema无变化。
- 前端119项tests、typecheck/lint/build通过；核心workspace/off test及off build、desktop workspace/off test及off vet/最新dist后的off build通过。6ec3cc4已推送，五项CI/三平台包与四资产完整核验success。
- 准确Mac原生self目标在勾/不勾等待均禁发、重复菜单唯一面板/零队列通过；正常目标由夹具业务API预置后，真实picker/取消重开/入队、45B固定身份QUIC/唯一目标hash/双方确认、后台唤回/真实重启无重复与菜单退出通过。原生目标下拉仍受AX外壳限制，不报选择成功；loopback不替代物理。app/receiver/fixture均已终态，clipboard=false，见[N4记录](evidence/DESKTOP-AE-QUICK-SEND.md)。

## 2026-10-18 0ae7cdf准确包修复复验与N4原生负例

- 0ae7cdf五项CI/三平台包与四资产完整核验通过；新准确Mac在同profile/URL使原先卡住的设备RPC恢复，菜单quick识别离线B，两次打开唯一面板、未等待禁发送，实际勾等待/入队形成唯一waiting_peer。原生picker补回文件/取消重开/main保持、后台关闭菜单唤起/真实重启需确认、远端撤权后即使勾等待仍禁发送均通过；tasks=0，无本轮文件或物理通过声明。
- 原生菜单退出后app和fixture均停止，端口关闭、SQLite校验/身份/偏好/源hash/两草稿保持，clipboard=false。初始目标/文件为夹具预置、双击送达与Win托盘仍未验；旧98469ce 31B结果不迁移。源码/失败/准确hash/job/收据见[N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)。Goal active，其余物理/OS/双NAT及授权条件保持。

## 2026-10-18 原生设备RPC被共享目录I/O阻塞的修复候选

- 6f29814准确Mac离线quick现场两次复现后端3设备/原生列表未返回；受控SIGQUIT栈定位App.Devices及native-entry worker卡在系统共享快照os.OpenRoot。未入队或传输，两草稿保持；主动诊断终止不当作自发崩溃。
- Devices RPC不再同步写共享快照；显式/非默认profile只访问自己入口目录，默认profile还须TeamIdentifier与准确App Group entitlement。三个隔离及两个真实目录副作用回归RED→GREEN，核心off/desktop两模式/off-race/vet/build通过；Mac SDK/C语法和ad-hoc探针通过。新包/原生复验继续，见[N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)与[ADR0003](adr/0003-desktop-ownership-and-local-denial.md)。无前端/wire/schema/宿主权限变化，Goal active。

## 2026-10-18 准确Mac同URL空库恢复与旧队列隔离

- 6f29814准确包在Mac loopback同URL切换独立新空控制库，原生prepare/cancel/再次prepare与真实重启保持、新码加入、再次重启/认证WebSocket通过；独立C核对旧身份加入新组。两草稿/身份/偏好/源hash/旧queue绑定保持，旧context退休。用户手动合成短码已新鲜AX实读9字符匹配，未提交该合成码；不升级旧18字符/legacy异常根因。
- 原B同identity经正常恢复API加入新组后，准确Mac真实点击旧queue确认，被授权代次拒绝，revision/绑定保持、tasks=0。app/loopback夹具已正常停止，两个控制库与私有备份保留、端口关闭、clipboard=false。夹具来源/hash、失败与收据见[恢复证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。无生产源码或宿主网络修改；Win原生/物理/OS/双NAT及Goal仍未完成。

## 2026-10-18 6f29814准确Mac恢复闭环

- 逐字符输入修复6f29814五项CI/三平台包全部success，四资产完整来源/hash/COMMITTED/clean/版本通过。新准确Mac以原profile、同started intent真实点击继续恢复，正式服务独立查询从2成员到3、原identity登记；重启后独立认证WSS在线，原身份/偏好/两草稿/源hash/唯一completed任务保持，旧context退休。属旧loopback组→不同正式URL，不是同URL空库或物理文件。
- 原生合成短码经实际点击+逐键/新鲜AX读取匹配，真实码提交前核对准确；43字符legacy额外原生键盘输入仍不匹配、未提交，不当作产品根因或PASS。收尾窗口无信令错误/继续恢复按钮，app已停止、clipboard=false，profile/文件/备份保留。详见[恢复证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)；Goal active，其余必交付缺项保持。

## 2026-10-18 逐字符邀请码兼容性修复

- React逐字符输入复现legacy邀请码被前端大写/截短；普通配对、明确切组和prepared/started恢复回归6 FAIL→GREEN。改为编辑保留原文、提交只规范化完整短码，非法输入不静默截成有效码；不改变Go/wire/schema。Mac原生18字符观察的根因仍未确认，原started请求保持。
- 40定向与完整115前端tests、typecheck/lint/build、核心off完整test、desktop workspace/off test及off vet/build通过。新准确包/原生复验待继续，见[恢复证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。Goal active，物理/OS/双NAT及授权缺口保持。

## 2026-10-18 cf19349准确包错误提示复验

- cf19349五项CI/Windows与Mac两架构打包success，四资产完整来源/校验通过；新准确Mac arm64真实无效配对已显示中文提示，独立空profile包/签名/hash核验及收尾通过。见[准确包证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)。没有升级旧包的文件/网络验收范围。
- 98469ce准确Mac原生31B quick发送、取消重开/主草稿保持/后台菜单唤起/重启无重复已有收据。公共恢复原生prepare/cancel/重启保持通过，但join输入无效仍started、未加入；进一步重启同intent保持、窗口已还原，新码字段空，等手动合成输入观察。新CF错误测试app已停，原恢复43588保留，clipboard=false，Goal active。

## 2026-10-18 Mac原生quick实收与错误提示修复

- 98469ce准确Mac真实菜单/picker/取消重开/入队完成31B Pion/固定身份QUIC，实际接收hash一致、双方确认、仅quick清空、main整行保持；后台关闭/菜单quick唤起与真实重启无重复通过。目标由夹具业务API预置且用户协助选择，该边界与Win托盘/物理缺项保持，见[N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)。
- 同post-quick profile私有备份后连接已部署正式信令，原生恢复prepare/cancel/实际重启保持通过；join自动输入后PAIRING_CODE_INVALID，独立server仍2成员/身份未加入、intent=started。合成文本自动输入长度异常正在核实，不能标恢复完成，见[恢复证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。
- 原生HTTP包装令首个HTTP token遮蔽协议码，新增回归RED→GREEN，局部解析后使用既有中文提示。前端109tests/typecheck/lint/build、核心GOWORK=off完整test（缓存）、desktop workspace/off test及off vet/build通过；Go/wire/schema保持。新源码CI/包另验，以上原生结果仍绑定98469ce。

## 2026-10-18 98469ce准确包与公共信令复验

- 98469ce已推送、五项CI/三平台包全success，四资产完整校验；新准确Mac菜单栏LinkSend文字/主点击菜单/quick面板通过。main revision1整行保持；目标下拉框后AX仅外壳，录屏preflight false/未请求新权限，完整quick发送待现场观察，见[N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)。
- 用户提醒信令错误后，明确原实例127.0.0.1:1隔离条件，另实测Win/Mac正式HTTPS/正常配对/邀请码重放拒绝/两路认证WSS/同身份verify通过；6edddad准确Mac公共实例主页可接收，独立peer确认online。旧TLS超时未复现/根因未确认，未改宿主网络。JP信令/STUN-only/备份timer active，见[信令证据](evidence/DESKTOP-AE-SIGNALING-REBUILD.md)。
- 旧Mac包38062及公共实例39366已停；当前准确98469ce39645与专用Mac loopback fixture保留供UI续验，clipboard=false。Windows策略拒绝、JP外部UDP授权、物理/OS/双NAT缺项保持，Goal active。

## 2026-10-18 Mac原生quick续验与菜单入口修复

- 用户开启AX后，6edddad准确Mac包真实菜单→快速面板→文件Open/Cancel→面板取消通过；空send_drafts整行保持，无配对/发送。用户确认托盘存在。此前“图标或菜单未出现”保留为观察，未证实具体触发原因，见[N4证据](evidence/DESKTOP-AE-QUICK-SEND.md)。
- Mac菜单栏候选加LinkSend文字、主点击保留Wails原生菜单；Windows行为保持。核心off全test与desktop workspace/off-race/vet/build通过，标签追加后off-race/vet/build再次通过。新包原生主点击/非空草稿/完整quick发送待验。
- 当前Mac隔离实例故意连接127.0.0.1:1；正式公共HTTPS/WSS及客户端仍独立验收，旧TLS超时未升级通过。Goal active。

## 2026-10-18 6edddad 准确包复验

- 取消修复6edddad五项CI/Windows与Mac两架构包全部success，四资产完整核验；准确Win三个原生picker取消无红色错误，草稿整行/接收偏好字节不变，f23383d旧失败保留。见[本轮原生证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)。
- 同准确Win包原生选择源码预设/两个已配对CLI，产生一个批次两个子项、顺序真实Pion/固定身份QUIC完成；各1MiB/hash/中文空目录/排除集合及双方确认通过。256KiB/s原生设置持久，正文阶段3.979/3.942s为loopback粗略口径。后台关闭后单实例唤回、主草稿精确保持；真实托盘quick入口仍工具未暴露，不能替代。
- 同提交Mac arm64实际启动/签名/架构/hash通过，未操作UI。新Win独立LAN现场启动被工具自动审批在CreateProcess前拒绝，仅blocked by policy/无细分原因，scene目录不存在，未绕过；该旅程NOT RUN。Win/Mac owned包/fixture全停止、clipboard=false、文件保留，Goal active及外部缺项保持。

## 2026-10-18 准确包原生检查点与取消选择器修复

- f23383d五项CI/三平台包success，四资产/BUILD-INFO/COMMITTED/clean/hash核验通过；准确Windows包完成原生N3设置、N1预览/CLI入口/真实QUIC接收集合与hash。Mac准确arm64包启动/签名核验通过，辅助功能关闭，UI/配对未验；托盘/菜单栏及物理文件不冒充通过。见[证据](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)。
- 原生取消目录picker保留草稿却显示红色错误，已在三个Windows picker边界精确归一取消，其余错误保持。桌面workspace/off-race/vet/build、核心off完整test/vet/build通过；核心旧设备数断言受正常LAN公告影响，改为按identity核对信任与唯一性，同现场race三轮20.091s通过。新包UX复验继续。
- 同测量器十轮后测双方发现/刷新10/10，Win控制10/10、Mac0/10/no route to host；发现中位数Win226→255.5ms、Mac141→81.5ms，不报告整体提速/文件吞吐。旧原生包/fixture已停止，profile/文件保留，剪贴板关闭，详见[LAN证据](evidence/DESKTOP-AE-LAN-RECOVERY.md)。Goal active，外部授权缺口保持。

## 2026-10-18 N4 托盘快速发送候选

- 托盘/菜单栏入口、typed事件/轮询补偿、独立quick持久草稿与轻量面板完成；Go按保存revision稳定入队，8并发/清空/重启不重复，main保持。取消保留选择，最近设备排序不增信任，离线等待与撤权门槛沿用原队列。
- 两Go模块/核心off/vet/build、完整app race192.535s、108前端tests/typecheck/lint/build通过；真实QUIC中文文件/实际接收/主草稿不变通过。浏览器960浅色/1100深色、分页/键盘/取消重开通过，真实浏览器发现的异步select目标回退已修复并补测试。
- 绑定99methods/66models/2events，任务库11/wire保持；[ADR0012](adr/0012-tray-quick-send.md)及[证据](evidence/DESKTOP-AE-QUICK-SEND.md)。Windows托盘/Mac菜单栏准确包点击、后台唤起与其他原生/物理缺项继续，Goal active。

## 2026-10-18 N3 正文预算候选

- 进程共享文件正文预算、分类CAS偏好、KiB/MiB界面与动态生效完成；突发有界、恢复/应用级重传计费，控制/自动剪贴板独立，不宣称精确UDP线速。任务库11/wire保持，旧偏好默认不限速；旧程序忽略可选限速字段。
- 假时钟、帧兼容/真实重复块计费、非法偏好无修改、两模块/core off/vet/build、完整app/transfer race195.242s/10.617s、101前端tests及typecheck/lint/build通过。1MiB/256KiB/s真实QUIC首末块间隔3.763s、hash一致；1KiB/s文件等待时剪贴板夹具7.129ms提交，暂停/恢复/取消通过。均不替代物理双机/OS剪贴板。
- 浏览器960浅色/1100深色、键盘单位换算与非法值通过；截图发现的深色focus对比问题已局部修复再验。CI、准确包与剩余N4/原生网络分别继续，见[ADR0011](adr/0011-process-file-body-budget.md)及[证据](evidence/DESKTOP-AE-FILE-BUDGET.md)。

## 2026-10-18 N2 Windows CI首流过期负例

- N2提交77b831b四项源码workflow通过，包39079813168两Mac成功、Windows旧远端取消后立即发送测试失败于opening_stream。故障夹具确定复现缓存检查后远端关闭的同类窗口，原CI准确关连接者仍未知。
- 仅首offer前OpenStream失败/已关闭连接可新建一次认证连接，固定原授权代次，不重放已发出的传输。新负例RED→GREEN与原取消/N3限速控制组合race五轮58.504s通过；最终针对性验证与修复CI另记。见[N2证据](evidence/DESKTOP-AE-DEVICE-BATCHES.md)。

## 2026-10-18 N2 原子多设备分发候选

- Go从精确持久草稿读取2–16目标、路径和N1规则，预检后单事务创建parent/全部子项；8并发幂等、第三子项真实SQLite故障全回滚。任务库10→11校验备份迁移，旧schema10真实库拒绝且DB hash不变，草稿清空后仍可核对原批次收据。
- 独立子项继续原顺序队列，源变化/撤权单项确认；持久取消覆盖重启与直接RPC，durable完成优先且发状态通知。真实同机Pion/QUIC三目标验证混合上线、拒绝后仅重试对应目标、中文/空目录/筛选/hash与无意外提交副本。
- 核心off完整test/vet/build、desktop workspace/off-race/vet/build、完整app race225.334s和最新批次race23.751s、前端100tests/typecheck/lint/build均PASS；真实绑定95methods/66models。浏览器夹具与准确包/物理分别记录，详见[ADR0010](adr/0010-atomic-device-batches.md)和[证据](evidence/DESKTOP-AE-DEVICE-BATCHES.md)。N3/N4及A–E原生/网络缺项继续，Goal active。

## 2026-10-18 C/N1 目录预设候选

- 共用Go规则/受限遍历、源码预设/有限自定义、数量体积/有界展开与恢复完整发送已实现；规则快照贯穿草稿、入队、派发、暂停/重启恢复、重试与历史重发。schema9→10备份迁移，筛选恢复记录version2，文件wire与身份边界保持。队列内容摘要绑定规范化规则与已保存草稿的精确revision，提交事务再次检查，草稿清空后幂等收据保持。见[ADR0009](adr/0009-versioned-source-selection.md)和[证据](evidence/DESKTOP-AE-SOURCE-SELECTION.md)。
- 实际Pion+固定身份QUIC接收集合/中文/空目录/hash、真实首块暂停重启后原transfer恢复、入选源变化拒绝等通过，排除子树没有落盘。核心off完整test/vet/build、完整app+transfer race（169.926/14.049s）、desktop两模式/off-race/vet/build及95前端测试/typecheck/lint/build通过。旧schema9真实库拒绝10且DB hash不变；浏览器窄窗/深色/键盘检查通过。Windows链接权限不足明确SKIP，已在JP专用TMPDIR以linksend低权限用户补验5项全部PASS/无SKIP，job及hash见证据。
- N1提交0499cb5已推送并核对远端SHA；两项core、两项desktop和三平台package workflow39076292203均success。临时浏览器/Vite/入口已收尾，原生/物理继续单列。N2仅完成[设计](adr/0010-atomic-device-batches.md)，接着实施多目标草稿和原子批次/回滚，再接界面；不以UI循环入队或阶段记录替代实际功能。
- 2b9ee1c的五项workflow与Windows/Mac三平台包全部success。恢复阶段任务库仍为9（此前ADR0008写7为笔误已纠正）；本N1才迁移到10。N2/N3/N4和原生/物理矩阵继续，Goal active。

## 2026-10-18 A 刷新调度、降级诊断与早期host路径候选

- 发现刷新共享单一在途结果，完成后750ms内合并重复请求；周期公告及记忆地址退避加入有界±12.5%抖动。设备页进入/后端重连主动请求发现，重渲染不反复发起；固定端口、网卡、组播/广播发送等降级通过Inbox/诊断DTO到页面，保留手动IP入口与明确配对要求。
- 真实Pion/shared UDP及签名候选夹具复现已选host↔host仍等待STUN约3.001s；改为按实际host nomination结束trickle，保持真实路由分类及pin，race三轮就绪8.9–12.5ms。这是loopback时序夹具，不是物理LAN性能收益。首次新增诊断测试暴露启动时具体原因被initial_refresh_failed覆盖，已保留具体原因并复验；首次桌面并发前端重建缺embed文件是检查编排失败，已在dist稳定后重跑。见[证据](evidence/DESKTOP-AE-LAN-RECOVERY.md)。
- B恢复e43043d的五项workflow与三平台包均success，远端SHA一致。准确包原生LAN/恢复、跨NAT与B2/N1–N4仍继续；宿主规则/STUN DNS保持，Goal active。

## 2026-10-18 B 显式客户端恢复候选

- 新增持久Prepare/Continue/Cancel/Status、同URL空库正常初始化/新码加入、回执丢失后核对/暂停/重启，以及真实Wails绑定和设备页入口。恢复和普通上下文替换升级trust schema3，退休旧快照并保留LAN/LocalBlock，旧queue/outbox不借新context自动发送；升级精确备份，旧库拒绝schema3。wire/控制库/任务库保持，详见[ADR0008](adr/0008-membership-recovery-and-retired-contexts.md)与[证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。
- 正常新码重配保留，撤销首次DELETE前验证live actor/原目标；成员快照完整读取/应用串行，LAN签发/补组在恢复期间跳过，独立LAN不因恢复失去授权。并发入队误拒候选已修复，race五轮通过。核心off完整test/vet/build、desktop workspace/off-race/vet/build、前端90tests/typecheck/lint/build和旧库只读兼容均通过；最终identity/app完整race终态exit0，app122.250s。
- 浏览器夹具检查960浅色/1100深色及Tab/Enter，入口已移至页首；owned浏览器/Vite/临时页面已收尾。不将该结果替代准确包原生、跨NAT或整个A–E完成。STUN补充授权仍待答、宿主规则保持；接着回到A的刷新/早期host ICE与真实双机，再继续B2/N1–N4。

## 2026-10-18 B 客户端恢复前置授权屏障修复

- 已复现group+lan跨组降为LAN不提升代次导致旧活动任务未取消，及成员刷新丢LAN request ID/换上下文丢地址历史与GrantedAt归零。补齐授权代次/时间屏障、保留独立LAN证据、代次耗尽原子拒绝；旧队列重启后仍不能重绑，新显式LAN入队可用。修复前FAIL、修复后定向/完整off核心/完整identity+app race/两模式desktop及vet/build均PASS；见[证据](evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)。wire/schema未改。
- 这批是安全前置修复，显式灾后恢复intent/API/UI、旧快照退休、旧pending revoke发送前上下文隔离仍待实现。B/Goal保持未完成；JP UDP3478授权异步待答，现有宿主规则及STUN DNS未动。

## 2026-10-18 B STUN-only 首轮部署修复

- eb421f9准确unit恢复事务exit0，本机srflx/Binding与TURN Allocate负例通过，67/96 tasks、约7MiB、独立UDP3478、无TCP；信令/备份保持active。外部Win/Mac均超时，只读确认UFW仅放行TCP22/80/443；精确UDP3478授权异步待答，未改规则/未切STUN DNS。旧默认STUN在Windows成功，Mac假IP解析及两个严格CF IPv4进程对照仍失败。完整来源/hash/job/备份见[信令重建证据](evidence/DESKTOP-AE-SIGNALING-REBUILD.md#stun-only-实际安装修复与外部限制2026-10-18-0439)，客户端工作继续。
- `18a88f2` 已推送并核对远端SHA。新unit首轮就绪失败后事务已停用，未切DNS；信令/备份保持active。日志明确 `Cannot create auth thread`，核对coturn4.6.1源码 `netengine.c` 按可见CPU/2+1创建auth worker；目标虽nproc=2但可见0–127 CPU，超过TasksMax64。专属unit改为有界96，不改宿主cgroup/CPU/安全策略；待准确资产再次事务复验。

## 2026-10-18 B STUN-only 准备

- 新增独立 coturn STUN-only unit、精确IPv4配置、首次安装备份/验证/停用回退事务，以及复用生产Pion的 `scripts/stunprobe`。目前是工具准备，实际UDP/DNS部署验收尚未完成。标准coturn服务已在安装前mask并保持inactive，Debian精确安装4.6.1-2；本批不改宿主防火墙、路由、代理。
- 固定项目工具链已实际确认 Node24.21.0 / pnpm12.4.1；pnpm12使用真实 `bin/pnpm.mjs` 包装入口并取得原生binary。它们位于忽略的 `.tools`，不修改全局PATH。
- 核心 `GOWORK=off go test -count=1 -timeout 5m ./...`、vet/build及探针普通/race通过。Debian `sh -n` / `systemd-analyze verify` 通过，job `/tmp/codex-ssh/job-linksend-ae-jp-stun-assets-check-20261018-01`；临时Windows tar权限提示不影响语法结果，正式安装unit使用0644。精确STUN DNS目前仍指旧IP且DNS-only，不在实际外部验证前切换。
- 桌面Go模块workspace/off两次test及off vet/build通过；准确固定工具链前端86 tests、typecheck、lint、build通过，嵌套pnpm也已验证为12.4.1/Node24.21.0。首次嵌套命令曾误调用系统pnpm11.25，已修正隔离入口并重跑，未改项目依赖版本或全局工具链。

## 2026-10-18 B 实际重建与恢复演练修复

- b8ad809的新备份helper已备份/原子安装/活跃库验证；完整隔离恢复v2、公共撤销跨重启、包含最新撤销记录的再次隔离恢复全部PASS。恢复服务使用非特权linksend和独立8788，结束后unit/监听停止，主库未覆盖，主服务及每日备份timer保持active。原失败保留为修复前证据。Mac公网curl本轮TLS失败，单进程IPv4/无proxy对照待原job读回，不改宿主网络；客户端恢复/STUN/跨NAT仍待继续。

- 新信令实际部署并切换 `linksend.oooai.de` 精确A到日本猫猫机；Cloudflare只对本hostname追加strict源站校验，整区Full保持。Origin正常Initialize/Join、邀请码重放准确拒绝、两路WSS，公共路径同profile与WSS、Windows公共HTTPS、主服务重启后组/incarnation/密钥保持均通过。首次reload后404触发安全回退，恢复事务补有界就绪验证后成功；切换窗口401未触发profile重置。详见[真实命令与job证据](evidence/DESKTOP-AE-SIGNALING-REBUILD.md)。
- 隔离恢复服务首轮实际失败：副本遗留root600的WAL/SHM，非特权linksend无法打开DB。新增回归取得Windows FAIL，修复仅归一完成副本journal并immutable读取完成快照，源WAL不变；Windows/Debian新回归通过，待安装并完整重验。不把SQLite校验或批次退出替代服务可恢复结果，旧失败与原库保留。
- 源码资产 `b6db1b2` 五项workflow已全部success。客户端灾后恢复、STUN、真实双NAT及其他A–E功能继续，不标B或Goal完成。

## 2026-10-18 B 部署与恢复工具准备

- 新增专属 systemd 服务/备份 timer、Nginx 精确站点、本地 CA 严格上游 TLS、独立日志 namespace 和首次安装事务。安装遇到已有目标即停止；失败停本项目并移除自己创建的站点链接，始终保留新库。工具和操作契约见[日本猫猫机部署](DEPLOY-JP.md)。此条为工具准备，尚不代表实际部署或 DNS 切换。
- 在线备份使用 SQLite backup API，校验 integrity/foreign keys/SHA256，保留14份完整自有快照；隔离 restore 拒绝覆盖。Windows 首轮测试暴露只读 fd 的 fsync 与 SQLite context manager 不关闭句柄问题，已用可写 fsync 和显式 closing 修复。Windows 与目标 Debian 两项测试均通过，后者同时 `sh -n` 通过；job `/tmp/codex-ssh/job-linksend-ae-jp-deploy-preflight-20261018-01`。CI增加真实备份/恢复测试，不用复制活跃 DB 冒充一致性备份。
- 服务器候选源码固定 `d541264`。Git内部archive构建仍会发现父Git并被标记dirty，两个尝试均不部署；仓库外导出重建的二进制SHA256为 `688702887cf7b87351d81384e941a352e13c3cd4701e437757f0922241e83d45`，源归档 `13d69b85062a2a2dcba506081bfa7648b1df7b70fca93d09a8f415610868af6b`，Go1.27.1 linux/amd64、CGO0。来源由导出提交+归档/二进制hash证明，不编造(devel)二进制内嵌的VCS字段。
- `d541264` 准确 Windows portable 与Mac arm64 DMG/二进制来源和SHA256已核验；在专用profile、不可达信令/STUN下启动，剪贴板master=false。Mac package codesign/arm64/本地网络说明字段核对通过并监听53318及TLS；Windows UI显示LAN接收在线。Mac System Events报辅助访问拒绝(-25211)，Windows Computer Use尚未形成可验证设备页导航，未声称原生配对或文件通过；保留owned进程信息供收尾。

## 2026-10-18 A–E Goal 启动与首批 LAN 恢复

- `d541264` 已提交/推送并核对远端SHA；五项源码/desktop/三平台包workflow均success，包run `39060667017`。原生产发现模块修复前十轮harness：Windows/Mac自动发现各10/10，中位数226ms/141ms（从各自ready，含启动顺序差，范围106–444/21–1061ms）；Windows→Mac固定身份TLS10/10，Mac→Windows TCP dial均EHOSTUNREACH。系统nc同端口/同源对照成功，尚在核对后台工具/原生包上下文，不能称双向文件通过。原始证据与边界已加入本轮LAN证据。

- 用户明确执行目标文档后，通过平台创建 active Goal，沿用 `codex/lan-rebuild-ae` / Draft PR #9，不恢复旧自动化或代理。唯一 TODO 新增 A/B/B2/C/D/E 及 N1–N4 当前状态，全部必交付证据满足前不标 complete。
- 核对旧 U3 九个文件 SHA256 与保存收据及原工作树一致，逐文件复用候选；复现固定端口冲突导致 LAN 全停用，以及候选旧 reader 晚到覆盖新 socket 降级状态。补齐同锁代际检查、临时监听及固定端口恢复、多源有界探测、Windows 响应 worker 关闭、配对后立即记忆；不使用 `.invalid-eol.patch`。命令与边界见[本轮 LAN 证据](evidence/DESKTOP-AE-LAN-RECOVERY.md)。
- 修复后 discovery 普通/race、app LAN 普通/race、关键关闭/旧 reader race 五轮通过；核心 `GOWORK=off go test -count=1 -timeout 5m ./...`、vet/build 和桌面 `GOWORK=off` test/vet/build 通过。未改前端，本批不重新声明前端或准确包结果。
- 严格已知 host key 连接 Mac `192.168.1.243` 与日本猫猫机 `85.149.217.165` 成功，备份后只更新这两个 SSH 条目。Mac en0 为 `/24`，存在 utun 接口，Windows 以太网 `192.168.1.122/24`；这只证明接口/路由快照，不等于双机发现或双 NAT 通过。远端 inspect 作业已完成，未部署或变更站点。
- 用户睡前另明确授权自动允许本任务 Windows“公用网络”弹窗；仅用于能核对到本任务程序/测试的提示。Computer Use 当前窗口列表无该弹窗，未改变防火墙规则。其余宿主防火墙、路由、代理授权边界保持目标文档约定。

## 2026-10-18 规划修订与 SSH 只读核验

- 新增 [A–E完整Goal提示词](prompts/DESKTOP-AE-IMPLEMENTATION-GOAL.md)，覆盖A/B/B2/C/D/E、N1–N4、信令重建/LAN独立优化与逐项测试；用户明确要求较大改动后上传GitHub，提示词写清阶段commit/push、远端SHA/CI核对和draft PR边界。本次仅撰写并同步规划阶段成果，尚未启动功能Goal。

- 用户进一步确认原信令数据丢失；计划改为新控制库重建、显式重新配对和旧组请求/任务隔离，不再承诺迁移原组或回滚到已丢失旧库。源码核对发现普通SwitchMembership依赖当前Devices/current membership，故将灾后恢复列独立开发项。LAN发现/授权/双向传输与性能前后对照升为P0，并新增无信令/STUN旅程、多接口/降级/恢复矩阵；局域网主线不等待新服务。未执行部署、真实网络测试或功能开发。

- 按用户纠正补充功能建设主线，逐项列明发现/连接诊断、剪贴板、撤销恢复和既有系统入口的实现修复范围；提出目录筛选预设、多设备分发、正文限速、托盘快速发送及对应单元/集成/原生测试，一次性临时分享单独后续批次。源码已存在的托盘、草稿、队列与收件箱不重复列作新增；旧产品规划加历史边界说明。以上仍为规划修订，未开发这些新功能。

- 按用户要求修订收尾计划：IPv6 暂缓，信令目标改为日本猫猫机 85.149.217.165；Mac 当前联调地址为 192.168.1.243。两机严格主机密钥校验 SSH 已通过；Mac 为 macOS 26.5 / arm64，新机 Debian 13 且已有 Nginx 占用 80/443。
- 新增 [修订计划](DESKTOP-RESUMPTION-PLAN.md)，补齐信令数据一致性/兼容回滚、WSS 与 STUN 独立门槛、同 LAN 与跨 NAT IPv4 分离、同版候选验收和暂缓边界。候选方案尚非已执行迁移；没有部署、DNS/宿主网络修改或产品测试，功能源码、旧 Goal/heartbeat 保持原状。

## 2026-09-23 用户终止后的主分支收尾

- 用户要求终止开发、推送主分支并整理日志。当前 Goal 已暂停，两个活动子代理已中断，相关 Go/Wails 测试构建进程为零；旧 `linksend` heartbeat 核对仍 PAUSED。香港/荷兰没有本轮未收尾作业，待授权临时宿主规则从未执行，终止后不再自动等待或启动。
- 主分支只收录七个 CI jobs 全通过的 `8fcc76d` 源码，以及本次收尾文档；文档提交使用 `[skip ci]`，不把它称为新源码/包检查。原 main `3bcb730` 是该候选祖先，按普通 fast-forward 同步，不 force-push、不部署或发布新 Release。
- 最后的 U3 增量保留在原实施树，未合入主分支。其定向与中间集成日志保留，但最后旧 reader 降级状态竞态修复未完成最终验证；完整边界见 E5-X。`D:/apps/Osend/.artifacts/astra-stop-20260923/` 保存九个源码/测试文件的逐文件 SHA256、原生 Git patch 和 `PRESERVED-WORK.json`；反向 apply 检查通过，仅检查未实际应用。最初 PowerShell 换行转换导致的无效 patch 单独保留，不作恢复依据。
- 当前唯一 TODO 保留全部未验收项。Mac、签名/原生系统共享、物理剪贴板/反向网络和部分 DPI/键盘等缺项没有被写为完成；以后只有明确恢复指令才继续。

## 2026-09-23 E5-W 事实状态与网络恢复补齐

- `8fcc76d` 已推送原分支 / Draft PR #8，准确 SHA 的五个 workflows / 七个 jobs 全 success；三平台 artifact 来源与日志摘要核对通过，没有下载新载荷，见 DELIVERY。

- 已修复设备页/发送选择器的发现即“可达”、未知即“离线”，以及 Go 将握手前或暂停任务的 SessionID 当作 connected 的误报。现在只认存活且授权代次匹配的认证 QUIC，旧端和无活动任务的空闲复用均覆盖；撤销、旧授权与关闭证据不会变回已连接。
- 实际 LAN 同意后无需服务器在线记录即可入队；服务不可用但连接存活时也可尝试新任务，连接关闭后恢复等待门槛。发现未配对仍被拒绝，授权与后续握手未绕过。两个入队错误均有 RED→GREEN。
- Windows 原生事件从仅接口补齐至接口/地址/路由三类，仍使用有界事件泵；真实初始通知/注销、失败逆序清理、并发停止和专项 race 十轮通过，没有修改宿主网络。
- 根 workspace/off 完整 tests（app 135.906s / 121.008s）、off vet/build 和完整 app race（166.576s），desktop 双模式/完整 race/vet、前端 86 tests/typecheck/lint/build、Windows Wails production build 全 PASS；GitHub 状态的最终回执见 E5-W/DELIVERY。协议及持久 schema 未变，准确包和实机结果不继承。
- U3 仍有可本机推进的固定 UDP 初始降级、即时记忆地址与重叠网段多路由缺口，已落唯一 TODO；Mac、GUI 策略、宿主防火墙授权和物理矩阵继续分列。

## 2026-09-22 E5-V 桌面待同步撤销闭环

- `6cb0705` 已提交推送原分支 / Draft PR #8；core/desktop push/PR 和三平台包七个 jobs 全 success，最新包来源与 workflow 摘要已核对。包级完整下载仍固定在 `36302a9`，不覆盖后续 UI；最终纯文档收尾复用该成功源码 CI，使用 `[skip ci]` 避免重复全平台构建。
- 核对实际 UI 后补齐 pending 的显示与显式操作：已删除行只在 `pending_revoke_sync` 时显示“继续撤销”，确认后调用既有 RemoveDevice / Revoke；忙碌、不可用、取消、失败、同 ID 新成员状态均按真实边界处理，不使用 Unblock 来恢复删除。
- 源码回归先 4 FAIL，再 GREEN；含成员/焦点边界共 82 tests 全 PASS，typecheck/lint/build 通过。Playwright source fixture 实测 960×640 / 1100×720 长中文名、无横向溢出、确认与取消可见；修复 body 焦点后，取消默认焦点、Enter 取消回焦与 Shift+Tab/Enter 明确确认通过。模拟 backend 只验证页面行为，不声称准确包或真实网络调用通过。
- desktop workspace test、off 完整 race/vet、Windows Wails production build 全 PASS；frontend dist 已按项目锁定工具链更新。根 Go 源码未变，复用 `36302a9` 的完整 root workspace/off/race 与七个成功 CI。source fixture 浏览器和 Vite server 已按所有权关闭。
- `36302a9` 的三平台四包已完整下载，原验包器及来源/平台补充核对全部 PASS，详见 DELIVERY；该候选不含本节后续 UI 增量。Mac 实机、Windows GUI 策略限制和待授权的临时宿主规则仍分别保留，不标 U1–U7 全完成。

## 2026-09-22 E5-T 撤销组版本冲突与原 actor 绑定

- `8c6e1e8` 已同步原分支 / Draft PR #8，core push/PR、desktop push/PR 与三平台 packages 七个 jobs 全 success；四个包的 CI 输出摘要、artifact metadata 和来源 SHA 已核对，尚未下载该候选完整验包，见最新 DELIVERY。
- 新同版 HK→NL 单轮在 WSS 阶段 HTTP 525 退出，新诊断正确记录 `SIGNALING_UNREACHABLE / signaling_connect`、0 bytes、未建立 session/TLS；未进入 QUIC。13:34:15Z 源站有来自官方 Cloudflare 网段的 TCP/TLS read timeout，落在接收进程窗口内，但无请求关联 ID，不能断言具体请求或链路根因。生产进程/配置/TLS 未改，现有诊断不支持调整 TLS 版本。
- 收尾连续删除暴露 U2 缺陷：HK 删除使组 revision 20→21，NL 仍用 20 被服务端严格 CAS 拒绝，pending 无法恢复。新客户端在特定 403 后仅核对一次完整认证快照并重试一次；自动同步固定原 actor group/incarnation、原 target incarnation、原 request ID。actor 被撤销后重入不得复用旧删除意图；schema 2 新增可选 actor 绑定，legacy 缺绑定不自动猜填。只有新的显式删除经核验后通过 CAS 建立新请求，旧返回不得覆盖新意图。没有放宽服务端 CAS、改变 wire 或改写文件历史。
- 实际集成全 PASS：根 workspace/off `go test ./... -count=1 -timeout 240s`（app 103.607s / 111.246s）、off vet/build；off `go test -race ./internal/app ./internal/identity -count=1 -timeout 300s`（159.567s / 7.040s）；desktop workspace test、off 完整 race/vet；锁定工具链 Windows `wails3 task build ARCH=amd64`。定向真实 HTTP 的连续/离线版本冲突、actor 重入、legacy 重启、新意图与迟到回写、15 项负例均通过；独立授权审查通过。日志 `.artifacts/astra-resume/checks-revoke/` 和 `revoke-diagnosis/SUMMARY.md`，前端源/依赖未变，复用已通过的 76 tests/lint，Wails 重跑 typecheck/build。
- `36302a9` 已提交推送。准确 archive 构建的 Windows CLI 于 14:08Z 以明确删除操作成功处理 NL legacy pending：新 request ID、原目标 incarnation、当前 actor 绑定，pending=false / revision 22。HK 独立确认两精确目标均 revoked、原 23 条设备和 owner 不变；原实验资源已清理，没有手工编辑 trust/DB 或恢复旧备份。
- 独立临时 HTTPS 信令实验 E5-U 使用 HK 已知空闲 80 端口、独立 DB/CA/IP SAN 与进程内信任：HK 本地验证通过，NL 唯一请求 curl 28 / HTTP 000，未进入 bootstrap/join 或文件传输。临时服务正常停止，私钥/DB/配置/fixture/profile/bin 已清理，80 监听消失；一次 HK cleanup 的 SSH 255 在确认原 job missing 后恢复，完整失败/收尾回执保留。生产和宿主地址/路由/rule 均保持。此结果不证明原 QUIC 根因，Mac、原生系统与物理网络缺项继续分列。
- 后续只读策略核对确认 HK 的 NEW IPv4 TCP80 会落现有 INPUT 默认 DROP；NL 主机 INPUT/OUTPUT 为 ACCEPT，历史 ICE 五元组仍缺失。E5-U 已准备仅 NL 单一源 IP→HK TCP80、最多 600 秒且按唯一 handle 自动撤销的 runtime 提案。该宿主规则变更明确在原提示词授权外，已单独询问用户，未执行。源码候选 `36302a9` 七个 CI jobs 均 success，完整四包下载校验另记 DELIVERY。

## 2026-09-22 E5-S 跨主机失败取证与诊断修补

- `096d79a` 的 core push/PR、desktop push/PR 与三平台 packages 共七个 jobs success；Windows/Mac ARM/Mac Intel 均执行原生交接自测。四个包已完整下载，经既有校验器核对 hash、内外 BUILD-INFO、COMMITTED/clean 来源、平台/架构与 Windows 版本，见 E5-DELIVERY。该候选不包含下面的新诊断补丁。
- HK 旧 `387b57c` → NL 新 `096d79a` 首轮在信令 HTTP 525 退出；只读健康/TLS 复核后一次恢复得到 sender `QUIC_HANDSHAKE_FAILED` / receiver `QUIC_HANDSHAKE_TIMEOUT`、0 bytes。仅将 HK 切到同新版的区分实验仍失败，反向 NOT RUN；不能据此认定版本不兼容、UDP 阻断或故障已修复。
- 无有效协议 JSON 的 HTTP 5xx 现在映射到 `SIGNALING_UNREACHABLE`，504 映射到 `SIGNALING_TIMEOUT`，保留 HTTP status、合法协议错误优先与未知正文脱敏。CLI `--evidence` 失败输出新增可选阶段和有界类型化诊断，并保留已观测的 session、ICE 路径和计数；未建立 TLS 时 version=0 / ALPN 为空。并行准备、正文拒收、开流、授权与关闭各按真实错误阶段归类，不新增重试或改变身份/TLS 验证。
- 实际检查全 PASS：根 workspace 与 `GOWORK=off go test ./... -count=1 -timeout 240s`（app 102.040s / 103.856s），根独立 vet/build；`GOWORK=off go test -race ./internal/app ./internal/signaling ./cmd/linksend -count=1 -timeout 300s`（app 149.279s）；desktop workspace test、独立完整 race/vet；锁定工具链 Windows `wails3 task build ARCH=amd64`；diff check。首轮旧诊断用例预期 `INVALID_MESSAGE` 失败，按新契约改为 `SIGNALING_UNREACHABLE` 后完整重跑通过，原失败日志保留于 `.artifacts/astra-resume/checks-diagnostics/`。前端源码/依赖未变，复用 E5-R 的 76 tests/lint；本轮 Wails 再次 typecheck/build。
- 两个测试身份已按精确 incarnation 正常撤销，原 21 条成员记录与 owner 不变；远端测试 profile/fixture/receive/bin/invite/进程/UDP 已清理，仅 root-only 日志和脱敏证据保留。HK 仍运行 `387b57c`、PID 911287、schema 4，health/integrity 正常；在线备份未覆盖 live，宿主网络未变。两端作业、摘要与原始失败见 E5-S 和 `.artifacts/astra-resume/mixed-096d79a/RESULT.json`。
- 本轮诊断源码已完成审查与测试，下一次跨主机实验将绑定新的已提交来源；Mac、准确 Windows GUI 策略限制、签名和系统共享等缺项继续分列，不标整轮完成。

## 2026-09-22 E5-R Astra 续接与三个缺陷修复

- 安全接管原 `208a/Osend` 实现树，起点 `6eb23ba` 与 origin / Draft PR #8 一致，旧任务 idle、旧 heartbeat PAUSED；同步指定新提示词，唯一 TODO 和过期续接索引已更新。Mac 本轮暂不可用；香港/荷兰只读确认健康服务及旧实验收尾，无部署或宿主网络变更。
- `1849c31`：修复 `group+lan` 离线目录误标及撤销未入持久 outbox。真实 HTTP listener 关闭/原端点恢复、profile 两次重启、旧快照不复活、旧 outbox 不误撤新 incarnation、保留文件历史和 LAN-only 不删除其他组均通过，保留 RED/GREEN 与 race。
- `7d3bfb8`：修复两端独立复制历史不同时，双方 ready 但新剪贴板事件被判为旧事件。lease 携带签发时钟，成功安装后观察因果顺序；新增时钟跳跃上限、运算余量、租约重放、固定事件/旧复制/本地复制优先及高时钟多轮续租回归。旧 wire 可解析，完整修复需双方更新；没有证明旧 E5-B 物理失败已解决。
- `535a77c`：两平台原生共享满 64 条 journal 时先处理既有请求幂等/冲突，再限制新请求；Windows 临时目录自测 RED→GREEN，Mac 自测已加入两种架构 packages CI，实际结果待本批远端核实。签名与系统激活没有冒称通过。
- `9f15fba`：根完整测试首次在远端取消流的 consent 等待失败；原用例定向三次未复现。发现它绕过 `StartSend` 的 inbox/WSS 所有权交接后，仅修夹具为真实 task API，保留首块门闩/远端取消/同 QUIC session，新增 WSS 不增长与失败诊断，定向 race 三次通过。没有增大超时或修改产品连接路径。
- `9953601`：通过项目已安装的 Node 24.21.0 / pnpm 12.4.1 重新生成跟踪的 frontend dist，与已有 U7 源码一致；旧嵌入资源停在 `1fa21b4`。没有新增界面功能。
- 实际集成检查全 PASS：根 `go test ./... -count=1 -timeout 240s`、`GOWORK=off go test ./... -count=1 -timeout 240s`（app 分别 101.012s / 95.877s），根独立 mod verify/vet/build；desktop workspace test、独立 mod verify/vet/build 和最终 `go test -race ./... -count=1 -timeout 180s`；受影响 core race；前端 typecheck/lint/76 tests/build；Wails 3 beta.18 `GOWORK=off wails3 task build ARCH=amd64`；.NET share self-test；gofmt/diff check。日志为 `.artifacts/astra-resume/checks/` 及分项证据，普通本机测试未开启剪贴板正文读写。最初 PATH 的 Node22/pnpm11 检查虽通过，已改用项目锁定工具链完整复核。
- preview.2 准确 Windows 包哈希再次匹配，但隔离离线启动被自动审批在 CreateProcess 前以 `blocked by policy` 拒绝，GUI 未启动；fixture 正常退出。Windows 包级 U2、Mac 物理双向/剪贴板、签名/系统共享、真实 DPI/键盘/网络恢复仍有缺项。旧 FAIL/NOT RUN 与旧包来源保留，本轮不标整轮完成。

## 2026-09-20 E5-H 4d36132 候选来源、重配对与设置继承

- 4d `wails3-packages` run `35459981462` 的 Windows artifact 和 Mac arm64 DMG 均已实际下载/核验：Windows outer ZIP `d0344809...c08f37cb`、portable EXE `c22c3d2b...abbc4edd`；Mac DMG `8145ede7...0b4b80c9`，实际 Mac bundle/架构/codesign strict 通过。它们明确属于 4d，不称为后续测试提交的产物。
- 复用已撤销 f090 identity 在 4d Windows 准确包重新配对，HK 只读确认新 incarnation/revision；设备设置准确 UI 用 ExpandCollapse 展开，目录继承全局、`keep_both` 生效，六项剪贴板 checkbox 默认 Off，持久库没有设备覆盖或 grant。另以 `SelectionItemPattern` 将 E5-A Mac peer 的策略覆盖为 `error`（revision 6）后保存，再恢复为继承（revision 7）；全局值不变。随后同一准确 UI 精确删除，HK 复核 `revoked=1`，profile 在确认后清理。
- Mac AX 只读探针为 `AX_TRUSTED=false` / `AX_WINDOWS_ERROR=-25211`；未申请权限、未 AXPress/改值，未用新框架或读写剪贴板。此项保留为用户手动开启 Accessibility 后的 Mac 状态页验证条件。
- 目录选择器的受管复核已以 E5A Mac 0c59 同行的 `ghost` 设置按钮进入正确编辑器；“选择接收目录”由同一 4d package PID 拥有，树稳定后公开 Edit `1152` 的 `ValuePattern`、确认 Button `1` 的 `InvokePattern` 和取消 Button `2`。同一入口完成受管目录的 `ValuePattern` 写入和确认；返回态采样后以既有 `ScrollItemPattern` 显示初始 offscreen 但启用的“保存设备偏好”，Invoke 保存使 SQLite 写入目录、revision 8，UI read-only Edit 一致。再 Invoke“继承全局目录”并保存，SQLite 恢复目录/策略空、revision 9。早期未加 owner/就绪约束的不可达观察不再作为结论。
- 18 分钟剪贴板复测仍为 `NOT_RUN`：watchdog 已绑定 4d Windows EXE SHA256、4d Mac DMG/App 与 E5-A 两端 profile/identity；PowerShell 解析、Mac `sh -n` 和候选根/哈希锚点通过。用户已授权临时覆盖，但聚合启动在执行层 `CreateProcess` 前被 `blocked by policy` 拒绝，未启动 watchdog、package、master/grant 或读写剪贴板；审查文本保留在 supervision e5h-4d36132。
- `e065fe2` packages Windows core fixture暴露 `CLIPBOARD_CONTENT_LIMIT`：满窗 fixture 未和续租串行，且后续在满窗上等待 before-write hook。`5038003` 仅重构测试 fixture 为受锁满窗策略刷新与空槽旧快照门闩，未改产品实现。定向测试和受影响 race 本地 PASS；503 的 core 35464629366/35464626930、desktop 35464629379/35464627054、packages 35464626987 均已 success。

## 2026-09-20 E5-G 准确 package U1/U2/U5 合并验收

- 固定 `0c59ae2` Windows package（SHA256 `97b3a63a...16e0e3e`）在保留的唯一隔离 profile 完成后台 UIA 通用设置保存（本机名称、revision=1）和一次性码配对；新 DeviceID/加入时间/incarnation 记录在 supervision 回执，未保存邀请码正文、私钥或用户剪贴板内容。
- Mac 准确 package 通过受控 `macos_share` journal 向该 ID 发送 1 MiB 文件。Windows 出现独立“接收确认”弹窗，文件名可见且详情保持折叠；只 Invoke 一次“接收文件”。默认 `keep_both` 保留 54-byte 同名 fixture，并写出带 `(1)` 的 1 MiB 文件，双端 SHA256 一致。Windows receive 与 Mac send task 均 completed，verified/committed 1 MiB、0 retransmit、IPv4 `lan_direct`/QUIC/TLS1.3/`relay=false`。
- 邀请方的准确 Windows package 设备页以唯一显示名定位设备，按实际 UIA 模式展开“设置”后，Invoke“删除设备”与“确认删除”。HK `hk-main` 只读查询确认同一 ID/incarnation `revoked=1` 和 membership request；邀请方刷新呈现 `removed/revoked/blocked`，测试 profile 重启后 identity 未变且未重新加入。两端 package 已停止，profile 因服务端清理已通过而暂留审计。
- 早先 U2 配对的首次撤销仅保留 exit 1，stderr 和临时 identity/join/owner 输出已由旧脚本删除，无法恢复真实稳定错误码；后续按 invite 使用时间、issuer、ID、名称和 incarnation 关联正确对象，以正常 Service.Revoke 和 HK read-only `revoked=1` 完成精确清理，另一候选未删除。不要把脚本的本地 denied 项误判为服务成员，或推测为 UNPAIRED。
- `4d36132` CI 已全部 success：core 35459984881/35459981488、desktop 35459984870/35459981479、wails3-packages 35459981462。Mac tail 包装曾报 zsh `status` 变量错误；即时受管回执和独立任务库只读核验均 PASS，二者分列。

## 2026-09-20 E5-D U3 IPv6/mDNS 有界原型

- 干净 77ce6bf Linux CLI/rendezvous 在荷兰隔离同链路 ULA namespace 完成 IPv6 SAN HTTPS/WSS、真实 Pion ICE host pair、QUIC TLS 1.3 和双向 1 MiB 文件；两端 SHA256 一致、relay=false。这不是公网 IPv6、LAN discovery 或准确桌面 package 验收。
- 提交 4edb742 增加 test-only IPv6 signed discovery/LAN TLS prototype 与可复现 namespace 脚本。真实 job 证明同一 DeviceID 的 fd42:5d:1::2/::4 两条已签名 route 合并，并由 B→A 一次 TLS 1.3 pin/heartbeat 控制连接验证；生产 Manager 未改变，默认 IPv6 discovery 未启用。
- Pion mDNS v2.2.0 的 IPv6 DNS-SD publish/browse 在相同范围通过；DNS-SD 只提供未信任候选 hint，当前无 production provider。未来集成须复用现有签名 announcement 与 TLS pin 验证，不能信任 TXT。
- 77ce6bf 的 core push/PR、desktop push/PR 与 packages 五个 CI run 均 success。实际命令、二进制摘要、job 与限制见 docs/evidence/DESKTOP-E5-ACCEPTANCE.md；可复现入口见 scripts/e5d-ipv6-discovery-netns.sh、scripts/e5d-mdns-netns.sh。

## 2026-09-14 E4-01 profile SQLite owner 测量

- taskManager 持有 profile 生命周期唯一 task-history.sqlite 连接并共享给 desktopStore，构造失败和 Shutdown 显式关闭；MaxOpenConns=1，失败仍撤销 durability claim。
- Windows 微基准 100x、3轮：persistent 2.024/2.076/2.084 ms/op；reopen_each_write 3.663/4.254/3.699 ms/op，中位数减少约43.9%。根模块 test/vet 与历史/构造失败定向测试 PASS。
- 仅存储 owner 子项完成；认证 QUIC 会话复用、流取消和公平调度仍为 NOT RUN，自动剪贴板未实现。见 docs/evidence/DESKTOP-E4-STORAGE-SESSIONS.md。
## 2026-09-14 E3-C3 共享接管竞态与刷新

- 放弃请求与 Enqueue 共用串行边界，队列已有 request_id 时拒绝并要求在队列取消；资源清理失败继续保留 journal。
- Windows 按实际读取字节流式复制临时源，源增长受 16 GiB 总预算约束，WriteThrough + Flush(true)，失败清理半成品。
- 两端打开共享面板先静默唤醒 owner；Windows 等待新鲜快照超时后显示状态未知和刷新入口。Mac 定向编译与测试作业 /tmp/codex-ssh/linksend-e3-c3-mac-swift-20260913T172620Z PASS。
## 2026-09-14 E3-C2 原生共享闭环

- 修正 Windows package root/manifest/wake 路径；在线目标按钮直接提交，离线等待显式确认；失败可重试。临时来源采用 16 GiB 实际累计限额、长度校验、Flush(true) 与失败清理。
- Mac 宿主冷启动使用 NSWorkspace 显式 --native-share-background，设备时间支持 RFC3339Nano，provider 并发限制 4 且按实际 chunk 计总预算。
- owned 清理失败时保留 journal；未入队失败请求在主界面提供明确放弃入口。Mac 定向作业 /tmp/codex-ssh/linksend-e3-c2-mac-swift-20260913T171611Z PASS。
# 2026-09-14 E3-C1 原生共享审查修复

- 原生共享来源 journal 与 queue request 生命周期绑定：入队后保留 bookmark/owned source，重启幂等恢复；completed/cancelled/expired 后按请求释放 security scope、临时文件和 journal。失败请求隔离，不阻塞后续记录。
- 后台共享唤起不显示主窗口；macOS LaunchServices URL 进入 Wails ApplicationOpenedWithFile consumer。设备快照由后台每 15 秒发布，两端读端按 60 秒 TTL 降级陈旧可达状态。
- 两端选择设备不自动提交；离线等待必须单独确认，并提供添加设备入口。Windows 临时/虚拟 StorageFile 必要时持久复制；macOS 依据真实 inPlace 回调决定 bookmark 或同步复制。
- 本机 desktop Go tests、.NET build/self-test 通过；Mac Swift 定向作业 /tmp/codex-ssh/linksend-e3-c1-mac-swift-r2-20260913T165721Z 通过。签名安装与系统激活仍按用户要求 NOT RUN。
# LinkSend implementation progress

## 2026-09-20 E5 剪贴板 lease window 修复与后台 UIA

- packages run `35456901989` 的 Windows amd64 在 `562bf6b` 上失败，唯一产品断言为 `TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists: image lease did not arrive`；该失败保留，不写为已消失。根因是 10 秒 TTL / 7 秒续租时，同 peer 的两张未过期 text lease 已占满 `MaxLeases=2`，receive:image 更新不能签发完整 text+image lease。
- `da53ed6` 使 receive policy 更新清除本端已签发窗口，并由每 session 容量一唤醒复用既有续租 worker；对端仅在完整 grant/revision/generation 真正变化时替换已安装窗口。lease 写入保持 2 秒 deadline，取消时 `CancelWrite` 并释放权限锁；wire、TTL、MaxLeases 和 legacy `Install` 上限未改变。受控双端满窗、旧 text 快照门闩、新 image lease/大图路径和取消释放均通过。
- 准确 `0c59` Windows package（SHA256 `97b3a63a...16e0e3e`）的只读后台 UIA 回执为 `.artifacts/e5-windows-uia-reachability-20260920T011406Z/result.json`：设置和设备导航的 `InvokePattern` 可用，配对码 `Edit` 导出 `ValuePattern`/`TextPattern`；不设前台、不发键盘、不用 CDP/新框架或剪贴板，临时 profile 已删除。传输导航 Invoke 返回成功，但没有导出名为“接收”的 `SelectionItem`，没有开始接收或提交配对。
- 同一准确包的 U5 实际保存回执为 `.artifacts/e5-windows-uia-settings-save-20260920T020154Z/result.json`：后台 UIA 将隔离 profile 的“本机名称”设为临时 marker，Invoke“保存通用设置”后直接读取该 profile 的 `desktop-preferences.json`，名称一致且 revision=1。脚本未设前台、未发键盘、未用 CDP/新框架或剪贴板；进程退出、隔离 profile 删除。该项只覆盖通用分类的一次本地持久化，文件接收、设备覆盖和物理双机设置矩阵仍分列待验收。
- 实际通过：`GOWORK=off go test ./...`、`go vet ./...`、`GOWORK=off go test/vet/build ./...`（`apps/desktop`）和 `GOWORK=off go test -race ./internal/app ./internal/clipboardsync -run 'TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists|TestCancelledClipboardLeaseSendReleasesGrantLock|TestReceivePolicyChangeReplacesBoundedIssuedLeaseWindow|TestReceiveGrantChangeReplacesIssuedLeaseWindow' -count=1`。后续 CI 需重新运行；本地结果不替代准确包或物理剪贴板验收。

## 2026-09-14 E3 原生共享源码候选（待审查）

- Windows 正式 Share Target 使用 `ShareOperation/StorageItems` 显示真实已配对设备小面板，只持久交接 broker 可打开的绝对路径，不复制正文；无绝对路径的临时/云端项目明确失败。Go 发布最小设备快照并消费 schema 2 journal，以 request ID 幂等后进入现有授权队列。
- macOS 正式 Share Extension 使用 `com.linksend.desktop.share`/`group.com.linksend.desktop`；原位文件交接只读 security-scoped bookmark，系统临时表示才在回调内接管。旧 E0 4097 已定位为无 Team ID 导致 App Group 被拒绝且失败分支未 cancel；失败分支现全部终结 request。
- Windows .NET Release build/self-test、desktop test/vet/build PASS。真实 Mac arm64 完成 plist、provider/store 4 项测试、appex 源码构建及 Go/Objective-C desktop test/vet/build；extension SHA256 `fb9d17affcdc43f189c75c3d3dde1a04db9ff6c9a8973ffa6e379970e1f6182b`。用户没有两平台证书并明确暂缓签名，系统安装、App Group 生效、系统激活、升级卸载准确保持 NOT RUN，不开发假身份绕过。

## 2026-09-13 E2-C1 集中审查修复候选（待复审）

- 主发送动作移到有界文件列表之前；完整 App 外壳 960×640、已选 LAN-only 目标和两个文件时按钮位于视口内、启用且点击回执成功。
- 同名策略支持全局持久化与设备覆盖继承，任务库迁至 schema 7；普通确认和免确认均使用有效策略，活动/恢复计划不变。设置改为字段级 dirty 合并，保存期间锁定当前分类；CAS 冲突会读取后端最新 revision 并保留 dirty 输入，普通校验/I/O 错误不进入合并流程。
- LAN 请求确认提升到应用级；请求结果区分 joined/switch_required/pending/not_joined，切组提供配对码后续；LAN-only 不再误报离线，删除记录可显式允许重新添加。
- 存储页恢复旧版内容草稿的查看/取消入口，并可直达待发送管理；不恢复新建内容入口、不自动续发、不删除接收文件。根 `go test ./...`、`go vet ./...`、desktop `GOWORK=off` test/vet/build、前端 typecheck/lint/64 tests/build和Wails production通过；新候选CI待提交后运行。

## 2026-09-13 E2桌面接收、设置与四视图候选（待复审）

- U1：新增绑定 attempt/revision 的 Go 一次接收命令，默认 `keep_both`，计划/空间/授权/持久化完成后才同意；恢复仍沿用原计划，免确认保存失败不反转本次接收。
- U5/U7：设置改为六个用途分类，偏好增加 revision 和分类局部 CAS；设备专属目录留空继承全局。主导航收敛为传输、记录、设备、设置，长列表使用单主滚动区与局部有界滚动。
- U4：传输页移除文字、链接、图片手动创作入口；旧队列升级/重启后保持 `needs_attention`，不会自动续发。自动剪贴板仍属于 E4，本轮未开始。
- 针对性 Go、前端 typecheck/lint/58 tests/build 已通过；Playwright production preview 覆盖 1100×720、960×640、CSS小可用区、深色、reduced motion 和键盘焦点；原生125%/150% DPI未运行，0 console error。准确 Wails 包与双机 UI 未运行，网络事实继续引用 E1，不把浏览器预览当网络结果。见 [默认接收](evidence/DESKTOP-E2-DEFAULT-RECEIVE.md)、[设置](evidence/DESKTOP-E2-SETTINGS.md)、[旧内容](evidence/DESKTOP-E2-LEGACY-CONTENT.md)、[布局](evidence/DESKTOP-E2-LAYOUT.md)。

## 2026-09-13 E1物理LAN最小闭环（PARTIAL）

- 固定源码 `e896f92`、Windows `10.234.232.205` 与Mac `10.234.212.116` 的全新隔离profile完成双向发现、独立LAN同确同意、LAN grant及Windows→Mac 1,179,648 bytes传输；两端SHA256同为 `8b1dc5b2...17a336d1`。
- Mac→Windows在 `lan_control_connect` 后失败并回退不可用信令，0正文；独立Mac→Windows TCP/54446探针3,002ms timeout且Windows 15秒无accept。未改防火墙，因此整体网络PARTIAL，不冒充双向/准确包PASS。完整证据见 [E1物理LAN](evidence/DESKTOP-E1-PHYSICAL-LAN.md)。

## 2026-09-13 E1-C2残余授权/LAN恢复/发现竞态修复（后端验收通过）

- 完整成员快照移除peer后取消其活动task；queue保存的expected authorization generation贯穿StartSend、PeerSession和开流检查，LAN-only可信nearby设备可独立尝试，不再伪造online。持久authorization epoch阻止解除屏蔽后generation回退。
- LAN发起端在正常及query恢复路径都只在验证done后启用pin；恢复helper共用credential消费，能继续报告joined/switch_required/pending。断线保留最多60秒、上限64的provisional/completed事务，query可在重启后恢复ready/done和target凭证，撤销/重配清除旧事务。发现多地址签名稳定排序，UDP重建与Close串行收尾，取消后不能遗留新socket。
- 受影响三个包完整测试PASS；定向race覆盖活动快照撤销、旧queue generation、LAN同意及UDP rebuild/Close。准确包物理LAN、跨NAT和系统确认仍NOT RUN，不部署当前授权变更。
- 物理Windows→Mac首次独立LAN同意在双方时钟相差约1秒时暴露provisional本地期限误按“本机now+60秒”复核，合法签名帧被接收端提前关闭为EOF。签名验证原本已约束 `ExpiresAt-IssuedAt<=60秒` 且允许15秒时钟偏差；持久层上限现与之对齐为75秒，并新增74秒接受/76秒拒绝边界回归。该失败未记为网络PASS，修复后重新从全新隔离profile执行。

## 2026-09-13 E1-C3原生网络与睡眠事件源码闭环（已验收，物理待测）

- Windows使用系统 `NotifyIpInterfaceChange`/`CancelMibChangeNotify2` 监听所有地址族接口变化；macOS使用 `SCDynamicStore` 监听全局和接口IPv4/IPv6状态。Wails 3的系统睡眠/唤醒事件与网络回调进入容量1串行泵，再调用既有core门闩；5秒接口快照fallback保留。
- 关闭先注销Wails和系统订阅，停止并join macOS CFRunLoop/Windows通知，再等待事件泵退出和关闭core。通知只刷新未来发现/endpoint选择，不取消健康活动QUIC。
- Windows桌面专项test/vet/build、前端typecheck/lint/58项测试/build、Wails 3 beta.18 production build PASS。Mac `10.234.212.116`（macOS26.5 arm64）对固定源码 `844e286` 的真实SCDynamicStore注册/停止测试、desktop vet/build PASS，作业 `/tmp/codex-ssh/linksend-e1-c3-mac-build-final-20260913T114047Z`；链接器deployment-target warning保留。提交 `248e1ee` 的core、desktop、Windows/macOS双架构包CI全部PASS，总控接受源码和两平台注册/停止构建闭环。准确包Windows/macOS事件延迟、网络切换、睡眠唤醒与文件hash仍NOT RUN；物理结果不得由源码检查替代。

## 2026-09-13 E1第二候选与C1审查修复（待复审）

- 修复C1六组问题：完整成员快照撤销第三方消失成员且拒绝revision回滚；组/LAN双关系不互相覆盖，旧LAN generation不能附着新关系；schema1本机block在授权读取前迁移；任务/恢复/queue/迟到确认绑定真实授权generation，任务库升级schema6。
- LAN同意改为持久provisional的commit/ready/confirm/done并支持query恢复，pending上限64；服务端凭双方签名transcript签发target-bound 60秒凭证。新增空组签名初始化和显式跨组原子切换，控制库升级schema4。
- 发现按同index地址变化退组重入、稳定刷新不增generation、失效UDP socket有界重建、持久记忆地址持续退避探测；LAN runtime每5秒以真实接口地址快照触发恢复，目录连接状态来自活动session。原生OS事件低延迟回调、IPv6/mDNS及物理矩阵仍准确保留为未完成。
- 第二候选实际通过根 `go test ./...`、`go vet ./...`、`GOWORK=off go test ./...`、`git diff --check`；桌面独立模块 `GOWORK=off go test/go vet/go build ./...`；前端 typecheck、lint、58项测试和production build。当前隔离工作树未找到 `wails3.exe`，本候选本地Wails production build为NOT RUN；由阶段提交的GitHub CI继续验证跨平台构建。Node实际为24.19.0，低于项目声明的`^24.21.0`并产生engine warning，未造成上述命令失败。

## 2026-09-13 E1设备与发现后端第一候选（待总控审查）

- E1-01/02：成员控制面升级为 `membership_version=2` 与数据库schema 3，加入随机incarnation、组revision、邀请码代际绑定和普通成员幂等撤销事务；本机trust schema 2先持久拒绝/outbox并推进grant generation，旧快照、跨组、迟到actor、旧客户端与删后旧码负例通过。
- E1-03：新增最近签名公告固定公钥后的TLS 1.3 `lan_pair` request/accept|reject/commit/ack；接收端明确同意，双方成功才写独立LAN grant，拒绝零pin。服务短期入组凭证和ack丢失查询仍待下一候选。
- E1-04/05：同接口保留全部IPv4和多route目录；组播/广播/单播/TLS独立降级并限制资源。Windows使用真实source-bound UDP socket接收回包；网络/唤醒入口刷新未来路径但不取消健康任务。IPv6/mDNS与准确包物理网络矩阵未运行。
- 实际通过：根 `go test ./...`、`go vet ./...`、`GOWORK=off go test ./...`；桌面模块 `GOWORK=off go test/go vet/go build ./...`；`git diff --check`。E0 draft PR #8 在 `1518e31` 的core、desktop、Windows和双Mac包CI全部SUCCESS。

## 2026-09-13 E0候选同步与E1后端实施授权

- `ec55db3`、`11c44df`、`2342009` 的既有证据已复用；总控接受 `2342009` 作为Mac共享原型的有界IO修补，不把它视为U6、4097 helper通信或文件交接通过，也不重复十项SafeIO测试。
- 最新用户提示词明确恢复实施并授权阶段工作分支推送、复用draft PR和CI；旧TODO中的“未审不push”“等待派发E1”门槛已经失效。当前唯一产品写入方一次交接至 `/root/desktop_executor`，本批继续 E1-01 至 E1-05 的设备配对/撤销、LAN同意、发现、统一目录与恢复后端闭环。
- 仍未证明且不会冒充通过：Windows安装信任、Mac共享4097/文件交接、准确候选包Windows↔Mac真实双向、双 NAT。网络原型缺口补齐前不扩张UI。

## 2026-09-13 E0 Mac资源安全修补收尾（待审提交）

按E0-supervision-ui-v1/E0-safeio-close-v1，Host的payload/receipt改为读入时有界校验；扩展失败只清理独占创建且身份仍匹配的本请求文件。真实Mac10项文件IO边界测试、host/.appex构建/签名通过。标准系统Share菜单展示及action已实际派发；分离服务复测仍NSCocoaErrorDomain/4097、最终退出1、无captured回执，保持E0-04待修技术缺陷。精确注册已注销，owned进程计数0；见 [安全修补回执](evidence/DESKTOP-E0-MAC-SAFEIO.md)。
总控已复审接受11c44df的ADR语义，具体API/schema/实现及兼容测试仍待E1/E4。未宣布E0整体通过，未push；仓库外executor-progress.json持续记录当前状态。下一步等待总控复审后派发阶段同步和E0-B，不再在此批扩大共享加载器重构。

## 2026-09-13 E0-ADR-review-v1 修订（待复审）

已读取总控仓库外pending-e0-adr-review.md并修订ADR0007：接收端预签发lease/不可延长单调deadline阻止首帧或重传滞留后重获TTL；删除推进peer级generation使全部旧组/LAN grant失效；origin_sequence、Lamport与串行落板revision门闩分离。新增对应后续验收负例，见 [审查回复](evidence/DESKTOP-E0-ADR-REVIEW-V1.md)。
本次仅工程文档，未改变产品代码/wire/schema，新增场景测试NOT RUN；E0-01/02现有证据与剩余物理/准确安装包矩阵不变，两平台U6仍PARTIAL。未推送，不自行宣称ADR冻结或E0通过。

## 2026-09-13 桌面体验 E0 中间检查点（未验收）

本轮按新的 U1–U7 授权继续，不受旧 M5 停止点限制。唯一工作分支 `codex/desktop-experience-upgrade`，基线3bcb730；全轮22条 [TODO](DESKTOP-EXPERIENCE-TODO.md) 已建立，E1尚未启动。
准确M5四包本地SHA256与当前GitHub digest重新一致；Windows准确ZIP payload真实UIA复现手动内容、长设置、只有屏蔽入口及预览前禁用接收。NSIS和Mac完整准确包矩阵未运行；详见 [E0基线](evidence/DESKTOP-E0-BASELINE.md)。
Mac/香港/登记荷兰节点当前manager登录与只读审计成功；香港health/真实WSS认证/在线列表分列通过，物理桌面双向文件链尚未运行，namespace能力检查不算双NAT；见 [E0网络](evidence/DESKTOP-E0-NETWORK.md)。
Windows真Share Target开发MSIX编译/打包/签名成功，但系统拒绝证书信任和开发注册，未改主机安全策略；Mac真.appex编译/注册/系统共享服务激活有证据，文件授权交接仍在验证。原型不含假设备/发送，不称U6完成。
[ADR0007](adr/0007-desktop-membership-consent-and-clipboard.md) 提出成员代际、同事务撤销、独立LAN同意、旧端拒绝和自动剪贴板支撑，待总控审查，尚未冻结或改变产品协议/schema。
实际根基线普通/workspace/独立vet由总控在3bcb730验证（缓存边界保留）；执行任务desktop独立test及新增control-inspect编译/vet/真实HTTPS运行退出0。未推送/部署/合并/正式发布。本检查点不能把原型、源码或提交等同阶段通过。

## 2026-09-12 桌面能力（用户指定M5收尾）

M5已合并main并于2026-09-12T10:13:32Z发布 [v0.5.0-m5](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-m5)。三平台CI、精确Windows/Mac ARM payload、6个远端asset digest与香港最终部署验证通过；详见 [M5发布回执](evidence/DESKTOP-M5-RELEASE.md)。本轮按用户要求停止，不继续M6。

基线 `fbfc250`，工作分支 `codex/desktop-six-features`；用户明确授权逐目标推送、部署和发布。
依赖官方核实与安装完成：Go1.27.1、Node24.21.0、pnpm12.4.1、React19.3、TS6.0.3、Vite8.3、Vitest5、Query5.102.8，Wails保留beta.18。
已实现本机拒绝与trust schema1迁移、profile内核锁、保存退出及调度/迟到事件屏障；真实QUIC保存退出与针对性race通过。
用户最新要求完成M5后合并、测试、改文档并上传GitHub/Release，然后停止。M6已有代码与局部证据保留，不再继续其完整验收。M5最终实现、原生动作及真实限制见 [M5执行记录](evidence/DESKTOP-M5-EXECUTION.md)。
完整命令/失败修复/各阶段状态见 [六项执行记录](evidence/DESKTOP-SIX-FEATURES-EXECUTION.md)。
Mac地址按用户最新信息更新为10.234.14.15，SSH已通过，隔离工具链安装已完成。

用户后续明确最低macOS13、取消12支持，并允许M0/M1合并发布；M2/M3平台层和M4接收协议在隔离分支并行推进。
M1已增加task schema3、设备偏好、持久草稿、幂等队列、离线旁路、源变化确认及200ms合并的epoch/revision事件；
前端已拆分且使用Query/真实绑定，29项测试通过。根完整普通/race及双模块检查通过，原生UI与包级验证进行中。
具体实现与真实QUIC负例见 [M1执行记录](evidence/DESKTOP-M1-EXECUTION.md)。

M0/M1 精确提交 `2f2656d` 的三平台 CI、四个包哈希/来源校验及 Windows CI payload 原生六项操作 PASS，
香港 M1 事务部署与独立复核 PASS；[部署和备份](evidence/DESKTOP-M1-DEPLOYMENT.md)。
M2 已接入单实例、有界持久入口日志、窗口拖放、SendTo/Finder provider、数量大小预览；
Windows 同时入口/相对路径/隐藏/双 profile/冷启动/重启原生检查 PASS，完整菜单/拖放验收继续。
M0/M1 已正式发布为 [v0.5.0-m1 测试预发布](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-m1)，
包含精确 CI Windows/两种 Mac 包，非 Latest；Mac ARM 原生启动/退出也已通过。
发布后深测发现 M1 Windows 自定义退出按钮兼容问题，Release 已追加已知问题；当前 M3 源码改用原生 TaskDialog。
M3 已接托盘/后台/通知/防睡眠，Windows 原生隐藏唤回、实际4MiB接收中的继续和保存退出均 PASS。
还修正后台偏好误取消接收、shutdown 状态发布早于实际取消及 M2 CI 测试上下文问题。
具体失败、修复和哈希见 [M3 执行记录](evidence/DESKTOP-M3-EXECUTION.md)。
M4接收应用层、M5完整内容链路与已完成的收件箱UI已串行合入；后续不再扩展M6。

M4 传输与 App 后端已接入不可变清单、分页选收、冲突策略、目录/计划持久化、实际空间预检、NoContent、发送选择摘要恢复绑定以及M6完整索引登记。真实QUIC同时修复SQLite并发写锁和接受后error/body帧歧义，并验证活跃接收时改全局目录不会取消任务；完整根普通/race、双模块检查通过。UI/物理双机原生验收与里程碑发布另行记录，不因后端测试而标为完成；见 [M4 App证据](evidence/DESKTOP-M4-APP.md)。

M4/M5 wire整合保留三个可选能力；发送端选择和实际内容解释的持久化回调都成功后才发accepted，并同时验证manifest/selection/content摘要。联合真实QUIC及旧端兼容验证见 [整合证据](evidence/DESKTOP-M4-M5-ACCEPTANCE-INTEGRATION.md)。

M4/M6 已完成 Windows 真窗口选收、原生目录选择、保留两份、空目录、全跳过、真实 QUIC 双方确认及收件箱检索、Explorer 定位、仅清记录和孤立暂存清理；证据见 [原生验收](evidence/DESKTOP-M4-WINDOWS-UI.md)。Mac M3 --background 启动退出缺陷已通过实机修复验证，通知真实含点任务 ID 已修复；未将源码通过等同已发布。

Updated: 2026-09-12. Current source: `0.5.0`; published milestone: `v0.5.0-m5`; protocol: V1.
The following `v0.4.0` sections preserve earlier evidence. No prerelease is an accepted production release.

## 2026-09-12 v0.4.0 文档、主分支与测试预发布

- 产品版本从 `0.3.0` 提升到 `0.4.0`，协议仍为 V1。该 minor 版本汇总配对幂等与错误语义、安全 LAN 发现、WSS 读泵复用、trickle ICE、响应端 QUIC listener 预注册、显式双方完成确认、DHCP/EFS 修复和接收热路径优化。
- 公开 README 按使用者视角说明功能、数据路径、快速开始和边界；架构、协议、安全、性能、测试、验收、路线图、部署及平台测试文档同步到 `0.4.0`。旧版本发布记录和当时的失败/未运行结论继续作为历史事实保留。
- Release 只使用 GitHub Actions 从 tag 对应干净提交生成的 Windows amd64 ZIP、Windows NSIS 安装程序与 macOS arm64/amd64 DMG；本地 dirty snapshot 和实机临时包不作为 Release 资产。Windows 未代码签名，macOS 仅 ad-hoc 签名且未公证。
- 仍保留真实限制：无 Relay/TURN 文件中继；MASQUERADE-only 双 NAT 曾返回 `CHECK_TIMEOUT`；公共 IPv6、更多 NAT 类型、网络切换/睡眠唤醒、完整安装卸载与全量原生交互没有因本次发布而自动变为 PASS。
- 实现与第一版公开文档已直接合入并推送 `main`，提交 `7303f34260c09b8ad71c117ee2d6837fac4dfe46`；推送前 `origin/main` 与本地没有分叉。提交前 Windows 根普通/race/vet/`GOWORK=off`、桌面独立 verify/test/vet/build、前端 typecheck/lint/11 tests/build、真实 loopback ICE/TLS 1.3/QUIC 摘要校验和 Wails/NSIS `0.4.0` 版本资源检查全部 PASS。
- 香港测试主站从该提交的独立干净 clone 构建并事务部署：`vcs.modified=false`，服务端 SHA256 `227e2ec9fb029fcc8a568637376318e4eae4099f1cc2ce79bc7224557f07fa45`，最终 systemd MainPID `393922`，schema 2、数据库完整性、443、公网 health、产品/协议版本、QUIC、`relay=false` 与 recent fatal=0 均 PASS。公网配对再次确认 TTL=600 秒、同身份幂等与其他身份 `PAIRING_CODE_USED`。
- 部署审计同时修复了即将到期的 Origin 证书续期链路：移除缺失的 `jq` 依赖、规避 Cloudflare HTTP/2 `PROTOCOL_ERROR`、将 rendezvous 迁移为独立 systemd service，并让续期任务只执行证书事务和 service restart。真实续期、定时器和独立复核 PASS，失败单元为 0；备份与回滚见 `DEPLOY-HK.md`。
- 最终文档提交 `e17e0cb793a2f69162394d3d53e5cda8b13912fa` 的首轮 GitHub core run `34667737461` 真实 FAIL：Linux runner 分别出现 simultaneous session 首个 QUIC payload 为空和 persistent inbox 未创建确认任务；desktop/package job 继续独立运行，但该提交不允许打 tag。两项在 Windows 各重复 100/20 次未复现，失败共同发生于 trickle 收敛后的首个数据流附近。
- 根因是 Pion 可能在 `Connect` 返回后报告初始化阶段最后一次 selected-pair 优化；旧代码已立即武装运行期路径监控，会把这次初始化尾声误判成网络迁移并关闭刚建立的 QUIC。endpoint 现在使用 1 秒初始稳定窗口：窗口内只更新 pair 基准并重新计时，不增加建连等待；窗口后不同 pair 仍触发关闭与显式恢复。新增纯状态回归测试后 simultaneous QUIC 重复 100 次、persistent inbox 重复 50 次、完整普通/race/vet/`GOWORK=off` 再次 PASS。

## 2026-09-12 配对幂等、LAN 直接发现与接收热路径优化（v0.4.0）

- 配对服务将原先统一的“invalid/expired/used”拆分为稳定错误码，控制库迁移到 schema 2，增加 `used_by/used_at`；同一身份在成功响应丢失或前端重复点击后可幂等取得既有成员，其他身份仍被拒绝。邀请响应增加服务端相对 TTL，前端显示真实倒计时并用同步门闩阻止同一渲染帧重复提交；输入兼容大小写、空格和常见连字符。新码生成事务清理超过 24 小时的失效摘要并对极低概率摘要碰撞重试。
- 新增 `internal/discovery`：UDP/53318 签名组播、每接口定向广播、单播响应和受限手工 IP 探测；公告 2048-byte 上限、TTL=1、直连前缀校验、15 秒过期、nonce 重放窗、响应限频。发现后用临时 TLS 1.3 双向 Ed25519 控制通道交换现有签名 ICE envelope，文件仍只经 Pion ICE + QUIC。未配对 LAN 设备只有在发送方选择、接收方确认且双方完整完成后才 pin；失败/拒绝不留信任。成功地址随 pin 保存，重启后定向探测，不扫网段。
- Windows 物理网卡为 `10.234.232.205/16`、Private，Mac 为 `en0=10.234.171.192/16`，双方另有 Mihomo/utun。校园混合有线/Wi-Fi过滤组播与定向广播；受限单播实测 Windows 正确选择“以太网”、Mac 正确选择 `en0`，互相发现签名身份，Windows→Mac TLS 控制心跳 PASS。Mac→Windows 被测试探针现有两条 Windows Private/Public 入站 Block 规则拒绝，未修改防火墙，正式应用仍需首次运行允许专用网络。证据 `/tmp/codex-ssh/linksend-lanprobe-unicast-v3-20260911T235205Z`。
- 接收热路径把已验证字节统计从每 ACK 全量扫描改为 O(1) 单调计数；正文仍逐块 `file.Sync`，恢复位图最多每 8 块或 500ms 批量 checkpoint。checkpoint 落后不丢数据，因为重启仍逐块重算 staging 哈希。传输中保存设置不会取消 LAN 任务，发现配置在当前任务完成后再重启。
- 已通过 Windows 根普通/race/vet/`GOWORK=off`、桌面独立 verify/test/vet/build、前端 typecheck/lint/11 tests/build、Wails 3 production（1 service / 33 methods / 17 models）。香港主站已部署 `0.4.0` 与 schema 2 并完成公网配对复核；Mac 已安装包含 LAN envelope 签名修复的候选，受限单播发现、mTLS 控制心跳及停止后台接收后的并行文件准备/ICE/QUIC 精确路径 PASS。该精确诊断路径耗时约 0.36 秒，但新版原生确认弹窗的完整人工点击仍不提前标记 PASS。

## 2026-09-12 DHCP、trickle ICE 与双机连续发送修复

- 后续及时性优化把认证 WSS 改为单后台读泵，空闲接收与发送借还同一连接；业务 context 取消不再破坏 socket，心跳响应由泵消化。Mac 10 轮接收始终 `connection_count=1`，本地双向复用回归两端总 WSS 数保持 2。香港主站在双方 `end_of_candidates` 转发后立即释放协商，事务部署前备份 `/opt/linksend-lan-test/backups/20260911T223124Z-transport-wss-reuse` 校验可读且数据库 integrity=ok；新 PID `389489`、二进制 SHA256 `d48c2c9ec50a71eddfdcf8caf97307c7b45c70b18c629e63fc2127fd3ef83d13`，公网 health/协议 V1/QUIC/`relay=false`、schema 1 和 recent fatal=0 均 PASS。部署 job `/tmp/codex-ssh/deploy-linksend-wss-reuse-20260911T223111Z`，未触发回滚。
- 源文件 Prepare/hash 与信令、ICE、QUIC 并行；安全自动网卡结果在后台接收启动时预热，并在每次复用前按原接口和当前 IP 复核，DHCP 变化会重新枚举。任务保存最多 64 个本地 phase 时间点及连接阶段耗时；纯 UI 进度/phase 不再同步提交 SQLite，块发送/接收、恢复元数据和终态仍持久化，避免历史写入阻塞每个 QUIC ACK。
- 响应端在发出 `connect_response` 前注册 pinned TLS QUIC listener，避免 initiator 首包早到后等待 PTO；成功终态增加兼容 `confirmed_ack`，标准 QUIC close 的 draining/endpoint 回收移出用户完成路径。物理 Windows→Mac 新版连续 10/10 PASS：1,296–1,604 ms，平均 1,452 ms，相对同现场旧 3,521–3,749 ms 平均缩短约 59%；发送端 WSS 0–0.7 ms、endpoint 81–90 ms、QUIC 多数 11–15 ms。十个不同文件两端 SHA256 逐一一致，证据 `/tmp/codex-ssh/inbox-opt-v16-20260911T223358Z`、复核 `/tmp/codex-ssh/verify-linksend-files-v16-20260911T223613Z`。跨 NAT、原生弹窗人工点击、IPv6/网络切换/睡眠唤醒仍未据此宣称通过。
- 最终源码快照在 Windows 通过根普通/race/vet/`GOWORK=off`，桌面独立 verify/test/vet/build，前端 typecheck/lint/8 tests/build 与 Wails production。Windows 当前运行 `apps/desktop/bin/LinkSend.exe`，版本 0.3.0、SHA256 `9df2116bb23ebaaf6b4790fd7b01b21f488d99f761a9880ccf8129729473dacc`。Mac 同源码普通/race、桌面、前端及 arm64 DMG 构建通过，DMG SHA256 `7988bf1bf99f1155de4d4c2d3e382faf185ebeaf35ac0f8ecb4c896d8061ae25`；已事务安装到 `/Applications/LinkSend.app`，运行二进制 SHA256 `8fd1c04724dd97e5b49cf5917fd254d78f6f0406550ac0647f7882f98405eb2c`，替换前备份为 `/Users/wen/Library/Application Support/LinkSend/app-backups/20260911T224737Z-transport-v16/LinkSend.app.copy`。

- 使用 `codex-ssh-manager` 将 `mac-test-102342413` 更新为 `wen@10.234.171.192`，resolve/probe/audit-host 通过。Mac 当时运行 0.3.0，但偏好仍固定旧地址 `10.234.0.86:0`；已先备份再清空为自动选择并重启。备份为 `/Users/wen/Library/Application Support/LinkSend/desktop-preferences.json.pre-dhcp-bind-fix-20260911T193855Z.bak`。
- 桌面运行时现在检查已保存的字面量 IP 是否仍属于当前接口；DHCP 旧地址本次运行回退自动选择，环境变量显式绑定不被覆盖。endpoint 接受纯 IPv4/IPv6 并补 `:0`；普通 MTU 相同时优先非 point-to-point 接口，显式优先级仍可选择隧道。现场稳定选择 Mac `en0 10.234.171.192` 与 Windows `以太网 10.234.232.205`，未选 `198.18.0.1` TUN。
- 原实现串行等待两端完整 STUN gathering，小文件在 ICE checks 前约空等 8 秒，而真实 ICE 只需约 0.1–0.4 秒。现改为请求/响应立即发送、候选逐个签名 trickle、Pion checks 与 gathering 并行；验证到 `lan_direct` host path 后双方正常提前结束无关 STUN 等待。
- 初版并行实现曾在高重复/race 中复现 QUIC code 0 提前关闭：Pion 初始从 peer-reflexive 切换为 host pair，被旧逻辑误判为运行中迁移。endpoint 现分 provisional/final 两阶段，初始收敛不关闭 socket，候选交换结束后才固定 pair 并开启后续路径监控。修复后关键连接套件连续 20 轮及 race 通过。
- 响应端在已验证请求后若 endpoint、候选、ICE/QUIC 或期望 peer 检查失败，会返回绑定 sender/recipient/session/generation 且经 Ed25519 签名的受限 `status` 稳定码；发起端验证后立即结束，不再把确定的对端失败误报成 30 秒超时。消息不含系统 cause、路径、token 或 ICE credential。
- Windows profile 启用了 EFS，历史 `trust.json` 的加密状态不同，成员同步时 `os.Rename` 返回无法移动到不同磁盘，造成 session 前间歇性 `DIRECT_FAILED`。连接热路径现优先使用本地 pin，仅缺 pin 时刷新服务端设备；名称变化不再重写 pin。信任保存增加 Windows `ReplaceFileW` 与带 `trust.json.previous` 校验/崩溃恢复的原位兜底。真实 EFS 目录上 `devices` 已退出 0、JSON 可解析且恢复日志清理；修复前备份为 `.artifacts/diagnostics-20260912-bind-fix/trust.json.before-efs-replace-fix`。
- 物理 Windows→Mac 连续 10/10 PASS，每轮新建 WSS/session/UDP endpoint，耗时 3521–3749 ms，全部 host↔host、`lan_direct`、TLS 1.3、QUIC、`relay=false`。Mac 十份文件 SHA256 均为 `40548e1b1e13d54f38ef44be02a25fe6f1a82490f4d33f475043c1d965317955`，与 Windows 源一致。证据 `/tmp/codex-ssh/live-repeat-v5-20260911T205855Z`，复核 job `/tmp/codex-ssh/verify-mac-repeat-v5-20260911T210112Z`。自动接收覆盖连接到 `AwaitingAcceptance`，原生弹窗人工点击仍未冒充自动化 PASS。
- Mac v5 源包 SHA256 `57b17507d6d8feeef708e4c4ecad4e2018e640f1c3d3939156a9c7e9ee8d1272`，arm64 DMG SHA256 `e4c115dd49576459289c75b628f5e482aca3324e5a8bee7f3649a524ddd42c80`；Mac 普通/race/desktop/原生构建通过并安装到 `/Applications/LinkSend.app`，运行二进制 SHA256 `907ebaa6a7f25158c959effd4eb50aec90afb3ff4ef122c0fcad69cd928b7519`。替换前备份为 `/Users/wen/Library/Application Support/LinkSend/app-backups/20260911T205524Z-connectivity-v5/LinkSend.app.copy`。
- Windows 当前运行 `apps/desktop/bin/LinkSend.exe`，SHA256 `1650b6ad6ac690c9fd29e29d0b31a783b8a407c596d2c8845b3465e908f548b7`，版本 0.3.0；Windows 与 Mac 的保存 bind 均已改为自动，Windows 偏好备份为 `.artifacts/diagnostics-20260912-bind-fix/desktop-preferences.json.before-auto-bind-windows`。两端最终 GUI 均已恢复，Windows 2 条、Mac 2 条 TCP 连接 Established；Mac 最终复核 job `/tmp/codex-ssh/verify-mac-gui-v5-final-20260911T210933Z`。最终实际通过：根普通测试、完整 race、vet、`GOWORK=off`；桌面独立 test/vet/build；前端 typecheck/lint/8 tests/build；Wails Windows build。当前仍是未提交测试快照，未推送、未发布，跨 NAT/IPv6/睡眠唤醒未因本轮 LAN 结果改写。

## 2026-09-12 Windows 在线但 `DIRECT_FAILED` 排查与绑定地址修复

- 本机持久任务历史显示最近两次发送均在约 3 秒内结束于 `connecting` / `DIRECT_FAILED`，实际发送 0 B，且没有 `session_id`、ICE 候选、STUN 计数或路径证据；因此失败发生在本地 UDP/ICE endpoint 创建之前，不是文件内容、接收目录或已建立直连后的传输失败。
- 当前 `%APPDATA%\LinkSend\desktop-preferences.json` 保存的 `bind_address` 为纯 IP `10.234.232.205`。该地址仍是物理以太网的有效地址，但旧 endpoint 构造直接传给 `net.ResolveUDPAddr`，要求 `IP:port`，从而在 presence 仍显示在线时提前失败；桌面任务层将该非协议底层错误归并成了笼统的 `DIRECT_FAILED`。
- 当前配置已修正为 `10.234.232.205:0`，应用正常退出后重新启动；窗口创建成功，后台信令连接保持 Established。没有自动向对端发送测试文件，物理双机重试仍需用户在界面发起，不能用启动/信令检查冒充传输通过。
- `internal/connectivity.New` 现兼容纯 IPv4/IPv6 字面量并自动补临时 UDP 端口 `:0`；新增 `TestIPOnlyBindUsesEphemeralPort`，避免用户再次因省略端口进入同一失败路径。协议、ICE、QUIC、TLS 和文件数据路径未改变。
- 实际通过：`go test -count=1 ./internal/connectivity`、根模块 `go test -count=1 ./...`、`go vet ./...`、根 `GOWORK=off go test -count=1 ./...`；桌面 `GOWORK=off go test -count=1 ./...`、`go vet ./...`、`go build ./...`；Wails 3 Windows/amd64 production build。首次构建因独立 `task` 命令不存在退出 1，改用已固定的 `wails3 task` 后首次因 PATH 未包含 `wails3` 退出 1，显式加入 `.tools/bin` 后通过。
- 修复构建 `apps/desktop/bin/LinkSend.exe` 为产品 `0.3.0`，SHA256 `EDFA11F57EEE4C477AC95346212F48BD0AEF30D9DAD99F37DF3A2AC92C7658C5`。直接覆盖 `C:\Program Files\LinkSend contributors\LinkSend\LinkSend.exe` 被 Windows 权限拒绝，未绕过权限；旧安装文件已备份到 ignored 本地诊断目录。已安装版当前依靠修正后的 `IP:0` 配置工作，源码兼容修复待下一次安装包更新。

## 2026-09-11 v0.3.0 局域网连接、配对码与自动接收简化（当前交付源码）

- 按最新产品决策降低首次配对门槛：动态配对码改为 40 位随机量的 `ABCD-EFGH`，10 分钟有效、一次性、服务端仅存规范化摘要；输入忽略大小写/连字符/空格，旧 43 字符邀请继续兼容。配对成功后客户端自动固定认证成员列表中的 Ed25519 公钥，不再要求手工核对 64 位指纹；既有 pin 发生密钥变化仍拒绝。首次配对现在明确信任信令服务，安全边界已同步更新到 SPEC/SECURITY/PROTOCOL。
- `internal/app/inbox.go` 增加不产生空闲任务历史的常驻接收器：桌面打开且存在接收目录时自动连接 WSS、保持设备在线、收到连接后才创建接收任务；任务结束后自动恢复监听。发送会有界停止本机空闲监听，完成后恢复；对端刚恢复监听的 `PEER_OFFLINE` 有 4 次短退避重试，ICE/QUIC 失败不会盲目重试。
- 接收 manifest 后进入 `AwaitingAcceptance` 并由 React 弹出确认层，显示发送设备、内容、数量、总量和目标目录。用户可勾选“以后自动接收此设备”，偏好写入 `trust.json`；后续仍执行固定设备密钥、TLS 1.3、块哈希、完整摘要和安全落盘校验。设备页可恢复为每次确认。
- 当前 Windows 网卡审计确认旧自动顺序会先选 Mihomo TUN `198.18.0.1`（Go MTU 65535），而不是物理以太网 `10.234.232.205`。自动选择现在在没有显式优先级时先选 1280–9000 的常规链路 MTU；不按接口名或私网段硬编码，显式优先级仍可选 TUN。实际选中远端地址属于本接口直接配置网段时才报告 `lan_direct`。
- UI 移除“设备组、手工信任、开始等待接收”，收敛为生成码、输入码、选择设备发送和接收弹窗。`frontend-skill` 指导下保留既有克制桌面系统并强化发送主动作；Playwright 实际检查桌面传输页与 700px 配对页布局，截图位于 `output/playwright/`。浏览器预览缺 Wails runtime 的 404 仅作为预览限制，不计原生功能通过。
- 实际通过：根模块普通测试、完整 race、vet、`GOWORK=off go test`；配对码规范化/单次消费测试；真实 loopback Pion ICE + quic-go 的“无手工 trust + 无 StartReceive + 首次确认 + 第二次自动接收”回归；桌面模块 `GOWORK=off` mod verify/test/vet/build；前端 typecheck/lint/8 tests/build；Wails 3 production Windows/amd64 build（1 service / 31 methods / 15 models）。最终本地 `apps/desktop/bin/LinkSend.exe` SHA256 为 `814C4AF7587733F199F081DF1211C715C4830F50005AA88AFADF35FB8C2AF7A1`；隔离 profile 隐藏启动 4 秒仍存活后由本轮检查结束。
- 当前交付请求：产品版本提升为 `0.3.0`（协议仍为 V1），以同一源码提交更新香港测试主站并生成 Windows 与 macOS arm64 测试包；部署与产物证据追加在本节。新版物理 Windows↔macOS 局域网、真实原生接收弹窗、按设备免确认、跨 NAT、IPv6、网络切换和睡眠唤醒仍需单独实测，不能用 loopback 或浏览器预览冒充。
- 源码提交为 `df8732b72ea18727b455441c94debc0ebf4b5d49`，本轮未覆盖 `v0.2.0` tag、未创建 Release。Linux rendezvous 从该提交的独立干净 clone 构建，`vcs.modified=false`，SHA256 `431a6939ab074e12a27a6e321691cd0be1a35f6529f09ce8e2dc8cdf8d0bfa99`。
- `hk-main` 经 SSH manager resolve/probe/audit 后执行事务部署。首次只读脚本因错误假定数据库路径及源站系统 CA 信任退出 1，没有改动；修正后确认旧 PID `381064`、版本 `0.2.0`、数据库 `/opt/linksend-lan-test/data/server.db`、schema 1、integrity=ok。部署备份 `/opt/linksend-lan-test/backups/20260911T145102Z-v0.3.0-df8732b` 可读且备份数据库 integrity=ok；新 PID `384944`，公网 health 为 `0.3.0`/协议 V1/QUIC/`relay=false`。动态短码格式、单次加入和双方自动 pin 经隔离公网 profile 验证 PASS，测试设备已撤销、临时身份与候选已清理；独立复核 `RECENT_FATAL_COUNT=0`。部署 job `/tmp/codex-ssh/linksend-v030-deploy-20260911T145049Z`，独立复核 `/tmp/codex-ssh/linksend-v030-independent-verify-20260911T145144Z`；未触发回滚。
- Windows amd64 从同一干净提交使用 Go 1.26.5、Wails `v3.0.0-beta.18`、pnpm 11.19.0、NSIS 3.12 构建。EXE 与安装器 FileVersion/ProductVersion 均为 `0.3.0`；便携 ZIP 完整性及精确 EXE 隔离启动 4 秒 PASS，安装器 7-Zip 检查包含 `LinkSend.exe` 和 WebView2 bootstrapper。便携 ZIP SHA256 `8fe347c28043dcce3083f6c0c664192da9ff1d13789b9429b57feb99b6007dcb`，NSIS 安装器 SHA256 `ba7b5f45f2ae149bef49c4e3b2d78995b5e0b1f525713896a5822a42a9a42b12`；两者均为 `UNSIGNED_TEST_BUILD`，安装/卸载未运行。
- Mac 构建源包先由同一提交通过 `git archive` 生成并排除历史 `.artifacts`，SHA256 `d0c8224b39b424ef368bd3b57aa25d678652a1ba5f35412bb6a67fda9f59b2d6`。`mac-test-102342413` 两次 manager probe 均 TCP/22 timeout，audit 同样失败；用户随后明确授权推送、合并并改走 GitHub Actions，因此该物理 Mac 阻塞不再阻止生成，但物理启动仍为 `NOT_RUN`。
- [PR #7](https://github.com/Wen5555/LinkSend/pull/7) 从 `codex/v0.3.0-pairing-inbox` 合并到 `main`，merge commit 为 `0fc36f1ca79dc8af7e304b8c27a0b0607d3654c0`。分支的两组 core、两组 desktop 与三平台 packaging 全部 PASS；合并后的 main core run `34635195414`、desktop run `34635195393`、packaging run `34635195522` 也全部 PASS。仓库没有配置 required-check 规则；合并前仍显式核对了 7 个实际 check 全部成功，没有借此绕过失败。
- main run `34635195522` 的 Windows ZIP/NSIS 与 macOS arm64 DMG 已下载并逐项复核，三者 `source_commit=workflow_head_sha=0fc36f1...`、`source_state=COMMITTED`、`source_checkout_clean=true`。Windows ZIP SHA256 `c88109e2c9d011bb66fdebbefa571516e856f8ea95f98626bc567d3b4db7d742`，安装器 SHA256 `bc984b6499a2b48f346f8990f1fd77b861f1753917e751bc27dd73ed75ec32c6`；版本资源、ZIP 完整性、NSIS 内置 EXE/WebView2 和精确 CI EXE 隔离启动 4 秒 PASS，仍为 unsigned，安装/卸载 NOT_RUN。
- macOS arm64 DMG SHA256 `07604a4bd3efe69c12b34dbad502ad9aada4209d43aaab7e63bd961a7285a232`。runner 已执行真实 DMG 挂载、bundle id `com.linksend.desktop`、版本 `0.3.0`、arm64 与 `codesign --verify --deep --strict` 检查；本机又完成下载哈希、7-Zip HFS+ 完整性、plist 和 Mach-O `GOOS=darwin/GOARCH=arm64` 复核。Windows 展开 DMG 时仅因 `Applications` symlink 权限返回退出 2，其余 app 内容可读；这不替代物理 Mac 启动。DMG 仅 ad-hoc 签名、未公证。
- 最终文件集中在 `.artifacts/v0.3.0-0fc36f1/deliverables/`，包含简化命名的 Windows 安装器、Windows 便携 ZIP、macOS arm64 DMG、BUILD-INFO、README 和 SHA256SUMS。没有创建或覆盖 `v0.2.0` tag，也没有创建 `v0.3.0` Release。

## 2026-09-11 v0.2.0 已合并并发布测试预发布

- [PR #6](https://github.com/Wen5555/LinkSend/pull/6) 通过 fast-forward 合并到 `main`，merge/tag 目标均为 `426d58b6ab62ab7213475007305c0a403955c00f`。main 的 core run [`34600609146`](https://github.com/Wen5555/LinkSend/actions/runs/34600609146)、desktop run [`34600609047`](https://github.com/Wen5555/LinkSend/actions/runs/34600609047) 和三平台 packaging run [`34600609161`](https://github.com/Wen5555/LinkSend/actions/runs/34600609161) 全部 PASS。
- [GitHub Release v0.2.0](https://github.com/Wen5555/LinkSend/releases/tag/v0.2.0) 已公开为 Pre-release，并明确设为非 Latest。用户后续明确要求合并和发布，因此覆盖了发布前“暂不合并/发布”的决策；已知产品 FAIL 与 NOT_RUN 项没有被改写为 PASS。
- Release 上传 Windows ZIP、Windows installer、macOS arm64/amd64 DMG、合并 SHA256 清单和构建 manifest。GitHub 返回的四个二进制资产 size/digest 与本地重新计算、workflow artifact 内 `SHA256SUMS.txt` 全部一致；三份 BUILD-INFO 均记录 `source_commit=workflow_head_sha=426d58b6ab62ab7213475007305c0a403955c00f`、`workflow_run=34600609161`、`source_state=COMMITTED`、`source_checkout_clean=true`。
- Release Windows ZIP 内 EXE 为 20,635,648 bytes、SHA256 `afd7bcbf9bc58a2a5d590b8a470a2c74a6caf6d2a80cc4dc4e0729627c55a0b0`，FileVersion/ProductVersion `0.2.0`，Wails 3 beta.18。对该确切 EXE 的隔离可见原生冒烟为窗口句柄非零、idle `WM_CLOSE` 接受、10 秒内 exit 0；隐藏窗口方式不能取得句柄的测试方式 FAIL 也未冒充产品结果。
- 两份 Release DMG 的 runner 挂载、bundle/版本、架构和 strict ad-hoc codesign PASS，但本 release 资产未在物理 Mac 启动；无 Developer ID、未公证。Windows 安装/卸载也仍为 NOT_RUN。
- 香港测试主站继续运行服务端实际源码 `4b7ccf66...` 构建的产品 `0.2.0`；后续客户端/文档提交未改变 rendezvous/server/signaling/config/schema，因此未无意义重启。下一次产品版本或服务端依赖变化继续按 [DEPLOY-HK](DEPLOY-HK.md) 的事务流程同步。

| Release 资产 | 大小（bytes） | SHA256 | 构建 UTC | 签名 / 公证 |
|---|---:|---|---|---|
| Windows amd64 ZIP | 8,403,529 | `dcdd7127c527b1464f0ea025046ad871a35ed6eaba4a2c725884779071d81577` | `2026-09-11T12:49:31Z` | unsigned / N/A |
| Windows amd64 installer | 9,996,124 | `99a4d5d25dbc9761e4839ff435f362ec65aa75fcdafdf46f285f05defdde7370` | `2026-09-11T12:49:31Z` | unsigned / N/A |
| macOS arm64 DMG | 8,170,928 | `c637897290677c7deb8c50395b95568f55a93a14bdf51e2babbf7eaf974fdf4c` | `2026-09-11T12:48:16Z` | ad-hoc / NOT_RUN |
| macOS amd64 DMG | 8,808,743 | `f5f066634429e61312066c056b836482ca228315ea858cab3de28653e67120fe` | `2026-09-11T12:51:36Z` | ad-hoc / NOT_RUN |

准确边界保持不变：MASQUERADE-only 独立双 NAT 是产品 `CHECK_TIMEOUT` FAIL；Relay 未实现；完整原生交互、物理跨平台恢复、安装/卸载、签名和公证未完成。`v0.2.0` 只能称为测试预发布。

## 2026-09-11 v0.2.0 发布前候选记录（历史）

- 版本决策：从 `0.1.0` 提升到 `0.2.0`，因为本轮在 1.0 前加入任务 schema 2、暂停/重启恢复、验证后缺块续传、多网卡诊断与连接生命周期修复，属于明显功能扩展；不提升到 `1.0.0`，因为 MASQUERADE-only NAT、物理网络切换、完整原生交互、签名/公证等仍未通过。
- 产品版本与协议分离：`internal/protocol.ProductVersion=0.2.0`；协议 `Version=1`、TLS ALPN `linksend/1` 和 V1 文件帧保持不变。CLI/rendezvous `--version`、`/healthz.version`、capabilities、Wails DTO、Windows/NSIS、macOS plist 和前端 package metadata 统一使用 `0.2.0`。
- 仓库实际使用 Wails 3 `v3.0.0-beta.18`；旧 AGENTS 项目说明中的 Wails 2 不是有效实现状态，不允许据此回迁。
- `.artifacts`、本地 Wails 工具、Task cache、输出目录、WebView2 bootstrapper 和临时远程脚本已加入忽略规则，但没有删除任何既有文件。历史 `0.1.0` Stage/Release 文档保留原事实并标记为历史快照。
- 用户已明确香港入口是测试主站，并授权以后每次产品版本或服务端依赖更新时同步服务。每次仍执行 manager resolve→probe→audit-host 与 `inspect → backup → change → verify → rollback-ready`；只操作 `/opt/linksend-lan-test` 的测试服务，不修改防火墙、路由、DNS、代理或正式身份数据。本轮部署、验证和回滚方法见 [DEPLOY-HK](DEPLOY-HK.md)。
- 本节开始时是基线 HEAD `fef3e3f59797a6de25cb7f9b1f2a1850512808d5` 加未提交修改；旧 r2 资产继续标为 `UNCOMMITTED_TEST_SNAPSHOT`。后续已从真实干净提交重新构建 committed 候选；旧资产没有被放入新 ZIP/DMG，也没有改写提交元数据。
- `0.2.0` 本机预提交门槛：根 `diff --check`、mod verify、全量 test、race、vet、GOWORK=off test/build PASS；P0 立即重连 20 次和真实 QUIC 终态/恢复定向套件 3 次 PASS；Wails bindings 1 service / 27 methods / 14 models，生成后前端 frozen install/typecheck/lint/8 tests/build PASS；桌面独立 mod verify/test/vet/build、Wails production 与 NSIS 3.12 PASS。
- 版本资源复现并修复：Wails 3 beta.18 自动 build-assets 生成的 Windows version info 缺 `FileVersion` 显示字符串，使主 EXE 的 PowerShell VersionInfo 为空；将 fixed file/product version 规范为 `0.2.0.0`、使用 `0409` string table 并补 `FileVersion=0.2.0` 后，主 EXE 与 installer 的 FileVersion/ProductVersion 均实际返回 `0.2.0`。新增跨源码/Wails/安装元数据同步测试防止回退。
- 源码提交 `4b7ccf66f89c9720d9b5970e5d610b453048d69c` 已建立。直接在保留 untracked 的主工作树和共享 Git worktree 构建均被 Go 标为 `vcs.modified=true`，两次资产均拒绝部署；改用独立干净 clone 后 Linux amd64 rendezvous 的 `go version -m` 为该 revision、`vcs.modified=false`，大小 11,309,218 bytes，SHA256 `c549cdbf1bd461d6f583302f96700471000aacba1fed912c66d6e74912f30247`。
- 香港测试主站已按 manager 事务完成 `0.2.0` 同步：备份 `/opt/linksend-lan-test/backups/20260911T101714Z-v0.2.0-4b7ccf6`，旧 PID `352572` → 新 PID `381064`；配置 SHA 与数据库 schema/integrity 不变。公网 health 报告产品 `0.2.0` / 协议 V1；两次完成后立即重连、拒绝后立即完成各传 2 MiB 均 PASS。测试运行时已删除，两条测试设备记录均撤销，额外数据库清理备份为 `/opt/linksend-lan-test/backups/20260911T102329Z-v0.2.0-lifecycle-cleanup`。旧主站立即重试 FAIL 因实际部署复核而关闭。
- 候选提交 `9cce3eef20a6224f56c9da79d16f8966cabb61f9` 的三平台 packaging run `34592549254` 全部成功，但同 SHA 的 push core run `34592549314` 在真实 QUIC `TestDirectServiceSendAndReceive` 复现 `Application error 0x0 (remote): closed`；PR core 的成功 run 不抵消这个 FAIL。根因是接收端读到 `confirmed` 后正常关闭单次传输连接时，quic-go 可能先交付 application close code 0、后交付 stream FIN，发送端 terminal flush 将正常终态误判为失败。
- 最小修复只在“已验证 completed、已写出 confirmed”的 terminal flush 内接受远端 QUIC application code 0；非零 close、reset、timeout 和其他阶段错误仍失败，协议 V1 不变。新增真实 QUIC 正向及非零 close 负向测试；Windows 上完成用例 100 轮、含负例套件 50 轮、race 20 轮和 app 端到端 100 轮均 PASS。修复提交为 `a88553180bd1defac5b236d75fe4dff5046734dd`；`9cce3ee` 及其包已被取代，只保留为缺陷前对照证据。
- 发布前当时，`a885531` 的 GitHub required checks 已完成：core push run `34596127764` PASS、core PR run `34596131146` PASS、desktop push run `34596127757` PASS、desktop PR run `34596131090` PASS；三平台 packaging run [`34596127738`](https://github.com/Wen5555/LinkSend/actions/runs/34596127738) 的 Windows amd64、macOS arm64 和 macOS amd64 jobs 全部 PASS。该检查点的决定是暂不合并或创建 Release；之后用户明确覆盖该决定，实际发布状态见本页顶部。
- committed Windows ZIP 内 EXE 为 20,635,648 bytes、SHA256 `afd7bcbf9bc58a2a5d590b8a470a2c74a6caf6d2a80cc4dc4e0729627c55a0b0`，FileVersion/ProductVersion 都是 `0.2.0`，Wails 为 `v3.0.0-beta.18`。隔离 profile 下原生窗口句柄非零，`CloseMainWindow()`/`WM_CLOSE` 被接受并在 10 秒内 exit 0；这只验证窗口创建和空闲关闭。测试 profile 的两个文件因本地命令策略未能清理，仍留在 ignored `.artifacts` 检查目录且不得提交。
- committed macOS runner 已完成 DMG 挂载、bundle、架构和 `codesign --verify --deep --strict`，但仅有 ad-hoc 签名，无 Developer ID、未公证。物理 Mac `mac-test-102342413` 在收尾时 resolve 成功，probe/audit-host 均 TCP/22 timeout，因此 committed arm64 DMG 启动为 `BLOCKED_BY_EXTERNAL_ENV`；不能用 runner 或历史 dirty r2 窗口证据替代。

### `a885531` committed 候选资产

所有资产均来自 workflow/head `a88553180bd1defac5b236d75fe4dff5046734dd`、`source_state=COMMITTED`、`source_checkout_clean=true`。工具为 Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails 3 `v3.0.0-beta.18`；Windows 使用 NSIS 3.10，macOS 使用 Apple clang 17.0.0。

| 资产 | 大小（bytes） | SHA256 | 构建时间 UTC | 签名 / 公证 | 原生验证 |
|---|---:|---|---|---|---|
| Windows amd64 ZIP | 8,403,530 | `316a11b74cf146762138384c0cd84ce76b8ec19a48943b1b0abf23f89de37690` | `2026-09-11T11:55:19Z` | unsigned / N/A | 窗口创建与 idle `WM_CLOSE` PASS；其余 NOT_RUN |
| Windows amd64 installer | 9,996,125 | `aa346acecd333ea8757bb0ed2f6466de4767af82fdc6640332cd2d0c83311092` | `2026-09-11T11:55:19Z` | unsigned / N/A | 安装/卸载 NOT_RUN |
| macOS arm64 DMG | 8,170,931 | `79857ce7ba336e6ceec516b19b737a1386100e9f6dfdf328a44deaeaee142e91` | `2026-09-11T11:54:30Z` | ad-hoc / NOT_RUN | runner 包级 PASS；物理启动 BLOCKED_BY_EXTERNAL_ENV |
| macOS amd64 DMG | 8,808,731 | `15c34a786f5aba9b72e7e2e09eaf6502aebf315cfee6161b9918104543de621f` | `2026-09-11T11:57:59Z` | ad-hoc / NOT_RUN | runner 包级 PASS；Intel 真机 NOT_RUN |

发布前按原门槛判断仍未满足：MASQUERADE-only 独立双 NAT 保持产品 `CHECK_TIMEOUT` FAIL；物理网络切换、committed Mac 原生启动、完整 Windows/macOS 原生交互、Windows 安装/卸载、Developer ID 签名和公证仍未完成。后续虽按用户明确授权发布为 Pre-release，这些缺口仍然有效，不能声称生产可用或跨平台完整验收。

## 2026-09-11 P0/P1/P2 分阶段可靠性改进（历史 dirty snapshot）

- 基线仍为 `main` / `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`；本节全部结果来自保留既有修改的 dirty working tree。未 reset、clean、提交、推送、创建 Release 或复用旧资产冒充本轮构建。桌面实际为 Wails 3 `v3.0.0-beta.18`。
- P0：同设备信令连接替换时按连接所有权清理旧协商；旧 handler 和旧 generation 不能删除/污染新会话；同时发起以设备 ID 字典序收敛到一个 initiator/session；信令短断不关闭健康 QUIC；`confirmed`、拒绝和 error 使用真实 QUIC 半关闭/有界读取完成终态交付。真实 Pion ICE + quic-go 回归覆盖同时发起（10 次）、旧 handler（20 次）、无响应超时后立即重试、信令断开后继续 QUIC、拒绝/冲突/confirmed 终态；受影响包普通和 race 均 PASS。
- 香港测试主站当时仍运行旧 dirty revision `8baaa1cf1d67a48adbceb32ecb469999361a1e09`，二进制 SHA256 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`；短时间重试仍为产品 `FAIL`，不是外部环境阻塞。本轮新构建 `fef3e3f+dirty` 只在 `hk-main` 的 `127.0.0.1:39117` 隔离实例验证：两次完成后立即重连、拒绝后立即重连均 PASS，每轮 2,097,152 bytes 且接收内容 `cmp` 一致。主站二进制与 PID 前后相同，实验进程、配置、DB、身份和正文已回滚删除。成功 job：`/tmp/codex-ssh/linksend-p0-isolated-healthz-retry-20260911T062201Z`；首次误用 `/v1/health` 的脚本 FAIL 证据保留于 `/tmp/codex-ssh/linksend-p0-isolated-validation-20260911T061946Z`。
- P1：任务快照明确区分 `task_id`、`attempt_id`、`session_id`、ICE generation 与单调 `revision`；旧 attempt 回调不能覆盖新 attempt。状态覆盖 Preparing、AwaitingAcceptance、Transferring、Verifying、Paused、Recovering、Completed、Rejected、Cancelled、Failed，暂停/取消停止新块调度，校验/提交失败不能进入 Completed。
- SQLite 历史升级为 schema 2：snapshot 与私有 recovery metadata 按 revision 原子更新；v1→v2 前用 SQLite `VACUUM INTO` 生成权限为 `0600` 的可读 v1 备份；损坏行进入 `task_quarantine` 且不阻塞启动；写失败撤销 `history_persisted`。旧二进制回滚必须在应用关闭后恢复 `task-history.sqlite.schema-v1-*.bak`，不能直接打开 schema 2。
- 真暂停/重启恢复/缺块续传已接入现有 transfer 协议。恢复绑定 TransferID、manifest digest、chunk size、源路径及内容、接收目录、对端身份/指纹；`ResumeTask` 是正文恢复的显式用户确认，每次生成新 attempt/session/ICE credentials。接收端重哈希 staging，只请求缺失或损坏块；源内容变化和本地 pinned peer 变化在继续正文前失败。
- 真实 QUIC 续传证据：12 MiB 首块后暂停，未损坏时本轮/累计实际发送与接收 12,582,912 bytes、重传 0、唯一验证/提交 12,582,912；损坏首个 4 MiB staging block 时实际发送/接收 16,777,216、重传 4,194,304、唯一验证/提交仍为 12,582,912；完整重启两个 Service 后恢复 8,388,608 bytes，沿用 task/TransferID、使用新 attempt/session，内容一致且无重传。只有这些缺块协商与计数通过后，当前实现才报告 `byte_resume_supported=true`。
- `history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 独立报告：模拟历史写失败时前两者为 false，而进程内 byte resume 实现仍为 true。部分文件提交记录会在每个成功 commit 后 checkpoint；后续文件失败时已提交文件和字节保持可验证，不显示整任务 Completed。
- CLI 新增只读 `status [--task]` 与显式 `resume --task ...`；resume 只拥有当前 CLI 进程启动的 attempt，不宣称能暂停/取消另一个桌面进程。Wails 增加 PauseTask/ResumeTask 并重新生成 bindings；React 按 `can_pause/can_resume` 门控，使用 revision 合并防止迟到轮询覆盖新快照，分别展示实际发送/接收、重传、验证、提交与双方确认；固定测试配对码不再常驻界面。
- P2 网络源码新增可用接口/地址族发现、显式接口优先级与排除、IPv6 link-local 排除、实际 base socket/interface/family、ICE 状态时间线、候选类型、TLS/ALPN、STUN 请求/响应计数和信令 JSON 字节计数。未按网卡名称、私网地址、presence 或 candidate type 推断 LAN/公网，连接方法仍为 `direct_unknown`。已选本地地址消失会废弃旧 endpoint、关闭当前 QUIC，并由任务恢复流程等待双方显式建立新 session。
- `scripts/dual-nat-netns.sh` 已在 `lab-server` 的 rootless 外层 user/mount/network/PID namespace 中真实运行：内部创建两个独立 NAT namespace、重叠 `10.77.0.0/24`、独立 WAN、coturn 4.5.2 STUN-only 与 HTTPS 信令，不使用 loopback 或单 Docker bridge。MASQUERADE-only/状态过滤运行真实返回 `CHECK_TIMEOUT`；这是当前无 relay 情况下的端口受限 NAT 能力 FAIL。显式固定 UDP SNAT/DNAT 的可穿透 NAT 运行完成 8,388,608 bytes QUIC，双方同一 session、内容 SHA256 一致，STUN request/response 为发送侧 16/6、接收侧 20/8，NAT_A/NAT_B 计数 9/7，`relay=false` 且仍为 `direct_unknown`。脱敏包 `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz` SHA256 `ba7447fe3eb603b669db3a76ab7ba2be049d7abeeefdb94b4927161f43f7eebf`；远端身份、私钥、正文、coturn、namespace 和测试根已回滚删除。宿主地址/路由前后相同；宿主 iptables 因无非交互 sudo 不能读取，但实验规则全部位于 rootless 外层 namespace。
- 仍未验收：真实网卡切换/睡眠唤醒、公共 IPv6、未配置固定映射的更多 NAT 类型、Windows/macOS 新版原生文件/目录选择、打开目录、红点/Cmd+Q 按键、退出保护和重启恢复入口。浏览器或窗口创建证据不能替代这些原生操作。
- 最终 Windows 门槛：根模块格式、`git diff --check`、`go mod verify`、普通测试、race、vet、`GOWORK=off` 测试全部 PASS；桌面独立模块 `GOWORK=off` verify/test/vet/build PASS；前端 frozen install/typecheck/lint/8 tests/build PASS；最后一次 Wails production build 确认 EXE 中包含当前 `index-CQOAaBGW.css` 与 `index-Dz-ecqi4.js`。
- 发布脚本复现出 NSIS 默认项目名仍为 `LinkSendTemplate`，会生成错误命名的安装器并把内置 EXE 改名。`apps/desktop/build/windows/nsis/project.nsi` 现显式固定 LinkSend 项目、公司、产品、版本、可执行文件和卸载注册键；NSIS 3.12 重建后输出 `LinkSend-amd64-installer.exe`，7-Zip 确认内含 `LinkSend.exe`。本机已有 `%APPDATA%\\LinkSend.exe` WebView 数据，因此未实际安装/卸载，避免触碰现有用户状态。
- Windows 原生冒烟使用隔离 profile 启动本轮 EXE：进程存活、主窗口句柄非零、Windows close message 返回 true 且进程在 10 秒内退出；随后精确删除测试 identity/SQLite。该结果不替代文件/目录对话框、打开目录或活跃任务退出保护点击验收。
- Mac 修复后包使用 `mac-test-102342413`，按 manager 的 resolve→probe→audit-host 和隔离目录流程执行。158 个明确源文件打包为 `linksend-source-fef3e3f-dirty-r2.tar.gz`，SHA256 `8612b015bdbdacae26d9d3915b95f9d9744d3576fbd280bfbef751faa940e3f8`，排除全部旧 `.artifacts`、bin、node_modules 和临时脚本；根普通/race/vet、桌面 test/vet/build、arm64/amd64 顺序构建均 PASS。r2 构建 job `/tmp/codex-ssh/linksend-mac-rebuild-r2-20260911T085119Z` 的构建步骤完成，但验证脚本把请求架构 `amd64` 与 Mach-O 名称 `x86_64` 直接比较而最终退出 1；修正后的只读验证 `/tmp/codex-ssh/linksend-mac-rebuild-r2-verify-final-20260911T085952Z` 退出 0。该脚本缺陷不改写为构建 PASS 的退出码，也不影响两份已独立核验的 DMG。
- Wails 3 原生退出回调语义已修复：`ShouldQuit` 在无活跃任务时返回 true；`Rejected` 纳入终态；有活跃任务时先阻止原退出事件，由异步原生对话框回调取消任务，等待终态后再调用 `App.Quit`。桌面单元测试 PASS；Windows r2 EXE 的 `WM_CLOSE` 与 10 秒内退出 PASS；Mac arm64 r2 包创建 1 个约 1008×684 的原生窗口，idle quit Apple Event 与进程退出 PASS。由于 macOS Accessibility=false，红点实际点击、Cmd+Q 按键、原生对话框、键盘导航和活跃任务退出保护仍为 `NOT_RUN`，不能由 Apple Event 代替。
- r2 两个 DMG 的只读挂载、bundle id `com.linksend.desktop`、架构和严格 codesign 验证 PASS，签名均为 ad-hoc、无 TeamIdentifier、未公证。Mac 远端 r2 源码、build root、身份和两个原生测试 profile 已由 `/tmp/codex-ssh/linksend-mac-rebuild-r2-cleanup-20260911T090331Z` 清理；用户原有 `/Volumes/LinkSend` 挂载不属于本轮测试，未操作。
- 四个历史 r2 资产位于 `.artifacts/final-20260911/`，均标记 `UNCOMMITTED_TEST_SNAPSHOT` / 基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5` / `vcs_modified=true`。其大小和 SHA256 保留于该目录 `ASSET-MANIFEST.json`；旧资产不能改名或复用为 `0.2.0` 提交构建。后续真实提交、香港部署和新资产在本页最顶部另行记录。
- 2026-09-11T09:24Z 对最终工作树重验：gofmt、`git diff --check`、根 verify/test/race/vet/`GOWORK=off test`、桌面独立 verify/test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 全部退出 0；P0 server 立即重连重复 20 次、真实 QUIC 生命周期/终态重复 3 次及 P1 缺块/重启恢复定向回归再次 PASS。`scripts/dual-nat-netns.sh` 的清理补丁 `bash -n` PASS，运行态证据仍来自上述隔离实测，不冒充重跑。强制 bindings 首次因 Taskfile 子进程 PATH 不含 `wails3` 退出 1，临时加入仓库 `.wails-bin` 后生成 PASS（1 service / 27 methods / 14 models），生成后 TypeScript/lint 再验通过。最终 Windows Wails build 退出 0，EXE SHA256 `c808e31a54c8d4819373902f460ef2d5ffbe28b6603f9ab5353465e908f548b7` 与 r2 staging EXE 相同，并嵌入当前 `index-Dz-ecqi4.js` / `index-CQOAaBGW.css`。

## 2026-09-11 连续验收执行记录

- 环境基线记录于 `.artifacts/acceptance-20260911/baseline.json`；当前工作树原有未跟踪构建/测试产物保持不变。Windows amd64、PowerShell 7.6.5、Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Docker CLI 29.2.1；Wails 3 `v3.0.0-beta.18` 已按锁定版本安装到本地工具目录。
- 根模块 `gofmt`、`git diff --check`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`GOWORK=off go test -count=1 ./...` 均 PASS；桌面模块 GOWORK=off test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 均 PASS。
- Wails `task build ARCH=amd64` 首次因 PATH 未包含 CLI 而失败（环境配置缺陷，原始错误保留在会话记录）；补充 PATH 后重新运行 PASS，并重新生成 TypeScript bindings。生成器发现 `internal/protocol.Capabilities` 的三个历史/恢复字段未同步到 `apps/desktop/frontend/bindings/.../internal/protocol/models.ts`，已更新该绑定文件；未改变协议或数据路径。
- `demo-local` PASS 仅限 loopback-direct（真实 TLS 1.3/QUIC、1 MiB 摘要一致）；`test-nat` 退出码 1，标记 BLOCKED_BY_EXTERNAL_ENV（Windows 无受控 Linux namespace/NAT fixture）。Docker Compose 静态解析 PASS（使用非生产 dummy token），Docker daemon 不可用，运行态 NOT_RUN。
- `hk-main` 与 `nl-highdefense` 使用 codex-ssh-manager 完成 resolve/probe/audit 及扩展只读审计，均 PASS；未执行 sudo、配置变更、防火墙/路由/DNS/代理操作。扩展审计证据中的服务状态、监听端口和配置路径已脱敏记录于 `.artifacts/acceptance-20260911/results.txt`。
- 该连续验收检查点当时尚缺 NSIS 和新 Mac 构建；两项已在本文顶部记录的后续发布准备阶段补齐。双 NAT/IPv6/网络切换/睡眠唤醒及完整原生交互仍为 NOT_RUN 或 BLOCKED_BY_EXTERNAL_ENV；模拟与 loopback 结果没有替代真实双机验收。

## 2026-09-09 Wails 3 migration and Hong Kong production verification

- Wails 3 CLI and Go module fixed at `v3.0.0-beta.18`; desktop entry/service/window/dialog lifecycle and generated TypeScript bindings now use Wails 3. The old v2 module, `wails.json` and `frontend/wailsjs` outputs were removed. Source snapshot and working-tree patch are retained under `.artifacts/pre-wails3-20260909-*`.
- Wails 3 Windows production build completed from the dirty working tree: `apps/desktop/bin/LinkSend.exe`, SHA256 `0DD18B54495B1320518B0E61903B2E9A155DDC7B902ACE320B564CA81BF5B27E`; `go version -m` reports `github.com/wailsapp/wails/v3 v3.0.0-beta.18`. The process remained alive for 4 seconds in a native WebView2 smoke check; no manual native click acceptance was performed.
- Hong Kong `hk-main` was inspected, backed up and updated using the local SSH credential. Candidate rendezvous SHA256 `53445BE88943FBABDAA7AABFAF98E299AD9780B1084DAA764CF8804E48E5A338` is running at `/opt/linksend-lan-test/rendezvous`; backup `/opt/linksend-lan-test/backups/20260909T133335Z-wails3` was verified with SQLite `PRAGMA integrity_check=ok`. `test_pairing_code="orion123"` and an explicit existing group are enabled; generic configs remain off.
- Real Hong Kong HTTPS/WSS control-plane evidence: fixed-code joins for `hk-win-wails3` and `hk-peer-wails3` returned `admin=true` in group `73cde936…`; a dynamic invitation joined `hk-dynamic-wails3` with `admin=false`; reusing that invitation was rejected. After a controlled server restart, health remained `status=ok`, `relay=false`, `transport=quic`, and the group persisted with 8 members/5 admins/0 revoked.
- Real two-way transfers through Hong Kong signaling and direct QUIC on the physical Windows interface `10.234.232.205`: 1 MiB Windows→peer transfer ID `4773432abda776c76882261f55c0b491`, SHA256 `30E14955…`; 2 MiB peer→Windows transfer ID `cf38b13233b0214c7d309e28bf823d0f`, SHA256 `5647F05E…`. Both reported TLS 1.3 (`772`), ALPN `linksend/1`, `relay=false`, host↔host candidates, and matching received hashes.

## Environment and limits

- Windows amd64, PowerShell 7, Go 1.26.5, Node 22.15.0, pnpm 11.19.0.
- The default PATH exposed MinGW-w64 GCC 8.1, which made Windows race binaries exit with `0xc0000139`. Scoop MinGW 16.2.0 is now installed and selected explicitly for race verification.
- Docker 29.2.1 client is installed, but the Linux engine is unavailable.
- Wails 3.0.0-beta.18 is available at `.tools/bin/wails3.exe`; WebView2 is installed, but macOS runtime validation is not available here.
- This round changed only the LinkSend service binary/config on `hk-main`; no unrelated firewall, DNS, proxy or certificate records were modified. At the time of this deployment snapshot no Git commit/push/reset/clean/stash had been performed; the later candidate commits and push are recorded in the 2026-09-10 section below.

## 2026-09-08 Hong Kong host connectivity preparation

- The reusable `hk-main` SSH alias was resolved and probed successfully (`root@109.66.88.204`, SSH port `60851`); a read-only host audit reported no failed systemd units and 22% root filesystem usage.
- The remote LinkSend rendezvous process is running from `/opt/linksend-lan-test/rendezvous` with `listen = "0.0.0.0:443"`, the configured origin certificate for `linksend.oooai.de`, and `relay=false`/QUIC capabilities. `GET /healthz` returned `status=ok` through both loopback and the public listener.
- coturn is running in STUN-only mode on UDP `3478`; no TURN allocation or relay port range is configured. nftables permits TCP `443` and UDP `3478` and retains a default-drop input policy.
- Windows reachability checks to `109.66.88.204:443` and `109.66.88.204:3478` succeeded. The origin certificate is self-signed but has SANs `linksend.oooai.de` and `*.oooai.de`; normal clients must use the Cloudflare edge hostname and must not disable certificate verification.
- Earlier check (superseded by the domain recheck below): DNS returned NXDOMAIN for both `linksend.oooai.de` and `stun.oooai.de`. Formal Windows/macOS WSS pairing and transfer testing is therefore pending the user's Cloudflare records: proxied `A linksend.oooai.de -> 109.66.88.204` and DNS-only `A stun.oooai.de -> 109.66.88.204`.

## Version control

The repository is managed with Git on `main`, tracks `https://github.com/Wen5555/LinkSend.git`, and has been pushed without force updates. Local identities, databases, build binaries, toolchains and temporary profiles are excluded by `.gitignore`.

## Milestones

| Milestone | Source implemented | Current evidence | Remaining |
|---|---|---|---|
| M0 | Two Go modules, workspace, Wails template, configs, docs, CI and commands | Root tests/vet/race, CLI/devtool, and desktop GOWORK=off checks pass; desktop CI path corrected in source | CI runner confirmation |
| M1 | Pion ICE + quic-go UDP demux, TLS 1.3 pinning, timeout/close paths | Windows loopback host ICE, encrypted bidirectional QUIC, wrong-pin negative tests, benchmark and `demo-local` pass | Real two-host LAN, public IPv6 and controlled dual NAT not run |
| M2 | Ed25519 identity, invite pairing, SQLite group, WSS auth, presence/session routing, revoke and local trust | signaling package tests and demo control plane pass | Reconnect/backoff and multi-session coordinator |
| M3 | Manifest/BLAKE3/secure receiver plus shared direct session API and CLI send/receive | App service test completes paired profiles through WSS, ICE, TLS 1.3, QUIC and transfer; demo transfers 1 MiB with matching content hashes and no file-sized signaling forwarding | Real two-machine LAN, public IPv6 and cross-NAT |
| M4 | Chunk checkpoint/recovery, safe staging, pause/cancel primitives | transfer unit tests and real quic-go loopback cancellation lifecycle tests pass | Process-kill restart acceptance through app/session coordinator |
| M5 | Thin Wails binding to shared identity/devices/diagnostics | Desktop Go source, frontend checks and Windows Wails production build pass | Windows interactive, macOS runtime and task UI |
| M6 | Deployment examples, explicit STUN-only config, diagnostics and benchmark entry points | Compose config parses; STUN-only runtime and HTTPS identity are not run | Docker daemon, HTTPS certificates, STUN Binding/TURN Allocate negative test, NAT lab and package signing |

## Commands run in this milestone

Passed:

```text
gofmt -w internal/signaling/client_test.go internal/app/service.go internal/app/demo.go cmd/linksend/main.go cmd/rendezvous/main.go cmd/devtool/main.go apps/desktop/app.go
go test ./...
go test ./internal/signaling
go run ./cmd/devtool demo-local
```

`demo-local` reported authenticated TLS 1.3, `relay=false`, a direct loopback candidate pair, 1,048,576 file bytes and about 3,171 signaling-forwarded bytes. ICE library close logs mention closed sockets during cleanup; they do not change the result.

Not run or blocked:

- The old GCC 8.1 race attempt exited with `0xc0000139` before tests; it is retained as a toolchain failure, not a race result.
- Real two-machine LAN, cross-NAT srflx, public IPv6, Windows to macOS and Linux namespace NAT: no target topology/privilege available.
- Docker compose and production HTTPS: Docker Linux engine/certificates unavailable.
- Independent desktop module and frontend checks were run below; macOS runtime and interactive desktop acceptance remain pending.

Additional verification completed after the initial update:

- `go vet ./...`: passed.
- `GOWORK=off go test ./...` at the root: passed.
- With Scoop MinGW 16.2.0 selected via `CC`, `CXX` and `PATH`, `go test -race ./...` passed across all root packages and integration tests.
- `apps/desktop` with `GOWORK=off`: `go test ./...` and `go build ./...` passed.
- `apps/desktop/frontend`: `pnpm run typecheck`, `pnpm run lint`, `pnpm run test` (3 tests) and `pnpm run build` passed.
- Historical Wails 2.15.0 production build passed before migration; the current Wails 3 build is recorded in the 2026-09-09 section above.
- After shared direct-session changes, Wails production build was rerun and passed again.
- `go run ./cmd/devtool test-nat` correctly returned nonzero with an explicit Windows/Linux-privilege explanation.
- `go run ./cmd/devtool bench-transport` passed on this Windows host: native 102.18 MB/s, ICE integration 98.87 MB/s for one exploratory iteration; this is loopback evidence only.
- `docker compose -f deploy/compose.yml config` passed with an explicit bootstrap token; the Docker engine itself remains unavailable, so image build/start was not run.
- `internal/app.TestDirectServiceSendAndReceive` passed: two profiles used the shared `ConnectDirect`/`AcceptDirect` API and `SendFiles`/`ReceiveOnce` API for an authenticated transfer.
- `go run ./cmd/devtool check-core` now runs ordinary tests, vet and race tests; it passed with the installed Scoop MinGW 16.2.0 toolchain.

## Stage 1A current scope

- Source changes: coturn 4.6.2 now receives the explicit `stun-only` option; the compose healthcheck is documented as liveness only; desktop CI enters `apps/desktop` and prints `GOMOD/GOWORK`; reserved CLI task commands return `NOT_IMPLEMENTED` without dispatching to unrelated state-changing methods.
- This Stage 1A run must separately record root tests, desktop-module tests, frontend checks and compose parsing. Wails production build, HTTPS certificate validation, coturn runtime and real Windows/macOS LAN/NAT traversal are not implied by those checks. Real quic-go loopback cancellation lifecycle is separately covered by Stage 1B.
- Stage 1A is complete in source and local automated checks; Stage 1B below records the current lifecycle implementation and evidence. Real network acceptance remains separate.

## Stage 1B current scope

- `internal/transport.QUICStream` now keeps quic-go `CancelRead`/`CancelWrite` behind a small transport boundary. Transfer cancellation and protocol failure abort both stream directions; normal completion still uses `Close`.
- Candidate exchange now uses a phase context. The first terminal sender/receiver error cancels its sibling, parent cancellation is classified as `CANCELLED`, and a missing `end_of_candidates` is bounded by a candidate phase budget.
- Signaling establishment, candidate exchange, ICE checks and QUIC handshake retain separate cancellation/timeout boundaries. The existing `CHECK_TIMEOUT` code remains the ICE phase code; signaling and QUIC timeout causes are no longer intentionally collapsed into it.
- Real quic-go loopback tests cover blocked acceptance read, blocked receive read, blocked ACK wait, repeated abort, candidate sibling failure, missing end message, malformed message and parent cancellation. These are lifecycle tests, not LAN/NAT evidence.

## Locked versions

- Go 1.26.5, module minimum 1.26.0.
- Pion ICE v4.4.2, quic-go v0.62.0.
- modernc.org/sqlite v1.58.0, coder/websocket v1.8.15, go-toml/v2 v2.4.3, zeebo/blake3 v0.2.4.
- Wails 3.0.0-beta.18, React 19.2.8, TypeScript 5.9.3, Vite 7.3.6, pnpm 11.19.0.

## Next concrete actions

1. After the two Cloudflare records resolve, run Windows to macOS real two-machine LAN acceptance with the Hong Kong rendezvous service and STUN. Preserve DirectEvidence, transfer hashes and failure-phase output.
2. Stage 2B: run different-network cross-NAT acceptance without relay.
3. Verify IPv6 and Linux interoperability.
4. Add persistent task orchestration, then Wails task UI, after real-network evidence is recorded.

## 2026-09-08 engineering optimization evidence

- Diagnostics now use a separate redacted identity DTO; health failures expose only a stable code/summary. Direct transfers can export read-only `DirectEvidence` from the selected ICE pair, endpoint counters and negotiated TLS/ALPN through the CLI `--evidence` flag.
- Transfer controls use typed operation constants and semantic validation. Accept/ack verified values are bounded, peer error detail is limited to 4 KiB and sanitized, frame limits reject invalid bounds, and progress rates are finite. Resume state parsing is bounded and rejects trailing JSON/unknown file IDs; fresh staging and checkpoint temporary files have best-effort cleanup. Cancellation persists `Cancelled`.
- ICE failures preserve cancellation, timeout, no-candidate and generic ICE failure categories. Signaling/QUIC wrappers retain underlying causes. Trust files are bounded; SQLite rejects unknown schema versions; server expiry cleanup and network closes avoid holding the mutex during websocket close.
- The root module path and desktop module path are now `github.com/Wen5555/LinkSend` and `/apps/desktop`; CI verifies modules with `GOWORK=off` and `go mod verify`. `devtool network-info` reports interface state/address data without credentials.

Verification on Windows amd64: root `gofmt`, `git diff --check`, `go mod verify`, `go test ./...`, `go vet ./...`, `go run ./cmd/devtool check-core` (including race), root and desktop `GOWORK=off` test/build, frontend typecheck/lint/test/build, Wails 3 production build, `demo-local`, and `network-info` passed. A 20x transfer repeat was first interrupted after an erroneous test harness blocked; the corrected transfer package passed its focused run. `test-nat` remains an explicit nonzero not-run result because this Windows host lacks the privileged Linux namespace fixture.

## 2026-09-08 domain recheck

- `linksend.oooai.de` resolves to Cloudflare addresses `172.67.208.195` and `104.21.45.36`; `stun.oooai.de` resolves directly to `109.66.88.204`.
- Windows STUN Binding request to UDP 3478 received an 88-byte success response with matching cookie and transaction ID. This is STUN reachability, not ICE/NAT acceptance.
- Public HTTPS `/healthz` returns Cloudflare 525. The source certificate is self-signed Ed25519; corresponding Cloudflare source connections fail with `tls: peer doesn't support any of the certificate's signature algorithms`. Origin-local HTTPS with the origin certificate explicitly trusted returns healthy capabilities and `relay=false`.
- `--server` must use `https://linksend.oooai.de`; the CLI converts HTTPS to WSS internally. Stage 2A commands have been corrected accordingly.
- No server configuration, certificate, firewall, or service was changed. Public pairing/transfer remains `BLOCKED_BY_EXTERNAL_ENV` until TLS is corrected and public HTTPS/WSS are verified.
- Read-only evidence: hk-main `/tmp/codex-ssh/linksend-domain-inspect-20260908T150418Z`.
## 2026-09-08 Cloudflare origin TLS repair

- Using the user-provided Cloudflare credential, an Origin RSA certificate for `linksend.oooai.de` was issued and installed on `hk-main`.
- Existing `/opt/linksend-lan-test/origin.crt`, `origin.key` and `server-public.toml` were backed up to `/root/linksend-lan-test-backups/20260908T151853Z/` before replacement.
- The rendezvous process was restarted with the same configuration. Public `https://linksend.oooai.de/healthz` now returns `status=ok`, `protocol_version=1`, `transport=quic`, `relay=false`.
- STUN remains available at `stun:stun.oooai.de:3478` (DNS-only). The certificate is valid for 7 days and must be renewed before `2026-09-15 15:12 UTC`.
- Verification job logs are retained at `/tmp/codex-ssh/linksend-origin-cert-rotate-20260908T151839Z`; rollback is restoring the recorded backup files and restarting the same rendezvous command.

## 2026-09-09 Stage 2A result

- Real Windows↔macOS LAN transfer passed. A 16 MiB random fixture completed through authenticated QUIC with TLS 1.3, `relay=false`, host candidates `10.234.49.250:62995` and `10.234.232.205:62109`, and 1,680 bytes of observed STUN traffic.
- Receiver and sender SHA256 values matched. Full evidence is recorded in `docs/STAGE2A-20260909.md`; sender output is under `.stage2a/20260909-000044/` and is ignored by Git.
- This closes the Stage 2A same-LAN evidence item. Cross-NAT, IPv6, different-network and desktop UI acceptance remain separate.

## 2026-09-09 Stage 2B readiness

- 加固两端 Stage 2A 入口：SkipBuild 复用并校验显式二进制及 SHA256；脚本固定仓库根目录并限制 role。
- CLI 在 --evidence 模式下对失败也输出结构化结果，stderr 与 result.json 分离；接收端新增独立 --wait-timeout。
- 2B-01 跨 NAT、Linux 双 NAT fixture、IPv6、网络切换和桌面 UI 均未运行，保持待验收。详见 docs/STAGE2B.md。

## 2026-09-09 LAN reliability pass

- 当前分支 main，HEAD e2129b5，工作树含未提交 Stage 2B readiness 加固及本轮文档。
- 本轮实际运行：linksend help/receive help、internal/transfer、internal/app、tests/integration、devtool network-info；Go 测试与网络信息检查通过。用户此前报告的 gofmt/go test/go vet/git diff --check 单独标注为既有证据。
- Windows 物理 LAN 候选为以太网 10.234.232.205/16；198.18.0.1/30 Mihomo 虚拟接口排除。macOS 双向实机重复待现场执行。详见 docs/LAN-VALIDATION.md。

## 2026-09-09 任务接口与 Wails UI

- `internal/app/tasks.go` 新增进程内任务编排：异步发送、接收等待、任务快照、单活动任务忙碌保护、取消请求与确认终态、接收确认/拒绝、失败发送重新发送。任务只存当前进程，未宣称重启恢复；原始失败任务保留，重试生成新本地 ID。
- `internal/app/direct.go` 增加本地阶段观察回调，继续复用既有 `SendFilesDetailed`/`ReceiveOnceDetailed` 真实 ICE、TLS、QUIC 和安全落盘路径。
- `apps/desktop/app.go` 增加薄 Wails 绑定、原生文件/目录选择器和环境变量驱动的 bind/STUN 配置；`frontend/src/App.tsx` 提供传输、设备、设置/诊断三个区域，任务操作连接真实后端快照。
- 自动化：`go test ./...`、`go vet ./...`、前端 `typecheck`/`lint`/`test`/`build` 通过；本轮 Wails 交互运行和 Windows/macOS 新版本双机验收尚未运行。
- 暂缓：跨 NAT、网络迁移、进程重启恢复、持久化历史、并发队列、复杂桌面交互和安装签名。

## 2026-09-09 Windows 初版产品化

- `apps/desktop/app.go` 增加本地偏好原子保存（服务地址、绑定地址、STUN、设备名、接收目录）、真实网卡枚举、加入设备组、完整指纹信任和安全打开接收目录的 Wails 桥接；环境变量仍优先覆盖本地设置，既有身份目录不变。
- `frontend/src/App.tsx` 重做为传输、设备、设置与诊断三页应用壳：发送端只显示等待确认，接收端才显示接受/拒绝；任务轮询与远端状态刷新分离；浏览器桥未注入时显示明确不可用提示。
- `frontend/src/App.css` 使用浅色语义 tokens、侧栏导航、状态条、分组表面、可见焦点和减少动效规则，完成 Playwright 传输/设备/设置三页截图检查。
- 自动化通过：桌面 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...`；前端 `typecheck`、`lint`、`test`、`build`；Wails 3 production build；`git diff --check`。
- 产物位于 `.artifacts/windows-preview/20260909-0405/`，含 `LinkSend.exe`、`BUILD-INFO.json`、`SHA256SUMS.txt`、中文启动说明、验证记录和浏览器渲染截图；ZIP 位于 `.artifacts/windows-preview/LinkSend-windows-preview-20260909-0405.zip`。
- Wails 原生窗口点击与截图、跨 NAT/IPv6、网络迁移和重启恢复仍未运行，均保持 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV` 边界。
- Windows 启动冒烟：运行 `apps/desktop/build/bin/LinkSend.exe` 后进程保持运行超过 3 秒并由本次检查结束；未进行人工点击和双机传输。

## 2026-09-09 Windows 初版复核修补

- 接收任务快照现在保留已认证 `peer_id`、manifest 内容摘要和条目数量；接收确认信息可在任务区核对发送者、内容和总量。
- `OpenTaskDirectory` 改为按已完成接收任务 ID 校验目录，避免前端传入任意本地路径；桌面接收等待时限固定为 10 分钟，ICE 检查时限为 30 秒。
- 前端任务/远端轮询加入 in-flight 防重叠；操作调用增加 pending 门闩；完成接收任务提供打开目录操作。
- 复核构建：根模块测试/桌面测试与 vet、前端 typecheck/lint/test/build、Wails 3 production build 均通过；交付目录已更新为最后一次构建的二进制和 SHA256。

## 2026-09-09 401 配对错误修复

- 通过当前桌面 profile 的真实身份与香港主站数据库只读比对确认：身份未加入设备组，服务器返回 401 是成员校验拒绝，不是签名算法或 TLS 故障。
- 前端新增后端错误人性化映射：区分“尚未加入设备组”“已加入但无管理员权限”“邀请无效/过期/已使用”，避免把安全拒绝显示成原始 HTTP 错误。
- 新增对应前端回归测试；隔离 profile、密钥和远程检查脚本均已清理。当前身份仍需由现有管理员设备生成一次性邀请后才能加入，客户端不会自动 bootstrap 或绕过管理员权限。

## 2026-09-09 隔离端到端与远端健康复核

- 在独立临时 rendezvous（`127.0.0.1:18887`、独立 SQLite/profile）上完成两个随机身份的 bootstrap、邀请、加入、双向完整指纹信任和 16 MiB 随机文件传输；发送端与接收端独立 SHA256 一致，TLS 1.3、QUIC、`relay=false`。临时 profile、密钥、数据库和大文件已在测试后清理，仅保留本记录摘要。
- 香港主站 `hk-main` 只读探测通过；`https://linksend.oooai.de/healthz` 返回 `status=ok`、`protocol_version=1`、`relay=false`、`transport=quic`；远端 443 由 rendezvous 监听，3478 由 coturn STUN-only 监听。
- 该隔离测试证明 CLI/核心真实链路，不等价于物理 LAN、跨 NAT 或 Wails 原生窗口验收。
- 生产 EXE 使用隔离 `LINKSEND_DATA_DIR` 启动冒烟保持运行超过 3 秒；WebView2 Runtime 152.0.4191.66 已安装。当前会话未进行人工窗口点击，桌面交互仍待现场验收。

## 2026-09-09 Windows Preview 首次配对与退出保护补齐

- `apps/desktop/app.go` 现在区分偏好不存在、有效、损坏、不支持版本和不可读状态；损坏/不支持文件原地保留，远端操作在用户保存修复前被阻止。新增 `PreferencesStatus`、`EffectiveConfig`，明确环境变量 > 已保存偏好 > 默认值及需重启生效边界。
- `internal/app/service.go` 新增基于已认证设备列表的成员/角色状态：`member/admin`、`not_member`、`auth_failed`、`unavailable` 分开表达；合并 401 不再被猜测为确定未入组。前端管理员邀请按钮按服务端角色禁用并解释原因，普通新设备主入口为粘贴邀请。
- `apps/desktop/main.go` 注册 Wails 3 `ShouldQuit`。存在准备、等待确认、连接、传输、校验或取消中的任务时，原生窗口提供继续任务/取消任务并退出；取消清理有界等待，超时保持窗口打开，重复关闭幂等。
- 新增桌面偏好恢复回归测试；根模块、桌面模块（`GOWORK=off`）、前端和 Wails Windows/amd64 production build 均通过，`git diff --check` 通过。
- 最终本地产物：`.artifacts/windows-preview/20260909-1312/` 及对应 ZIP。原生窗口人工点击、原生截图、真实主站管理员邀请和物理双机本轮仍为 `NOT_RUN`/`BLOCKED`，未以浏览器或隔离组结果替代。

## 2026-09-09 配对权限模型调整

- 根据产品决策，邀请生成不再要求管理员角色；任一已配对且未撤销设备都可以生成 32 字节高熵、10 分钟有效、一次性配对码。保留服务端签名认证、设备组隔离和逐对端完整指纹信任。
- 首次建立空设备组仍需使用服务端 bootstrap 入口；本轮没有把服务改成匿名开放注册。
- 更新服务端邀请授权、客户端错误文案、桌面按钮和 signaling 回归测试。管理员字段继续作为服务端事实展示，但不再作为生成配对码的前置条件。

## 2026-09-09 可用性与本地配对改进

- 新增 `docs/IMPROVEMENT-PROMPT.md`，记录全项目检查、配对边界、UI 视觉和验证要求，可作为后续迭代的固定改进提示词。
- `internal/server.Config.TestPairingCode` 默认关闭；本轮香港授权配置可在显式目标组上启用固定码 `orion123`，开发服务仍提供同一固定码。服务端仍要求 Ed25519 注册签名，生产高熵一次性邀请路径不变。
- `internal/store.JoinTestCode` 使用独立兼容路径加入明确设备组并授予管理员，并添加回归测试；公共配置缺少 `test_pairing_group` 时拒绝固定码配置。
- 桌面设备页展示“仅测试使用、未来可能关闭”的固定配对码并支持复制；输入框提示可直接输入 `orion123`。
- `apps/desktop/frontend/src/App.css` 增加 Apple 风格视觉层：系统字体、克制蓝色强调、半透明分组表面、统一圆角与焦点状态、窄屏导航折叠和减少动效规则。
- 本轮验证：根模块 `go test ./...` 通过；桌面模块 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...` 通过；前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（4 tests）、`pnpm run build` 通过；`git diff --check` 通过。
- 未运行：Wails 原生窗口人工点击、Windows↔macOS 新版 UI 验收、跨 NAT/IPv6/网络切换。上述项目仍保持 `NOT_RUN` 边界。

## 2026-09-09 全面产品改进复核

- 增量实现：任务层把接收拒绝、源文件变化、完整性失败、危险路径、磁盘空间/权限和取消映射为稳定协议错误码；Wails 任务 DTO 与 CLI 失败出口只展示中文操作建议，底层 HTTP、文件系统和栈信息保留为内部 cause，不直接进入用户界面。
- 成员资格把信令不可达、版本不兼容和身份拒绝分开；任一已加入且未撤销设备可生成一次性邀请的前端文案与服务端权限保持一致。
- 固定配对码对同一 profile 重试为幂等并授予管理员；生产 `Join` 的高熵、10 分钟、一次性语义未改变，静态入口通过显式目标组控制。
- 直连完成后任务阶段显式进入 `connected`，界面把成员资格、对端 presence、指纹信任和已建立直连分别呈现。新增 favicon 清除浏览器预览的无意义 404。
- 当前 Windows/PowerShell 环境实际通过：`gofmt -l .`、`git diff --check`、根 `go test ./...`、`go vet ./...`、根 `GOWORK=off go test ./...`、根普通及 `GOWORK=off go test -race ./...`、桌面 `GOWORK=off go test/go vet/go build ./...`、前端 typecheck/lint/4 tests/build、Wails 3 production build、`demo-local` 和 Compose 配置解析。Wails 3 EXE 隔离启动后保持运行超过 4 秒，再由本轮检查结束。
- 浏览器行为/视觉检查：实际打开传输、设备、设置三页，并在 700px 窄屏检查导航折叠、单列布局、表单宽度、禁用态和复制入口；这是 WebView 内容渲染证据，不等于 Wails 原生窗口点击验收。
- `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`：本轮未运行新的 Windows↔macOS 双机传输、跨 NAT、IPv6、网络切换、睡眠唤醒、macOS Wails、Windows 原生窗口人工点击、Docker 引擎运行或安装签名。`devtool test-nat` 在 Windows 上按设计非零退出并说明需要 Linux 特权 namespace fixture。
- 下一步：在两端现场执行 Stage 2B 跨 NAT 与新版原生 UI 工作流；之后实现持久任务历史、真正的应用重启恢复和暂停/恢复编排。

## 2026-09-10 Wails 3 候选打包与交付核验

- 任务分支为 `codex/wails3-hk-dmg-20260909`，当前候选 commit 为
  `555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`；本轮提交了 DMG 资源、Windows/Mac 测试说明，
  未修改 `main`、未合并、未发布正式 Release。源码快照仍保留在 `.artifacts/pre-wails3-20260909-*`。
- GitHub Actions run [34387753173](https://github.com/Wen5555/LinkSend/actions/runs/34387753173) 的
  `windows-amd64`、`macos-arm64`、`macos-amd64` 均 `success`。Windows ZIP 和两个 DMG 已下载到
  `.artifacts/ci-34387753173/` 并通过 ZIP 完整性、包内 `SHA256SUMS.txt`、DMG HFS+ 目录、
  `Info.plist`、`CFBundleIdentifier`、Go `GOARCH/GOOS` 与 Wails 依赖核验。
- 最终包 SHA256：Windows ZIP
  `1c43b7576118c5480d139bd691a1c84b8c3d09f67d96f8815c1877c4055f8f80`；macOS arm64 DMG
  `5ff0217fab3b257999eae710f29bc76f08979b37100b2f1b4347546a6d3645a2`；macOS amd64 DMG
  `653c2ceb098c5d81bc4c6f0a351cb20b446772c206888463aec7b34efff73819`。DMG 为内部测试包，
  仅 ad-hoc 签名，不含 Developer ID 或公证。
- Windows ZIP 现在包含真实 `LinkSend.exe` 与 `README-WINDOWS-TEST.txt`；DMG 包含完整
  `LinkSend.app`、`README-MACOS-TEST.txt` 和 Applications 拖拽入口。离线 `go version -m` 确认
  Go 1.26.5、`GOOS=darwin`、`GOARCH=arm64/amd64` 及 Wails `v3.0.0-beta.18`。
- 当前主机实际完成了 Windows 与香港服务的既有真实双向 QUIC 证据；没有新增 Mac 原生窗口、DMG
  挂载、红点/Cmd+Q、跨 NAT、IPv6 或网络切换验收，均标记 `NOT_RUN`/`BLOCKED_BY_EXTERNAL_ENV`。

## 2026-09-11 任务诊断文案与阶段可读性补丁

- 前端任务列表将内部阶段枚举映射为中文可操作文案（发现网络地址、交换连接信息、检查直连路径、建立安全直连、等待接收确认、恢复传输等）；未知未来阶段仍原样保留，便于诊断。
- `humanizeBackendError` 补齐稳定连接、传输、任务操作和桌面边界错误码（包括 `NO_CANDIDATES`、`CHECK_TIMEOUT`、`DISK_FULL`、`UNSAFE_PATH`、`RELAY_NOT_IMPLEMENTED`、`TASK_NOT_RETRYABLE` 等），避免将原始内部错误直接展示给用户。
- 修复测试配对面板 CSS 伪元素重复显示“仅测试使用”文案，并补充阶段与错误码前端回归测试。
- 实际验证：前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（6 tests）、`pnpm run build`；根模块 `gofmt -l .`、`git diff --check`、`go vet ./...`、`GOWORK=off go test ./...`；桌面模块 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`；Wails `v3.0.0-beta.18` production Windows build 均通过。
- `go run ./cmd/devtool demo-local` 实际完成 loopback TLS 1.3/QUIC 文件传输与摘要一致性；该结果仅证明本机链路。Windows↔macOS、跨 NAT、IPv6、网络切换、睡眠唤醒和原生窗口人工点击仍为 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`。
- Wails 最新构建产物 `apps/desktop/bin/LinkSend.exe` 隔离数据目录启动冒烟保持运行超过 4 秒后结束；未进行原生窗口人工点击。

## 2026-09-11 直连路径证据贯通任务界面

- `DirectConfig` 新增本地 evidence 回调，连接成功后把实际 `connection_method`、`transport_protocol` 和 `relay` 结果写入任务快照；未改变协议、ICE、QUIC 或文件数据路径。
- Wails/React 任务列表显示已验证的“局域网直连 / 互联网 P2P / 路径待确认”，避免把在线状态或 host candidate 当作局域网证据。
- 实际验证：根与桌面模块测试、桌面 vet/build、前端 typecheck/lint/6 tests/build、绑定重新生成和 `git diff --check` 均通过。跨设备、跨 NAT、IPv6、网络切换和原生窗口仍未运行。

## 2026-09-11 任务历史与候选构建复核

- `internal/app` 增加 `task-history.sqlite`（SQLite `user_version=1`，含 `schema_version` 等价版本元数据）持久化；启动时损坏记录被隔离，不阻塞应用；未完成任务在重启后标记为 `failed/TASK_INTERRUPTED`，清除旧会话证据并要求用户核对后重新发起；块检查点仍仅由 transfer 层使用，桌面级 byte resume 尚未实现。快照新增单调 `revision`、`history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 与暂停/恢复能力字段。
- 根模块 `go test ./...`、`go test -race ./...`、桌面模块 `GOWORK=off go test/go vet/go build ./...`、前端 frozen install/typecheck/lint/Vitest 6 项/production build 均通过；新增历史持久化和损坏记录回归测试。
- 提交 `9b72334d0fbe5e18649424924e96cf88756c6845` 已推送分支 `codex/wails3-hk-dmg-20260909`。Actions run [34515881198](https://github.com/Wen5555/LinkSend/actions/runs/34513870167) 的 Windows amd64、macOS arm64、macOS amd64 三平台均 success，产物与该提交对应。
- 本机可交付 Windows 便携 ZIP：`apps/desktop/bin/LinkSend-windows-amd64-0.1.0-preview.zip`，包含 EXE、测试说明、BUILD-INFO 和包内 SHA256。NSIS/Wails CLI 未安装，安装器未生成；macOS DMG 仅通过 CI 产出，未在本机挂载或进行原生窗口点击验收。

## 2026-09-11 全面网络提示词执行记录

- 香港主站 `hk-main` 经 codex-ssh-manager 完成只读 `resolve/probe/audit-host`：root、Debian 6.1.0-50-cloud-amd64、x86_64，根分区余量约 78%，`rebootRequired=false`；未修改生产配置。
- 荷兰 VPS `nl-highdefense` 的 SSH 管理器探测在本轮超时，未取得 STUN/TURN 或 NAT 实验运行证据，标记 `BLOCKED_BY_EXTERNAL_ENV`，未执行 sudo 或网络配置变更。
- `go run ./cmd/devtool demo-local` 退出码 0：真实 ICE host candidate、QUIC TLS 1.3、摘要一致（1 MiB），仅作为 loopback 证据，不能替代双机 LAN。
- `go run ./cmd/devtool test-nat` 退出码 1，输出明确为 Windows 缺少受控 Linux namespace/NAT fixture；标记 `BLOCKED_BY_EXTERNAL_ENV`，没有伪造 NAT 成功。
- GitHub Actions run [34523034011](https://github.com/Wen5555/LinkSend/actions/runs/34523034011) 对主分支提交 `a9646f21c3d088036ade6d7f0a44cd0cb4a75c9a` 的 Windows amd64、macOS arm64、macOS amd64 jobs 均 `success`；对应构建附件可从该 run 下载。尚未替换旧候选 release `v0.1.0-preview-e27`（其目标提交不是当前主分支），避免上传不匹配资产。

## 2026-09-11 Windows ↔ Mac 实机联调与修复（阶段性历史记录）

- 本节保留暂停/恢复实现之前的现场事实；当前实现和构建状态以本文顶部“P0/P1/P2 分阶段可靠性改进”为准，不应把本节当作最终能力声明。环境：Windows 物理以太网 `10.234.232.205/16` ↔ Mac 物理 Wi-Fi `10.234.241.3/16`；两端原有 TUN 保持启用。SSH 全部使用 codex-ssh-manager 的 `mac-test-102342413`。完整状态、命令和限制见 [本轮验收与复测步骤](MAC-WINDOWS-VALIDATION-20260911.md)。
- `IMPLEMENTED / PASS`：真实双向 QUIC/TLS 1.3 传输空文件、中文文件、多文件、12 MiB（三 chunk）文件及中文空目录，接收文件 SHA256 匹配；Mac 真实拒绝、权限拒绝、冲突、独立 64 MiB 卷磁盘不足均保留稳定错误分类，权限恢复和实验卷卸载已核验。
- `IMPLEMENTED / PASS`：修复候选诊断泄露 ICE ufrag；修复 QUIC 终态响应因立即 reset 而丢失；冲突/权限/磁盘错误在接收端、发送端、CLI/任务 DTO 和前端一致；移除仅凭 srflx 推断互联网路径的逻辑。前端分别显示历史持久化、重启恢复和字节级续传，移除常驻固定测试配对码。
- 信令协商清理为 `IMPLEMENTED / PASS`（本地/原生 Go 回归），香港现有旧服务短时间重试仍为 `FAIL`。本轮没有部署生产服务；不能以源码修复替代线上验收。
- `IMPLEMENTED / PASS`：Windows 根模块普通/race/vet/独立模块测试，桌面独立模块 test/vet/build，前端 frozen install/typecheck/lint/7 tests/build；Mac Go 1.26.5 根模块 test/race、桌面 test/vet；两端 Wails beta.18 bindings 及主要前端 JS SHA256 一致。
- 原生构建：Windows production EXE、Mac arm64/amd64 DMG 均实际构建；Mac `hdiutil`、`plutil`、`file`、`lipo`、`codesign` 校验通过。隔离 arm64 应用由 CoreGraphics 观察到 1 个真实窗口，未进行原生控件操作。修复 Windows 源码拷贝到 Mac 后的 CRLF shell/HTML 问题，加入 `.gitattributes`。
- 本轮包标记 `UNCOMMITTED_TEST_SNAPSHOT`，基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`，不冒充该提交的干净构建。Windows 为 `UNSIGNED_TEST_BUILD`；Mac `code_signature=ADHOC / distribution_identity=NONE / notarization=NOT_RUN`。本轮无新 PR/合并/Release。
- 该现场阶段当时 `NOT_IMPLEMENTED`：桌面重启恢复、字节级续传、桌面暂停/恢复；这些能力随后已在本文顶部所述源码快照中实现并通过本地真实 QUIC 回归，但尚未做新的物理双机恢复验收。中继仍为 `NOT_IMPLEMENTED`；真实双 NAT、Linux 双机、完整网卡矩阵、原生 UI 缺口仍见验收报告。
- 历史交付勘误：较早的 Windows 便携包曾缺包内 SHA256/准确合并提交来源信息，不能视为满足本轮发布要求；本轮 ZIP 单独记录未提交来源并包含包内校验和。默认 Windows 身份数据目录通常为 `%APPDATA%\LinkSend`（`os.UserConfigDir()`），不是 `%LOCALAPPDATA%`。
## 2026-09-14 E4-03 集中修复候选

- 所有真实本机变化先推进剪贴板状态owner；无lease、无发送权限、格式不支持和离线变化均不补发。每peer固定一个worker，只保留current和一个latest-only pending；等待两路全局slot、开流和64 KiB分块写都继承lease deadline。新复制、暂停、撤权及Shutdown主动cancel/reset旧流；Ensure按peer并行并以singleflight阻止周期调用在离线peer后积累。
- 连接协商增加独立`clipboard_sync`；只声明旧`session_reuse`不会启动剪贴板owner。双方各自本地authorization generation可不同，真实loopback用例以1/29非对称代次验证双向文字。origin绑定认证peer，lease/event绑定双方permission revision；关闭/重开同一权限后旧lease/正文拒绝，无关方向不删除健康lease。
- 图片解码/验证移出最终state锁；mutation前按grant→state锁序复核暂停、peer generation、receive revision、deadline和OS generation。macOS普通文字不误判link，命名pasteboard的文字、URL、PNG和concealed marker已在arm64通过；Windows读取明确exclude marker。Windows PNG跨应用粘贴仍按计划留E5。
- 设置新增独立自动剪贴板分类、持久且默认关闭的总开关、读取/远端写入说明、临时暂停，以及ready/connecting/unsupported/pause/error状态。后端只在内存中保留每peer最新有限等待/错误码，真实远端原生写入失败可见，下一次成功清除，不记录正文或历史。Wails bindings已重新生成。
- Windows根完整test/vet/GOWORK=off test、定向race、桌面独立race/vet/build、前端typecheck/lint/66 tests/build通过。Mac最终作业`/tmp/codex-ssh/linksend-e4-ab-mac-final-20260914T002102Z`的根定向测试、nativeclipboard race、desktop完整test/vet通过；保留既有SDK 26/deployment target warning。物理Win↔Mac自动剪贴板、准确包锁屏/睡眠/网络切换仍留E5，未恢复E5部署。

## 2026-09-14 E4-01 认证 QUIC 文件会话复用候选

- peer ID + authorization generation 绑定的短期池复用同一认证 QUIC；两端都有入站 stream owner，连续正向和原始响应方反向文件保持同一 session_id。
- 每 peer 仍限制一个活动文件发送流；传输中取消只终止当前 stream，下一文件可在同一连接完成。授权撤销、连接错误、3 秒空闲、空闲网络变化与 Shutdown 关闭池中连接；活动连接由真实 path watcher 决定。
- C2 将 closing 判定、pool map 发布和 worker `WaitGroup.Add` 收敛到同一全局门闩；dial/adopt 在发布前遇到 Shutdown 会关闭连接且不留下 profile owner。远端 stream reset、接收拒绝只结束当前流；未协商复用的旧端恢复单操作 `CloseAfterTerminal` 收尾并在下一文件新建会话。

## 2026-09-14 E4-02 自动剪贴板权限候选

- 任务库升级 schema 8；权限按 peer、send/receive 与 text/link/image 分离保存并使用 revision CAS，默认无记录即关闭。
- 启用前重验当前 peer 授权；本机阻止、成员移除或授权 generation 变化删除该 peer 全部剪贴板授权，解除阻止和重新配对不会自动恢复。
- 当前候选只建立本机权限和原生变化监听边界；正文读取、lease wire、跨端同步与接收写入仍待 E4-03。
- Windows 使用 `AddClipboardFormatListener` + `SetWindowSubclass`，回调只合并 sequence 和 text/link/image 格式标志；macOS 每 250ms 比较 `NSPasteboard.changeCount` 并只查询 types。仅存在 enabled grant 时注册，全部关闭、撤销、睡眠、锁屏或退出时停止；唤醒/解锁从新基线恢复，不补发停用期间变化。
- C1 将 grant 升级到 schema 9 并绑定当前可信配对 generation；启用提交与撤销共用门闩，清理失败或重新配对都不能复活旧 generation。watcher 由单一 owner 串行 start/stop，分别跟踪睡眠、锁屏和用户暂停；Windows 注册时记录 OS sequence 基线并以 latest-only 合并变化。
- C2 将锁屏/睡眠状态送入独立可靠队列，不受阻塞的 NetworkChanged 或网络 burst 推迟；保留事件顺序并独立合并网络通知。owner 在 Shutdown 先关注册门闩，迟到 RPC/workspace refresh 不能重启 watcher；并发 refresh 只注册一次。
## 2026-09-14 E5 准确候选与 Mac 启动/恢复

- E4 自动剪贴板源码范围已由总控在 `3363fd5+387b57c` 验收通过，E5 固定候选为 `387b57c76596975d0da61f01e060e0cc2940b0b5`。core push/PR、desktop push/PR 和 packages run `34792565786` 的 Windows amd64、macOS arm64、macOS amd64 全部 `success`；PR #8 保持 Draft。
- 三个平台 artifact 已按 GitHub 外层 digest 下载和展开，仓库包核验脚本退出 0；包内 `BUILD-INFO.txt` 均记录 committed/clean 的准确提交，实际载荷 SHA256 与 `SHA256SUMS.txt` 一致。Windows portable ZIP 为 9,826,428 bytes / `a9bb4ae0...e0e7`，installer 为 75,023,277 bytes / `c8b25dd1...cdd8`；arm64 DMG 为 9,443,902 bytes / `be69d1f1...f810`，amd64 DMG 为 10,176,906 bytes / `60fd64f7...581d`。
- `mac-test-102342413` 经 codex-ssh-manager resolve→probe→audit 后，在隔离 `/tmp/codex-ssh` 路径验证准确 arm64 DMG：布局、arm64、bundle ID、macOS 13 最低版本、ad-hoc codesign 严格校验通过；独立 profile 首次和二次启动均由 CoreGraphics 观察到一个真实 1097×745 窗口，进程 6 秒后存活并接受 SIGTERM。完成 jobs 为 `/tmp/codex-ssh/linksend-e5-mac-package-387b57c-20260914T004708Z` 和 `/tmp/codex-ssh/linksend-e5-mac-window-restore-r2-20260914T004932Z`。
- Windows 准确 portable 包在独立 profile 首次与二次启动均创建 1120×760 原生窗口、1105×721 客户区，当前物理显示为 120 DPI（125%），进程 6 秒后存活。首次关闭原生选择“退出应用”后保存 `close_mode=exit` 并 exit 0；同 profile 再启动后关闭不再询问并 exit 0。5 次顺序启动窗口就绪中位数 230.5 ms，3 秒 working set/private bytes 中位数 47,894,528/62,156,800，均 exit 0；150% DPI、键盘和深色待补。
- 私有 Windows window station 原型未触碰日常板：命名 station 被当前令牌以 Win32 5 拒绝；匿名 station 可创建 station/desktop/child，但子进程 user32.dll 初始化失败，现有 nativeclipboard 写读测试无法创建 HWND。general clipboard sequence 始终为 3882。准确系统剪贴板改由独立 Windows 用户会话/Windows Sandbox，或经一次明确授权临时覆盖两端系统板。
- `hk-main` 已部署干净 `387b57c` Linux amd64 候选 `2c9c352a...6372`：事务 job `...014829Z`、备份 `/opt/linksend-lan-test/backups/20260914T014842.005473007Z-desktop-e5-387b57c`、独立 verify `...014925Z` 均 PASS；新 PID 553926，DB integrity `ok`、schema 2→4、V1/QUIC/`relay=false`。旧 M5 二进制在 schema 4 隔离副本上 exit 2，因此旧二进制回滚入口已作废；线上未试回滚，后续只用经 schema 4 副本验证的二进制向前恢复，绝不以旧 schema 2 DB 覆盖新写入。
- `nl-highdefense` 在两个独立 NAT、重叠私网和独立 WAN 上完成 8 MiB 准确 `387b57c` CLI QUIC：同 session、摘要一致、`relay=false`、NAT 计数 8/6。run `...022524Z` 和独立 verify `...022658Z` PASS；本轮所有权门闩保证重入不删旧 namespace。敏感材料与 namespace/veth/process 已清理，宿主地址/路由/规则不变；脱敏包 SHA256 `4be226d1...b86b`。
- Windows 125% DPI 原生小窗请求 960×640 后受最小高度约束为 960×700，继续请求 600×400 时钳制为 900×700，对应 720×560 逻辑最小值乘 1.25；进程 exit 0。延迟 Raw UIA 可读取 production DOM，39 字中文设备名 IsOffscreen=false 且边界留在窗口内，传输按钮声明 keyboard-focusable；但 PostMessage/真实键盘事件后的 FocusedElement 仍为空；只读核对发现 active input desktop 为 `Screen-saver`，而测试线程在 `Default`，因此准确包键盘属于环境待复测。当前系统 light theme 未改，150%/深色 NOT RUN。
- `387b57c` 与 M5 `d0c4a4b` 的隔离 loopback 控制面做了两个方向：旧 client 加入当前 server 返回 `VERSION_INCOMPATIBLE`，准确候选当前 client 加入旧 server 原先返回 `AUTHENTICATION_FAILED`；两端管理视图都只保留原管理员，没有幽灵成员。后续源码修复让 `Join`/跨组切换先核对 `/healthz` 的 membership v2，桌面 `PairDevice` 对真实旧 M5 server 进程稳定返回 `VERSION_INCOMPATIBLE: server capabilities incompatible`，前端明确提示升级服务端；信令、桌面边界和前端测试通过。该修复尚未进入 `387b57c` 准确包，重新固定候选和构建包前不能改写原包结果；混合物理文件仍未运行。
- 香港升级后，全新 Windows 隔离 profile 已用当前固定测试配对入口加入 schema 4 控制面，准确 Windows 包运行时有 1 个 listen、2 个 established 连接。旧 LAN-only profile 上的准确包 V2 share journal 只推进到 `waiting_peer`，不算文件网络通过。Mac alias 随后连续 SSH timeout，尚未创建配套新 profile；恢复后继续准确包 Win↔Mac 文件。
- 物理双向文件、文本/链接/图片自动剪贴板、文件并存、网络切换/睡眠仍未闭合。Windows general clipboard 当前非空，fixture 返回 `CLIPBOARD_TEST_REQUIRES_EMPTY_DISPOSABLE_CLIPBOARD`；未读取、清空或覆盖用户剪贴板。Windows Sandbox 可执行文件不存在，未启用系统功能。
- 2026-09-14 暂停收尾：兼容诊断修复固定为 `1fa21b4073e12d40e87dc6e23f3ec4d67e5e1819` 并推送，`core` push/PR、`desktop-wails3-checks` push/PR 与 packages run `34802741572` 三平台 job 均 `success`，Draft PR #8 head 一致。GitHub artifact ID/外层摘要已记录；三平台下载在用户要求暂停时主动停止，监督目标目录为空，无部分包可被误用。恢复时从 run `34802741572` 下载至 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-ci-34802741572` 并运行 `scripts/verify-milestone-packages.ps1`；在此之前，包内载荷摘要与实机结果仍只属于 `387b57c`。

## 2026-09-19 E5-A 0c59 准确包双向文件

- 已发布预览 `0c59ae255d631b496a3e483e9c558f2998ee314c` 的 Windows amd64 ZIP payload（`97b3a63a...16e0e3e`）与 macOS arm64 DMG（`ce89b37f...460ee5`）在全新隔离 profile 加入当前 schema 4 / membership-v2 测试组；Mac alias 按受管 SSH 更新为 `10.234.35.5` 后 probe/audit PASS。
- Windows 包经生产 V2 `windows_share` journal 自身消费、队列和认证 QUIC 向 Mac 包发送 1 MiB；源与接收 SHA256 均为 `2c7c48b84ae0b706befdd874f058bde9b86d2cf78404c3aee7ddd92f244dd443`。任务完成，`lan_direct`、QUIC、TLS 1.3、ALPN `linksend/1`、`relay=false`，Windows base `10.234.16.254:58074` 与 Mac host `10.234.35.5:62194`。
- Mac 包连续消费两条 V2 `macos_share` journal 并向 Windows 包发送两份 1 MiB；`44abae6d...c6316` 与 `d5ca2258...d0f29` 各自和接收端一致。两条 Windows 接收 task 都为 completed、同一 session `0041445e4fe2bde11d8a0cd2b52db019`、同一 Windows base `10.234.16.254:51996`、同一 ICE generation，构成实际反向认证 QUIC 复用；正向 session 因超过 3 秒空闲边界而不同，未声称跨方向复用。
- Journal 是对已接受 V2 入口的受控测试 fixture；准确包是唯一 core/SQLite queue owner 和网络发送/接收者。它不替代 Windows ShareTarget、macOS Share Extension/App Group 的系统激活验收，相关签名与系统安装仍按用户暂缓。未读取、备份、清空或覆盖用户系统剪贴板；网切/睡眠、自动剪贴板、150%/深色/键盘仍待 E5。


## 2026-09-19 E5-B 0c59 双端系统剪贴板最小尝试

- 用户一次性授权的临时双端覆盖窗口为 `2026-09-19T14:50:18.3854890Z` 至 `2026-09-19T15:04:25.5588969Z`。两端隔离 profile 的 `clipboard_enabled` 与该测试 peer 的 send/receive × text/link/image 六项 grant 曾由不接触 native clipboard 的 helper 开启；Windows `System.Windows.Forms.Clipboard` 与 macOS `NSPasteboard.general` 均先写入、回读了各自已知预置文本。没有读取、导出、备份、恢复或检查写入前的日常剪贴板内容。
- Windows 准确包启动后以 `System.Windows.Forms.Clipboard` 写入并回读 `E5B-WIN-TEXT-20260919T1501Z`（`2026-09-19T15:00:57.1855645Z`）。Mac 对该值执行标准 `NSPasteboard.general` 断言时，受管 job `/tmp/codex-ssh/linksend-e5b-assert-mac-text-0c59-20260919T150130Z` exit 1，`SYSTEM_CONSUMER_TEXT_ASSERTION_FAILED`。因此 Windows→Mac 文本为 FAIL；链接、3×2 PNG、Mac→Windows、防回环 A/B/C 和文件并存均未执行，不能把预置板或源码测试写作跨端通过。
- 按窗口异常边界立即收尾：两端总开关写为 false，macOS package stop/verify jobs 确认 watcher 已终止，Windows 准确 package PID `93952` 已结束；随后两端 helper 均返回 `grant_count=6`、`master_enabled=false`。本地窗口账本和 Windows 写入证据在 `C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5a-0c59\`；完整远端 jobs 与精确包哈希见 [E5 联合验收](evidence/DESKTOP-E5-ACCEPTANCE.md)。已完成离线分析；用户现已允许同范围临时覆盖，但本轮窗口启动在执行层 `blocked by policy` 前被拒绝，未重写系统剪贴板。
- 离线定向回归 `TestClipboardConcurrentEnsureSessionsRemainReady` 在 memory adapter/loopback 下验证双端同时启动 inbox、`EnsureClipboardSessions`、双向 lease 与文本 commit，结果 PASS，未复现并发建会话假设；该结果不替代物理准确包 FAIL，也没有读取系统剪贴板。

## 2026-09-19 E5-C 0c59 Windows 键盘复测

- 只读确认 input desktop 为 `Default` 后，启动准确 Windows package PID `80752`，但 `SetForegroundWindow` 返回 false。前台所有权检查未通过即终止，未发送任何 Tab/键盘事件；package 已停止，clipboard master/grant 保持关闭。键盘顺序仍为 UNRESOLVED；下一次仅需用户点击准确 package 窗口使其前台后再检查一次，不强制前台或引入替代自动化。

## 2026-09-20 E5-J：4d 准确包中断/重启恢复

- 本节物理 package 来源为 4d361320bce1c0bb354a590fe0cc3c73be0242aa；Windows portable SHA256 为 c22c3d2bd6b128ec136d3b14e282fe2c94ae2081e6f1334cbf258951abbc4edd，Mac arm64 DMG SHA256 为 8145ede728723dac5ce8cbfb35a0e29a6468bd1322fe138c17bd94860b4b80c9。证据操作的工作树当时为 1a465642894651c5d4c893c41bcabb3b8923bc8f，不能把它称为 package 来源。
- 先完成 Windows 到 Mac 的正常 256 MiB 包：源和接收 SHA256 均为 d1aa5f5926e51a85a082a75099c4832c2ee4f450ca58f8f089a40e73445a0df6，lan_direct、QUIC、TLS 1.3、ALPN linksend/1、0 retransmit。
- 发送端恢复案例使用 1 GiB 专用文件，SHA256 为 5fd17c10b75c7052a339acb81ff9ee8884e9b08d4d7d8763bd52b5cd39f4a0ae。Windows sender task 20260919T213847.941678300Z-00000002、TransferID 4497a49a802ab56e6358577e1590ed47 的初始 attempt 为 4c09f0ad81b986f093d6aacf68eb7d73；观察器只在已验证 4 MiB、实际已发送 8 MiB、整体未完成时停止已核验 package。Mac 持久 state checkpoint 在中断后仍保存同一 TransferID、manifest、selection、receive plan 和目标目录，已验证 8 MiB、未提交。
- Windows 以同 profile 重启后，任务变为 restart_recovery_available。UI 仅 Invoke 进行中和恢复传输，未调用重新发送；同一 sender task 和 TransferID 进入新 attempt c56900eaf913564faf801fe038fecfd8。最终 sender verified/committed 为 1 GiB、retransmit 为 0、双方确认完成；新 session 为 36c7d0b5595609f2a92c2952d50ce2ae，仍为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1。
- Mac 连接恢复时按产品模型新建本地 receive task 20260919T214348.804670000Z-00000004 及 attempt c6b872ee064ef6e19f45de4ddd638b5b；这不是双端 task ID 不变的宣称。它与中断前 Mac receive task 20260919T213851.068949000Z-00000003 保持同一 TransferID、manifest、selection、receive plan 和目标目录。恢复 attempt 只接收缺失的 1,065,353,216 bytes；最终 verified/committed 为 1 GiB、retransmit 为 0，目标文件 SHA256 与源一致。
- Mac 到 Windows 的独立 V2 activation 只形成 0-byte QUIC_HANDSHAKE_FAILED，未重试，也未作为任何成功证据。本节只证明 Windows 发送端的应用重启续传；接收端重启、物理网络切换、睡眠恢复和反向传输仍未由此通过。
- 两端本轮 package 都已按路径/PID 所有权停止；Windows/Mac clipboard master 均为 false，六项 grant 均为 disabled。测试 peer 的 Windows 接收目录仍指向 E5J 受管目录，revision 12：产品 SaveDeviceProfile 清理 helper 在运行前被执行层 blocked by policy，未写入 profile；用户 profile 未操作。该限制不影响上述专用测试 profile 的恢复事实，但不能写为目录清理完成。

## 2026-09-20 E5-K 诊断与反向复验（NOT PASS）
- 2c81b40 typed handshake diagnostic，4889cbe 修复 clipboard/inbox WSS handoff；五项 CI SUCCESS。Mac DMG SHA256 5dd149a176f00fa44fff47d65db59d21fb8d8952e0d53b82a9f015241e2372a5，Windows artifact 内 portable ZIP SHA256 0bfa8cc1e722a22e0fb306551bf3167a17233466ae6f6898ad4019f51f0ae96c，BUILD-INFO 均指向 4889cbe。
- 新 Mac→Windows V2 request 在 sender ice_checking/CHECK_TIMEOUT 0 bytes 失败，Windows 无 incoming/无 UIA 接收；未进入 QUIC，旧 QUIC 失败仍未知。两端候选已停止。


## 2026-09-20 E5-L 4889cbe 反向 QUIC 写路径诊断（NOT PASS）

- 固定候选为 4889cbe14931f0b1743b4e618c4388e2587a7a5b：Mac arm64 DMG SHA256 5dd149a176f00fa44fff47d65db59d21fb8d8952e0d53b82a9f015241e2372a5，Windows portable ZIP SHA256 0bfa8cc1e722a22e0fb306551bf3167a17233466ae6f6898ad4019f51f0ae96c，EXE SHA256 5a2629ee520bebdea37d113dd86fa37b67d910357241fbb985b3f28f606537b7。两轮均使用同一隔离 profile、一次性 12 MiB 随机文件和新的 V2 activation request；未重新配对、改网络、触碰剪贴板或传递文件正文到信令/IPC。
- 首轮 request 2f2b3a5a724802e6fdcc9ded9c9ea713 的 Mac sender task 20260920T004043.317397000Z-00000001 在 quic_handshake 以 QUIC_HANDSHAKE_FAILED、0 bytes 终止。两端安全 ICE 时间线都实际经历 Checking → Connected 和一个 selected pair；Windows 随后 Disconnected → Closed，没有 incoming task 或 UIA 接收。持久的受限 failure diagnostic 是 udp_socket/0x41，Darwin 映射为 EHOSTUNREACH，不能把它称为 CHECK_TIMEOUT。
- 第二轮只在 Mac package 进程额外设置 QUIC_GO_DISABLE_ECN=true；QUIC_GO_LOG_LEVEL=error 只用于观察。新的 request 90445430033aaf9810bd5af49e8c0c8b 仍在 quic_handshake/QUIC_HANDSHAKE_FAILED、0 bytes 失败，安全错误白名单归一化为 QUIC_EVENT_OPERATION=write_EHOSTUNREACH。因此禁用 ECN 后仍复现，不能归因于 ECN 单因素；该结果也不能排除更广义的 WriteMsgUDP/sendmsg 或内核/链路可达性因素。
- Mac 到 Windows 10.234.16.254 的只读路由为 active en0 直连，Mac 本机地址 10.234.35.5；Application Firewall global state 为 disabled，包没有 sandbox entitlement。该快照不证明运行时没有链路错误。两轮 Mac/Windows owned package 均按 PID/路径停掉；自动剪贴板总开关仍为 false，测试 peer 的六项 grant 均 disabled。
- 为保留凭据边界，Pion stderr 仅经 FIFO/内存过滤写入固定 ICE 事件；quic-go error 只归一化保留上述 read/write 与 errno 类别。未持久化或回传任何原始 stderr。首轮和 ECN 轮机器回执分别位于 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5l-4889cbe\attempt-20260920T0033Z\e5l-machine-receipt.json 与 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5l-4889cbe\attempt-20260920T0055Z-ecn-disabled\e5l-ecn-machine-receipt.json。
- 请求现有权限的头部观察未能启动：sudo -n tcpdump --version 需要密码，普通用户对 /dev/bpf0 至 /dev/bpf3 都没有读写权限。未反复探测、没有运行 tcpdump、写 pcap 或修改 sudo/BPF/防火墙策略。待用户在已有权限下审阅并执行的最小草案见 E5 TODO；它只观察两台测试机 UDP/ICMP 头部，90 秒或 80 包上限，不记录正文或 pcap。

## 2026-09-20 E5-M 4889cbe Windows→Mac 接收端应用重启恢复（PASS）

- 固定 package 来源为 4889cbe14931f0b1743b4e618c4388e2587a7a5b、workflow 35475890292：Windows EXE SHA256 为 5a2629ee520bebdea37d113dd86fa37b67d910357241fbb985b3f28f606537b7，Mac DMG SHA256 为 5dd149a176f00fa44fff47d65db59d21fb8d8952e0d53b82a9f015241e2372a5。复用既有受管 1 GiB 源 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5j-1a46564\windows-source\e5j-win-to-mac-restart-1g.bin，未新建或复制 GiB fixture；其 SHA256 为 5fd17c10b75c7052a339acb81ff9ee8884e9b08d4d7d8763bd52b5cd39f4a0ae。
- 正确 fixture 是 Windows e5a-0c59/windows/profile，实际 sender identity 为 83c68b7afd4698cc8affca4f0aa67c2460521c7875f9ce326b7807b9d670561f，Mac profile 已为该 identity 保存 auto_accept=true。最初误用 e5a-0c59/windows-sender/profile（identity 58e437ccbf16ac9a951ef3f7fa9061277012c9affc1f8c288a7dabc6a56cb01e）只到 awaiting_acceptance、0 bytes；已停止且没有重新配对，不计入恢复失败或成功。
- 最终恢复轮 request 为 a6faacd12d62457b86acb09bb217acfd。Windows sender task 20260920T014430.033251000Z-00000004、TransferID 90135fbd1c1685f850346abd6f32aaf5 的初始 attempt 为 60f45c1844608d59d33ed670a898c1fb；Mac 初始 receive task 20260920T014434.352220000Z-00000001 在 verified 4,194,304、received 8,388,608、committed 0 的持久 checkpoint 后，才由只匹配新 TransferID 的受管 Python monitor SIGTERM。monitor 先排除所有旧同名 TransferID，避免把历史 task 误匹配为本轮检查点。
- 中断后 sender 进入 recovering/connection_interrupted，可 resume。Windows UIA 只 Invoke 恢复传输，没有 Invoke 重新发送，也没有键盘、前台、clipboard 或 CDP 操作。恢复 sender attempt 为 9baf86970d7053c0e491282d70539714；最终 sender verified/committed 均为 1,073,741,824、retransmitted 为 4,194,304、bilateral_confirmed=true，路径为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1。
- Mac 恢复 receive task 20260920T014743.650042000Z-00000001 只接收缺失的 1,069,547,520 bytes，最终 verified/committed 均为 1,073,741,824、receiver retransmitted=0、目标 SHA256 与源一致。初始和恢复 receive task 的 TransferID、manifest digest、selection digest、receive plan digest 与 target directory 均一致；binding_consistent=true，receive plan digest 为 191827a88f13e4d09a2ea34afa9b8eec484437d77a3d83eb3a20fd99c4dd0415，最终目录只有预期文件而无第二副本。
- 两次同名 TransferID 的旧 monitor 基线漏列/时序错误导致任务正常完成，保留记录但不计恢复证据。首个 UIA 脚本因 PowerShell 参数名 pid 与自动 PID 冲突而在输入前退出；v2 改用 processId 后只 Invoke 恢复传输。E5J/K/L 的已终态受管 activations 已可逆归档到 E5M 根，未删除源或用户文件，也没有将旧 recovering history 误判为外部阻塞。
- 两端 package 均按 owned PID/path 停止；clipboard master=false，测试 peer 的 enabled grants=0。最终机器回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5m-4889cbe\e5m-final-machine-receipt.json。此 PASS 只覆盖 Windows sender、Mac receiver 的接收端应用重启恢复；Mac→Windows write_EHOSTUNREACH 是独立的反向故障，未纳入本项。

## 2026-09-20 E5-N 4889cbe 小文件与空闲性能基线（PASS，样本范围有限）

- 固定 4889cbe 准确包和既有 Windows/Mac profile，新建受管 source 仅 9,437,184 bytes：连续 1 MiB 文件 3 个，批量 512 KiB 文件 12 个，共 15 文件，低于 32 MiB 上限。没有下载、构建、优化、重传既有 1 GiB 样本，亦未改系统设置、权限、网络、配对、clipboard 或日志边界。
- Windows→Mac 连续 run 的实际 stage-to-completed elapsed 为 3,588.242、1,312.396、1,856.090 ms，各为 1,048,576 bytes；12×512 KiB 批量为 6,291,456 bytes、2,509.479 ms。四次 sender task 均 completed、verified/committed 与 source bytes 相等、bilateral_confirmed=true、retransmitted=0，连接为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1。它们是小样本端到端耗时，不是吞吐承诺。
- Mac read-only receiver 核验得到 4 个 completed task、15 个预期目标文件、无 E5N 重名副本；每个 target bytes 和 SHA256 都与 Windows fixture manifest 一致，receiver retransmitted=0。完整收束回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5n-4889cbe\e5n-final-machine-receipt.json。
- 空闲期仅测 LinkSend 主进程，不包括 WebView/helper 子进程。Windows 12 点、2 秒间隔、22.221 秒窗口：CPU seconds delta=0.328125，按实测墙钟折算为单逻辑核平均 1.476627%，working set=51,552,256–52,989,952 bytes，private=65,372,160–66,588,672 bytes。Mac 12 点、2 秒间隔、22.195 秒窗口：RSS 恒为 130,514,944 bytes，系统 ps 的平滑 pcpu=0.1–0.7%、均值 0.316667%。两个 CPU 口径不同，不能直接横向等同。
- 准确包任务只提供连接 timing，未提供端到端 discovery timestamp；task snapshot 也没有单独 DB commit 时间边界，均为 NOT_OBSERVABLE。连续 n=3、批量 n=1，不报告 p95。Mac 初始采样的 run-script 结果已获记录；随后 SSH-manager tail-job helper 因 zsh status 变量失败，未为绕过读回问题新增任何样本。

## 2026-09-20 E5-O U2 离线删除准确包路径（NOT RUN，UIA 可达性限制）

- 只用当前 4889cbe Windows 准确包与本任务 Windows profile；确认无 package 进程后，以本次进程环境 LINKSEND_SERVER_URL=https://127.0.0.1:1 启动。该覆盖只使信令不可达，不写 desktop-preferences.json，不改 membership/group 归属、宿主网络、HK 服务或用户 profile。
- UIA 对 owned package PID 成功 Invoke 一次“设备”导航。精确测试 Mac identity 在 trust 中仍是 group peer，但 1400×950 窗口的 WebView UIA 树只导出当前可见的 10 个身份项，目标不在树中。树不提供 ScrollPattern；向该 package HWND 的设备列表坐标发送 12 次定向 WM_MOUSEWHEEL 后仍未出现目标。没有 SetForegroundWindow、键盘、全局 SendInput、CDP 或 clipboard 操作。
- 因而没有 Invoke 设置、删除设备或确认删除，也没有按第 N 个设置按钮猜测目标。package 随后按 owned PID 停止；target 仍 trusted、没有 denied entry，saved server_url 仍为空，clipboard master=false、enabled grants=0。此轮不是离线删除 PASS 或 FAIL，回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5o-4889cbe-u2\u2-noaction-machine-receipt.json。
- 要完成此项，只需用户现场滚动并选择精确测试 Mac 行，之后执行一次产品删除/确认并核验本地 pending-sync、重启保持撤销、原服务同步、刷新/LAN 不复活和既有流程重新配对。旧 4d 包的删除/重配对记录不替代本轮 4889 离线结论。

## 2026-09-20 E5-P 4889cbe 海量小文件基线（PASS，1000 文件边界）

- 固定现有 4889cbe Windows/Mac package、83c68b Windows profile 和已免确认 Mac peer；新增一个专用目录，内有 1000 个 1 KiB 文件，合计 1,024,000 bytes，低于 4 MiB。只发起该目录的一次 native-entry batch，未下载、构建、优化、改变系统设置、网络、clipboard 或用户 profile。
- sender/receiver 均 completed、verified/committed=1,024,000、bilateral_confirmed=true、retransmitted=0，连接为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1。产品 task file_count=1001：所选根目录是一个条目，另有 1000 个普通文件。Mac read-only 脚本按 Windows fixture manifest 逐一计算 1000 个目标文件的 SHA256，mismatch=0；完整 source hash 清单在最终机器回执内，不在日志或本文展开。
- sender persisted task start 至 bilateral completed 的实际耗时为 35,000 ms。activation entry 在最初 monitor 记录墙钟前已被消费，故 enqueue-to-completion 是 NOT_OBSERVABLE；没有以 journal 文件时间补造这一指标。首个 sender receipt 假定 file_count 只计普通文件，首个 Mac read-only 查询假定 manifest_summary 精确等于目录名；两项均在完成后只读修正回执谓词，未重传或改变任何任务。
- 本次记录的空闲主进程观测小于 180 秒：Windows 15 点/28.309 秒，CPU seconds delta=1.671875、单逻辑核平均 5.905756%，working set=51,404,800–53,501,952 bytes、private=64,425,984–66,347,008 bytes；Mac 12 点/22.181 秒，RSS=130,482,176–130,514,944 bytes，ps 平滑 pcpu=0.1–0.9%、均值 0.316667%。两端均排除 WebView/helper 子进程，CPU 口径不同；这不是长时稳定性或百万文件结论。
- 准确包仍没有端到端 discovery 或独立 DB commit 时间边界，继续为 NOT_OBSERVABLE。两端 package 已按 owned PID/path 停止，clipboard master=false、enabled grants=0。完整机器回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5p-4889cbe\e5p-final-machine-receipt.json。

## 2026-09-20 E5-Q U2/U7 有界目录与详情可达性（源码通过，非准确包结论）

- 以 `5a5f43f` 为基线完成源码收敛：配对、附近和已删除三类设备均支持名称、别名和完整 identity 搜索，每类独立每页最多 10 行；详情由稳定 `DeviceID` 驱动的抽屉承载。搜索、翻页或同 ID 后台快照不会错指目标或丢失未保存别名；目标消失时页码钳制并把焦点回搜索框，普通关闭回详情触发器，Escape 只关闭抽屉。
- 抽屉标题和操作栏固定，正文单独滚动；保存、删除和二次确认在 footer 中可达。其 Tab/Shift+Tab 闭环已在 Chromium 本地 Vite 源码 harness 验证；保存期间字段集禁用，旧 revision snapshot 不会覆盖刚保存的更高 revision。新收件确认与 LAN 配对遮罩升为 `z-index:1200`，高于设备抽屉的 1100；LAN 配对关闭时恢复此前焦点。
- 发送草稿、待发送队列和进行中任务均分页到每页 10 项，删除、控制和重排仍使用完整 path、queue ID 或 task ID；队列重排继续将完整 pending 集合交给 `ReorderQueue`。收件状态只保留在 App 顶栏，并从该行进入已有“文件接收”设置分类；`no_content` 不出现在进行中视图。传输工作区改为单列，移除接收块后不留旧双列空槽。
- 验证：`pnpm test` 为 8 files / 76 tests PASS；`pnpm run typecheck`、`pnpm run lint`、`pnpm run build` 均 PASS（本机 Node 24.19.0 与 pnpm 11.19.0 不满足 package 声明的 Node ^24.21.0 / pnpm 12.4.1，命令仍成功）。Chromium 源码 harness 在 960×640 和 1100×720、中文长名称下确认抽屉无水平溢出、正文独立滚动、关闭/保存/删除及二次确认均在视口内；传输页单列无接收空槽。这不是 Windows/macOS 准确 package、DPI 或系统辅助功能验收。
- 性能账本边界：百万文件、GC 和长时稳定性是后续观察，不是本里程碑新增必交付。E5-N 的 n=3/n=1 不产生 p50/p95，准确包的 discovery 与独立 DB commit 仍无测点。存储持久化不是“完全无证据”：`history_test.go:327` 的 `BenchmarkTaskHistoryConnectionLifecycle` 覆盖 `persistRecord → SQLite autocommit` 全路径；E4 的 100×3 为 2.140/2.176/2.190 ms/op，包含序列化、锁和 SQL，不能表述成单独 `tx.Commit`。

## 2026-09-20 preview.2 发布及停止

- 已按用户最新指令发布 [v0.5.0-desktop-preview.2](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-desktop-preview.2)，tag/程序包来源均为 `0229cbf3e8373c544431102ed904c4a19ed02dc4`，五项源码 CI success，packages run `35488500579`。四个程序包及两份校验/来源资产已上传，六项远端 digest 与本地相同；旧 tag/资产保留，PR #8 仍为 Draft，未合并 main。
- 已完成设备/文件/队列/任务分页、固定设备详情操作区、键盘焦点与设置保存竞态修复；前端 76 tests、typecheck/lint/build 及两种窗口尺寸的 Chromium 检查通过。新发行包仍未进行原生与物理网络完整验收；旧 `4889cbe` 的实机证据保持原来源。
- 本机超长 artifact 路径使安装器版本读取为空，缩短受管本地路径后原校验器通过；载荷 SHA256 未改变。详细资产表、CI 链接和发布回执见 [E5 交付记录](evidence/DESKTOP-E5-DELIVERY.md)。
- 执行模型容量不足后，总控接手提交同步和发布收尾；没有新建执行者。按用户要求，文档同步后停止所有后续实施/派发，等待下一次明确启动。保留未完成 TODO，不把发布或浏览器通过当作整轮完成。
