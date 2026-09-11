# Architecture

当前产品版本为 LinkSend `0.2.0`，协议为 V1。根 module `github.com/Wen5555/LinkSend` 包含协议、身份、信令客户端、Pion/quic-go 集成、文件传输、存储、应用服务、服务端和 CLI；`apps/desktop` 是唯一嵌套 Go module，使用 Wails 3 `v3.0.0-beta.18`，不是 Wails 2。

```text
WSS 信令：设备/会话/候选
        -> Pion ICE：检查、提名、保活
        -> quic.Transport：唯一 UDP socket 所有者
        -> TLS 1.3 QUIC：可靠双向流
        -> transfer：manifest、缺块、校验、staging、提交
        -> app/store：任务状态、revision 与 schema 2 恢复元数据
        -> CLI / Wails DTO / React：一致的状态和稳定错误语义
```

`quic.Transport` 是 UDP 的唯一读写所有者；Pion 通过受限 STUN `PacketConn` 使用同一 base socket。文件协议不导入 signaling，服务器不导入 transfer，文件正文不进入 WSS、HTTP 或 JavaScript。Pion ICE 与 quic-go 的职责边界保持不变。

连接生命周期由 `session_id + generation + connection owner` 约束。断开或被替换的认证信令连接会清理自己拥有的协商；旧 handler/generation 的迟到消息不能删除或污染新协商。完成、拒绝、失败和取消后的下一次连接无需等待旧会话过期。健康 QUIC 数据连接不依赖信令 WSS 持续存活。

逻辑任务在恢复过程中保留 `task_id`，每次执行创建新 `attempt_id`，每次连接创建新 `session_id`，ICE generation 只在该 session 内有效；完整任务快照通过单调 `revision` 防止旧回调覆盖。公开状态为 Preparing、AwaitingAcceptance、Transferring、Verifying、Paused、Recovering、Completed、Rejected、Cancelled、Failed。Completed 要求接收端唯一块验证、提交成功以及 completed/confirmed 双边终态完成。

QUIC terminal flush 先发送并半关闭 stream，再有界等待对端终态。对单次传输连接，接收端只有在读到匹配 `confirmed` 后才会以 application code 0 正常关闭；因此发送端在已经验证 `completed` 并写出 `confirmed` 的 terminal 边界可接受该特定正常关闭早于 stream FIN 到达。任何非零 close、reset、deadline 或普通传输阶段的 close 都不能转换为成功。

任务控制状态优先于同一 attempt 的迟到进度：Pause/Cancel 建立调度屏障后，ACK 可以补齐计数，但不能重新开放 `CanPause`、覆盖 `pause_requested` 或把任务误终结为 Cancelled。接收准备和测试同步使用明确的后端 phase/checkpoint 事件，不依赖固定 sleep。

恢复仍复用现有 V1 transfer 帧，不建立第二套协议栈。恢复身份绑定 TransferID、manifest digest、块策略、源内容、接收目录、对端身份/指纹；接收端重验 staging 和 commit records 后请求缺失或损坏块。`sent_bytes`/`received_bytes` 是实际数据面字节，`retransmitted_bytes` 是旧 attempt 已发送又重发的子集，`verified_bytes`/逻辑完成量只计算唯一已验证数据。

多网卡层自动枚举可用单播地址，并支持接口优先和排除；IPv6 link-local 当前排除。地址失效会废弃旧 endpoint 并进入新协商/恢复，不宣称 QUIC 透明迁移。路径分类只有独立路由证据充分时才可标为 `lan_direct` 或 `internet_p2p`，否则保持 `direct_unknown`。

真实网络证据见 [ADR 0001](adr/0001-ice-quic-integration.md)，任务/恢复决策见 [ADR 0002](adr/0002-task-recovery-and-history-schema.md)。独立双 NAT 固定映射场景已 PASS；MASQUERADE-only 仍 FAIL，因此不能宣称所有 NAT 类型均可直连。
