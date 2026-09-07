# Architecture

根 module `example.com/linksend` 包含协议、身份、信令客户端、Pion/quic-go 集成、传输协议、存储、服务端、应用服务和 CLI。`apps/desktop` 是唯一嵌套 Go module，Wails 绑定只调用 `internal/app`。

数据面流程是：

```text
WSS 信令：设备/会话/候选
        -> Pion ICE：检查、提名、保活
        -> quic.Transport：拥有同一个 UDP socket
        -> TLS 1.3 QUIC：可靠双向流
        -> transfer：manifest、块校验、恢复、安全提交
```

`quic.Transport` 是 UDP 的唯一读写所有者。Pion 只通过受限 STUN `PacketConn` 处理非 QUIC 数据。文件协议不导入 signaling，服务器不导入 transfer，文件字节不进入 WSS 或 JavaScript。

每次协商带 `session_id`、角色和 `generation`。候选过期或路径变化时关闭并重建端点，同时保留已验证块状态。健康的数据连接不依赖信令连接存活。

`internal/app` 的 `ConnectDirect`、`AcceptDirect`、`SendFiles` 和 `ReceiveOnce` 是 CLI/Wails 共用的直连编排入口。它要求本地 `trust.json` 中的公钥固定，候选交换完成后才调用 ICE/QUIC；接收确认通过 `transfer.Receive` 的回调执行。

传输 stream 的正常完成使用 quic-go 的有序 `Close`。取消或协议失败通过 `transport.QUICStream.Abort` 同时调用 `CancelRead` 和 `CancelWrite`，只影响当前 stream，不关闭共享 UDP socket 或 signaling session。候选交换拥有自己的 phase context；首个终态错误取消 sibling，阶段结束不会主动关闭整个 WSS 连接。

真实网络路径的证据和限制记录在 [`docs/adr/0001-ice-quic-integration.md`](adr/0001-ice-quic-integration.md)。当前只冻结了实现边界，双 NAT 通过前不宣称架构验收完成。
