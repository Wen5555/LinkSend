# ADR0017：一次性临时分享授权

日期：2026-10-19。状态：设计冻结，分增量实施；不可把本设计视为已有完整功能。实际范围只在唯一 TODO 与 evidence 维护。

## 边界

临时分享不写永久 devices/groups/trust，不授予成员枚举、邀请、自动接收、剪贴板或普通任务权限。采用现有 Ed25519 设备身份、标准 crypto/rand 与 SHA256，复用现有请求签名和认证 Pion ICE/QUIC、公钥固定及 V1 文件引擎；不创建新的密码学握手。正文、文件路径和文件清单不进入服务，`relay=false`。用户仍分别确认接收身份和保存位置。

完整链路实现前不广告 share_grants_v1，不给旧端默认能力。核心源码增量可以先合入工作分支，但不能因此宣称可创建或领取分享。未来 HTTP/WS 入口必须先完成下列签名、限频、原子持久化与成员接口隔离测试，再开放能力。

## 代码与持久 owner

发送端本机用 crypto/rand 生成20字节（160bit）随机分享码，以32字符 Base32 分组显示，可复制完整码；严格 ASCII 解析只接受完整码及固定分隔，不做模糊截断。创建接口只接收带固定用途 domain 的 SHA256 摘要，领取时经TLS接收的明文码仅用于派生该验证器，不持久化；新分享 ID、创建请求 ID 及 task ID 使用现有128bit随机 ID。码不从文件名、身份或时间推导，不落日志/诊断；文件正文仍不经JS。发送端重试创建使用同一 ID/摘要和清单，不生成新授权；码丢失时撤销并新建，不从服务回读明文。

服务借用既有 Control SQLite 连接和事务，新增版本化 share_grants 表及必要索引；不增第二数据库 owner。迁移在真实 SQLite 一致性备份后事务提交，回读版本和 quick_check；不覆盖既有备份。旧服务拒绝新 schema。回退程序与数据回退分开：原备份不能覆盖升级后的成员与业务写入。新库初始化无旧数据可备份；未来/损坏版本拒绝。

分阶段落地：存储核心先提供显式EnableShareGrants以测试4→5迁移；后续签名控制增量仅在服务配置experimental_share_control=true时激活，并在health标注file_transfer_ready=false。普通启动继续schema4且不广告完整临时分享。新代码可安全读回已经显式迁移的schema5；缺表/错误版本拒绝。完整链路接入时再激活默认迁移和正式能力，并重新验证准确包。任务库schema12不受该控制库版本影响。

记录绑定分享 ID、发送身份/公钥、既有V1清单 BLAKE3 摘要、授权 generation、revision、领取期限；领取后增加接收身份/公钥、claim ID、绑定 task ID、V1计划摘要和恢复期限。清单与计划摘要直接复用现有引擎的Digest结果，不另创不兼容的散列序列；分享码验证器独立使用SHA256。清单摘要必须来自实际冻结的引擎 manifest，不能把远端显示大小当授权内容。磁盘大小、哈希和源变化仍由原文件引擎核验。

## 状态与竞争

`offered → pending → approved → bound → completed`；任一非终态可由发送方 `revoked`，到期变 `expired`。拒绝或 pending 超时回到 offered 并推进 generation，旧确认不能绑定新领取人。pending 最长2分钟且不越过10分钟领取期限；approved 不延长领取期限。成功 bound 才签发最多24小时、不可滑动延期的原任务恢复期限；时间值来自服务可信时钟。

所有动作都经当前状态、精确 expected revision/generation 与行CAS；并发领取只有一个胜者。相同 actor/claim ID、相同绑定内容的合法重试幂等且不延长期限；相同请求 ID 换内容拒绝。不允许任意 caller 提交状态或期限来恢复旧授权。终态不回到 offered；再次发送创建新码。

sender approve 必须明确匹配当前 claim ID、接收身份/公钥、原 manifest 与 task ID。接收方确认保存方案后绑定；未 bound 不能进入文件引擎。resume 只对 bound 的原 actor、公钥、task、manifest、generation/plan 生效。完成后只允许双方核对该 task 的既有终态，不能再打开文件；撤销与完成事务竞争按先成功的终态为准，撤销先落库后通知两端取消。后到回调不恢复撤销授权。

服务重启保留持久授权、终态与绝对期限，不延长 pending/领取/恢复窗口；在途连接需重新签名认证和重新查授权。客户端重启不自动同意新领取或新清单；只呈现持久的原任务恢复入口。源变化不能沿用原摘要/码，用户重新确认后新建分享。完成确认丢失仅返回原终态。

## 接口与限额

能力协商使用新增显式 share_grants_v1；请求走独立 `/v1/shares` 路由及用途域，方法、路径、分享/claim/task、generation、内容摘要、nonce 和期限进入既有请求签名。服务验证实际签名、设备 ID 与公钥匹配、短期 nonce 防重放，不以 bearer code 代替设备认证。领取人可来自其他组或未加入组，临时公钥只在该 grant 范围有效；不借普通成员 websocket / devices 接口认证临时 actor。

签名控制增量复用既有AuthBytes长度前缀签名输入和Ed25519验证；health提供32字符公开随机运行代次epoch，实际签名nonce字段为epoch+冒号+32字符请求nonce。方法、RequestURI、设备ID、时间及完整有界JSON都进入既有签名函数，不新增密码学算法。X-LinkSend-Share-Epoch必须等于当前运行，重启前证明全部拒绝；原grant仍持久，合法原身份取新epoch并签新nonce后可恢复原任务，不滑动延长期限。新epoch是公开challenge，不是bearer凭据，不授予成员权限。

领取前做IP与身份双维度有界限频，失败码不泄露其他接收人、组或存在性。每身份最多8个未终态分享、最多4个待确认领取；服务总记录4096，nonce/限频表各有上界。pending 到期自动释放占位，猜码不能刷新占位。终态保留至少7天供幂等核对，超总预算拒绝新创建而不删活动授权；定时分页最多64条过期处理。代理IP仅在显式可信代理配置下使用，不能信任任意 X-Forwarded-For。

当前实验控制限频：领取IP突发20、已签名身份突发10，均每6秒补1次，使用实际RemoteAddr且忽略X-Forwarded-For；合计最多2048个bucket，5分钟空闲可淘汰，满表拒绝新key，不淘汰未空闲项以绕过限制。nonce复用既有65536上界ReplayWindow，签名验证通过后才占位，2分钟保留；服务重启换epoch阻止旧证明复用。限频不阻止所有受限网络或拒绝服务，不宣称能代替网络防护。

HTTP最大请求体、字符串、候选数、并发、deadline 复用现有协议上界。临时协调消息只能去 grant 的两端，必须带 grant ID/generation/task；每次新 ICE/QUIC 和文件操作重新核验。不能让一个临时授权访问对端其他任务或内容，也不能复用已撤销的普通授权来绕过临时绑定。失败返回稳定码、安全下一步；本地导出只显示白名单原因，不含码/摘要/公钥。

## 2026-10-26 临时协调与预览设计冻结

以下是下一增量的协议与授权约束，尚未实现，不改变file_transfer_ready=false或普通成员路径。采用独立GET /v1/shares/{id}/ws和linksend.share-signal.v1子协议；严格query只含原claim/task、grant generation、phase、manifest/plan摘要，所有值进入实际RequestURI的现有epoch请求签名。禁止重复/未知query、分享码、文件路径、正文或清单。临时actor不写devices/groups/trust，不注册普通s.clients/s.sessions，也不能调用普通成员WS。

ShareSignalScope绑定grant ID、claim ID、task ID、grant generation、phase、manifest摘要及plan摘要。Envelope.Generation继续表示ICE代次，不能替换grant generation。共享Frame保留原protocol.Envelope签名，外层用既有AuthBytes/Ed25519对完整scope、原Envelope、epoch、新nonce和有限时间再签名；这是沿用既有签名算法绑定用途，不新增密码学握手。不得解包改写payload后冒充原Envelope签名仍有效。两端原公钥分别从当前grant固定，服务和客户端都重验actor/recipient及scope。

preview仅为当前approved、原双方/claim/task/generation/manifest、领取期限内的独立认证QUIC元数据预览。清单与实际保存计划在这个加密P2P连接中确认，不进入信令；未bound不得进入V1文件引擎、创建目标storage或发送文件body。Bind成功立即退役preview；transfer需当前bound、原plan/task/manifest/双方公钥/恢复期限，调用原Record.Authorize，再用新的ICE SessionID和QUIC连接。禁止在preview连接上隐式升权。两阶段固定发送方发起、接收方响应，关闭session reuse/clipboard，均不进入普通连接池。

元数据预览只能返回一份经原Manifest.Validate/Digest核对的清单及一份明确确认计划，计划包含最终本机路径和冲突策略；绑定后不能由引擎自动改名悄然改变digest。源变化需撤销并由用户重新确认新分享。文件操作仍走现有安全落盘/完整性/V1引擎，原本地黑名单优先；完整ShareGrant与任务恢复owner接入前不开放产品入口。

shareHub独立管理最多32个活动scope/64条WS，每身份最多8条，每scope的每actor只有一个连接；替换和cleanup比较owner token/指针，旧回调不能删新连接。单个ICE negotiation、每方向32个candidate、frame32KiB/payload16KiB、鉴权/写入5秒、读空闲65秒、每连接60frame/s；候选仍用原准入，ICE描述严格拒绝复用/剪贴板权限，end为空对象、session_error只用白名单码，不允许任意JSON正文转发。

分享状态迁移、WS注册、转发准入和writer出队共享分享gate/owner token；持久撤销/完成/过期后先退役scope、丢弃未发队列并取消对应连接，不能触及普通连接注册表。网络写入不持有全局s.mu；已经开始发送的在途字节无法撤回，信令也不是文件权限。每scope按真实期限主动关闭，不能仅等一分钟reaper；所有新操作仍核验实际期限。Server.Close先禁止新注册、取消并等待分享reader/writer/timer，再关闭唯一Control数据库。

控制面断线不允许新preview/transfer或恢复任务跳过新证明；新连接必须新nonce、当前epoch及原scope核验。已经建立的bound健康QUIC只承载原任务，仍受本地grant owner、原实际期限、取消/撤权和完整性约束，无关控制面重连不重开或扩大权限；明确终态/撤销通知必须取消对应任务，其他普通QUIC保持。客户端owner与该策略的因果验证属于后续数据面必做项。

实现前后需用真实SQLite/签名/WS证明第三身份、跨grant/task/phase、错key、旧grant代次/ICE会话、错内外签名、epoch重启重放、正文/剪贴板字段及候选/队列/连接限额拒绝，并核对永久表零增加。屏障固定鉴权后注册前撤销、入队后写前撤销、旧cleanup晚于新连接三处竞争；Bind关闭preview、空闲到期、Close与注册竞争及普通连接不受分享终态影响分别验证。不能以这些服务原型测试抵扣两端UI、原生保存或真实跨网临时分享。

## 验收执行

先用假时钟和真实 SQLite/签名测试猜码/格式/限额、过期、重放、并发领取、明确批准/拒绝、撤销/完成竞争、错误公钥、跨组/未入组隔离、源摘要/任务替换、服务重启、原接收人有限恢复和旧端拒绝。拒绝后回读确认记录和永久信任未变；只断言返回错误不足以证明安全。

再覆盖真实文件引擎与两端 UI：Windows↔Mac 临时分享及不同网络双向分享，核对集合/目录/hash、双方终态及永久信任/权限零增加。自动测试、loopback、CI、设计文档均不能替代实机和跨网证据。
