# Protocol

2026-09-12 M0：当前源码产品为 0.5.0，V1 wire/ALPN/签名编码和任务 schema 2 保持兼容。
本机拒绝在 LAN/WSS/恢复/接收确认前执行；拒绝使用既有 AUTHENTICATION_FAILED，不暴露本地拒绝详情。
trust schema 1 与 profile 锁是本地迁移，不是协议版本。新内容及选收能力尚待 M4/M5 协商实现，
本阶段不声称旧端支持这些能力。Go1.27 的 JSON/签名/manifest/恢复回归列入 M0 验证。

M1/M2 的 task schema3、设备偏好、路径草稿、队列和激活日志均为本地状态；没有改变 V1 offer/accept、
ALPN 或签名。通过系统菜单选择文件不会在 WSS 中增加路径、文件清单或正文。

当前源码产品版本为 `0.4.0`，控制/文件协议版本仍为 `1`。`internal/protocol.ProductVersion`、`Capabilities.product_version` 和 `/healthz.version` 用于产品部署识别；`protocol_version` 继续决定 wire compatibility。升级产品小版本不会自动改变协议版本，旧服务缺少 `product_version` 时客户端可按 V1 能力兼容，但诊断必须明确显示版本未知。

控制面使用有界 JSON `Envelope`，签名输入是明确的长度前缀二进制编码，不直接签任意 JSON map。字段包括协议版本、消息 ID、会话、发送方、接收方、generation、有效时间、payload 和 Ed25519 签名。

消息类型为 `connect_request`、`connect_response`、`candidate`、`end_of_candidates` 和 `status`。服务端只转发已认证设备组内、会话归属和 generation 正确的消息。未知关键类型、版本错误、重放、过期和超限消息明确拒绝。

候选使用真实 trickle 语义：端点开始 gathering 后立即发送连接请求/响应，host/srflx 候选通过回调逐个签名发送，Pion ICE checks 与尚未结束的候选收集并行；不再等待两端各自完整 STUN 超时后串行开始检查。双方已验证同一 on-link 前缀的 host path 为 `lan_direct` 时，可主动发送 `end_of_candidates` 并停止向该会话补充无关公网候选；非 LAN 路径继续等待受限 STUN gathering。`end_of_candidates` 仍表示本 generation 不再发送新候选，候选和检查均受原有数量、时间与 session/generation 约束。服务端成功转发双方的 `end_of_candidates` 后释放该协商记录，使同一认证 WSS 可立即承载下一 session；不会关闭已建立且独立于信令的 QUIC。Pion 在 `Connect` 返回后仍可能报告初始化阶段最后一次 pair 优化；endpoint 在 1 秒稳定窗口内更新基准但不关闭 QUIC，窗口后不同 nominated pair 才被视为运行期路径变化并进入显式恢复。

响应端在已验证 `connect_request` 后若无法创建本地 UDP endpoint、收集候选、完成 ICE/QUIC 建连或请求设备不符合本次接收限制，会发送同 session/generation 的签名 `status`：payload 固定为 `{"state":"failed","code":"<stable connection code>"}`。允许的 code 仅限候选交换、无候选、ICE 检查、QUIC 握手、认证限制和通用直连失败；不得携带本地路径、原始系统错误、ICE credential 或其他私密 cause。发起端必须验证发送者、接收者、session、generation、Ed25519 签名和 code 允许列表，才能提前结束等待；篡改或未知 code 按 `INVALID_MESSAGE` 处理。旧客户端不识别该状态时仍可能回落为超时，因此产品包需两端同步升级。

桌面配对使用 40 位随机量的单次短码，规范显示为 `ABCD-EFGH`，服务端比较规范化后的摘要；输入时忽略大小写、连字符和空格。旧版 43 字符邀请在兼容期仍可加入。配对成功即由客户端固定认证成员列表中的 Ed25519 公钥，不再发送额外的人工指纹确认消息；已固定密钥不允许静默更换。

配对邀请响应新增 `server_time` 和 `ttl_seconds`，客户端倒计时以服务端相对 TTL 为准，不因两台设备的本地时钟偏差把有效码判为过期。服务端明确区分 `PAIRING_CODE_INVALID`、`PAIRING_CODE_EXPIRED`、`PAIRING_CODE_USED` 与 `PAIRING_IDENTITY_CONFLICT`；同一 Ed25519 身份在成功响应丢失后重复提交同一码属于幂等成功，其他身份仍不能二次消费。控制库 schema 2 只增加 `used_by/used_at` 摘要关联，不保存明文邀请码；超过 24 小时的失效摘要在生成新码时清理。

LAN discovery V1 使用 UDP/53318，默认发送管理域组播 `239.255.76.83`、每接口定向广播，并支持用户指定一个经过 on-link 校验的 IPv4 单播地址。公告包含协议版本、announce/response、设备 ID、显示名、Ed25519 公钥、临时 TLS 控制端口、128 位 nonce 与签发时间，整体不超过 2048 bytes；签名输入复用规范长度前缀编码。TTL 固定为 1，来源必须属于接收接口的直连前缀，公告 15 秒失效，nonce 防重放，响应限频。已发现路由每 4 秒单播续租；一次成功传输后只记住该已验证设备最近的 IPv4 地址，下一次启动仅定向探测该地址，不执行网段扫描。

LAN 控制通道为 TLS 1.3 双向 Ed25519 证书验证，允许的客户端必须刚刚通过签名公告出现。它复用现有 `connect_request/connect_response/candidate/end_of_candidates/status` envelope 与 Pion ICE、QUIC 数据面；文件正文不进入 UDP discovery 或 TCP control。陌生 LAN 设备被发现不建立长期信任；仅在发送方主动选择、接收方确认、文件完整性校验及双方终态确认全部成功后固定公钥。拒绝、超时、失败或只看到公告均不落盘信任。

文件控制帧使用长度前缀和独立的 QUIC stream，控制 metadata 上限为 8 MiB，文件内容不 Base64 化。传输顺序为 offer、用户 accept、块请求/数据、ack、finish、completed、confirmed，以及新版接收端返回的 `confirmed_ack`。4 MiB 是默认分块，演示使用 64 KiB 以缩短测试时间。

控制帧必须满足对应操作的语义：offer 的 manifest digest 必须匹配；accept 的已验证字节数必须在 `[0,total]`；ack 必须对应刚发送的块、单调且不超过初始恢复字节加实际发送字节；finish/completed/confirmed/confirmed_ack 必须绑定 manifest digest。空帧、未知关键操作、负索引和不可能的字段组合均会被拒绝。对端 error detail 限制为 4 KiB、有合法 UTF-8 且移除控制字符；未知 JSON 字段暂不作为 V1 兼容性错误。

恢复身份由 `TransferID + manifest digest + chunk policy + peer identity` 共同绑定。接收端重新哈希 staging 块后才继续，完成后通过 `os.Root` 约束路径并以不可覆盖方式提交。

接收端对已验证字节使用单调 O(1) 计数，不再在每个 ACK 前遍历全部块。每个正文块仍先 `file.Sync`；恢复位图最多每 8 块或 500ms 批量 checkpoint。崩溃后的 checkpoint 可以落后于文件数据，恢复时必须重新哈希 staging，不能把未记录或损坏块视为完成。

2026-09-11 错误诊断兼容补丁：V1 `error.error` 字符串保留原帧结构，只发送允许列表中的稳定码，不再发送本地文件系统路径。新增明确的 `FILE_CONFLICT`、`PERMISSION_DENIED`，并保留 `DISK_FULL`。接收方仅对完全匹配的已知码恢复错误类别；旧版本任意文本或未来未知码仍可解析，但仅按通用传输失败处理。身份验证、块帧和 completed/confirmed 语义未改变。

`local_candidate` / `remote_candidate` 诊断字段是 `udp4|udp6 address:port typ type` 描述，不是可回灌 ICE 的序列化候选。诊断不包含 ufrag、密码或候选扩展；经过签名的候选交换仍使用 Pion 原始序列化。

QUIC 的 `Write` 只保证数据进入发送缓冲。拒绝或 error 帧发送后，transport 先关闭发送方向，再有界等待对端结束读取，防止随后的 reset/连接关闭丢弃终态响应。该等待至多 2 秒且不延长调用者 deadline；原始错误仍作为任务结果。V1 帧结构和 completed/confirmed 语义保持不变，net.Pipe 兼容测试与真实 QUIC 负向回归分别覆盖同步和缓冲发送。

`completed`/`confirmed` 成功边界增加向后兼容的 `confirmed_ack`：新版发送端收到匹配回执即可证明接收端已读 `confirmed`；旧接收端在读到 `confirmed` 后关闭发送方向，EOF 或远端 application code 0 仍是兼容证据。新版接收端向旧发送端写出的 `confirmed_ack` 会被旧 terminal flush 当作普通终态字节读取，不改变旧成功语义。非零 application close、stream reset、timeout 和其他连接错误仍失败。双方确认后 QUIC 正常关闭的 draining 在后台完成，不延迟任务完成。

信令协商状态绑定当前认证连接。断开或被同设备的新认证连接替换时清理该设备参与的协商；旧连接的迟到帧不能写入新连接状态。已建立的 QUIC 数据连接不因此被关闭。此修复需要升级信令服务源码；客户端升级无法修复仍运行旧版本的服务。

## 任务、attempt 与恢复语义（schema 2）

`task_id` 表示用户可识别的逻辑任务，暂停、重启和恢复时保持不变；`attempt_id` 表示一次执行，任何恢复都必须新建；`session_id` 表示一次端到端连接；ICE generation 只在对应 session 内解释，候选和 end-of-candidates 必须同时匹配 session 与 generation；`revision` 是任务快照的单调版本。旧 attempt/session/generation 的回调或消息不得更新较新的 revision。

`pause_requested`/`cancel_requested` 是控制意图屏障：请求发出后，已经写入 QUIC 但稍后才确认的 ACK/进度仍可补记实际字节和唯一验证量，但不得把状态回退为 Transferring/Verifying，也不得启动新块。2026-09-11 的 macOS arm64 CI 首次复现了迟到 ACK 覆盖暂停请求；修复后 Windows 高重复真实 QUIC 和相同 arm64 runner 均通过，V1 帧格式没有变化。

桌面打开后维护一个不产生任务历史的后台接收监听。发送请求建立认证 QUIC 后，接收端读取 manifest 并进入 `AwaitingAcceptance`，前端弹出确认；只有确认后才请求正文块。对单个已配对设备保存 `auto_accept` 后，后续 manifest 自动通过同一决策点，身份和完整性校验不变。

合法主路径是 `Preparing → AwaitingAcceptance → Transferring → Verifying → Completed`。AwaitingAcceptance/Transferring 可以进入 Paused；可恢复连接中断进入 Recovering；Resume 从 Paused/Recovering 创建新 attempt 与 session 后重新进入连接/协商。Rejected、Cancelled、Failed、Completed 是终态。校验或文件提交失败必须进入 Failed；只有接收端完成唯一块验证与文件提交，且 sender 收到 completed 并返回 confirmed 后，双方才记录 Completed / `bilateral_confirmed=true`。

恢复仍使用 V1 offer、accept、块请求/data/ack、finish/completed/confirmed 帧，不增加旁路数据协议。新的 session 携带同一 TransferID 和重建后的 manifest；接收端将其与持久化的 manifest digest、chunk size、总字节、文件数、目标目录和 pinned peer 比较，并重新哈希 staging 后生成缺块请求。sender 同样重建并比较 manifest，源内容变化返回 `SOURCE_CHANGED`；恢复身份不匹配返回 `RESUME_IDENTITY_MISMATCH`。已发过但再次被请求的块计入 `retransmitted_bytes`；`verified_bytes`/`committed_bytes` 只按接收端唯一验证和成功提交计算，不能由重传重复增加。

每个成功提交的文件写入 checkpoint commit record（文件 ID、路径、大小和 digest）。接收端已提交而 sender 未收到 terminal confirmation 时，后续恢复先验证 commit record/目标文件，不能覆盖已有文件或盲目完整重发。部分文件已提交后失败时，任务保持 Failed/Recovering 语义并报告已提交文件/字节；未提交部分才可继续。

结构化连接证据仅包含实际 base socket、接口、地址族、脱敏候选描述/类型、ICE 状态时间线、TLS version/ALPN、连接方法、relay 实际值、STUN 请求/响应计数和信令 JSON 字节计数。邀请、令牌、私钥、ICE ufrag/password、候选扩展和文件正文禁止进入诊断。没有独立路由证据时 `connection_method=direct_unknown`。

2026-09-11 独立 Linux 双 NAT 的现场结果不改变协议：固定 UDP 映射的正向场景仍使用同一 V1 ICE/QUIC/文件帧，完成 8,388,608 bytes 且双方 session、摘要一致；`relay=false`、`connection_method=direct_unknown`。MASQUERADE-only 场景在 ICE checks 阶段返回稳定 `CHECK_TIMEOUT`，没有降级到 HTTP/WSS/JavaScript IPC 或正文中继。固定映射 PASS 只证明该 NAT 行为可用，不能外推为所有家庭 NAT 或公网分类已通过。

## M4 可选接收计划与子集能力

M4 传输层实现见 [ADR 0005](adr/0005-receive-plans.md)。V1、TLS ALPN、原始 Manifest/TransferID/digest 与正文路径不变；新 sender 仅在原 offer 增加可选 `capabilities:["receive_plan_v1"]`，不插入新 hello。旧 receiver 可忽略该字段；新 sender 读到无 selection 的旧 accept 时继续全量。新 receiver 不向未声明能力的旧 sender 协商子集，返回稳定 `RECEIVE_PLAN_UNSUPPORTED`。

新 accept 可包含 `selection:{version:1,ids:[...],digest:...}`。ID 必须严格递增、不重复、属于原 manifest 并包含所需目录祖先；摘要为固定字段顺序 JSON `{version:1,original_digest:<原摘要>,ids:<ID数组>}` 的 BLAKE3。原 control.digest 始终是 original manifest digest；本地接收目录与目标路径映射不发给对端。

随后 finish/completed/confirmed/confirmed_ack 必须携同一 `selection_digest`。新 sender 限制 chunk 请求为选中 ID，拒绝伪造进度或错误选择摘要，并在重新校验已选源内容后核对 completed.Verified 等于所选总字节；未选源文件变化不妨碍所选子集完成。使用新能力时必须收到显式 matching confirmed_ack，旧全量会话保留既有 EOF/交付观察兼容。

`Result.Bytes` 与 Total/Verified/Committed 只表示已接受内容；OriginalTotal、SelectedFiles/SelectedEntries、SkippedFiles/SkippedEntries/SkippedBytes 分别保留原 offer 与跳过事实。全部跳过仍完成一致终态握手并返回 `NoContent`/0 B，不进入正文请求循环；选中空目录是非空条目集合，实际创建后可零字节 Completed。

新增稳定错误：`RECEIVE_PLAN_UNSUPPORTED`、`RECEIVE_PLAN_MISMATCH`、`INVALID_RECEIVE_PLAN`、`RECEIVE_PLAN_PERSIST_FAILED`；`RESUME_IDENTITY_MISMATCH` 也在 V1 error envelope 中按精确代码传递。未知 peer 文本不能获得这些类型语义。

计划 checkpoint 保留原 manifest、接收决定和相对映射摘要；带计划的落盘状态使用 `ReceivePlanV1/` 前缀使旧 binary 明确拒绝不支持的本地恢复语义。新 reader 继续读取无计划的旧 checkpoint。App 接线必须显示持久 ResumePlan 并保持恢复选择；这些传输层能力不意味着接收清单 UI 已自动验收。
