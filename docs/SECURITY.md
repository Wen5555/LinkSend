# Security
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

2026-09-13 E0：本轮新授权/同意/剪贴板方案见 [ADR0007](adr/0007-desktop-membership-consent-and-clipboard.md)，语义已由总控E0-safeio-close-v1接受。当前 M5 wire/schema/授权实现尚未改变；具体API/schema/协议仍需在E1/E4冻结并补兼容与安全测试，不能将提案当作已实现能力。

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
