# Protocol

当前产品版本为 `0.2.0`，控制/文件协议版本仍为 `1`。`internal/protocol.ProductVersion`、`Capabilities.product_version` 和 `/healthz.version` 用于产品部署识别；`protocol_version` 继续决定 wire compatibility。升级产品小版本不会自动改变协议版本，旧服务缺少 `product_version` 时客户端可按 V1 能力兼容，但诊断必须明确显示版本未知。

控制面使用有界 JSON `Envelope`，签名输入是明确的长度前缀二进制编码，不直接签任意 JSON map。字段包括协议版本、消息 ID、会话、发送方、接收方、generation、有效时间、payload 和 Ed25519 签名。

消息类型为 `connect_request`、`connect_response`、`candidate`、`end_of_candidates` 和 `status`。服务端只转发已认证设备组内、会话归属和 generation 正确的消息。未知关键类型、版本错误、重放、过期和超限消息明确拒绝。

文件控制帧使用长度前缀和独立的 QUIC stream，控制 metadata 上限为 8 MiB，文件内容不 Base64 化。传输顺序为 offer、用户 accept、块请求/数据、ack、finish、completed、confirmed。4 MiB 是默认分块，演示使用 64 KiB 以缩短测试时间。

控制帧必须满足对应操作的语义：offer 的 manifest digest 必须匹配；accept 的已验证字节数必须在 `[0,total]`；ack 必须对应刚发送的块、单调且不超过初始恢复字节加实际发送字节；finish/completed/confirmed 必须绑定 manifest digest。空帧、未知关键操作、负索引和不可能的字段组合均会被拒绝。对端 error detail 限制为 4 KiB、有合法 UTF-8 且移除控制字符；未知 JSON 字段暂不作为 V1 兼容性错误。

恢复身份由 `TransferID + manifest digest + chunk policy + peer identity` 共同绑定。接收端重新哈希 staging 块后才继续，完成后通过 `os.Root` 约束路径并以不可覆盖方式提交。

2026-09-11 错误诊断兼容补丁：V1 `error.error` 字符串保留原帧结构，只发送允许列表中的稳定码，不再发送本地文件系统路径。新增明确的 `FILE_CONFLICT`、`PERMISSION_DENIED`，并保留 `DISK_FULL`。接收方仅对完全匹配的已知码恢复错误类别；旧版本任意文本或未来未知码仍可解析，但仅按通用传输失败处理。身份验证、块帧和 completed/confirmed 语义未改变。

`local_candidate` / `remote_candidate` 诊断字段是 `udp4|udp6 address:port typ type` 描述，不是可回灌 ICE 的序列化候选。诊断不包含 ufrag、密码或候选扩展；经过签名的候选交换仍使用 Pion 原始序列化。

QUIC 的 `Write` 只保证数据进入发送缓冲。拒绝或 error 帧发送后，transport 先关闭发送方向，再有界等待对端结束读取，防止随后的 reset/连接关闭丢弃终态响应。该等待至多 2 秒且不延长调用者 deadline；原始错误仍作为任务结果。V1 帧结构和 completed/confirmed 语义保持不变，net.Pipe 兼容测试与真实 QUIC 负向回归分别覆盖同步和缓冲发送。

信令协商状态绑定当前认证连接。断开或被同设备的新认证连接替换时清理该设备参与的协商；旧连接的迟到帧不能写入新连接状态。已建立的 QUIC 数据连接不因此被关闭。此修复需要升级信令服务源码；客户端升级无法修复仍运行旧版本的服务。

## 任务、attempt 与恢复语义（schema 2）

`task_id` 表示用户可识别的逻辑任务，暂停、重启和恢复时保持不变；`attempt_id` 表示一次执行，任何恢复都必须新建；`session_id` 表示一次端到端连接；ICE generation 只在对应 session 内解释，候选和 end-of-candidates 必须同时匹配 session 与 generation；`revision` 是任务快照的单调版本。旧 attempt/session/generation 的回调或消息不得更新较新的 revision。

合法主路径是 `Preparing → AwaitingAcceptance → Transferring → Verifying → Completed`。AwaitingAcceptance/Transferring 可以进入 Paused；可恢复连接中断进入 Recovering；Resume 从 Paused/Recovering 创建新 attempt 与 session 后重新进入连接/协商。Rejected、Cancelled、Failed、Completed 是终态。校验或文件提交失败必须进入 Failed；只有接收端完成唯一块验证与文件提交，且 sender 收到 completed 并返回 confirmed 后，双方才记录 Completed / `bilateral_confirmed=true`。

恢复仍使用 V1 offer、accept、块请求/data/ack、finish/completed/confirmed 帧，不增加旁路数据协议。新的 session 携带同一 TransferID 和重建后的 manifest；接收端将其与持久化的 manifest digest、chunk size、总字节、文件数、目标目录和 pinned peer 比较，并重新哈希 staging 后生成缺块请求。sender 同样重建并比较 manifest，源内容变化返回 `SOURCE_CHANGED`；恢复身份不匹配返回 `RESUME_IDENTITY_MISMATCH`。已发过但再次被请求的块计入 `retransmitted_bytes`；`verified_bytes`/`committed_bytes` 只按接收端唯一验证和成功提交计算，不能由重传重复增加。

每个成功提交的文件写入 checkpoint commit record（文件 ID、路径、大小和 digest）。接收端已提交而 sender 未收到 terminal confirmation 时，后续恢复先验证 commit record/目标文件，不能覆盖已有文件或盲目完整重发。部分文件已提交后失败时，任务保持 Failed/Recovering 语义并报告已提交文件/字节；未提交部分才可继续。

结构化连接证据仅包含实际 base socket、接口、地址族、脱敏候选描述/类型、ICE 状态时间线、TLS version/ALPN、连接方法、relay 实际值、STUN 请求/响应计数和信令 JSON 字节计数。邀请、令牌、私钥、ICE ufrag/password、候选扩展和文件正文禁止进入诊断。没有独立路由证据时 `connection_method=direct_unknown`。

2026-09-11 独立 Linux 双 NAT 的现场结果不改变协议：固定 UDP 映射的正向场景仍使用同一 V1 ICE/QUIC/文件帧，完成 8,388,608 bytes 且双方 session、摘要一致；`relay=false`、`connection_method=direct_unknown`。MASQUERADE-only 场景在 ICE checks 阶段返回稳定 `CHECK_TIMEOUT`，没有降级到 HTTP/WSS/JavaScript IPC 或正文中继。固定映射 PASS 只证明该 NAT 行为可用，不能外推为所有家庭 NAT 或公网分类已通过。
